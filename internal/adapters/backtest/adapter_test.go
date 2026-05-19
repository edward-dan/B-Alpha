package backtest

import (
	"context"
	"math"
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
	if first != second {
		t.Fatalf("same backtest inputs should produce identical result:\nfirst=%#v\nsecond=%#v", first, second)
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
