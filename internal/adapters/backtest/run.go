package backtest

import (
	"context"
	"errors"
	"math"
	"time"

	"bian-trade-go/internal/quant"
)

type StepFunc func(quant.StrategyInput) quant.StrategyOutput

type Config struct {
	Bars        []quant.Bar
	EvalStartMs int64
	Chromosome  quant.Chromosome
	SpawnPoint  quant.SpawnPoint
	Constraints quant.TradingConstraints
	Step        StepFunc
}

type Result struct {
	FinalEquity   float64 `json:"final_equity"`
	TotalInjected float64 `json:"total_injected"`
	ROI           float64 `json:"roi"`
	MaxDrawdown   float64 `json:"max_drawdown"`
}

type cashFlow struct {
	amount float64
	at     time.Time
}

// RunBacktest replays one closed-bar window and delegates every strategy
// decision to the supplied Step function.
func RunBacktest(ctx context.Context, cfg Config) (Result, error) {
	if cfg.Step == nil {
		return Result{}, errors.New("run backtest: Step function is required")
	}
	if len(cfg.Bars) == 0 {
		return Result{}, errors.New("run backtest: no bars")
	}

	evalIndex := firstEvalIndex(cfg.Bars, cfg.EvalStartMs)
	if evalIndex >= len(cfg.Bars) {
		return Result{}, errors.New("run backtest: no bars at or after EvalStartMs")
	}
	firstEvalBar := cfg.Bars[evalIndex]
	if firstEvalBar.Close <= 0 {
		return Result{}, errors.New("run backtest: first evaluation close must be positive")
	}

	constraints := mergeConstraints(cfg.SpawnPoint, cfg.Constraints)
	portfolio := initialPortfolio(cfg.SpawnPoint)
	lots := initialLots(cfg.SpawnPoint, firstEvalBar.Close, barTime(firstEvalBar.OpenTime))
	runtimeState := quant.RuntimeState{}

	initialEquity := totalEquity(portfolio, firstEvalBar.Close)
	totalInjected := math.Max(initialEquity, 0)
	unitCount := totalInjected
	flows := make([]cashFlow, 0)
	unitNAV := make([]float64, 0, len(cfg.Bars)-evalIndex)
	lastMonth := monthKey(barTime(firstEvalBar.OpenTime))

	for i, bar := range cfg.Bars {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if i < evalIndex {
			continue
		}
		if bar.Close <= 0 {
			continue
		}

		now := barTime(bar.OpenTime)
		currentMonth := monthKey(now)
		if i > evalIndex && cfg.SpawnPoint.Policy.MonthlyInjectUSDT > 0 && currentMonth != lastMonth {
			flow := cfg.SpawnPoint.Policy.MonthlyInjectUSDT
			equityBeforeFlow := totalEquity(portfolio, bar.Close)
			if unitCount <= 0 {
				unitCount = flow
			} else {
				unitValue := equityBeforeFlow / unitCount
				if unitValue <= 0 {
					unitValue = 1
				}
				unitCount += flow / unitValue
			}
			portfolio.USDTBalance += flow
			totalInjected += flow
			flows = append(flows, cashFlow{amount: flow, at: now})
			lastMonth = currentMonth
		}

		input := quant.StrategyInput{
			Symbol:            cfg.SpawnPoint.Symbol,
			QuoteAsset:        cfg.SpawnPoint.QuoteAsset,
			Closes:            closesThrough(cfg.Bars, i),
			Timestamps:        timestampsThrough(cfg.Bars, i),
			CurrentPrice:      bar.Close,
			Portfolio:         portfolio,
			Runtime:           runtimeState,
			SpotLots:          cloneLots(lots),
			Chromosome:        cfg.Chromosome,
			SpawnPoint:        cfg.SpawnPoint,
			Constraints:       constraints,
			CurrentBucketTime: bar.OpenTime,
			TotalEquity:       totalEquity(portfolio, bar.Close),
			SpendableUSDT:     spendableUSDT(portfolio, bar.Close, cfg.Chromosome),
		}
		output := cfg.Step(input)
		portfolio, lots = applyLedgerConversions(portfolio, lots, output.LedgerConversions, bar.Close, now)
		portfolio, lots = applyIntents(portfolio, lots, output.MacroIntents, bar.Close, constraints, now)
		portfolio, lots = applyIntents(portfolio, lots, output.MicroIntents, bar.Close, constraints, now)
		runtimeState = output.NextRuntime

		equity := totalEquity(portfolio, bar.Close)
		if unitCount > 0 {
			unitNAV = append(unitNAV, equity/unitCount)
		} else {
			unitNAV = append(unitNAV, equity)
		}
	}

	finalPrice := cfg.Bars[len(cfg.Bars)-1].Close
	finalEquity := totalEquity(portfolio, finalPrice)
	start := barTime(firstEvalBar.OpenTime)
	end := barTime(cfg.Bars[len(cfg.Bars)-1].OpenTime)
	return Result{
		FinalEquity:   finalEquity,
		TotalInjected: totalInjected,
		ROI:           modifiedDietzROI(initialEquity, finalEquity, flows, start, end),
		MaxDrawdown:   quant.MaxDrawdown(unitNAV),
	}, nil
}

