package quant

import (
	"math"
	"testing"
	"time"
)

func TestMathHelpers(t *testing.T) {
	if got := EMA([]float64{1, 2, 3}, 2); !almostEqual(got, 2.5555555556) {
		t.Fatalf("EMA = %.10f", got)
	}
	if got := StdDev([]float64{1, 2, 3}, 3); !almostEqual(got, 1) {
		t.Fatalf("StdDev = %.10f", got)
	}
	if got := MAVAbsChange([]float64{10, 12, 9, 15}, 4); !almostEqual(got, 11.0/3.0) {
		t.Fatalf("MAVAbsChange = %.10f", got)
	}
	if got := ClipFloat64(5, 1, 3); got != 3 {
		t.Fatalf("ClipFloat64 = %.10f", got)
	}
	if got := RoundToUSDT(12.345); got != 12.35 {
		t.Fatalf("RoundToUSDT = %.10f", got)
	}
}

func TestExtractClosesAndTimestamps(t *testing.T) {
	bars := []Bar{
		{OpenTime: 1000, Close: 10},
		{OpenTime: 2000, Close: 11},
	}
	closes := ExtractCloses(bars)
	timestamps := ExtractTimestamps(bars)
	if len(closes) != 2 || closes[0] != 10 || closes[1] != 11 {
		t.Fatalf("unexpected closes: %#v", closes)
	}
	if len(timestamps) != 2 || timestamps[0] != 1000 || timestamps[1] != 2000 {
		t.Fatalf("unexpected timestamps: %#v", timestamps)
	}
}

func TestClampChromosomeRepairsStructuralConstraints(t *testing.T) {
	c := DefaultSeedChromosome
	c.EMAFastN = 90
	c.EMAMidN = 48
	c.EMALongN = 240
	c.VolShortN = 168
	c.VolLongN = 240
	c.MicroReservePct = 0.30
	c.MicroWeightMax = 1.0

	got := ClampChromosome(c)
	if got.EMAMidN < 2*got.EMAFastN {
		t.Fatalf("EMA mid was not expanded: %#v", got)
	}
	if got.EMALongN < 2*got.EMAMidN {
		t.Fatalf("EMA long was not expanded: %#v", got)
	}
	if got.VolLongN < 3*got.VolShortN {
		t.Fatalf("vol long was not expanded: %#v", got)
	}
	if got.MicroWeightMax > 1-got.MicroReservePct {
		t.Fatalf("micro weight max violates reserve: %#v", got)
	}
}

func TestLotReleaseNeverUsesColdSealed(t *testing.T) {
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	lots := []SpotLot{
		{LotType: LotTypeDeadStack, Amount: 1, CostPrice: 100, CreatedAt: now.AddDate(-2, 0, 0)},
		{LotType: LotTypeDeadStack, Amount: 1, CostPrice: 100, CreatedAt: now.AddDate(-2, 0, 0), IsColdSealed: true},
		{LotType: LotTypeFloating, Amount: 0.2, CostPrice: 100, CreatedAt: now},
	}

	softLots, released := SoftReleaseDeadBTC(lots, now, 12, 0.1, 0.5)
	if !almostEqual(released, 0.1) {
		t.Fatalf("soft released %.10f", released)
	}
	if !almostEqual(TotalColdSealedBTC(softLots), 1) {
		t.Fatalf("cold sealed changed: %#v", softLots)
	}

	hardLots, hardReleased := HardReleaseDeadBTC(softLots, 0.6)
	if !almostEqual(hardReleased, 0.3) {
		t.Fatalf("hard released %.10f", hardReleased)
	}
	if !almostEqual(TotalFloatBTC(hardLots), 0.6) {
		t.Fatalf("floating total = %.10f", TotalFloatBTC(hardLots))
	}
	if !almostEqual(TotalColdSealedBTC(hardLots), 1) {
		t.Fatalf("cold sealed changed after hard release: %#v", hardLots)
	}
}

