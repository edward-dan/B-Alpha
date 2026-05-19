package example

import (
	"math"

	"bian-trade-go/internal/quant"
)

func Step(input quant.StrategyInput, params Params) quant.StrategyOutput {
	params = paramsOrDefault(params)
	chromosome := quant.ClampChromosome(params.Chromosome)
	price := currentPrice(input)
	bucket := currentBucket(input)

	out := quant.StrategyOutput{
		NextRuntime: input.Runtime,
	}
	if bucket > 0 {
		out.NextRuntime.LastProcessedBucket = bucket
	}

	if price <= 0 {
		out.SkipTrading = true
		out.SkipReason = "NO_PRICE"
		out.ReasonCodes = append(out.ReasonCodes, "NO_PRICE")
		out.NextRuntime.LastReasonCodes = out.ReasonCodes
		return out
	}
	if len(input.Closes) < requiredBars(chromosome) {
		out.SkipTrading = true
		out.SkipReason = "INSUFFICIENT_DATA"
		out.ReasonCodes = append(out.ReasonCodes, "INSUFFICIENT_DATA")
		out.NextRuntime.LastReasonCodes = out.ReasonCodes
		return out
	}

	totalEquity := computeTotalEquity(input.Portfolio, price)
	spendableUSDT := computeSpendableUSDT(input, totalEquity, chromosome)
	currentMicroWeight := computeCurrentMicroWeight(input.Portfolio, price, totalEquity)
	if totalEquity <= 0 {
		out.SkipTrading = true
		out.SkipReason = "NO_EQUITY"
		out.ReasonCodes = append(out.ReasonCodes, "NO_EQUITY")
		out.NextRuntime.LastReasonCodes = out.ReasonCodes
		return out
	}

	marketState := quant.ComputeMarketState(input.Closes, chromosome)
	ctx := stepContext{
		Params:             params,
		Chromosome:         chromosome,
		Closes:             input.Closes,
		CurrentPrice:       price,
		TotalEquity:        totalEquity,
		SpendableUSDT:      spendableUSDT,
		CurrentMicroWeight: currentMicroWeight,
		BucketTime:         bucket,
		MinOrderUSDT:       minOrderUSDT(input, params),
		MarketState:        marketState,
	}

	macro := computeMacro(input, ctx)
	if macro.ShouldBuy {
		out.MacroIntents = append(out.MacroIntents, macro.Intent)
		out.NextRuntime.LastMacroBuyAt = bucket
	}
	out.ReasonCodes = appendReason(out.ReasonCodes, macro.ReasonCode)

	micro := computeMicro(input, ctx, input.Portfolio.FloatBTC)
	release := computeDeadRelease(input, ctx, micro)
	if len(release.Conversions) > 0 {
		out.LedgerConversions = append(out.LedgerConversions, release.Conversions...)
		out.ReasonCodes = append(out.ReasonCodes, release.ReasonCodes...)
		if micro.TheoreticalUSD < 0 {
			micro = computeMicro(input, ctx, input.Portfolio.FloatBTC+release.ReleasedBTC)
		}
	}

	if intent, ok := microIntent(micro, ctx); ok {
		out.MicroIntents = append(out.MicroIntents, intent)
		out.ReasonCodes = appendReason(out.ReasonCodes, intent.ReasonCode)
	} else {
		out.ReasonCodes = appendReason(out.ReasonCodes, "MICRO_NO_ORDER")
	}

	out.NextRuntime.LastProcessedBucket = bucket
	out.NextRuntime.LastMarketState = marketState.State
	out.NextRuntime.LastMicroTargetBasis = micro.TargetWeight
	out.NextRuntime.LastReasonCodes = out.ReasonCodes
	return out
}

func currentPrice(input quant.StrategyInput) float64 {
	if input.CurrentPrice > 0 {
		return input.CurrentPrice
	}
	if len(input.Closes) == 0 {
		return 0
	}
	return input.Closes[len(input.Closes)-1]
}

func currentBucket(input quant.StrategyInput) int64 {
	if input.CurrentBucketTime > 0 {
		return input.CurrentBucketTime
	}
	if len(input.Timestamps) == 0 {
		return 0
	}
	return input.Timestamps[len(input.Timestamps)-1]
}

func requiredBars(c quant.Chromosome) int {
	required := maxInt(c.EMALongN, c.VolLongN+1)
	required = maxInt(required, c.DrawdownN)
	required = maxInt(required, c.MomentumN+1)
	required = maxInt(required, quant.MicroVolRatioLongBars)
	return required
}

func computeTotalEquity(portfolio quant.PortfolioSnapshot, price float64) float64 {
	totalBTC := portfolio.DeadBTC + portfolio.FloatBTC + portfolio.ColdSealedBTC
	return portfolio.USDTBalance + totalBTC*price
}

func computeSpendableUSDT(input quant.StrategyInput, totalEquity float64, c quant.Chromosome) float64 {
	reserveFloor := totalEquity * c.MicroReservePct
	spendable := math.Max(0, input.Portfolio.USDTBalance-reserveFloor)
	if input.SpendableUSDT > 0 && input.SpendableUSDT < spendable {
		return input.SpendableUSDT
	}
	return spendable
}

func computeCurrentMicroWeight(portfolio quant.PortfolioSnapshot, price, totalEquity float64) float64 {
	if totalEquity <= 0 || price <= 0 {
		return 0
	}
	return quant.ClipFloat64(portfolio.FloatBTC*price/totalEquity, 0, 1)
}

func minOrderUSDT(input quant.StrategyInput, params Params) float64 {
	switch {
	case input.Constraints.MinOrderUSDT > 0:
		return input.Constraints.MinOrderUSDT
	case params.SpawnPoint.Precision.MinOrderUSDT > 0:
		return params.SpawnPoint.Precision.MinOrderUSDT
	default:
		return quant.DefaultMinOrderUSDT
	}
}

func microIntent(micro quant.MicroEngineOutput, ctx stepContext) (quant.TradeIntent, bool) {
	if micro.OrderUSD > 0 {
		return quant.TradeIntent{
			Action:     quant.ActionBuy,
			Engine:     quant.EngineMicro,
			LotType:    quant.LotTypeFloating,
			AmountUSDT: micro.OrderUSD,
			ReasonCode: "MICRO_REBALANCE_BUY",
		}, true
	}
	if micro.OrderUSD < 0 && ctx.CurrentPrice > 0 {
		return quant.TradeIntent{
			Action:     quant.ActionSell,
			Engine:     quant.EngineMicro,
			LotType:    quant.LotTypeFloating,
			QtyAsset:   math.Abs(micro.OrderUSD) / ctx.CurrentPrice,
			ReasonCode: "MICRO_REBALANCE_SELL",
		}, true
	}
	return quant.TradeIntent{}, false
}

func appendReason(reasons []string, reason string) []string {
	if reason == "" {
		return reasons
	}
	return append(reasons, reason)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
