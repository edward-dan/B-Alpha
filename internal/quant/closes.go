package quant

// ExtractCloses degrades OHLCV bars into the close-only ACL used by strategy kernels.
func ExtractCloses(bars []Bar) []float64 {
	closes := make([]float64, len(bars))
	for i, bar := range bars {
		closes[i] = bar.Close
	}
	return closes
}

// ExtractTimestamps returns bar open timestamps in the same order as ExtractCloses.
func ExtractTimestamps(bars []Bar) []int64 {
	timestamps := make([]int64, len(bars))
	for i, bar := range bars {
		timestamps[i] = bar.OpenTime
	}
	return timestamps
}
