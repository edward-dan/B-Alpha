package quant

import "time"

type GhostDCAConfig struct {
	InitialCapital float64 `json:"initial_capital"`
	MonthlyInject  float64 `json:"monthly_inject"`
}

type GhostDCAResult struct {
	FinalEquity   float64 `json:"final_equity"`
	TotalInjected float64 `json:"total_injected"`
	MaxDrawdown   float64 `json:"max_drawdown"`
	ROI           float64 `json:"roi"`
}

type cashFlow struct {
	amount float64
	at     time.Time
}

// SimulateGhostDCA buys with initial capital on the first bar and then buys monthly at the first bar of each month.
func SimulateGhostDCA(bars []Bar, config GhostDCAConfig) GhostDCAResult {
	if len(bars) == 0 || bars[0].Close <= 0 {
		return GhostDCAResult{}
	}

	startTime := timestampToTime(bars[0].OpenTime)
	endTime := timestampToTime(bars[len(bars)-1].OpenTime)
	btc := 0.0
	if config.InitialCapital > 0 {
		btc = config.InitialCapital / bars[0].Close
	}

	totalInjected := maxFloat(config.InitialCapital, 0)
	flows := make([]cashFlow, 0)
	unitCount := maxFloat(config.InitialCapital, 0)
	unitNAVCurve := make([]float64, 0, len(bars))

	lastMonth := monthKey(startTime)
	for i, bar := range bars {
		if bar.Close <= 0 {
			continue
		}
		barTime := timestampToTime(bar.OpenTime)
		currentMonth := monthKey(barTime)
		if i > 0 && config.MonthlyInject > 0 && currentMonth != lastMonth {
			equityBeforeFlow := btc * bar.Close
			unitValue := 1.0
			if unitCount > 0 {
				unitValue = equityBeforeFlow / unitCount
			}
			if unitValue <= 0 {
				unitValue = 1
			}
			unitCount += config.MonthlyInject / unitValue
			btc += config.MonthlyInject / bar.Close
			totalInjected += config.MonthlyInject
			flows = append(flows, cashFlow{amount: config.MonthlyInject, at: barTime})
			lastMonth = currentMonth
		}

		equity := btc * bar.Close
		if unitCount > 0 {
			unitNAVCurve = append(unitNAVCurve, equity/unitCount)
		} else {
			unitNAVCurve = append(unitNAVCurve, equity)
		}
	}

	finalEquity := 0.0
	if len(bars) > 0 && bars[len(bars)-1].Close > 0 {
		finalEquity = btc * bars[len(bars)-1].Close
	}

	return GhostDCAResult{
		FinalEquity:   finalEquity,
		TotalInjected: totalInjected,
		MaxDrawdown:   MaxDrawdown(unitNAVCurve),
		ROI:           modifiedDietzROI(config.InitialCapital, finalEquity, flows, startTime, endTime),
	}
}

// MaxDrawdown returns the largest peak-to-trough relative decline in a NAV curve.
func MaxDrawdown(nav []float64) float64 {
	if len(nav) == 0 {
		return 0
	}

	peak := nav[0]
	maxDrawdown := 0.0
	for _, value := range nav {
		if value <= 0 {
			continue
		}
		if peak <= 0 || value > peak {
			peak = value
			continue
		}
		drawdown := (peak - value) / peak
		if drawdown > maxDrawdown {
			maxDrawdown = drawdown
		}
	}
	return maxDrawdown
}

func modifiedDietzROI(initialCapital, finalEquity float64, flows []cashFlow, start, end time.Time) float64 {
	initial := maxFloat(initialCapital, 0)
	totalDays := end.Sub(start).Hours() / 24
	cashFlowSum := 0.0
	weightedCashFlows := 0.0

	for _, flow := range flows {
		if flow.amount == 0 {
			continue
		}
		cashFlowSum += flow.amount
		weight := 0.0
		if totalDays > 0 {
			flowDay := flow.at.Sub(start).Hours() / 24
			weight = ClipFloat64((totalDays-flowDay)/totalDays, 0, 1)
		}
		weightedCashFlows += flow.amount * weight
	}

	denominator := initial + weightedCashFlows
	if denominator <= 0 {
		return 0
	}
	return (finalEquity - initial - cashFlowSum) / denominator
}

func timestampToTime(timestamp int64) time.Time {
	if timestamp > 1_000_000_000_000 {
		return time.UnixMilli(timestamp).UTC()
	}
	return time.Unix(timestamp, 0).UTC()
}

func monthKey(t time.Time) int {
	year, month, _ := t.Date()
	return year*100 + int(month)
}
