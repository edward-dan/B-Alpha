package quant

import "math"

const (
	MicroSignalEMABars      = 168
	MicroSignalStdDevBars   = 720
	MicroSignalMomentumBars = 72
	MicroVolRatioShortBars  = 24
	MicroVolRatioLongBars   = 720
	DefaultMicroSigmaFloor  = 0.0001
	DefaultMinOrderUSDT     = 10.1
)

type MicroEngineInput struct {
	Closes         []float64  `json:"closes"`
	CurrentPrice   float64    `json:"current_price"`
	CurrentWeight  float64    `json:"current_weight"`
	TotalEquity    float64    `json:"total_equity"`
	SpendableUSDT  float64    `json:"spendable_usdt"`
	FloatBTC       float64    `json:"float_btc"`
	MinOrderUSDT   float64    `json:"min_order_usdt"`
	SigmaFloor     float64    `json:"sigma_floor"`
	Chromosome     Chromosome `json:"chromosome"`
	BetaMultiplier float64    `json:"beta_multiplier"`
	IsQuiet        bool       `json:"is_quiet"`
}

type MicroEngineOutput struct {
	TargetWeight    float64 `json:"target_weight"`
	Signal          float64 `json:"signal"`
	TheoreticalUSD  float64 `json:"theoretical_usd"`
	OrderUSD        float64 `json:"order_usd"`
	VolatilityRatio float64 `json:"volatility_ratio"`
	DeltaWeight     float64 `json:"delta_weight"`
	EffectiveBeta   float64 `json:"effective_beta"`
	InventoryBias   float64 `json:"inventory_bias"`
}

// ComputeMicroDecision implements the Sigmoid dynamic balance.
//
// Design philosophy: Signal is the external force from market state, InventoryBias is
// the spring restoring force from current inventory, Beta is spring stiffness, Gamma
// decides whether the inventory spring is enabled, and VolatilityRatio wedge filtering
// suppresses quiet-period dust orders.
func ComputeMicroDecision(input MicroEngineInput) MicroEngineOutput {
	c := chromosomeOrDefault(input.Chromosome)
	minOrderUSDT := input.MinOrderUSDT
	if minOrderUSDT <= 0 {
		minOrderUSDT = DefaultMinOrderUSDT
	}
	sigmaFloor := input.SigmaFloor
	if sigmaFloor < 0 {
		sigmaFloor = 0
	}
	if sigmaFloor == 0 {
		sigmaFloor = DefaultMicroSigmaFloor
	}

	closes := closesWithCurrent(input.Closes, input.CurrentPrice)
	currentPrice := input.CurrentPrice
	if currentPrice <= 0 && len(closes) > 0 {
		currentPrice = closes[len(closes)-1]
	}
	currentWeight := ClipFloat64(input.CurrentWeight, 0, 1)
	if input.TotalEquity <= 0 || currentPrice <= 0 || len(closes) < 2 {
		return MicroEngineOutput{TargetWeight: currentWeight, VolatilityRatio: 1}
	}

	ema := EMA(closes, MicroSignalEMABars)
	returns := logReturns(closes)
	sigma := maxFloat(StdDev(returns, MicroSignalStdDevBars), sigmaFloor)
	if sigma == 0 {
		return MicroEngineOutput{TargetWeight: currentWeight, VolatilityRatio: 1}
	}

	signal := computeMicroSignal(closes, returns, currentPrice, ema, sigma, c)

	betaMultiplier := input.BetaMultiplier
	if betaMultiplier == 0 {
		betaMultiplier = 1
	}
	effectiveBeta := maxFloat(0.01, c.Beta*betaMultiplier)
	inventoryBias := ClipFloat64(currentWeight, 0, 1) - 0.5
	exponent := effectiveBeta*signal + c.Gamma*inventoryBias
	targetWeight := sigmoidFromExponent(exponent)
	targetWeight = ClipFloat64(targetWeight, c.MicroWeightMin, c.MicroWeightMax)
	targetWeight = ClipFloat64(targetWeight, 0, 1)

	deltaWeight := targetWeight - currentWeight
	theoreticalUSD := deltaWeight * input.TotalEquity
	volatilityRatio := computeMicroVolatilityRatio(closes)
	orderUSD := filterMicroOrderUSD(theoreticalUSD, deltaWeight, volatilityRatio, input, currentWeight, minOrderUSDT, c)

	return MicroEngineOutput{
		TargetWeight:    targetWeight,
		Signal:          signal,
		TheoreticalUSD:  theoreticalUSD,
		OrderUSD:        orderUSD,
		VolatilityRatio: volatilityRatio,
		DeltaWeight:     deltaWeight,
		EffectiveBeta:   effectiveBeta,
		InventoryBias:   inventoryBias,
	}
}

