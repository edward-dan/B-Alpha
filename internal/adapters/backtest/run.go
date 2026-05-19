package backtest

import (
	"context"
	"errors"
	"math"
	"strings"
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
	Leverage    int
	Direction   DirectionMode
	Step        StepFunc
}

type Result struct {
	FinalEquity     float64             `json:"final_equity"`
	TotalInjected   float64             `json:"total_injected"`
	ROI             float64             `json:"roi"`
	MaxDrawdown     float64             `json:"max_drawdown"`
	RealizedPnL     float64             `json:"realized_pnl,omitempty"`
	Fees            float64             `json:"fees,omitempty"`
	TradeCount      int                 `json:"trade_count,omitempty"`
	WinRate         float64             `json:"win_rate,omitempty"`
	ProfitLossRatio float64             `json:"profit_loss_ratio,omitempty"`
	Sharpe          float64             `json:"sharpe,omitempty"`
	Orders          []SimulatedOrder    `json:"orders,omitempty"`
	Positions       []SimulatedPosition `json:"positions,omitempty"`
}

type DirectionMode string

const (
	DirectionLong      DirectionMode = "long"
	DirectionShort     DirectionMode = "short"
	DirectionLongShort DirectionMode = "long_short"
)

type SimulatedOrder struct {
	Symbol         string  `json:"symbol"`
	Side           string  `json:"side"`
	Offset         string  `json:"offset"`
	PositionSide   string  `json:"position_side"`
	Leverage       int     `json:"leverage"`
	OrderPrice     float64 `json:"order_price"`
	ExecutedPrice  float64 `json:"executed_price"`
	ExecutedQty    float64 `json:"executed_qty"`
	Fee            float64 `json:"fee"`
	Status         string  `json:"status"`
	OrderedAtMs    int64   `json:"ordered_at_ms"`
	ReasonCode     string  `json:"reason_code,omitempty"`
	RealizedPnL    float64 `json:"realized_pnl,omitempty"`
	MarginReleased float64 `json:"margin_released,omitempty"`
}

