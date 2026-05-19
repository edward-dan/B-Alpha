package example

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"bian-trade-go/internal/quant"
)

func TestStepSkipsInsufficientData(t *testing.T) {
	out := Step(quant.StrategyInput{
		Closes:       []float64{100, 101},
		CurrentPrice: 101,
		Portfolio: quant.PortfolioSnapshot{
			USDTBalance: 1000,
		},
		CurrentBucketTime: mustUnixMilli(2026, time.May, 1),
	}, DefaultParams())

	if !out.SkipTrading || out.SkipReason != "INSUFFICIENT_DATA" {
		t.Fatalf("unexpected output: %#v", out)
	}
	if len(out.MacroIntents) != 0 || len(out.MicroIntents) != 0 || len(out.LedgerConversions) != 0 {
		t.Fatalf("expected empty intents: %#v", out)
	}
}

func TestStepProducesMacroBuyIntent(t *testing.T) {
	params := DefaultParams()
	params.SpawnPoint.Policy.MonthlyInjectUSDT = 300

	out := Step(quant.StrategyInput{
		Closes:            flatCloses(800, 100),
		CurrentPrice:      100,
		CurrentBucketTime: mustUnixMilli(2026, time.May, 1),
		Portfolio: quant.PortfolioSnapshot{
			USDTBalance: 10000,
		},
		Constraints: quant.TradingConstraints{MinOrderUSDT: 10.1},
	}, params)

	if out.SkipTrading {
		t.Fatalf("unexpected skip: %#v", out)
	}
	if len(out.MacroIntents) != 1 {
		t.Fatalf("expected one macro intent: %#v", out)
	}
	intent := out.MacroIntents[0]
	if intent.Action != quant.ActionBuy || intent.Engine != quant.EngineMacro || intent.LotType != quant.LotTypeDeadStack {
		t.Fatalf("unexpected macro intent: %#v", intent)
	}
	if intent.AmountUSDT <= 0 {
		t.Fatalf("macro amount not positive: %#v", intent)
	}
}

func TestStepHardReleaseEnablesMinimumSell(t *testing.T) {
	params := DefaultParams()
	params.Chromosome = quant.DefaultSeedChromosome
	params.Chromosome.MicroWeightMin = 0
	params.Chromosome.Beta = 8
	params.Chromosome.WDev = 3
	params.Chromosome.WMom = 3
	params.Chromosome.WedgeWeightThreshold = 0.002
	params.Chromosome.ReleaseMinAgeDays = 1460

	closes := risingCloses(800, 100, 0.001)
	price := closes[len(closes)-1]
	now := mustUnixMilli(2026, time.May, 1)

	out := Step(quant.StrategyInput{
		Closes:            closes,
		CurrentPrice:      price,
		CurrentBucketTime: now,
		Portfolio: quant.PortfolioSnapshot{
			USDTBalance:   0,
			DeadBTC:       1,
			FloatBTC:      0.01,
			ColdSealedBTC: 1,
		},
		SpotLots: []quant.SpotLot{
			{LotType: quant.LotTypeDeadStack, Amount: 1, CostPrice: price, CreatedAt: time.UnixMilli(now).UTC()},
			{LotType: quant.LotTypeFloating, Amount: 0.01, CostPrice: price, CreatedAt: time.UnixMilli(now).UTC()},
			{LotType: quant.LotTypeColdSealed, Amount: 1, CostPrice: price, CreatedAt: time.UnixMilli(now).UTC(), IsColdSealed: true},
		},
		Constraints: quant.TradingConstraints{MinOrderUSDT: 10.1},
	}, params)

	if len(out.LedgerConversions) != 1 {
		t.Fatalf("expected one release conversion: %#v", out)
	}
	release := out.LedgerConversions[0]
	if release.FromLotType != quant.LotTypeDeadStack || release.ToLotType != quant.LotTypeFloating {
		t.Fatalf("unexpected release: %#v", release)
	}
	if release.Amount <= 0 {
		t.Fatalf("release amount not positive: %#v", release)
	}
	if len(out.MicroIntents) != 1 || out.MicroIntents[0].Action != quant.ActionSell {
		t.Fatalf("expected micro sell intent: %#v", out)
	}
	if out.MicroIntents[0].QtyAsset*price < 10.1 {
		t.Fatalf("sell intent below minimum: %#v", out.MicroIntents[0])
	}
}

func TestParseParamPack(t *testing.T) {
	if DefaultParams().SpawnPoint.InitialAvailableUSDT <= 0 {
		t.Fatal("default params should provide initial virtual capital for runnable backtests")
	}

	raw, err := json.Marshal(Params{
		Chromosome: quant.Chromosome{Beta: 100},
		SpawnPoint: quant.SpawnPoint{
			Symbol:     "BTCUSDT",
			QuoteAsset: "USDT",
			Precision:  quant.TradingConstraints{MinOrderUSDT: 12},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	params, err := ParseParamPack(raw)
	if err != nil {
		t.Fatal(err)
	}
	if params.Chromosome.Beta != quant.HardBounds["beta"].Max {
		t.Fatalf("beta was not clamped: %#v", params.Chromosome)
	}
	if params.SpawnPoint.Precision.MinOrderUSDT != 12 {
		t.Fatalf("spawn point not decoded: %#v", params.SpawnPoint)
	}

	raw, err = json.Marshal(quant.Chromosome{Beta: 100})
	if err != nil {
		t.Fatal(err)
	}
	params, err = ParseParamPack(raw)
	if err != nil {
		t.Fatal(err)
	}
	if params.Chromosome.Beta != quant.HardBounds["beta"].Max {
		t.Fatalf("direct chromosome was not decoded: %#v", params.Chromosome)
	}
}

func flatCloses(n int, price float64) []float64 {
	closes := make([]float64, n)
	for i := range closes {
		closes[i] = price
	}
	return closes
}

func risingCloses(n int, start, logStep float64) []float64 {
	closes := make([]float64, n)
	for i := range closes {
		closes[i] = start * math.Exp(float64(i)*logStep)
	}
	return closes
}

func mustUnixMilli(year int, month time.Month, day int) int64 {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).UnixMilli()
}
