package quant

import "math"

const millisPerDay int64 = 24 * 60 * 60 * 1000

type MacroDecisionInput struct {
	Closes            []float64   `json:"closes"`
	CurrentPrice      float64     `json:"current_price"`
	TotalEquity       float64     `json:"total_equity"`
	SpendableUSDT     float64     `json:"spendable_usdt"`
	MonthlyInjectUSDT float64     `json:"monthly_inject_usdt"`
	LastMacroBuyAt    int64       `json:"last_macro_buy_at"`
	CurrentBucketTime int64       `json:"current_bucket_time"`
	Chromosome        Chromosome  `json:"chromosome"`
	MarketState       MarketState `json:"market_state"`
	MinOrderUSDT      float64     `json:"min_order_usdt"`
}

type MacroDecision struct {
	ShouldBuy       bool        `json:"should_buy"`
	Intent          TradeIntent `json:"intent"`
	OrderUSD        float64     `json:"order_usd"`
	ReasonCode      string      `json:"reason_code"`
	BaseBudget      float64     `json:"base_budget"`
	MacroMultiplier float64     `json:"macro_multiplier"`
	ValueGap        float64     `json:"value_gap"`
	OverheatGap     float64     `json:"overheat_gap"`
	DrawdownScore   float64     `json:"drawdown_score"`
	EffectiveDays   float64     `json:"effective_days"`
}

// ComputeMacroDecision produces only a BUY intent for DEAD_STACK, never SELL.
func ComputeMacroDecision(input MacroDecisionInput) MacroDecision {
	c := chromosomeOrDefault(input.Chromosome)
	minOrderUSDT := input.MinOrderUSDT
	if minOrderUSDT <= 0 {
		minOrderUSDT = DefaultMinOrderUSDT
	}

	if input.TotalEquity <= 0 {
		return MacroDecision{ReasonCode: "MACRO_NO_EQUITY"}
	}
	if input.SpendableUSDT <= 0 {
		return MacroDecision{ReasonCode: "MACRO_NO_SPENDABLE_USDT"}
	}

	marketState := input.MarketState
	if marketState.State == "" {
		marketState = transitionMarketState()
	}
	timeDilation := marketState.TimeDilationMultiplier
	if timeDilation < 1 {
		timeDilation = 1
	}
	effectiveDays := float64(c.MacroIntervalDays) * timeDilation
	if !macroIntervalElapsed(input.LastMacroBuyAt, input.CurrentBucketTime, effectiveDays) {
		return MacroDecision{ReasonCode: "MACRO_INTERVAL_NOT_ELAPSED", EffectiveDays: effectiveDays}
	}

	closes := closesWithCurrent(input.Closes, input.CurrentPrice)
	currentPrice := input.CurrentPrice
	if currentPrice <= 0 && len(closes) > 0 {
		currentPrice = closes[len(closes)-1]
	}
	if currentPrice <= 0 || len(closes) == 0 {
		return MacroDecision{ReasonCode: "MACRO_NO_PRICE", EffectiveDays: effectiveDays}
	}

	emaLong := EMA(closes, c.EMALongN)
	valueGap := math.Max(0, safeLogRatio(emaLong, currentPrice))
	overheatGap := math.Max(0, safeLogRatio(currentPrice, emaLong))
	drawdown := drawdownFromHigh(closes, c.DrawdownN)
	drawdownScore := ClipFloat64(drawdown/c.MacroDrawdownRef, 0, 1)

	periodBudgetFromInjection := input.MonthlyInjectUSDT * effectiveDays / 30
	periodBudgetFromEquity := input.TotalEquity * c.MacroEquityPctPerPeriod
	baseBudget := maxFloat(periodBudgetFromInjection, periodBudgetFromEquity)
	regimeMultiplier := macroRegimeMultiplier(marketState.State, c)
	macroMultiplier := regimeMultiplier *
		(1 + c.MacroValueBoost*valueGap + c.MacroDrawdownBoost*drawdownScore) *
		math.Max(0, 1-c.MacroOverheatPenalty*overheatGap)

	macroBuyUSDT := minFloat(
		input.SpendableUSDT*c.MacroMaxSpendablePct,
		baseBudget*macroMultiplier,
	)
	macroBuyUSDT = minFloat(macroBuyUSDT, input.SpendableUSDT)
	if macroBuyUSDT < minOrderUSDT {
		reason := "MACRO_MIN_ORDER_SKIP"
		if overheatGap > 0 {
			reason = "MACRO_OVERHEAT_SKIP"
		}
		return MacroDecision{
			ReasonCode:      reason,
			BaseBudget:      baseBudget,
			MacroMultiplier: macroMultiplier,
			ValueGap:        valueGap,
			OverheatGap:     overheatGap,
			DrawdownScore:   drawdownScore,
			EffectiveDays:   effectiveDays,
		}
	}

	reason := "MACRO_DCA_BASE"
	if drawdownScore > 0 {
		reason = "MACRO_DRAWDOWN_BOOST"
	}
	intent := TradeIntent{
		Action:     ActionBuy,
		Engine:     EngineMacro,
		LotType:    LotTypeDeadStack,
		AmountUSDT: macroBuyUSDT,
		ReasonCode: reason,
	}
	return MacroDecision{
		ShouldBuy:       true,
		Intent:          intent,
		OrderUSD:        macroBuyUSDT,
		ReasonCode:      reason,
		BaseBudget:      baseBudget,
		MacroMultiplier: macroMultiplier,
		ValueGap:        valueGap,
		OverheatGap:     overheatGap,
		DrawdownScore:   drawdownScore,
		EffectiveDays:   effectiveDays,
	}
}

func macroIntervalElapsed(lastMacroBuyAt, currentBucketTime int64, effectiveDays float64) bool {
	if lastMacroBuyAt <= 0 {
		return true
	}
	if currentBucketTime <= 0 {
		return false
	}
	requiredMillis := int64(effectiveDays * float64(millisPerDay))
	return currentBucketTime-lastMacroBuyAt >= requiredMillis
}

func macroRegimeMultiplier(state string, c Chromosome) float64 {
	switch state {
	case MarketStateBullTrend:
		return c.MacroMultBull
	case MarketStateBearTrend:
		return c.MacroMultBear
	case MarketStateQuiet:
		return c.MacroMultQuiet
	case MarketStateStressCrash:
		return c.MacroMultStress
	default:
		return 1
	}
}
