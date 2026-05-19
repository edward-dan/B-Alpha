package backtest

import (
	"context"
	"math"
	"reflect"
	"testing"
	"time"

	"bian-trade-go/internal/quant"
	"bian-trade-go/internal/strategies/example"
)

func TestRunBacktestDeterministicForSameInputs(t *testing.T) {
	params := example.DefaultParams()
	params.SpawnPoint.InitialAvailableUSDT = 10_000
	params.SpawnPoint.Policy.MonthlyInjectUSDT = 250
	params.SpawnPoint.Precision = quant.TradingConstraints{
		MinOrderUSDT: quant.DefaultMinOrderUSDT,
		LotStep:      0.00000001,
		LotMin:       0.00000001,
		FeeRate:      0.001,
		SlippagePct:  0.0005,
	}

	bars := deterministicBacktestBars(900)
	cfg := Config{
		Bars:        bars,
		EvalStartMs: bars[0].OpenTime,
		Chromosome:  params.Chromosome,
		SpawnPoint:  params.SpawnPoint,
		Constraints: params.SpawnPoint.Precision,
		Step: func(input quant.StrategyInput) quant.StrategyOutput {
			return example.Step(input, params)
		},
	}

	first, err := RunBacktest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("first backtest failed: %v", err)
	}
	second, err := RunBacktest(context.Background(), cfg)
	if err != nil {
		t.Fatalf("second backtest failed: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same backtest inputs should produce identical result:\nfirst=%#v\nsecond=%#v", first, second)
	}
}

func TestRunBacktestSupportsExplicitShortMarginLedger(t *testing.T) {
	params := example.DefaultParams()
	params.SpawnPoint.Symbol = "BTCUSDT"
	params.SpawnPoint.InitialAvailableUSDT = 1000
	params.SpawnPoint.Precision = quant.TradingConstraints{
		MinOrderUSDT: 10,
		LotStep:      0.00000001,
		LotMin:       0.00000001,
		FeeRate:      0,
		SlippagePct:  0,
	}
	bars := []quant.Bar{
		{OpenTime: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), Close: 100, Open: 100, High: 101, Low: 99, Volume: 10},
		{OpenTime: time.Date(2024, time.January, 1, 1, 0, 0, 0, time.UTC).UnixMilli(), Close: 90, Open: 90, High: 91, Low: 89, Volume: 10},
		{OpenTime: time.Date(2024, time.January, 1, 2, 0, 0, 0, time.UTC).UnixMilli(), Close: 80, Open: 80, High: 81, Low: 79, Volume: 10},
	}

	result, err := RunBacktest(context.Background(), Config{
		Bars:        bars,
		EvalStartMs: bars[0].OpenTime,
		Chromosome:  params.Chromosome,
		SpawnPoint:  params.SpawnPoint,
		Constraints: params.SpawnPoint.Precision,
		Leverage:    5,
		Direction:   DirectionLongShort,
		Step: func(input quant.StrategyInput) quant.StrategyOutput {
			out := quant.StrategyOutput{NextRuntime: input.Runtime}
			switch len(input.Closes) {
			case 1:
				out.MicroIntents = []quant.TradeIntent{{
					Action:       quant.ActionSell,
					Engine:       quant.EngineMicro,
					PositionSide: quant.PositionSideShort,
					Offset:       quant.OrderOffsetOpen,
					AmountUSDT:   100,
					ReasonCode:   "TEST_SHORT_OPEN",
				}}
			case 3:
				out.MicroIntents = []quant.TradeIntent{{
					Action:       quant.ActionBuy,
					Engine:       quant.EngineMicro,
					PositionSide: quant.PositionSideShort,
					Offset:       quant.OrderOffsetClose,
					ReasonCode:   "TEST_SHORT_CLOSE",
				}}
			}
			return out
		},
	})
	if err != nil {
		t.Fatalf("margin backtest failed: %v", err)
	}
	if result.TradeCount != 1 {
		t.Fatalf("expected one completed short trade, got %d", result.TradeCount)
	}
	if result.RealizedPnL <= 0 {
		t.Fatalf("expected profitable short trade, got realized pnl %.8f", result.RealizedPnL)
	}
	if len(result.Orders) != 2 || result.Orders[0].PositionSide != "SHORT" || result.Orders[1].Offset != "CLOSE" {
		t.Fatalf("unexpected simulated short orders: %#v", result.Orders)
	}
}

func deterministicBacktestBars(n int) []quant.Bar {
	start := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	bars := make([]quant.Bar, n)
	for i := range bars {
		closePrice := 100 * math.Exp(float64(i)*0.0002+0.01*math.Sin(float64(i)/17))
		openPrice := closePrice * (1 - 0.0008)
		bars[i] = quant.Bar{
			OpenTime: start.Add(time.Duration(i) * time.Hour).UnixMilli(),
			Open:     openPrice,
			High:     math.Max(openPrice, closePrice) * 1.001,
			Low:      math.Min(openPrice, closePrice) * 0.999,
			Close:    closePrice,
			Volume:   1000 + float64(i%23),
		}
	}
	return bars
}
