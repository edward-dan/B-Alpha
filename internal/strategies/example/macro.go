package example

import "bian-trade-go/internal/quant"

func computeMacro(input quant.StrategyInput, ctx stepContext) quant.MacroDecision {
	return quant.ComputeMacroDecision(quant.MacroDecisionInput{
		Closes:            input.Closes,
		CurrentPrice:      ctx.CurrentPrice,
		TotalEquity:       ctx.TotalEquity,
		SpendableUSDT:     ctx.SpendableUSDT,
		MonthlyInjectUSDT: ctx.Params.SpawnPoint.Policy.MonthlyInjectUSDT,
		LastMacroBuyAt:    input.Runtime.LastMacroBuyAt,
		CurrentBucketTime: ctx.BucketTime,
		Chromosome:        ctx.Chromosome,
		MarketState:       ctx.MarketState,
		MinOrderUSDT:      ctx.MinOrderUSDT,
	})
}
