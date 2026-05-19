package exchange

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"bian-trade-go/internal/agent/config"
)

func TestBitgetPlaceOrderUsesMarketBuyQuoteSize(t *testing.T) {
	fixedNow := time.UnixMilli(1700000000000)
	var requestBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != bitgetPlaceOrderPath {
			t.Fatalf("path = %q", r.URL.Path)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &requestBody); err != nil {
			t.Fatal(err)
		}
		wantSign := signBitget("1700000000000", http.MethodPost, bitgetPlaceOrderPath, "", string(raw), "secret")
		if got := r.Header.Get("ACCESS-SIGN"); got != wantSign {
			t.Fatalf("ACCESS-SIGN = %q, want %q", got, wantSign)
		}
		_, _ = w.Write([]byte(`{"code":"00000","msg":"success","requestTime":1700000000100,"data":{"orderId":"order-1","clientOid":"inst7-MACRO-123"}}`))
	}))
	defer server.Close()

	client := newTestBitgetClient(server.URL, fixedNow)
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
	if requestBody["side"] != "buy" || requestBody["orderType"] != "market" || requestBody["size"] != "25.5" {
		t.Fatalf("unexpected request body: %#v", requestBody)
	}
	if execution.OrderID != "order-1" || execution.ClientOrderID != "inst7-MACRO-123" || execution.Status != "FILLED" {
		t.Fatalf("unexpected execution: %#v", execution)
	}
}

func TestBitgetPlaceOrderUsesMarketSellBaseSize(t *testing.T) {
	fixedNow := time.UnixMilli(1700000000000)
	var requestBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &requestBody); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"code":"00000","msg":"success","requestTime":1700000000100,"data":{"orderId":"order-2","clientOid":"inst7-MICRO-123"}}`))
	}))
	defer server.Close()

	client := newTestBitgetClient(server.URL, fixedNow)
	if _, err := client.PlaceOrder(TradeCommand{
		ClientOrderID: "inst7-MICRO-123",
		Symbol:        "BTCUSDT",
		Action:        "SELL",
		QtyAsset:      "0.0123",
	}); err != nil {
		t.Fatalf("PlaceOrder returned error: %v", err)
	}
	if requestBody["side"] != "sell" || requestBody["size"] != "0.0123" {
		t.Fatalf("unexpected request body: %#v", requestBody)
	}
}

func TestBitgetGetBalances(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != bitgetAccountAssetURL {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.URL.RawQuery != "assetType=all" {
			t.Fatalf("query = %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"code":"00000","msg":"success","requestTime":1700000000100,"data":[{"coin":"usdt","available":"10","frozen":"1","locked":"0"}]}`))
	}))
	defer server.Close()

	client := newTestBitgetClient(server.URL, time.UnixMilli(1700000000000))
	balances, err := client.GetBalances()
	if err != nil {
		t.Fatalf("GetBalances returned error: %v", err)
	}
	if len(balances) != 1 || balances[0].Asset != "USDT" || balances[0].Available != "10" || balances[0].Frozen != "1" {
		t.Fatalf("unexpected balances: %#v", balances)
	}
}

func TestBitgetSandboxAddsPaperTradingHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("paptrading"); got != "1" {
			t.Fatalf("paptrading header = %q", got)
		}
		_, _ = w.Write([]byte(`{"code":"00000","msg":"success","requestTime":1700000000100,"data":[]}`))
	}))
	defer server.Close()

	client := newTestBitgetClient(server.URL, time.UnixMilli(1700000000000))
	client.cfg.Sandbox = true
	if _, err := client.GetBalances(); err != nil {
		t.Fatalf("GetBalances returned error: %v", err)
	}
}

func newTestBitgetClient(baseURL string, now time.Time) *BitgetClient {
	return &BitgetClient{
		cfg: config.ExchangeConfig{
			Name:       "bitget",
			APIKey:     "key",
			SecretKey:  "secret",
			Passphrase: "pass",
		},
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: time.Second},
		now:        func() time.Time { return now },
	}
}
