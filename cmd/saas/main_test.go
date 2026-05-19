package main

import (
	"testing"
	"time"

	"bian-trade-go/internal/saas/store"
)

func TestIntervalDurationSupportsRuntimeIntervals(t *testing.T) {
	tests := map[string]time.Duration{
		"15m": 15 * time.Minute,
		"1h":  1 * time.Hour,
		"4H":  4 * time.Hour,
		"1d":  24 * time.Hour,
		"1w":  7 * 24 * time.Hour,
	}
	for input, want := range tests {
		got, ok := intervalDuration(input)
		if !ok {
			t.Fatalf("intervalDuration(%q) rejected valid interval", input)
		}
		if got != want {
			t.Fatalf("intervalDuration(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestIntervalDurationRejectsUnsupportedIntervals(t *testing.T) {
	for _, input := range []string{"", "h", "0m", "-1h", "1mo", "abc"} {
		if got, ok := intervalDuration(input); ok {
			t.Fatalf("intervalDuration(%q) = %v, want rejection", input, got)
		}
	}
}

func TestKlineToBarParsesStoredDecimals(t *testing.T) {
	openTime := time.Unix(100, 0).UTC()
	bar, err := klineToBar(store.KLine{
		ID:       7,
		OpenTime: openTime,
		Open:     store.Decimal("10.1"),
		High:     store.Decimal("11.2"),
		Low:      store.Decimal("9.3"),
		Close:    store.Decimal("10.4"),
		Volume:   store.Decimal("123.45"),
	})
	if err != nil {
		t.Fatalf("klineToBar returned error: %v", err)
	}
	if bar.OpenTime != openTime.UnixMilli() {
		t.Fatalf("OpenTime = %d, want %d", bar.OpenTime, openTime.UnixMilli())
	}
	if bar.Open != 10.1 || bar.High != 11.2 || bar.Low != 9.3 || bar.Close != 10.4 || bar.Volume != 123.45 {
		t.Fatalf("unexpected bar: %#v", bar)
	}
}
