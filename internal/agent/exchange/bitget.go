package exchange

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
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
	bitgetBaseURL         = "https://api.bitget.com"
	bitgetPlaceOrderPath  = "/api/v2/spot/trade/place-order"
	bitgetAccountAssetURL = "/api/v2/spot/account/assets"
)

type TradeCommand struct {
	ClientOrderID string  `json:"client_order_id"`
	InstanceID    any     `json:"instance_id,omitempty"`
	Symbol        string  `json:"symbol"`
	Action        string  `json:"action"`
	Engine        string  `json:"engine"`
	LotType       string  `json:"lot_type"`
	AmountUSDT    string  `json:"amount_usdt,omitempty"`
	QtyAsset      string  `json:"qty_asset,omitempty"`
	LimitPrice    *string `json:"limit_price"`
	CreatedAtMS   int64   `json:"created_at_ms"`
	ExpiresAtMS   int64   `json:"expires_at_ms"`
	ReasonCode    string  `json:"reason_code"`
}

type Execution struct {
	OrderID        string          `json:"order_id,omitempty"`
	ClientOrderID  string          `json:"client_order_id"`
	Status         string          `json:"status"`
	FilledQty      string          `json:"filled_qty,omitempty"`
	FilledPrice    string          `json:"filled_price,omitempty"`
	FilledAmount   string          `json:"filled_amount,omitempty"`
	Fee            string          `json:"fee,omitempty"`
	ExchangeTimeMS int64           `json:"exchange_time_ms,omitempty"`
	Raw            json.RawMessage `json:"raw,omitempty"`
}

type Balance struct {
	Asset     string `json:"asset"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
	Locked    string `json:"locked,omitempty"`
}

type BitgetClient struct {
	cfg        config.ExchangeConfig
	baseURL    string
	httpClient *http.Client
	now        func() time.Time
}

type placeOrderRequest struct {
	Symbol    string `json:"symbol"`
	Side      string `json:"side"`
	OrderType string `json:"orderType"`
	Force     string `json:"force,omitempty"`
	Size      string `json:"size"`
	ClientOID string `json:"clientOid,omitempty"`
}

type bitgetResponse struct {
	Code        string          `json:"code"`
	Msg         string          `json:"msg"`
	Message     string          `json:"message"`
	RequestTime int64           `json:"requestTime"`
	Data        json.RawMessage `json:"data"`
}

type placeOrderData struct {
	OrderID   string `json:"orderId"`
	ClientOID string `json:"clientOid"`
}

type accountAsset struct {
	Coin      string `json:"coin"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
	Locked    string `json:"locked"`
}

func NewBitgetClient(cfg config.ExchangeConfig) (*BitgetClient, error) {
	cfg.Name = strings.ToLower(strings.TrimSpace(cfg.Name))
	if cfg.Name != "" && cfg.Name != "bitget" {
		return nil, fmt.Errorf("new Bitget client: unsupported exchange %q", cfg.Name)
	}
	if cfg.APIKey == "" || cfg.SecretKey == "" || cfg.Passphrase == "" {
		return nil, errors.New("new Bitget client: api_key, secret_key and passphrase are required")
	}
	return &BitgetClient{
		cfg:        cfg,
		baseURL:    bitgetBaseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		now:        func() time.Time { return time.Now().UTC() },
	}, nil
}

func (c *BitgetClient) PlaceOrder(cmd TradeCommand) (Execution, error) {
	if c == nil {
		return Execution{}, errors.New("place Bitget order: client is nil")
	}
	if err := validateCommand(cmd, c.now()); err != nil {
		return Execution{}, err
	}

	action := strings.ToUpper(strings.TrimSpace(cmd.Action))
	reqBody := placeOrderRequest{
		Symbol:    strings.ToUpper(strings.TrimSpace(cmd.Symbol)),
		OrderType: "market",
		Force:     "ioc",
		ClientOID: cmd.ClientOrderID,
	}
	switch action {
	case "BUY":
		reqBody.Side = "buy"
		reqBody.Size = cmd.AmountUSDT
	case "SELL":
		reqBody.Side = "sell"
		reqBody.Size = cmd.QtyAsset
	default:
		return Execution{}, fmt.Errorf("place Bitget order: unsupported action %q", cmd.Action)
	}
	if reqBody.Size == "" {
		return Execution{}, fmt.Errorf("place Bitget order: size is required for %s", action)
	}

	rawBody, err := json.Marshal(reqBody)
	if err != nil {
		return Execution{}, err
	}
	var data placeOrderData
	resp, err := c.doJSON(context.Background(), http.MethodPost, bitgetPlaceOrderPath, "", rawBody, &data)
	if err != nil {
		return Execution{}, err
	}
	if data.ClientOID == "" {
		data.ClientOID = cmd.ClientOrderID
	}
	return Execution{
		OrderID:        data.OrderID,
		ClientOrderID:  data.ClientOID,
		Status:         "FILLED",
		ExchangeTimeMS: resp.RequestTime,
		Raw:            append(json.RawMessage(nil), resp.Data...),
	}, nil
}

