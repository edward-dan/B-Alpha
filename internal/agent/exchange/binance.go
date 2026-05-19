package exchange

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"bian-trade-go/internal/agent/config"
)

const (
	binanceBaseURL        = "https://api.binance.com"
	binanceTestnetBaseURL = "https://testnet.binance.vision"
	binanceOrderPath      = "/api/v3/order"
	binanceAccountPath    = "/api/v3/account"
	binanceRecvWindow     = "5000"
)

type BinanceClient struct {
	cfg        config.ExchangeConfig
	baseURL    string
	httpClient *http.Client
	now        func() time.Time
}

type binanceOrderResponse struct {
	Symbol              string          `json:"symbol"`
	OrderID             int64           `json:"orderId"`
	ClientOrderID       string          `json:"clientOrderId"`
	TransactTime        int64           `json:"transactTime"`
	ExecutedQty         string          `json:"executedQty"`
	CummulativeQuoteQty string          `json:"cummulativeQuoteQty"`
	Status              string          `json:"status"`
	Raw                 json.RawMessage `json:"-"`
}

type binanceAccountResponse struct {
	Balances []binanceBalance `json:"balances"`
}

type binanceBalance struct {
	Asset  string `json:"asset"`
	Free   string `json:"free"`
	Locked string `json:"locked"`
}

type binanceErrorResponse struct {
	Code int64  `json:"code"`
	Msg  string `json:"msg"`
}

func NewBinanceClient(cfg config.ExchangeConfig) (*BinanceClient, error) {
	cfg.Name = strings.ToLower(strings.TrimSpace(cfg.Name))
	if cfg.Name != "" && cfg.Name != "binance" {
		return nil, fmt.Errorf("new Binance client: unsupported exchange %q", cfg.Name)
	}
	if cfg.APIKey == "" || cfg.SecretKey == "" {
		return nil, errors.New("new Binance client: api_key and secret_key are required")
	}
	baseURL := binanceBaseURL
	if cfg.Sandbox {
		baseURL = binanceTestnetBaseURL
	}
	return &BinanceClient{
		cfg:        cfg,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		now:        func() time.Time { return time.Now().UTC() },
	}, nil
}

func (c *BinanceClient) PlaceOrder(cmd TradeCommand) (Execution, error) {
	if c == nil {
		return Execution{}, errors.New("place Binance order: client is nil")
	}
	if err := validateCommand(cmd, c.now()); err != nil {
		return Execution{}, err
	}

	action := strings.ToUpper(strings.TrimSpace(cmd.Action))
	params := url.Values{}
	params.Set("symbol", strings.ToUpper(strings.TrimSpace(cmd.Symbol)))
	params.Set("type", "MARKET")
	params.Set("newClientOrderId", cmd.ClientOrderID)
	params.Set("newOrderRespType", "FULL")
	switch action {
	case "BUY":
		params.Set("side", "BUY")
		if cmd.AmountUSDT == "" {
			return Execution{}, errors.New("place Binance order: quote amount is required for BUY")
		}
		params.Set("quoteOrderQty", cmd.AmountUSDT)
	case "SELL":
		params.Set("side", "SELL")
		if cmd.QtyAsset == "" {
			return Execution{}, errors.New("place Binance order: quantity is required for SELL")
		}
		params.Set("quantity", cmd.QtyAsset)
	default:
		return Execution{}, fmt.Errorf("place Binance order: unsupported action %q", cmd.Action)
	}

	var data binanceOrderResponse
	raw, err := c.doSigned(context.Background(), http.MethodPost, binanceOrderPath, params, &data)
	if err != nil {
		return Execution{}, err
	}
	data.Raw = raw
	status := strings.ToUpper(strings.TrimSpace(data.Status))
	if status == "" {
		status = "FILLED"
	}
	clientOrderID := data.ClientOrderID
	if clientOrderID == "" {
		clientOrderID = cmd.ClientOrderID
	}
	return Execution{
		OrderID:        strconv.FormatInt(data.OrderID, 10),
		ClientOrderID:  clientOrderID,
		Status:         status,
		FilledQty:      data.ExecutedQty,
		FilledAmount:   data.CummulativeQuoteQty,
		ExchangeTimeMS: data.TransactTime,
		Raw:            append(json.RawMessage(nil), data.Raw...),
	}, nil
}

func (c *BinanceClient) GetBalances() ([]Balance, error) {
	if c == nil {
		return nil, errors.New("get Binance balances: client is nil")
	}
	var account binanceAccountResponse
	if _, err := c.doSigned(context.Background(), http.MethodGet, binanceAccountPath, url.Values{}, &account); err != nil {
		return nil, err
	}
	balances := make([]Balance, 0, len(account.Balances))
	for _, balance := range account.Balances {
		balances = append(balances, Balance{
			Asset:     strings.ToUpper(balance.Asset),
			Available: balance.Free,
			Frozen:    balance.Locked,
			Locked:    balance.Locked,
		})
	}
	return balances, nil
}

func (c *BinanceClient) doSigned(ctx context.Context, method string, requestPath string, params url.Values, out any) (json.RawMessage, error) {
	endpoint, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	endpoint.Path = requestPath

	signedParams := cloneValues(params)
	signedParams.Set("recvWindow", binanceRecvWindow)
	signedParams.Set("timestamp", strconv.FormatInt(c.now().UnixMilli(), 10))
	payload := signedParams.Encode()
	signature := signBinance(payload, c.cfg.SecretKey)
	endpoint.RawQuery = payload + "&signature=" + signature

	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-MBX-APIKEY", c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	rawResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("binance %s %s returned HTTP %d: %s", method, requestPath, resp.StatusCode, string(rawResp))
	}

	var binanceErr binanceErrorResponse
	if err := json.Unmarshal(rawResp, &binanceErr); err == nil && binanceErr.Code != 0 {
		return nil, fmt.Errorf("binance %s %s returned code %d: %s", method, requestPath, binanceErr.Code, binanceErr.Msg)
	}
	if out != nil && len(rawResp) > 0 {
		if err := json.Unmarshal(rawResp, out); err != nil {
			return nil, err
		}
	}
	return append(json.RawMessage(nil), rawResp...), nil
}

func cloneValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, items := range values {
		cloned[key] = append([]string(nil), items...)
	}
	return cloned
}

func signBinance(payload string, secretKey string) string {
	mac := hmac.New(sha256.New, []byte(secretKey))
	_, _ = mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}
