package example

import (
	"math"
	"time"

	"bian-trade-go/internal/quant"
)

type releaseDecision struct {
	Conversions []quant.LedgerConversion
	ReleasedBTC float64
	ReasonCodes []string
}

func computeDeadRelease(input quant.StrategyInput, ctx stepContext, micro quant.MicroEngineOutput) releaseDecision {
	lots := lotsForRelease(input, ctx)
	if len(lots) == 0 || ctx.CurrentPrice <= 0 || ctx.TotalEquity <= 0 {
		return releaseDecision{}
	}

	now := bucketTime(ctx.BucketTime)
	decision := releaseDecision{}

	if softReleaseAllowed(lots, now, ctx, micro) {
		target := softReleaseTargetBTC(lots, ctx)
		releasedLots, released := quant.SoftReleaseDeadBTC(lots, now, releaseAgeMonths(ctx.Chromosome), ctx.Chromosome.ReleaseMaxPctPerStep, target)
		if released > 0 {
			lots = releasedLots
			reason := softReleaseReason(lots, ctx)
			decision.ReleasedBTC += released
			decision.ReasonCodes = append(decision.ReasonCodes, reason)
			decision.Conversions = append(decision.Conversions, quant.LedgerConversion{
				FromLotType: quant.LotTypeDeadStack,
				ToLotType:   quant.LotTypeFloating,
				Amount:      released,
				ReasonCode:  reason,
			})
		}
	}

	requiredSellBTC := requiredSellBTCForHardRelease(micro, ctx)
	if requiredSellBTC > quant.TotalFloatBTC(lots) {
		_, released := quant.HardReleaseDeadBTC(lots, requiredSellBTC)
		if released > 0 {
			decision.ReleasedBTC += released
			decision.ReasonCodes = append(decision.ReasonCodes, "DEAD_RELEASE_HARD_MICRO_SELL")
			decision.Conversions = append(decision.Conversions, quant.LedgerConversion{
				FromLotType: quant.LotTypeDeadStack,
				ToLotType:   quant.LotTypeFloating,
				Amount:      released,
				ReasonCode:  "DEAD_RELEASE_HARD_MICRO_SELL",
			})
		}
	}

	return decision
}

func softReleaseAllowed(lots []quant.SpotLot, now time.Time, ctx stepContext, micro quant.MicroEngineOutput) bool {
	if ctx.MarketState.State == quant.MarketStateStressCrash || ctx.MarketState.State == quant.MarketStateBearTrend {
		return false
	}
	if !microNeedsSellableInventory(ctx, micro) {
		return false
	}
	if weightedDeadLotAgeDays(lots, now) < float64(ctx.Chromosome.ReleaseMinAgeDays) {
		return false
	}
	return profitable(lots, ctx) || overheated(ctx)
}

func softReleaseTargetBTC(lots []quant.SpotLot, ctx stepContext) float64 {
	deadBTC := quant.TotalDeadBTC(lots)
	if deadBTC <= 0 || ctx.CurrentPrice <= 0 {
		return 0
	}
	deadFloor := math.Max(0, ctx.Params.SpawnPoint.Policy.DeadBTCFloor)
	return minFloat(
		deadBTC*ctx.Chromosome.ReleaseMaxPctPerStep,
		math.Max(0, deadBTC-deadFloor),
		ctx.Chromosome.ReleaseTargetFloatWeightGap*ctx.TotalEquity/ctx.CurrentPrice,
	)
}

func requiredSellBTCForHardRelease(micro quant.MicroEngineOutput, ctx stepContext) float64 {
	if micro.TheoreticalUSD >= 0 || ctx.CurrentPrice <= 0 {
		return 0
	}
	requiredUSD := math.Abs(micro.TheoreticalUSD)
	if requiredUSD < ctx.MinOrderUSDT && !ctx.MarketState.IsQuiet &&
		(math.Abs(micro.DeltaWeight) >= ctx.Chromosome.WedgeWeightThreshold ||
			micro.VolatilityRatio >= ctx.Chromosome.WedgeVolatilityThreshold) {
		requiredUSD = ctx.MinOrderUSDT
	}
	if requiredUSD <= 0 {
		return 0
	}
	return requiredUSD / ctx.CurrentPrice
}