func firstEvalIndex(bars []quant.Bar, evalStartMs int64) int {
	for i, bar := range bars {
		if bar.OpenTime >= evalStartMs {
			return i
		}
	}
	return len(bars)
}

func mergeConstraints(spawn quant.SpawnPoint, override quant.TradingConstraints) quant.TradingConstraints {
	out := spawn.Precision
	if out.MinOrderUSDT <= 0 {
		out.MinOrderUSDT = quant.DefaultMinOrderUSDT
	}
	if out.FeeRate <= 0 {
		out.FeeRate = spawn.Risk.FeeRate
	}
	if out.SlippagePct <= 0 {
		out.SlippagePct = spawn.Risk.SlippagePct
	}
	if override.MinOrderUSDT > 0 {
		out.MinOrderUSDT = override.MinOrderUSDT
	}
	if override.LotStep > 0 {
		out.LotStep = override.LotStep
	}
	if override.LotMin > 0 {
		out.LotMin = override.LotMin
	}
	if override.PriceTick > 0 {
		out.PriceTick = override.PriceTick
	}
	if override.FeeRate > 0 {
		out.FeeRate = override.FeeRate
	}
	if override.SlippagePct > 0 {
		out.SlippagePct = override.SlippagePct
	}
	return out
}

func initialPortfolio(spawn quant.SpawnPoint) quant.PortfolioSnapshot {
	return quant.PortfolioSnapshot{
		USDTBalance:   math.Max(spawn.InitialAvailableUSDT, 0),
		DeadBTC:       math.Max(spawn.InitialDeadBTC, 0),
		FloatBTC:      math.Max(spawn.InitialFloatBTC, 0),
		ColdSealedBTC: math.Max(spawn.InitialColdSealedBTC, 0),
	}
}

func initialLots(spawn quant.SpawnPoint, price float64, createdAt time.Time) []quant.SpotLot {
	lots := make([]quant.SpotLot, 0, 3)
	if spawn.InitialDeadBTC > 0 {
		lots = append(lots, quant.SpotLot{LotType: quant.LotTypeDeadStack, Amount: spawn.InitialDeadBTC, CostPrice: price, CreatedAt: createdAt})
	}
	if spawn.InitialFloatBTC > 0 {
		lots = append(lots, quant.SpotLot{LotType: quant.LotTypeFloating, Amount: spawn.InitialFloatBTC, CostPrice: price, CreatedAt: createdAt})
	}
	if spawn.InitialColdSealedBTC > 0 {
		lots = append(lots, quant.SpotLot{LotType: quant.LotTypeColdSealed, Amount: spawn.InitialColdSealedBTC, CostPrice: price, CreatedAt: createdAt, IsColdSealed: true})
	}
	return lots
}