func (c *BitgetClient) GetBalances() ([]Balance, error) {
	if c == nil {
		return nil, errors.New("get Bitget balances: client is nil")
	}
	query := url.Values{}
	query.Set("assetType", "all")
	var assets []accountAsset
	if _, err := c.doJSON(context.Background(), http.MethodGet, bitgetAccountAssetURL, query.Encode(), nil, &assets); err != nil {
		return nil, err
	}
	balances := make([]Balance, 0, len(assets))
	for _, asset := range assets {
		balances = append(balances, Balance{
			Asset:     strings.ToUpper(asset.Coin),
			Available: asset.Available,
			Frozen:    asset.Frozen,
			Locked:    asset.Locked,
		})
	}
	return balances, nil
}

func (c *BitgetClient) doJSON(ctx context.Context, method string, requestPath string, queryString string, body []byte, out any) (bitgetResponse, error) {
	endpoint, err := url.Parse(c.baseURL)
	if err != nil {
		return bitgetResponse{}, err
	}
	endpoint.Path = requestPath
	endpoint.RawQuery = queryString

	var reader io.Reader
	bodyString := ""
	if len(body) > 0 {
		bodyString = string(body)
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), reader)
	if err != nil {
		return bitgetResponse{}, err
	}
	timestamp := strconv.FormatInt(c.now().UnixMilli(), 10)
	req.Header.Set("ACCESS-KEY", c.cfg.APIKey)
	req.Header.Set("ACCESS-TIMESTAMP", timestamp)
	req.Header.Set("ACCESS-PASSPHRASE", c.cfg.Passphrase)
	req.Header.Set("ACCESS-SIGN", signBitget(timestamp, method, requestPath, queryString, bodyString, c.cfg.SecretKey))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("locale", "en-US")
	if c.cfg.Sandbox {
		req.Header.Set("paptrading", "1")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return bitgetResponse{}, err
	}
	defer resp.Body.Close()
	rawResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return bitgetResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return bitgetResponse{}, fmt.Errorf("bitget %s %s returned HTTP %d: %s", method, requestPath, resp.StatusCode, string(rawResp))
	}

	var decoded bitgetResponse
	if err := json.Unmarshal(rawResp, &decoded); err != nil {
		return bitgetResponse{}, err
	}
	if decoded.Code != "00000" {
		msg := decoded.Msg
		if msg == "" {
			msg = decoded.Message
		}
		return decoded, fmt.Errorf("bitget %s %s returned code %s: %s", method, requestPath, decoded.Code, msg)
	}
	if out != nil && len(decoded.Data) > 0 {
		if err := json.Unmarshal(decoded.Data, out); err != nil {
			return decoded, err
		}
	}
	return decoded, nil
}

func signBitget(timestamp string, method string, requestPath string, queryString string, body string, secretKey string) string {
	method = strings.ToUpper(method)
	preHash := timestamp + method + requestPath
	if queryString != "" {
		preHash += "?" + queryString
	}
	preHash += body

	mac := hmac.New(sha256.New, []byte(secretKey))
	_, _ = mac.Write([]byte(preHash))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func validateCommand(cmd TradeCommand, now time.Time) error {
	if cmd.ClientOrderID == "" {
		return errors.New("trade command client_order_id is required")
	}
	if strings.TrimSpace(cmd.Symbol) == "" {
		return errors.New("trade command symbol is required")
	}
	if cmd.ExpiresAtMS > 0 && now.UTC().UnixMilli() > cmd.ExpiresAtMS {
		return fmt.Errorf("trade command %s expired", cmd.ClientOrderID)
	}
	return nil
}
