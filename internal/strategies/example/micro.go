package example

import "bian-trade-go/internal/quant"

func computeMicro(input quant.StrategyInput, ctx stepContext, floatBTC float64) quant.MicroEngineOutput {
	return quant.ComputeMicroDecisionV4(quant.MicroEngineInput{
		Closes:         input.Closes,
		CurrentPrice:   ctx.CurrentPrice,
		CurrentWeight:  ctx.CurrentMicroWeight,
		TotalEquity:    ctx.TotalEquity,
		SpendableUSDT:  ctx.SpendableUSDT,
		FloatBTC:       floatBTC,
		MinOrderUSDT:   ctx.MinOrderUSDT,
		Chromosome:     ctx.Chromosome,
		BetaMultiplier: ctx.MarketState.BetaMultiplier,
		IsQuiet:        ctx.MarketState.IsQuiet,
	})
}
