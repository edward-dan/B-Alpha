package instance

import (
	"testing"
	"time"

	"bian-trade-go/internal/quant"
)

func TestBuildTradeCommandsUsesDocumentedClientOrderID(t *testing.T) {
	createdAt := time.Unix(100, 0).UTC()
	expiresAt := createdAt.Add(2 * time.Minute)

	commands, err := buildTradeCommands(7, "BTCUSDT", 123456, createdAt, expiresAt, quant.StrategyOutput{
		MacroIntents: []quant.TradeIntent{
			{
				Action:     quant.ActionBuy,
				Engine:     quant.EngineMacro,
				LotType:    quant.LotTypeDeadStack,
				AmountUSDT: 25.5,
				ReasonCode: "MACRO_DCA",
			},
		},
		MicroIntents: []quant.TradeIntent{
			{
				Action:     quant.ActionSell,
				Engine:     quant.EngineMicro,
				LotType:    quant.LotTypeFloating,
				QtyAsset:   0.0123,
				ReasonCode: "MICRO_REBALANCE_SELL",
			},
		},
	})
	if err != nil {
		t.Fatalf("buildTradeCommands returned error: %v", err)
	}
	if len(commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(commands))
	}
	if commands[0].ClientOrderID != "inst7-MACRO-123456" {
		t.Fatalf("macro client_order_id = %q", commands[0].ClientOrderID)
	}
	if commands[1].ClientOrderID != "inst7-MICRO-123456" {
		t.Fatalf("micro client_order_id = %q", commands[1].ClientOrderID)
	}
	if commands[0].AmountUSDT != "25.5" || commands[1].QtyAsset != "0.0123" {
		t.Fatalf("unexpected decimal fields: %#v", commands)
	}
	if commands[0].CreatedAtMS != createdAt.UnixMilli() || commands[0].ExpiresAtMS != expiresAt.UnixMilli() {
		t.Fatalf("unexpected command timestamps: %#v", commands[0])
	}
}

func TestBuildTradeCommandsRejectsDuplicateEngineBucket(t *testing.T) {
	_, err := buildTradeCommands(7, "BTCUSDT", 123456, time.Unix(100, 0), time.Unix(101, 0), quant.StrategyOutput{
		MicroIntents: []quant.TradeIntent{
			{Action: quant.ActionBuy, Engine: quant.EngineMicro, LotType: quant.LotTypeFloating, AmountUSDT: 10},
			{Action: quant.ActionBuy, Engine: quant.EngineMicro, LotType: quant.LotTypeFloating, AmountUSDT: 11},
		},
	})
	if err == nil {
		t.Fatal("expected duplicate client_order_id error")
	}
}

func TestNormalizeBarsAndACLExtraction(t *testing.T) {
	bars := normalizeBars([]quant.Bar{
		{OpenTime: 3000, Close: 3},
		{OpenTime: 0, Close: 9},
		{OpenTime: 1000, Close: 1},
		{OpenTime: 2000, Close: 0},
		{OpenTime: 2000, Close: 2},
	})
	closes, timestamps := extractACLSeries(bars)
	if len(bars) != 3 {
		t.Fatalf("expected 3 usable bars, got %d", len(bars))
	}
	wantCloses := []float64{1, 2, 3}
	wantTimestamps := []int64{1000, 2000, 3000}
	for i := range wantCloses {
		if closes[i] != wantCloses[i] || timestamps[i] != wantTimestamps[i] {
			t.Fatalf("series[%d] = close %v timestamp %v", i, closes[i], timestamps[i])
		}
	}
}

func TestRuntimeConfigIntervalPriority(t *testing.T) {
	cfg := RuntimeConfig{TMicro: "15m", Interval: "1h"}
	if got := cfg.interval("4h", "1d"); got != "15m" {
		t.Fatalf("t_micro should win, got %q", got)
	}
	cfg.TMicro = ""
	if got := cfg.interval("4h", "1d"); got != "1h" {
		t.Fatalf("config interval should win after t_micro, got %q", got)
	}
	cfg.Interval = ""
	if got := cfg.interval("4h", "1d"); got != "4h" {
		t.Fatalf("instance interval should win after config, got %q", got)
	}
	if got := (RuntimeConfig{}).interval("", "1d"); got != "1d" {
		t.Fatalf("spawn interval fallback got %q", got)
	}
}
