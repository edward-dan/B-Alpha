package quant

import "math"

// EMA returns the latest exponential moving average for values.
func EMA(values []float64, period int) float64 {
	if len(values) == 0 || period <= 0 {
		return 0
	}

	alpha := 2.0 / (float64(period) + 1.0)
	ema := values[0]
	for _, value := range values[1:] {
		ema = alpha*value + (1-alpha)*ema
	}
	return ema
}

// StdDev returns the sample standard deviation of the latest period values.
func StdDev(values []float64, period int) float64 {
	if len(values) < 2 || period <= 0 {
		return 0
	}

	start := len(values) - period
	if start < 0 {
		start = 0
	}
	window := values[start:]
	if len(window) < 2 {
		return 0
	}

	var sum float64
	for _, value := range window {
		sum += value
	}
	mean := sum / float64(len(window))

	var varianceSum float64
	for _, value := range window {
		delta := value - mean
		varianceSum += delta * delta
	}

	return math.Sqrt(varianceSum / float64(len(window)-1))
}

// MAVAbsChange returns the mean absolute close-to-close change over the latest length closes.
func MAVAbsChange(closes []float64, length int) float64 {
	if len(closes) < 2 || length < 2 {
		return 0
	}

	start := len(closes) - length
	if start < 0 {
		start = 0
	}
	window := closes[start:]
	if len(window) < 2 {
		return 0
	}

	var sum float64
	for i := 1; i < len(window); i++ {
		sum += math.Abs(window[i] - window[i-1])
	}
	return sum / float64(len(window)-1)
}

// ClipFloat64 clamps value into [lo, hi].
func ClipFloat64(value, lo, hi float64) float64 {
	if lo > hi {
		lo, hi = hi, lo
	}
	if value < lo {
		return lo
	}
	if value > hi {
		return hi
	}
	return value
}

// RoundToUSDT rounds a USDT-denominated value to cents.
func RoundToUSDT(value float64) float64 {
	return math.Round(value*100) / 100
}
