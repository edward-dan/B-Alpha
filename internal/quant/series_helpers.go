package quant

import "math"

const epsilon = 1e-12

func closesWithCurrent(closes []float64, currentPrice float64) []float64 {
	out := make([]float64, 0, len(closes)+1)
	for _, close := range closes {
		if close > 0 {
			out = append(out, close)
		}
	}
	if currentPrice <= 0 {
		return out
	}
	if len(out) == 0 || math.Abs(out[len(out)-1]-currentPrice) > epsilon {
		out = append(out, currentPrice)
	}
	return out
}

func logReturns(closes []float64) []float64 {
	if len(closes) < 2 {
		return nil
	}
	returns := make([]float64, 0, len(closes)-1)
	for i := 1; i < len(closes); i++ {
		if closes[i-1] <= 0 || closes[i] <= 0 {
			returns = append(returns, 0)
			continue
		}
		returns = append(returns, math.Log(closes[i]/closes[i-1]))
	}
	return returns
}

func safeLogRatio(numerator, denominator float64) float64 {
	if numerator <= 0 || denominator <= 0 {
		return 0
	}
	return math.Log(numerator / denominator)
}

func rollingMax(values []float64, period int) float64 {
	if len(values) == 0 || period <= 0 {
		return 0
	}
	start := len(values) - period
	if start < 0 {
		start = 0
	}
	maxValue := values[start]
	for _, value := range values[start+1:] {
		if value > maxValue {
			maxValue = value
		}
	}
	return maxValue
}

func drawdownFromHigh(closes []float64, period int) float64 {
	if len(closes) == 0 {
		return 0
	}
	high := rollingMax(closes, period)
	current := closes[len(closes)-1]
	if high <= 0 || current <= 0 {
		return 0
	}
	return ClipFloat64(1-current/high, 0, 1)
}

func latestMomentum(closes []float64, lookback int, endIndex int) float64 {
	if lookback <= 0 || endIndex < 0 || endIndex >= len(closes) {
		return 0
	}
	start := endIndex - lookback
	if start < 0 {
		return 0
	}
	return safeLogRatio(closes[endIndex], closes[start])
}

func sigmoidFromExponent(exponent float64) float64 {
	if exponent > 709 {
		return 0
	}
	if exponent < -709 {
		return 1
	}
	return 1 / (1 + math.Exp(exponent))
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