type SimulatedPosition struct {
	Symbol          string  `json:"symbol"`
	PositionSide    string  `json:"position_side"`
	AverageEntry    float64 `json:"average_entry"`
	Quantity        float64 `json:"quantity"`
	Leverage        int     `json:"leverage"`
	UsedMargin      float64 `json:"used_margin"`
	UnrealizedPnL   float64 `json:"unrealized_pnl"`
	LiquidationHint float64 `json:"liquidation_hint"`
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
	if useMarginLedger(cfg) {
		return runMarginBacktest(ctx, cfg)
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

func useMarginLedger(cfg Config) bool {
	return cfg.Leverage > 1 || normalizeDirection(cfg.Direction) == DirectionShort || normalizeDirection(cfg.Direction) == DirectionLongShort
}

type marginPosition struct {
	side     quant.PositionSide
	qty      float64
	entry    float64
	margin   float64
	leverage int
}

type marginAccount struct {
	symbol       string
	balance      float64
	frozenMargin float64
	realizedPnL  float64
	fees         float64
	trades       int
	wins         int
	grossProfit  float64
	grossLoss    float64
	positions    map[quant.PositionSide]*marginPosition
	orders       []SimulatedOrder
}

func runMarginBacktest(ctx context.Context, cfg Config) (Result, error) {
	evalIndex := firstEvalIndex(cfg.Bars, cfg.EvalStartMs)
	if evalIndex >= len(cfg.Bars) {
		return Result{}, errors.New("run margin backtest: no bars at or after EvalStartMs")
	}
	firstEvalBar := cfg.Bars[evalIndex]
	if firstEvalBar.Close <= 0 {
		return Result{}, errors.New("run margin backtest: first evaluation close must be positive")
	}

	constraints := mergeConstraints(cfg.SpawnPoint, cfg.Constraints)
	leverage := cfg.Leverage
	if leverage <= 0 {
		leverage = 1
	}
	direction := normalizeDirection(cfg.Direction)
	account := marginAccount{
		symbol:    cfg.SpawnPoint.Symbol,
		balance:   math.Max(cfg.SpawnPoint.InitialAvailableUSDT, 0),
		positions: make(map[quant.PositionSide]*marginPosition),
		orders:    make([]SimulatedOrder, 0),
	}
	if account.balance <= 0 {
		account.balance = totalEquity(initialPortfolio(cfg.SpawnPoint), firstEvalBar.Close)
	}
	runtimeState := quant.RuntimeState{}
	initialEquity := account.equity(firstEvalBar.Close)
	equityCurve := make([]float64, 0, len(cfg.Bars)-evalIndex)

	for i, bar := range cfg.Bars {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if i < evalIndex || bar.Close <= 0 {
			continue
		}

		longQty := 0.0
		if long := account.positions[quant.PositionSideLong]; long != nil {
			longQty = long.qty
		}
		input := quant.StrategyInput{
			Symbol:            cfg.SpawnPoint.Symbol,
			QuoteAsset:        cfg.SpawnPoint.QuoteAsset,
			Closes:            closesThrough(cfg.Bars, i),
			Timestamps:        timestampsThrough(cfg.Bars, i),
			CurrentPrice:      bar.Close,
			Portfolio:         quant.PortfolioSnapshot{USDTBalance: account.availableBalance(), FloatBTC: longQty},
			Runtime:           runtimeState,
			Chromosome:        cfg.Chromosome,
			SpawnPoint:        cfg.SpawnPoint,
			Constraints:       constraints,
			CurrentBucketTime: bar.OpenTime,
			TotalEquity:       account.equity(bar.Close),
			SpendableUSDT:     math.Max(0, account.availableBalance()-account.equity(bar.Close)*cfg.Chromosome.MicroReservePct),
		}
		output := cfg.Step(input)
		account.applyMarginIntents(output.MacroIntents, bar, constraints, leverage, direction)
		account.applyMarginIntents(output.MicroIntents, bar, constraints, leverage, direction)
		runtimeState = output.NextRuntime
		equityCurve = append(equityCurve, account.equity(bar.Close))
	}

	finalPrice := cfg.Bars[len(cfg.Bars)-1].Close
	finalEquity := account.equity(finalPrice)
	winRate := 0.0
	if account.trades > 0 {
		winRate = float64(account.wins) / float64(account.trades)
	}
	plRatio := 0.0
	if account.grossLoss > 0 {
		plRatio = account.grossProfit / account.grossLoss
	}
	return Result{
		FinalEquity:     finalEquity,
		TotalInjected:   initialEquity,
		ROI:             simpleROI(initialEquity, finalEquity),
		MaxDrawdown:     quant.MaxDrawdown(equityCurve),
		RealizedPnL:     account.realizedPnL,
		Fees:            account.fees,
		TradeCount:      account.trades,
		WinRate:         winRate,
		ProfitLossRatio: plRatio,
		Sharpe:          sharpeRatio(equityCurve),
		Orders:          account.orders,
		Positions:       account.simulatedPositions(finalPrice),
	}, nil
}

func normalizeDirection(direction DirectionMode) DirectionMode {
	switch DirectionMode(strings.ToLower(strings.TrimSpace(string(direction)))) {
	case DirectionShort:
		return DirectionShort
	case DirectionLongShort:
		return DirectionLongShort
	default:
		return DirectionLong
	}
}

func (a *marginAccount) applyMarginIntents(intents []quant.TradeIntent, bar quant.Bar, constraints quant.TradingConstraints, leverage int, direction DirectionMode) {
	for _, intent := range intents {
		side, offset, ok := inferMarginIntent(intent, direction)
		if !ok || bar.Close <= 0 {
			continue
		}
		intentLeverage := leverage
		if intent.Leverage > 0 {
			intentLeverage = intent.Leverage
		}
		if intentLeverage <= 0 {
			intentLeverage = 1
		}
		switch offset {
		case quant.OrderOffsetOpen:
			a.openPosition(intent, side, bar, constraints, intentLeverage)
		case quant.OrderOffsetClose:
			a.closePosition(intent, side, bar, constraints, intentLeverage)
		}
	}
}

func inferMarginIntent(intent quant.TradeIntent, direction DirectionMode) (quant.PositionSide, quant.OrderOffset, bool) {
	side := intent.PositionSide
	if side == "" {
		if direction == DirectionShort {
			side = quant.PositionSideShort
		} else {
			side = quant.PositionSideLong
		}
	}
	offset := intent.Offset
	if offset == "" {
		switch side {
		case quant.PositionSideLong:
			if intent.Action == quant.ActionBuy {
				offset = quant.OrderOffsetOpen
			} else {
				offset = quant.OrderOffsetClose
			}
		case quant.PositionSideShort:
			if intent.Action == quant.ActionSell {
				offset = quant.OrderOffsetOpen
			} else {
				offset = quant.OrderOffsetClose
			}
		default:
			return "", "", false
		}
	}
	if direction == DirectionLong && side == quant.PositionSideShort {
		return "", "", false
	}
	if direction == DirectionShort && side == quant.PositionSideLong {
		return "", "", false
	}
	if offset != quant.OrderOffsetOpen && offset != quant.OrderOffsetClose {
		return "", "", false
	}
	return side, offset, true
}

func (a *marginAccount) openPosition(intent quant.TradeIntent, side quant.PositionSide, bar quant.Bar, constraints quant.TradingConstraints, leverage int) {
	execPrice := executionPrice(side, quant.OrderOffsetOpen, bar.Close, constraints.SlippagePct)
	qty, notional := intentNotional(intent, execPrice, leverage, true)
	if notional < constraints.MinOrderUSDT || execPrice <= 0 {
		return
	}
	maxNotional := a.availableBalance() / (1/float64(leverage) + math.Max(constraints.FeeRate, 0))
	if maxNotional <= 0 {
		return
	}
	if notional > maxNotional {
		notional = maxNotional
		qty = notional / execPrice
	}
	qty = floorToStep(qty, constraints.LotStep)
	if constraints.LotMin > 0 && qty < constraints.LotMin {
		return
	}
	if qty <= 0 {
		return
	}
	notional = qty * execPrice
	margin := notional / float64(leverage)
	fee := notional * math.Max(constraints.FeeRate, 0)
	if margin+fee > a.availableBalance() {
		return
	}

	position := a.positions[side]
	if position == nil {
		position = &marginPosition{side: side, leverage: leverage}
		a.positions[side] = position
	}
	nextQty := position.qty + qty
	if nextQty <= 0 {
		return
	}
	position.entry = (position.entry*position.qty + execPrice*qty) / nextQty
	position.qty = nextQty
	position.margin += margin
	position.leverage = leverage
	a.balance -= margin + fee
	a.frozenMargin += margin
	a.fees += fee
	a.orders = append(a.orders, SimulatedOrder{
		Symbol:        a.symbol,
		Side:          sideOpenAction(side),
		Offset:        string(quant.OrderOffsetOpen),
		PositionSide:  string(side),
		Leverage:      leverage,
		OrderPrice:    bar.Close,
		ExecutedPrice: execPrice,
		ExecutedQty:   qty,
		Fee:           fee,
		Status:        "filled",
		OrderedAtMs:   bar.OpenTime,
		ReasonCode:    intent.ReasonCode,
	})
}

func (a *marginAccount) closePosition(intent quant.TradeIntent, side quant.PositionSide, bar quant.Bar, constraints quant.TradingConstraints, leverage int) {
	position := a.positions[side]
	if position == nil || position.qty <= 0 {
		return
	}
	execPrice := executionPrice(side, quant.OrderOffsetClose, bar.Close, constraints.SlippagePct)
	qty, notional := intentNotional(intent, execPrice, leverage, false)
	if qty <= 0 {
		qty = position.qty
		notional = qty * execPrice
	}
	qty = math.Min(qty, position.qty)
	qty = floorToStep(qty, constraints.LotStep)
	if constraints.LotMin > 0 && qty < constraints.LotMin {
		return
	}
	if qty <= 0 {
		return
	}
	notional = qty * execPrice
	if notional < constraints.MinOrderUSDT {
		return
	}
	released := position.margin * (qty / position.qty)
	grossPnL := marginPnL(side, position.entry, execPrice, qty)
	fee := notional * math.Max(constraints.FeeRate, 0)
	netPnL := grossPnL - fee

	position.qty -= qty
	position.margin -= released
	a.frozenMargin -= released
	a.balance += released + netPnL
	a.realizedPnL += netPnL
	a.fees += fee
	a.trades++
	if netPnL > 0 {
		a.wins++
		a.grossProfit += netPnL
	} else if netPnL < 0 {
		a.grossLoss += math.Abs(netPnL)
	}
	if position.qty <= 0 {
		delete(a.positions, side)
	}
	a.orders = append(a.orders, SimulatedOrder{
		Symbol:         a.symbol,
		Side:           sideCloseAction(side),
		Offset:         string(quant.OrderOffsetClose),
		PositionSide:   string(side),
		Leverage:       leverage,
		OrderPrice:     bar.Close,
		ExecutedPrice:  execPrice,
		ExecutedQty:    qty,
		Fee:            fee,
		Status:         "filled",
		OrderedAtMs:    bar.OpenTime,
		ReasonCode:     intent.ReasonCode,
		RealizedPnL:    netPnL,
		MarginReleased: released,
	})
}

func intentNotional(intent quant.TradeIntent, price float64, leverage int, opening bool) (float64, float64) {
	if price <= 0 {
		return 0, 0
	}
	if intent.QtyAsset > 0 {
		qty := math.Max(intent.QtyAsset, 0)
		return qty, qty * price
	}
	if intent.AmountUSDT > 0 {
		notional := math.Max(intent.AmountUSDT, 0)
		if opening {
			notional *= float64(leverage)
		}
		return notional / price, notional
	}
	return 0, 0
}

func executionPrice(side quant.PositionSide, offset quant.OrderOffset, price float64, slippage float64) float64 {
	slip := math.Max(slippage, 0)
	switch {
	case side == quant.PositionSideLong && offset == quant.OrderOffsetOpen:
		return price * (1 + slip)
	case side == quant.PositionSideLong && offset == quant.OrderOffsetClose:
		return price * (1 - slip)
	case side == quant.PositionSideShort && offset == quant.OrderOffsetOpen:
		return price * (1 - slip)
	case side == quant.PositionSideShort && offset == quant.OrderOffsetClose:
		return price * (1 + slip)
	default:
		return price
	}
}

func marginPnL(side quant.PositionSide, entry float64, exit float64, qty float64) float64 {
	if side == quant.PositionSideShort {
		return (entry - exit) * qty
	}
	return (exit - entry) * qty
}

func sideOpenAction(side quant.PositionSide) string {
	if side == quant.PositionSideShort {
		return string(quant.ActionSell)
	}
	return string(quant.ActionBuy)
}

func sideCloseAction(side quant.PositionSide) string {
	if side == quant.PositionSideShort {
		return string(quant.ActionBuy)
	}
	return string(quant.ActionSell)
}

func (a *marginAccount) availableBalance() float64 {
	return math.Max(a.balance, 0)
}

func (a *marginAccount) equity(price float64) float64 {
	equity := a.balance + a.frozenMargin
	for _, position := range a.positions {
		equity += marginPnL(position.side, position.entry, price, position.qty)
	}
	return equity
}

func (a *marginAccount) simulatedPositions(price float64) []SimulatedPosition {
	out := make([]SimulatedPosition, 0, len(a.positions))
	for _, position := range a.positions {
		if position.qty <= 0 {
			continue
		}
		out = append(out, SimulatedPosition{
			Symbol:          a.symbol,
			PositionSide:    string(position.side),
			AverageEntry:    position.entry,
			Quantity:        position.qty,
			Leverage:        position.leverage,
			UsedMargin:      position.margin,
			UnrealizedPnL:   marginPnL(position.side, position.entry, price, position.qty),
			LiquidationHint: liquidationHint(position),
		})
	}
	return out
}

func liquidationHint(position *marginPosition) float64 {
	if position == nil || position.leverage <= 0 {
		return 0
	}
	move := position.entry / float64(position.leverage)
	if position.side == quant.PositionSideShort {
		return position.entry + move
	}
	return math.Max(0, position.entry-move)
}

func simpleROI(initialEquity, finalEquity float64) float64 {
	if initialEquity <= 0 {
		return 0
	}
	return (finalEquity - initialEquity) / initialEquity
}

func sharpeRatio(equityCurve []float64) float64 {
	if len(equityCurve) < 3 {
		return 0
	}
	returns := make([]float64, 0, len(equityCurve)-1)
	for i := 1; i < len(equityCurve); i++ {
		if equityCurve[i-1] <= 0 {
			continue
		}
		returns = append(returns, (equityCurve[i]-equityCurve[i-1])/equityCurve[i-1])
	}
	if len(returns) < 2 {
		return 0
	}
	mean := 0.0
	for _, value := range returns {
		mean += value
	}
	mean /= float64(len(returns))
	variance := 0.0
	for _, value := range returns {
		diff := value - mean
		variance += diff * diff
	}
	std := math.Sqrt(variance / float64(len(returns)-1))
	if std <= 0 {
		return 0
	}
	return mean / std * math.Sqrt(float64(len(returns)))
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
