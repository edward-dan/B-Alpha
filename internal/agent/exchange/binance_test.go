package exchange

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bian-trade-go/internal/agent/config"
)

func TestBinancePlaceOrderUsesMarketBuyQuoteOrderQty(t *testing.T) {
	fixedNow := time.UnixMilli(1700000000000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != binanceOrderPath {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q", r.Method)
		}
		assertBinanceSignature(t, r.URL.RawQuery)
		if got := r.Header.Get("X-MBX-APIKEY"); got != "key" {
			t.Fatalf("X-MBX-APIKEY = %q", got)
		}
		query := r.URL.Query()
		if query.Get("symbol") != "BTCUSDT" ||
			query.Get("side") != "BUY" ||
			query.Get("type") != "MARKET" ||
			query.Get("quoteOrderQty") != "25.5" ||
			query.Get("newClientOrderId") != "inst7-MACRO-123" ||
			query.Get("timestamp") != "1700000000000" ||
			query.Get("recvWindow") != binanceRecvWindow {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"symbol":"BTCUSDT","orderId":12345,"clientOrderId":"inst7-MACRO-123","transactTime":1700000000100,"executedQty":"0.001","cummulativeQuoteQty":"25.5","status":"FILLED","type":"MARKET","side":"BUY"}`))
	}))
	defer server.Close()

	client := newTestBinanceClient(server.URL, fixedNow)
	execution, err := client.PlaceOrder(TradeCommand{
		ClientOrderID: "inst7-MACRO-123",
		Symbol:        "btcusdt",
		Action:        "BUY",
		AmountUSDT:    "25.5",
		ExpiresAtMS:   fixedNow.Add(time.Minute).UnixMilli(),
	})
	if err != nil {
		t.Fatalf("PlaceOrder returned error: %v", err)
	}
	if execution.OrderID != "12345" ||
		execution.ClientOrderID != "inst7-MACRO-123" ||
		execution.Status != "FILLED" ||
		execution.FilledQty != "0.001" ||
		execution.FilledAmount != "25.5" ||
		execution.ExchangeTimeMS != 1700000000100 {
		t.Fatalf("unexpected execution: %#v", execution)
	}
	if !json.Valid(execution.Raw) {
		t.Fatalf("execution raw is not valid JSON: %s", string(execution.Raw))
	}
}

func TestBinancePlaceOrderUsesMarketSellQuantity(t *testing.T) {
	fixedNow := time.UnixMilli(1700000000000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("side") != "SELL" || query.Get("quantity") != "0.0123" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		if query.Get("quoteOrderQty") != "" {
			t.Fatalf("quoteOrderQty should be empty for sell: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"symbol":"BTCUSDT","orderId":12346,"clientOrderId":"inst7-MICRO-123","transactTime":1700000000100,"executedQty":"0.0123","cummulativeQuoteQty":"615","status":"FILLED"}`))
	}))
	defer server.Close()

	client := newTestBinanceClient(server.URL, fixedNow)
	if _, err := client.PlaceOrder(TradeCommand{
		ClientOrderID: "inst7-MICRO-123",
		Symbol:        "BTCUSDT",
		Action:        "SELL",
		QtyAsset:      "0.0123",
	}); err != nil {
		t.Fatalf("PlaceOrder returned error: %v", err)
	}
}

func TestBinanceGetBalances(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != binanceAccountPath {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf("method = %q", r.Method)
		}
		assertBinanceSignature(t, r.URL.RawQuery)
		_, _ = w.Write([]byte(`{"makerCommission":10,"takerCommission":10,"balances":[{"asset":"usdt","free":"10","locked":"1"},{"asset":"btc","free":"0.01","locked":"0"}]}`))
	}))
	defer server.Close()

	client := newTestBinanceClient(server.URL, time.UnixMilli(1700000000000))
	balances, err := client.GetBalances()
	if err != nil {
		t.Fatalf("GetBalances returned error: %v", err)
	}
	if len(balances) != 2 ||
		balances[0].Asset != "USDT" ||
		balances[0].Available != "10" ||
		balances[0].Frozen != "1" ||
		balances[1].Asset != "BTC" {
		t.Fatalf("unexpected balances: %#v", balances)
	}
}

func TestBinanceSandboxUsesSpotTestnetBaseURL(t *testing.T) {
	client, err := NewBinanceClient(config.ExchangeConfig{
		Name:      "binance",
		APIKey:    "key",
		SecretKey: "secret",
		Sandbox:   true,
	})
	if err != nil {
		t.Fatalf("NewBinanceClient returned error: %v", err)
	}
	if client.baseURL != binanceTestnetBaseURL {
		t.Fatalf("baseURL = %q", client.baseURL)
	}
}

func TestNewClientSelectsBinance(t *testing.T) {
	client, err := NewClient(config.ExchangeConfig{Name: "binance", APIKey: "key", SecretKey: "secret"})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	if _, ok := client.(*BinanceClient); !ok {
		t.Fatalf("client type = %T", client)
	}
}

func assertBinanceSignature(t *testing.T, rawQuery string) {
	t.Helper()
	parts := strings.Split(rawQuery, "&signature=")
	if len(parts) != 2 {
		t.Fatalf("raw query missing signature suffix: %s", rawQuery)
	}
	wantSign := signBinance(parts[0], "secret")
	if parts[1] != wantSign {
		t.Fatalf("signature = %q, want %q", parts[1], wantSign)
	}
}

func newTestBinanceClient(baseURL string, now time.Time) *BinanceClient {
	return &BinanceClient{
		cfg: config.ExchangeConfig{
			Name:      "binance",
			APIKey:    "key",
			SecretKey: "secret",
		},
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: time.Second},
		now:        func() time.Time { return now },
	}
}

func TestBinanceErrorIncludesResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":-2010,"msg":"Account has insufficient balance"}`)
	}))
	defer server.Close()

	client := newTestBinanceClient(server.URL, time.UnixMilli(1700000000000))
	_, err := client.PlaceOrder(TradeCommand{
		ClientOrderID: "inst7-MACRO-123",
		Symbol:        "BTCUSDT",
		Action:        "BUY",
		AmountUSDT:    "25.5",
	})
	if err == nil {
		t.Fatal("expected Binance error")
	}
	if !strings.Contains(err.Error(), "insufficient balance") {
		t.Fatalf("error = %v", err)
	}
}