func closesThrough(bars []quant.Bar, end int) []float64 {
	out := make([]float64, 0, end+1)
	for i := 0; i <= end; i++ {
		out = append(out, bars[i].Close)
	}
	return out
}

func timestampsThrough(bars []quant.Bar, end int) []int64 {
	out := make([]int64, 0, end+1)
	for i := 0; i <= end; i++ {
		out = append(out, bars[i].OpenTime)
	}
	return out
}

func cloneLots(lots []quant.SpotLot) []quant.SpotLot {
	out := make([]quant.SpotLot, len(lots))
	copy(out, lots)
	return out
}

func totalEquity(portfolio quant.PortfolioSnapshot, price float64) float64 {
	return portfolio.USDTBalance + (portfolio.DeadBTC+portfolio.FloatBTC+portfolio.ColdSealedBTC)*price
}

func spendableUSDT(portfolio quant.PortfolioSnapshot, price float64, chromosome quant.Chromosome) float64 {
	equity := totalEquity(portfolio, price)
	return math.Max(0, portfolio.USDTBalance-equity*chromosome.MicroReservePct)
}

func applyLedgerConversions(portfolio quant.PortfolioSnapshot, lots []quant.SpotLot, conversions []quant.LedgerConversion, price float64, now time.Time) (quant.PortfolioSnapshot, []quant.SpotLot) {
	for _, conversion := range conversions {
		amount := math.Max(conversion.Amount, 0)
		if amount <= 0 {
			continue
		}
		switch {
		case conversion.FromLotType == quant.LotTypeDeadStack && conversion.ToLotType == quant.LotTypeFloating:
			move := math.Min(amount, portfolio.DeadBTC)
			portfolio.DeadBTC -= move
			portfolio.FloatBTC += move
			lots = moveLots(lots, quant.LotTypeDeadStack, quant.LotTypeFloating, move, price, now)
		case conversion.FromLotType == quant.LotTypeFloating && conversion.ToLotType == quant.LotTypeColdSealed:
			move := math.Min(amount, portfolio.FloatBTC)
			portfolio.FloatBTC -= move
			portfolio.ColdSealedBTC += move
			lots = moveLots(lots, quant.LotTypeFloating, quant.LotTypeColdSealed, move, price, now)
		}
	}
	return portfolio, lots
}

func applyIntents(portfolio quant.PortfolioSnapshot, lots []quant.SpotLot, intents []quant.TradeIntent, price float64, constraints quant.TradingConstraints, now time.Time) (quant.PortfolioSnapshot, []quant.SpotLot) {
	for _, intent := range intents {
		if price <= 0 {
			continue
		}
		switch intent.Action {
		case quant.ActionBuy:
			portfolio, lots = applyBuy(portfolio, lots, intent, price, constraints, now)
		case quant.ActionSell:
			portfolio, lots = applySell(portfolio, lots, intent, price, constraints)
		}
	}
	return portfolio, lots
}

func applyBuy(portfolio quant.PortfolioSnapshot, lots []quant.SpotLot, intent quant.TradeIntent, price float64, constraints quant.TradingConstraints, now time.Time) (quant.PortfolioSnapshot, []quant.SpotLot) {
	spend := math.Min(math.Max(intent.AmountUSDT, 0), portfolio.USDTBalance)
	if spend < constraints.MinOrderUSDT {
		return portfolio, lots
	}
	execPrice := price * (1 + math.Max(constraints.SlippagePct, 0))
	if execPrice <= 0 {
		return portfolio, lots
	}
	qty := spend * (1 - math.Max(constraints.FeeRate, 0)) / execPrice
	qty = floorToStep(qty, constraints.LotStep)
	if constraints.LotMin > 0 && qty < constraints.LotMin {
		return portfolio, lots
	}
	if qty <= 0 {
		return portfolio, lots
	}

	portfolio.USDTBalance -= spend
	switch intent.LotType {
	case quant.LotTypeDeadStack:
		portfolio.DeadBTC += qty
	case quant.LotTypeColdSealed:
		portfolio.ColdSealedBTC += qty
	default:
		portfolio.FloatBTC += qty
		intent.LotType = quant.LotTypeFloating
	}
	lots = append(lots, quant.SpotLot{LotType: intent.LotType, Amount: qty, CostPrice: execPrice, CreatedAt: now, IsColdSealed: intent.LotType == quant.LotTypeColdSealed})
	return portfolio, lots
}