func ComputeMicroDecisionV4(input MicroEngineInput) MicroEngineOutput {
	return ComputeMicroDecision(input)
}

func computeMicroSignal(closes []float64, returns []float64, currentPrice, ema, sigma float64, c Chromosome) float64 {
	xDev := ClipFloat64(safeLogRatio(currentPrice, ema)/sigma, -3, 3)

	lastIndex := len(closes) - 1
	momentumDenominator := maxFloat(sigma*math.Sqrt(float64(MicroSignalMomentumBars)), epsilon)
	xMom := ClipFloat64(latestMomentum(closes, MicroSignalMomentumBars, lastIndex)/momentumDenominator, -3, 3)
	xMomPrev := ClipFloat64(latestMomentum(closes, MicroSignalMomentumBars, lastIndex-1)/momentumDenominator, -3, 3)
	xAccel := ClipFloat64(xMom-xMomPrev, -3, 3)

	xVol := 0.0
	if len(returns) >= MicroVolRatioLongBars {
		volShort := StdDev(returns, MicroVolRatioShortBars)
		volLong := maxFloat(StdDev(returns, MicroVolRatioLongBars), epsilon)
		xVol = ClipFloat64(volShort/volLong-1, -2, 2)
	}

	raw := c.WDev*xDev + c.WMom*xMom + c.WAccel*xAccel + c.WVol*xVol
	return ClipFloat64(raw, -c.SignalClip, c.SignalClip)
}

func computeMicroVolatilityRatio(closes []float64) float64 {
	if len(closes) < MicroVolRatioLongBars {
		return 1
	}
	shortMAV := MAVAbsChange(closes, MicroVolRatioShortBars)
	longMAV := MAVAbsChange(closes, MicroVolRatioLongBars)
	if longMAV <= 0 {
		return 1
	}
	return ClipFloat64(shortMAV/longMAV, 0.1, 3.0)
}

func filterMicroOrderUSD(theoreticalUSD, deltaWeight, volatilityRatio float64, input MicroEngineInput, currentWeight, minOrderUSDT float64, c Chromosome) float64 {
	absTheoretical := math.Abs(theoreticalUSD)
	if absTheoretical == 0 {
		return 0
	}

	orderUSD := 0.0
	switch {
	case absTheoretical >= minOrderUSDT:
		orderUSD = theoreticalUSD
	case !input.IsQuiet && (math.Abs(deltaWeight) >= c.WedgeWeightThreshold || volatilityRatio >= c.WedgeVolatilityThreshold):
		orderUSD = math.Copysign(minOrderUSDT, theoreticalUSD)
	default:
		return 0
	}

	return clampExecutableMicroOrderUSD(orderUSD, input, currentWeight, minOrderUSDT)
}

func clampExecutableMicroOrderUSD(orderUSD float64, input MicroEngineInput, currentWeight, minOrderUSDT float64) float64 {
	if orderUSD > 0 {
		if input.SpendableUSDT <= 0 {
			return 0
		}
		orderUSD = minFloat(orderUSD, input.SpendableUSDT)
		if orderUSD < minOrderUSDT {
			return 0
		}
		return orderUSD
	}

	availableFloatBTC := input.FloatBTC
	if availableFloatBTC <= 0 && input.CurrentPrice > 0 && input.TotalEquity > 0 && currentWeight > 0 {
		availableFloatBTC = currentWeight * input.TotalEquity / input.CurrentPrice
	}
	maxSellUSD := availableFloatBTC * input.CurrentPrice
	if maxSellUSD <= 0 {
		return 0
	}
	orderAbs := minFloat(math.Abs(orderUSD), maxSellUSD)
	if orderAbs < minOrderUSDT {
		return 0
	}
	return -orderAbs
}
