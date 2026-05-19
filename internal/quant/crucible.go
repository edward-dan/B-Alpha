package quant

import (
	"errors"
	"fmt"
	"sort"
)

const crucibleDayMillis int64 = 24 * 60 * 60 * 1000

// CrucibleWindow is one immutable backtest slice. Bars may include a warmup
// prefix before EvalStartMs; only bars at or after EvalStartMs are scored.
type CrucibleWindow struct {
	Label       string  `json:"label"`
	Weight      float64 `json:"weight"`
	Bars        []Bar   `json:"bars"`
	EvalStartMs int64   `json:"eval_start_ms"`
}

type CrucibleResult struct {
	Window string  `json:"window"`
	Score  float64 `json:"score"`
	ROI    float64 `json:"roi"`
	MaxDD  float64 `json:"max_dd"`
	Alpha  float64 `json:"alpha"`
}

type crucibleSpec struct {
	label  string
	days   int
	weight float64
	full   bool
}

// BuildCrucibleWindows constructs the fixed 6m -> 2y -> 5y -> full cascade.
func BuildCrucibleWindows(bars []Bar, warmupDays int) ([]CrucibleWindow, error) {
	if len(bars) == 0 {
		return nil, errors.New("build crucible windows: no bars")
	}
	if warmupDays < 0 {
		warmupDays = 0
	}

	ordered := append([]Bar(nil), bars...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].OpenTime < ordered[j].OpenTime
	})
	if ordered[0].OpenTime <= 0 || ordered[len(ordered)-1].OpenTime <= 0 {
		return nil, errors.New("build crucible windows: bars must have positive timestamps")
	}

	specs := []crucibleSpec{
		{label: "6m", days: 183, weight: 0.10},
		{label: "2y", days: 730, weight: 0.20},
		{label: "5y", days: 1825, weight: 0.30},
		{label: "full", weight: 0.40, full: true},
	}

	windows := make([]CrucibleWindow, 0, len(specs))
	for _, spec := range specs {
		window, err := buildCrucibleWindow(ordered, warmupDays, spec)
		if err != nil {
			return nil, err
		}
		windows = append(windows, window)
	}
	return windows, nil
}

func buildCrucibleWindow(bars []Bar, warmupDays int, spec crucibleSpec) (CrucibleWindow, error) {
	latest := bars[len(bars)-1].OpenTime
	if spec.full {
		return CrucibleWindow{
			Label:       spec.label,
			Weight:      spec.weight,
			Bars:        append([]Bar(nil), bars...),
			EvalStartMs: bars[0].OpenTime,
		}, nil
	}

	targetEvalStart := latest - int64(spec.days)*crucibleDayMillis
	if bars[0].OpenTime > targetEvalStart {
		return CrucibleWindow{}, fmt.Errorf("build crucible window %s: insufficient history for %d-day evaluation", spec.label, spec.days)
	}

	evalStartIndex := lowerBoundBarTime(bars, targetEvalStart)
	if evalStartIndex >= len(bars) {
		return CrucibleWindow{}, fmt.Errorf("build crucible window %s: no evaluation bars", spec.label)
	}

	warmupStart := targetEvalStart - int64(warmupDays)*crucibleDayMillis
	warmupStartIndex := lowerBoundBarTime(bars, warmupStart)
	windowBars := append([]Bar(nil), bars[warmupStartIndex:]...)
	return CrucibleWindow{
		Label:       spec.label,
		Weight:      spec.weight,
		Bars:        windowBars,
		EvalStartMs: bars[evalStartIndex].OpenTime,
	}, nil
}

func lowerBoundBarTime(bars []Bar, timestamp int64) int {
	return sort.Search(len(bars), func(i int) bool {
		return bars[i].OpenTime >= timestamp
	})
}