func applySell(portfolio quant.PortfolioSnapshot, lots []quant.SpotLot, intent quant.TradeIntent, price float64, constraints quant.TradingConstraints) (quant.PortfolioSnapshot, []quant.SpotLot) {
	qty := math.Min(math.Max(intent.QtyAsset, 0), portfolio.FloatBTC)
	qty = floorToStep(qty, constraints.LotStep)
	if constraints.LotMin > 0 && qty < constraints.LotMin {
		return portfolio, lots
	}
	if qty <= 0 {
		return portfolio, lots
	}
	execPrice := price * (1 - math.Max(constraints.SlippagePct, 0))
	notional := qty * execPrice
	if notional < constraints.MinOrderUSDT {
		return portfolio, lots
	}
	proceeds := notional * (1 - math.Max(constraints.FeeRate, 0))
	portfolio.FloatBTC -= qty
	portfolio.USDTBalance += proceeds
	lots = consumeLots(lots, quant.LotTypeFloating, qty)
	return portfolio, lots
}

func moveLots(lots []quant.SpotLot, from, to quant.LotType, amount, price float64, now time.Time) []quant.SpotLot {
	if amount <= 0 {
		return lots
	}
	lots = consumeLots(lots, from, amount)
	lots = append(lots, quant.SpotLot{LotType: to, Amount: amount, CostPrice: price, CreatedAt: now, IsColdSealed: to == quant.LotTypeColdSealed})
	return lots
}

func consumeLots(lots []quant.SpotLot, lotType quant.LotType, amount float64) []quant.SpotLot {
	remaining := amount
	out := make([]quant.SpotLot, 0, len(lots))
	for _, lot := range lots {
		if remaining <= 0 || lot.LotType != lotType || lot.Amount <= 0 {
			out = append(out, lot)
			continue
		}
		consume := math.Min(lot.Amount, remaining)
		lot.Amount -= consume
		remaining -= consume
		if lot.Amount > 0 {
			out = append(out, lot)
		}
	}
	return out
}

func floorToStep(value, step float64) float64 {
	if step <= 0 {
		return value
	}
	return math.Floor(value/step) * step
}

func modifiedDietzROI(initialCapital, finalEquity float64, flows []cashFlow, start, end time.Time) float64 {
	totalDays := end.Sub(start).Hours() / 24
	cashFlowSum := 0.0
	weightedCashFlows := 0.0
	for _, flow := range flows {
		cashFlowSum += flow.amount
		weight := 0.0
		if totalDays > 0 {
			flowDay := flow.at.Sub(start).Hours() / 24
			weight = quant.ClipFloat64((totalDays-flowDay)/totalDays, 0, 1)
		}
		weightedCashFlows += flow.amount * weight
	}
	denominator := initialCapital + weightedCashFlows
	if denominator <= 0 {
		return 0
	}
	return (finalEquity - initialCapital - cashFlowSum) / denominator
}

func barTime(timestamp int64) time.Time {
	if timestamp > 1_000_000_000_000 {
		return time.UnixMilli(timestamp).UTC()
	}
	return time.Unix(timestamp, 0).UTC()
}

func monthKey(t time.Time) int {
	year, month, _ := t.Date()
	return year*100 + int(month)
}