func TestComputeMarketStateQuiet(t *testing.T) {
	closes := make([]float64, 800)
	for i := range closes {
		closes[i] = 100
	}

	state := ComputeMarketState(closes, DefaultSeedChromosome)
	if state.State != MarketStateQuiet || !state.IsQuiet {
		t.Fatalf("state = %#v", state)
	}
}

func TestComputeMicroDecisionRespectsQuietDustFilter(t *testing.T) {
	closes := risingCloses(800, 100, 0.0005)
	input := MicroEngineInput{
		Closes:         closes,
		CurrentPrice:   closes[len(closes)-1],
		CurrentWeight:  0.5,
		TotalEquity:    100,
		SpendableUSDT:  100,
		FloatBTC:       0.5,
		MinOrderUSDT:   10.1,
		Chromosome:     DefaultSeedChromosome,
		BetaMultiplier: 1,
		IsQuiet:        true,
	}

	out := ComputeMicroDecision(input)
	if out.TargetWeight < DefaultSeedChromosome.MicroWeightMin || out.TargetWeight > DefaultSeedChromosome.MicroWeightMax {
		t.Fatalf("target out of bounds: %#v", out)
	}
	if math.Abs(out.TheoreticalUSD) < input.MinOrderUSDT && out.OrderUSD != 0 {
		t.Fatalf("quiet dust order was not filtered: %#v", out)
	}
}

func TestComputeMacroDecisionOnlyBuysDeadStack(t *testing.T) {
	closes := flatCloses(800, 100)
	out := ComputeMacroDecision(MacroDecisionInput{
		Closes:            closes,
		CurrentPrice:      100,
		TotalEquity:       10000,
		SpendableUSDT:     1000,
		MonthlyInjectUSDT: 500,
		CurrentBucketTime: 7 * millisPerDay,
		Chromosome:        DefaultSeedChromosome,
		MarketState:       transitionMarketState(),
		MinOrderUSDT:      10.1,
	})

	if !out.ShouldBuy {
		t.Fatalf("expected buy decision: %#v", out)
	}
	if out.Intent.Action != ActionBuy || out.Intent.LotType != LotTypeDeadStack || out.Intent.Engine != EngineMacro {
		t.Fatalf("unexpected intent: %#v", out.Intent)
	}
}

func TestSimulateGhostDCA(t *testing.T) {
	bars := []Bar{
		{OpenTime: mustUnixMilli(2023, time.January, 1), Close: 100},
		{OpenTime: mustUnixMilli(2023, time.February, 1), Close: 100},
		{OpenTime: mustUnixMilli(2023, time.March, 1), Close: 200},
	}

	result := SimulateGhostDCA(bars, GhostDCAConfig{InitialCapital: 100, MonthlyInject: 100})
	if !almostEqual(result.FinalEquity, 500) {
		t.Fatalf("FinalEquity = %.10f", result.FinalEquity)
	}
	if !almostEqual(result.TotalInjected, 300) {
		t.Fatalf("TotalInjected = %.10f", result.TotalInjected)
	}
	if result.ROI <= 0 {
		t.Fatalf("expected positive Modified Dietz ROI: %#v", result)
	}
}

func TestMaxDrawdown(t *testing.T) {
	if got := MaxDrawdown([]float64{100, 120, 90, 150}); !almostEqual(got, 0.25) {
		t.Fatalf("MaxDrawdown = %.10f", got)
	}
}

func risingCloses(n int, start, logStep float64) []float64 {
	closes := make([]float64, n)
	for i := range closes {
		closes[i] = start * math.Exp(float64(i)*logStep)
	}
	return closes
}

func flatCloses(n int, price float64) []float64 {
	closes := make([]float64, n)
	for i := range closes {
		closes[i] = price
	}
	return closes
}

func mustUnixMilli(year int, month time.Month, day int) int64 {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).UnixMilli()
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9
}
