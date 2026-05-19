package ws

import (
	"encoding/json"
	"testing"

	"bian-trade-go/internal/agent/config"
)

func TestWSURLConvertsHTTPToWS(t *testing.T) {
	client := AgentClient{cfg: config.AgentConfig{SaaSURL: "http://localhost:8080"}}
	if got := client.wsURL(); got != "ws://localhost:8080/ws/agent" {
		t.Fatalf("wsURL = %q", got)
	}
}

func TestWSURLConvertsHTTPSToWSS(t *testing.T) {
	client := AgentClient{cfg: config.AgentConfig{SaaSURL: "https://saas.example.com/base"}}
	if got := client.wsURL(); got != "wss://saas.example.com/base/ws/agent" {
		t.Fatalf("wsURL = %q", got)
	}
}

func TestDecodeCommandPayload(t *testing.T) {
	raw := []byte(`{"type":"command","payload":{"client_order_id":"inst7-MACRO-123","symbol":"BTCUSDT","action":"BUY","amount_usdt":"25"}}`)
	var msg wireMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatal(err)
	}
	cmd, err := decodeCommand(raw, msg)
	if err != nil {
		t.Fatalf("decodeCommand returned error: %v", err)
	}
	if cmd.ClientOrderID != "inst7-MACRO-123" || cmd.AmountUSDT != "25" {
		t.Fatalf("unexpected command: %#v", cmd)
	}
}
