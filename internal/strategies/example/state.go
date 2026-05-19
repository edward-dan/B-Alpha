package example

import "bian-trade-go/internal/quant"

type RuntimeState = quant.RuntimeState

type stepContext struct {
	Params             Params
	Chromosome         quant.Chromosome
	Closes             []float64
	CurrentPrice       float64
	TotalEquity        float64
	SpendableUSDT      float64
	CurrentMicroWeight float64
	BucketTime         int64
	MinOrderUSDT       float64
	MarketState        quant.MarketState
}