func lotsForRelease(input quant.StrategyInput, ctx stepContext) []quant.SpotLot {
	if len(input.SpotLots) > 0 {
		lots := make([]quant.SpotLot, len(input.SpotLots))
		copy(lots, input.SpotLots)
		return lots
	}

	lots := make([]quant.SpotLot, 0, 3)
	createdAt := bucketTime(ctx.BucketTime)
	if input.Portfolio.DeadBTC > 0 {
		lots = append(lots, quant.SpotLot{
			LotType:   quant.LotTypeDeadStack,
			Amount:    input.Portfolio.DeadBTC,
			CostPrice: ctx.CurrentPrice,
			CreatedAt: createdAt,
		})
	}
	if input.Portfolio.FloatBTC > 0 {
		lots = append(lots, quant.SpotLot{
			LotType:   quant.LotTypeFloating,
			Amount:    input.Portfolio.FloatBTC,
			CostPrice: ctx.CurrentPrice,
			CreatedAt: createdAt,
		})
	}
	if input.Portfolio.ColdSealedBTC > 0 {
		lots = append(lots, quant.SpotLot{
			LotType:      quant.LotTypeColdSealed,
			Amount:       input.Portfolio.ColdSealedBTC,
			CostPrice:    ctx.CurrentPrice,
			CreatedAt:    createdAt,
			IsColdSealed: true,
		})
	}
	return lots
}

func microNeedsSellableInventory(ctx stepContext, micro quant.MicroEngineOutput) bool {
	return micro.TargetWeight < ctx.CurrentMicroWeight || ctx.CurrentMicroWeight >= ctx.Chromosome.MicroWeightMax
}

func profitable(lots []quant.SpotLot, ctx stepContext) bool {
	cost := weightedDeadLotCost(lots)
	if cost <= 0 || ctx.CurrentPrice <= 0 {
		return false
	}
	return math.Log(ctx.CurrentPrice/cost) >= ctx.Chromosome.ReleaseProfitLogMin
}

func overheated(ctx stepContext) bool {
	if ctx.MarketState.State != quant.MarketStateBullTrend || len(ctx.Closes) == 0 || ctx.CurrentPrice <= 0 {
		return false
	}
	emaLong := quant.EMA(ctx.Closes, ctx.Chromosome.EMALongN)
	if emaLong <= 0 {
		return false
	}
	return math.Log(ctx.CurrentPrice/emaLong) >= ctx.Chromosome.ReleaseOverheatLogGap
}

func softReleaseReason(lots []quant.SpotLot, ctx stepContext) string {
	if overheated(ctx) {
		return "DEAD_RELEASE_OVERHEAT"
	}
	if profitable(lots, ctx) {
		return "DEAD_RELEASE_PROFIT_MATURED"
	}
	return "DEAD_RELEASE_MATURED"
}

func weightedDeadLotCost(lots []quant.SpotLot) float64 {
	var amountSum float64
	var costSum float64
	for _, lot := range lots {
		if lot.LotType != quant.LotTypeDeadStack || lot.IsColdSealed || lot.Amount <= 0 || lot.CostPrice <= 0 {
			continue
		}
		amountSum += lot.Amount
		costSum += lot.Amount * lot.CostPrice
	}
	if amountSum <= 0 {
		return 0
	}
	return costSum / amountSum
}

func weightedDeadLotAgeDays(lots []quant.SpotLot, now time.Time) float64 {
	var amountSum float64
	var ageSum float64
	for _, lot := range lots {
		if lot.LotType != quant.LotTypeDeadStack || lot.IsColdSealed || lot.Amount <= 0 {
			continue
		}
		ageDays := now.Sub(lot.CreatedAt).Hours() / 24
		if ageDays < 0 {
			ageDays = 0
		}
		amountSum += lot.Amount
		ageSum += lot.Amount * ageDays
	}
	if amountSum <= 0 {
		return 0
	}
	return ageSum / amountSum
}

func releaseAgeMonths(c quant.Chromosome) int {
	months := (c.ReleaseMinAgeDays + 29) / 30
	if months < 1 {
		return 1
	}
	return months
}

func bucketTime(ms int64) time.Time {
	if ms > 1_000_000_000_000 {
		return time.UnixMilli(ms).UTC()
	}
	return time.Unix(ms, 0).UTC()
}

func minFloat(values ...float64) float64 {
	min := math.Inf(1)
	for _, value := range values {
		if value < min {
			min = value
		}
	}
	if math.IsInf(min, 1) {
		return 0
	}
	return min
}
