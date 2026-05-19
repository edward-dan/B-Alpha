package quant

import "math"

const (
	MarketStateStressCrash = "STRESS_CRASH"
	MarketStateQuiet       = "QUIET"
	MarketStateBullTrend   = "BULL_TREND"
	MarketStateBearTrend   = "BEAR_TREND"
	MarketStateTransition  = "TRANSITION"
)

type MarketState struct {
	State                  string  `json:"state"`
	TimeDilationMultiplier float64 `json:"time_dilation_multiplier"`
	BetaMultiplier         float64 `json:"beta_multiplier"`
	IsQuiet                bool    `json:"is_quiet"`
}

// ComputeMarketState classifies the latest closed price sequence using dimensionless features.
func ComputeMarketState(closes []float64, chromosome Chromosome) MarketState {
	c := chromosomeOrDefault(chromosome)
	if len(closes) < 2 {
		return transitionMarketState()
	}

	cleanCloses := closesWithCurrent(closes, 0)
	required := maxInt(c.EMALongN, c.MomentumN+1)
	required = maxInt(required, c.VolLongN+1)
	required = maxInt(required, c.DrawdownN)
	if len(cleanCloses) < required {
		return transitionMarketState()
	}

	returns := logReturns(cleanCloses)
	volShort := StdDev(returns, c.VolShortN)
	volLong := maxFloat(StdDev(returns, c.VolLongN), epsilon)
	volatilityRatio := ClipFloat64(volShort/volLong, 0.1, 3.0)

	emaFast := EMA(cleanCloses, c.EMAFastN)
	emaMid := EMA(cleanCloses, c.EMAMidN)
	emaLong := EMA(cleanCloses, c.EMALongN)
	trendFast := safeLogRatio(emaFast, emaMid)
	trendSlow := safeLogRatio(emaMid, emaLong)
	momentum := latestMomentum(cleanCloses, c.MomentumN, len(cleanCloses)-1)
	drawdown := drawdownFromHigh(cleanCloses, c.DrawdownN)
	latestReturn := returns[len(returns)-1]
	returnZ := latestReturn / maxFloat(volLong, epsilon)

	switch {
	case drawdown >= c.StressDrawdownPct || (returnZ <= -c.StressReturnZ && volatilityRatio >= c.StressVolRatioMin):
		return MarketState{
			State:                  MarketStateStressCrash,
			TimeDilationMultiplier: 1.0,
			BetaMultiplier:         c.MarketBetaStress,
			IsQuiet:                false,
		}
	case math.Abs(trendFast) <= c.QuietTrendThreshold &&
		math.Abs(momentum) <= c.QuietMomentumThreshold &&
		volatilityRatio <= c.QuietVolRatioMax:
		return MarketState{
			State:                  MarketStateQuiet,
			TimeDilationMultiplier: 1.0,
			BetaMultiplier:         c.MarketBetaQuiet,
			IsQuiet:                true,
		}
	case trendFast >= c.BullTrendMin && trendSlow >= 0 && momentum >= c.BullMomentumMin:
		return MarketState{
			State:                  MarketStateBullTrend,
			TimeDilationMultiplier: 1.0,
			BetaMultiplier:         c.MarketBetaBull,
			IsQuiet:                false,
		}
	case trendFast <= -c.BearTrendMin && trendSlow <= 0 && momentum <= -c.BearMomentumMin:
		return MarketState{
			State:                  MarketStateBearTrend,
			TimeDilationMultiplier: 1.0,
			BetaMultiplier:         c.MarketBetaBear,
			IsQuiet:                false,
		}
	default:
		return transitionMarketState()
	}
}

func transitionMarketState() MarketState {
	return MarketState{
		State:                  MarketStateTransition,
		TimeDilationMultiplier: 1.0,
		BetaMultiplier:         1.0,
		IsQuiet:                false,
	}
}
