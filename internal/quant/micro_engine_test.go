package quant

import (
	"math"
	"reflect"
	"testing"
)

func TestMicroSigmoidPositiveSignalTargetsBelowNeutral(t *testing.T) {
	out := ComputeMicroDecision(microSigmoidFixture(105, 1000, false))

	if out.Signal <= 0 {
		t.Fatalf("expected positive signal, got %#v", out)
	}
	if out.TargetWeight >= 0.5 {
		t.Fatalf("positive signal should target below neutral: %#v", out)
	}
}

func TestMicroSigmoidNegativeSignalTargetsAboveNeutral(t *testing.T) {
	out := ComputeMicroDecision(microSigmoidFixture(95, 1000, false))

	if out.Signal >= 0 {
		t.Fatalf("expected negative signal, got %#v", out)
	}
	if out.TargetWeight <= 0.5 {
		t.Fatalf("negative signal should target above neutral: %#v", out)
	}
}

func TestMicroSigmoidNeutralWeightGammaBiasIsZero(t *testing.T) {
	input := microNeutralSignalFixture()
	input.Chromosome.Gamma = 2
	input.CurrentWeight = 0.5

	out := ComputeMicroDecision(input)
	if out.Signal != 0 {
		t.Fatalf("expected neutral signal, got %#v", out)
	}
	if out.InventoryBias != 0 {
		t.Fatalf("expected zero inventory bias at neutral weight, got %#v", out)
	}
	if !microFloatEqual(out.TargetWeight, 0.5) {
		t.Fatalf("neutral weight with gamma should keep target neutral: %#v", out)
	}
}

func TestMicroDecisionDeterministicForSameInput(t *testing.T) {
	input := microSigmoidFixture(95, 20, false)

	first := ComputeMicroDecision(input)
	second := ComputeMicroDecision(input)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same input produced different outputs:\nfirst=%#v\nsecond=%#v", first, second)
	}
}

func TestMicroQuietDustOrderIsFiltered(t *testing.T) {
	input := microSigmoidFixture(105, 20, true)

	out := ComputeMicroDecision(input)
	if math.Abs(out.TheoreticalUSD) >= input.MinOrderUSDT {
		t.Fatalf("fixture should remain below min order: %#v", out)
	}
	if out.OrderUSD != 0 {
		t.Fatalf("quiet dust order should be filtered: %#v", out)
	}
}

func TestMicroNonQuietWedgeForcesMinimumOrder(t *testing.T) {
	input := microSigmoidFixture(95, 20, false)

	out := ComputeMicroDecision(input)
	if math.Abs(out.TheoreticalUSD) >= input.MinOrderUSDT {
		t.Fatalf("fixture should remain below min order: %#v", out)
	}
	if math.Abs(out.DeltaWeight) < input.Chromosome.WedgeWeightThreshold {
		t.Fatalf("fixture should satisfy wedge weight threshold: %#v", out)
	}
	if !microFloatEqual(math.Abs(out.OrderUSD), input.MinOrderUSDT) {
		t.Fatalf("non-quiet wedge should force minimum order: %#v", out)
	}
	if math.Signbit(out.OrderUSD) != math.Signbit(out.TheoreticalUSD) {
		t.Fatalf("forced wedge order should keep theoretical direction: %#v", out)
	}
}

func microSigmoidFixture(currentPrice float64, totalEquity float64, isQuiet bool) MicroEngineInput {
	chromosome := DefaultSeedChromosome
	chromosome.WDev = 1
	chromosome.WMom = 0
	chromosome.WAccel = 0
	chromosome.WVol = 0
	chromosome.Beta = 1.6
	chromosome.Gamma = 0
	chromosome.MicroWeightMin = 0
	chromosome.MicroWeightMax = 1
	chromosome.WedgeWeightThreshold = 0.002

	currentWeight := 0.5
	return MicroEngineInput{
		Closes:         constantMicroCloses(800, 100),
		CurrentPrice:   currentPrice,
		CurrentWeight:  currentWeight,
		TotalEquity:    totalEquity,
		SpendableUSDT:  totalEquity,
		FloatBTC:       currentWeight * totalEquity / currentPrice,
		MinOrderUSDT:   DefaultMinOrderUSDT,
		Chromosome:     chromosome,
		BetaMultiplier: 1,
		IsQuiet:        isQuiet,
	}
}

func microNeutralSignalFixture() MicroEngineInput {
	chromosome := DefaultSeedChromosome
	chromosome.WDev = 0
	chromosome.WMom = 0
	chromosome.WAccel = 0
	chromosome.WVol = 0
	chromosome.Beta = 1.6
	chromosome.MicroWeightMin = 0
	chromosome.MicroWeightMax = 1

	return MicroEngineInput{
		Closes:         constantMicroCloses(800, 100),
		CurrentPrice:   100,
		CurrentWeight:  0.5,
		TotalEquity:    1000,
		SpendableUSDT:  500,
		FloatBTC:       5,
		MinOrderUSDT:   DefaultMinOrderUSDT,
		Chromosome:     chromosome,
		BetaMultiplier: 1,
	}
}

func constantMicroCloses(n int, price float64) []float64 {
	closes := make([]float64, n)
	for i := range closes {
		closes[i] = price
	}
	return closes
}

func microFloatEqual(a, b float64) bool {
	return math.Abs(a-b) <= 1e-12
}
