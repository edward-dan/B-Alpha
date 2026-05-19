package quant

import "math"

type HardBound struct {
	Min   float64
	Max   float64
	IsInt bool
}

// Chromosome is the evolvable parameter set defined by strategy-math-engine chapter 7.
type Chromosome struct {
	MicroReservePct             float64 `json:"micro_reserve_pct"`
	EMAFastN                    int     `json:"ema_fast_n"`
	EMAMidN                     int     `json:"ema_mid_n"`
	EMALongN                    int     `json:"ema_long_n"`
	MomentumN                   int     `json:"momentum_n"`
	VolShortN                   int     `json:"vol_short_n"`
	VolLongN                    int     `json:"vol_long_n"`
	DrawdownN                   int     `json:"drawdown_n"`
	QuietTrendThreshold         float64 `json:"quiet_trend_threshold"`
	QuietMomentumThreshold      float64 `json:"quiet_momentum_threshold"`
	QuietVolRatioMax            float64 `json:"quiet_vol_ratio_max"`
	BullTrendMin                float64 `json:"bull_trend_min"`
	BullMomentumMin             float64 `json:"bull_momentum_min"`
	BearTrendMin                float64 `json:"bear_trend_min"`
	BearMomentumMin             float64 `json:"bear_momentum_min"`
	StressDrawdownPct           float64 `json:"stress_drawdown_pct"`
	StressReturnZ               float64 `json:"stress_return_z"`
	StressVolRatioMin           float64 `json:"stress_vol_ratio_min"`
	MarketBetaQuiet             float64 `json:"market_beta_quiet"`
	MarketBetaBull              float64 `json:"market_beta_bull"`
	MarketBetaBear              float64 `json:"market_beta_bear"`
	MarketBetaStress            float64 `json:"market_beta_stress"`
	WDev                        float64 `json:"w_dev"`
	WMom                        float64 `json:"w_mom"`
	WAccel                      float64 `json:"w_accel"`
	WVol                        float64 `json:"w_vol"`
	SignalClip                  float64 `json:"signal_clip"`
	Beta                        float64 `json:"beta"`
	Gamma                       float64 `json:"gamma"`
	MicroWeightMin              float64 `json:"micro_weight_min"`
	MicroWeightMax              float64 `json:"micro_weight_max"`
	WedgeWeightThreshold        float64 `json:"wedge_weight_threshold"`
	WedgeVolatilityThreshold    float64 `json:"wedge_volatility_threshold"`
	MacroIntervalDays           int     `json:"macro_interval_days"`
	MacroEquityPctPerPeriod     float64 `json:"macro_equity_pct_per_period"`
	MacroMaxSpendablePct        float64 `json:"macro_max_spendable_pct"`
	MacroDrawdownRef            float64 `json:"macro_drawdown_ref"`
	MacroValueBoost             float64 `json:"macro_value_boost"`
	MacroDrawdownBoost          float64 `json:"macro_drawdown_boost"`
	MacroOverheatPenalty        float64 `json:"macro_overheat_penalty"`
	MacroMultBull               float64 `json:"macro_mult_bull"`
	MacroMultBear               float64 `json:"macro_mult_bear"`
	MacroMultQuiet              float64 `json:"macro_mult_quiet"`
	MacroMultStress             float64 `json:"macro_mult_stress"`
	ReleaseMinAgeDays           int     `json:"release_min_age_days"`
	ReleaseProfitLogMin         float64 `json:"release_profit_log_min"`
	ReleaseOverheatLogGap       float64 `json:"release_overheat_log_gap"`
	ReleaseMaxPctPerStep        float64 `json:"release_max_pct_per_step"`
	ReleaseTargetFloatWeightGap float64 `json:"release_target_float_weight_gap"`
}

var HardBounds = map[string]HardBound{
	"micro_reserve_pct":               {Min: 0.02, Max: 0.30},
	"ema_fast_n":                      {Min: 6, Max: 96, IsInt: true},
	"ema_mid_n":                       {Min: 48, Max: 720, IsInt: true},
	"ema_long_n":                      {Min: 240, Max: 4320, IsInt: true},
	"momentum_n":                      {Min: 12, Max: 720, IsInt: true},
	"vol_short_n":                     {Min: 6, Max: 168, IsInt: true},
	"vol_long_n":                      {Min: 240, Max: 4320, IsInt: true},
	"drawdown_n":                      {Min: 240, Max: 4320, IsInt: true},
	"quiet_trend_threshold":           {Min: 0.003, Max: 0.050},
	"quiet_momentum_threshold":        {Min: 0.005, Max: 0.080},
	"quiet_vol_ratio_max":             {Min: 0.30, Max: 1.20},
	"bull_trend_min":                  {Min: 0.003, Max: 0.080},
	"bull_momentum_min":               {Min: 0.005, Max: 0.120},
	"bear_trend_min":                  {Min: 0.003, Max: 0.080},
	"bear_momentum_min":               {Min: 0.005, Max: 0.120},
	"stress_drawdown_pct":             {Min: 0.08, Max: 0.45},
	"stress_return_z":                 {Min: 1.5, Max: 6.0},
	"stress_vol_ratio_min":            {Min: 1.00, Max: 3.00},
	"market_beta_quiet":               {Min: 0.30, Max: 1.20},
	"market_beta_bull":                {Min: 0.50, Max: 2.00},
	"market_beta_bear":                {Min: 0.30, Max: 1.50},
	"market_beta_stress":              {Min: 0.20, Max: 1.00},
	"w_dev":                           {Min: -3.00, Max: 3.00},
	"w_mom":                           {Min: -3.00, Max: 3.00},
	"w_accel":                         {Min: -2.00, Max: 2.00},
	"w_vol":                           {Min: -2.00, Max: 2.00},
	"signal_clip":                     {Min: 1.0, Max: 8.0},
	"beta":                            {Min: 0.10, Max: 8.00},
	"gamma":                           {Min: 0.00, Max: 5.00},
	"micro_weight_min":                {Min: 0.00, Max: 0.30},
	"micro_weight_max":                {Min: 0.40, Max: 1.00},
	"wedge_weight_threshold":          {Min: 0.002, Max: 0.080},
	"wedge_volatility_threshold":      {Min: 1.00, Max: 3.00},
	"macro_interval_days":             {Min: 1, Max: 30, IsInt: true},
	"macro_equity_pct_per_period":     {Min: 0.001, Max: 0.050},
	"macro_max_spendable_pct":         {Min: 0.05, Max: 0.80},
	"macro_drawdown_ref":              {Min: 0.10, Max: 0.70},
	"macro_value_boost":               {Min: 0.00, Max: 8.00},
	"macro_drawdown_boost":            {Min: 0.00, Max: 6.00},
	"macro_overheat_penalty":          {Min: 0.00, Max: 8.00},
	"macro_mult_bull":                 {Min: 0.20, Max: 1.50},
	"macro_mult_bear":                 {Min: 0.50, Max: 2.50},
	"macro_mult_quiet":                {Min: 0.30, Max: 1.50},
	"macro_mult_stress":               {Min: 0.10, Max: 1.20},
	"release_min_age_days":            {Min: 30, Max: 1460, IsInt: true},
	"release_profit_log_min":          {Min: 0.05, Max: 1.50},
	"release_overheat_log_gap":        {Min: 0.05, Max: 1.20},
	"release_max_pct_per_step":        {Min: 0.00, Max: 0.20},
	"release_target_float_weight_gap": {Min: 0.01, Max: 0.40},
}

var DefaultSeedChromosome = Chromosome{
	MicroReservePct:             0.08,
	EMAFastN:                    24,
	EMAMidN:                     168,
	EMALongN:                    720,
	MomentumN:                   72,
	VolShortN:                   24,
	VolLongN:                    720,
	DrawdownN:                   720,
	QuietTrendThreshold:         0.010,
	QuietMomentumThreshold:      0.015,
	QuietVolRatioMax:            0.75,
	BullTrendMin:                0.012,
	BullMomentumMin:             0.020,
	BearTrendMin:                0.012,
	BearMomentumMin:             0.020,
	StressDrawdownPct:           0.22,
	StressReturnZ:               3.0,
	StressVolRatioMin:           1.60,
	MarketBetaQuiet:             0.60,
	MarketBetaBull:              1.15,
	MarketBetaBear:              0.85,
	MarketBetaStress:            0.50,
	WDev:                        0.80,
	WMom:                        -0.35,
	WAccel:                      0.20,
	WVol:                        0.15,
	SignalClip:                  4.0,
	Beta:                        1.60,
	Gamma:                       1.00,
	MicroWeightMin:              0.02,
	MicroWeightMax:              0.85,
	WedgeWeightThreshold:        0.015,
	WedgeVolatilityThreshold:    1.35,
	MacroIntervalDays:           7,
	MacroEquityPctPerPeriod:     0.010,
	MacroMaxSpendablePct:        0.35,
	MacroDrawdownRef:            0.35,
	MacroValueBoost:             2.00,
	MacroDrawdownBoost:          1.50,
	MacroOverheatPenalty:        2.00,
	MacroMultBull:               0.70,
	MacroMultBear:               1.20,
	MacroMultQuiet:              0.90,
	MacroMultStress:             0.50,
	ReleaseMinAgeDays:           365,
	ReleaseProfitLogMin:         0.35,
	ReleaseOverheatLogGap:       0.30,
	ReleaseMaxPctPerStep:        0.05,
	ReleaseTargetFloatWeightGap: 0.10,
}

// ClampChromosome repairs numeric bounds and documented structural constraints.
func ClampChromosome(c Chromosome) Chromosome {
	c.MicroReservePct = clampByKey("micro_reserve_pct", c.MicroReservePct)
	c.EMAFastN = clampIntByKey("ema_fast_n", c.EMAFastN)
	c.EMAMidN = clampIntByKey("ema_mid_n", c.EMAMidN)
	c.EMALongN = clampIntByKey("ema_long_n", c.EMALongN)
	c.MomentumN = clampIntByKey("momentum_n", c.MomentumN)
	c.VolShortN = clampIntByKey("vol_short_n", c.VolShortN)
	c.VolLongN = clampIntByKey("vol_long_n", c.VolLongN)
	c.DrawdownN = clampIntByKey("drawdown_n", c.DrawdownN)
	c.QuietTrendThreshold = clampByKey("quiet_trend_threshold", c.QuietTrendThreshold)
	c.QuietMomentumThreshold = clampByKey("quiet_momentum_threshold", c.QuietMomentumThreshold)
	c.QuietVolRatioMax = clampByKey("quiet_vol_ratio_max", c.QuietVolRatioMax)
	c.BullTrendMin = clampByKey("bull_trend_min", c.BullTrendMin)
	c.BullMomentumMin = clampByKey("bull_momentum_min", c.BullMomentumMin)
	c.BearTrendMin = clampByKey("bear_trend_min", c.BearTrendMin)
	c.BearMomentumMin = clampByKey("bear_momentum_min", c.BearMomentumMin)
	c.StressDrawdownPct = clampByKey("stress_drawdown_pct", c.StressDrawdownPct)
	c.StressReturnZ = clampByKey("stress_return_z", c.StressReturnZ)
	c.StressVolRatioMin = clampByKey("stress_vol_ratio_min", c.StressVolRatioMin)
	c.MarketBetaQuiet = clampByKey("market_beta_quiet", c.MarketBetaQuiet)
	c.MarketBetaBull = clampByKey("market_beta_bull", c.MarketBetaBull)
	c.MarketBetaBear = clampByKey("market_beta_bear", c.MarketBetaBear)
	c.MarketBetaStress = clampByKey("market_beta_stress", c.MarketBetaStress)
	c.WDev = clampByKey("w_dev", c.WDev)
	c.WMom = clampByKey("w_mom", c.WMom)
	c.WAccel = clampByKey("w_accel", c.WAccel)
	c.WVol = clampByKey("w_vol", c.WVol)
	c.SignalClip = clampByKey("signal_clip", c.SignalClip)
	c.Beta = clampByKey("beta", c.Beta)
	c.Gamma = clampByKey("gamma", c.Gamma)
	c.MicroWeightMin = clampByKey("micro_weight_min", c.MicroWeightMin)
	c.MicroWeightMax = clampByKey("micro_weight_max", c.MicroWeightMax)
	c.WedgeWeightThreshold = clampByKey("wedge_weight_threshold", c.WedgeWeightThreshold)
	c.WedgeVolatilityThreshold = clampByKey("wedge_volatility_threshold", c.WedgeVolatilityThreshold)
	c.MacroIntervalDays = clampIntByKey("macro_interval_days", c.MacroIntervalDays)
	c.MacroEquityPctPerPeriod = clampByKey("macro_equity_pct_per_period", c.MacroEquityPctPerPeriod)
	c.MacroMaxSpendablePct = clampByKey("macro_max_spendable_pct", c.MacroMaxSpendablePct)
	c.MacroDrawdownRef = clampByKey("macro_drawdown_ref", c.MacroDrawdownRef)
	c.MacroValueBoost = clampByKey("macro_value_boost", c.MacroValueBoost)
	c.MacroDrawdownBoost = clampByKey("macro_drawdown_boost", c.MacroDrawdownBoost)
	c.MacroOverheatPenalty = clampByKey("macro_overheat_penalty", c.MacroOverheatPenalty)
	c.MacroMultBull = clampByKey("macro_mult_bull", c.MacroMultBull)
	c.MacroMultBear = clampByKey("macro_mult_bear", c.MacroMultBear)
	c.MacroMultQuiet = clampByKey("macro_mult_quiet", c.MacroMultQuiet)
	c.MacroMultStress = clampByKey("macro_mult_stress", c.MacroMultStress)
	c.ReleaseMinAgeDays = clampIntByKey("release_min_age_days", c.ReleaseMinAgeDays)
	c.ReleaseProfitLogMin = clampByKey("release_profit_log_min", c.ReleaseProfitLogMin)
	c.ReleaseOverheatLogGap = clampByKey("release_overheat_log_gap", c.ReleaseOverheatLogGap)
	c.ReleaseMaxPctPerStep = clampByKey("release_max_pct_per_step", c.ReleaseMaxPctPerStep)
	c.ReleaseTargetFloatWeightGap = clampByKey("release_target_float_weight_gap", c.ReleaseTargetFloatWeightGap)

	if c.EMAMidN < 2*c.EMAFastN {
		c.EMAMidN = 2 * c.EMAFastN
	}
	c.EMAMidN = clampIntByKey("ema_mid_n", c.EMAMidN)
	if c.EMALongN < 2*c.EMAMidN {
		c.EMALongN = 2 * c.EMAMidN
	}
	c.EMALongN = clampIntByKey("ema_long_n", c.EMALongN)

	if c.VolLongN < 3*c.VolShortN {
		c.VolLongN = 3 * c.VolShortN
	}
	c.VolLongN = clampIntByKey("vol_long_n", c.VolLongN)

	if c.MomentumN < c.EMAFastN {
		c.MomentumN = c.EMAFastN
	}
	if c.MomentumN > c.EMALongN {
		c.MomentumN = c.EMALongN
	}
	c.MomentumN = clampIntByKey("momentum_n", c.MomentumN)

	maxWeightByReserve := 1 - c.MicroReservePct
	if c.MicroWeightMax > maxWeightByReserve {
		c.MicroWeightMax = maxWeightByReserve
	}
	if c.MicroWeightMin >= c.MicroWeightMax {
		c.MicroWeightMin = DefaultSeedChromosome.MicroWeightMin
		c.MicroWeightMax = DefaultSeedChromosome.MicroWeightMax
		if c.MicroWeightMax > maxWeightByReserve {
			c.MicroWeightMax = maxWeightByReserve
		}
	}
	c.MicroWeightMin = ClipFloat64(c.MicroWeightMin, 0, 1)
	c.MicroWeightMax = ClipFloat64(c.MicroWeightMax, c.MicroWeightMin+0.000001, 1)

	return c
}

func chromosomeOrDefault(c Chromosome) Chromosome {
	if c == (Chromosome{}) {
		return DefaultSeedChromosome
	}
	return ClampChromosome(c)
}

// RescaleChromosomeWindows keeps wall-clock semantics when base bar size changes.
func RescaleChromosomeWindows(c Chromosome, oldBaseMinutes, newBaseMinutes int) Chromosome {
	if oldBaseMinutes <= 0 || newBaseMinutes <= 0 {
		return ClampChromosome(c)
	}
	scale := float64(oldBaseMinutes) / float64(newBaseMinutes)
	c.EMAFastN = int(math.Round(float64(c.EMAFastN) * scale))
	c.EMAMidN = int(math.Round(float64(c.EMAMidN) * scale))
	c.EMALongN = int(math.Round(float64(c.EMALongN) * scale))
	c.MomentumN = int(math.Round(float64(c.MomentumN) * scale))
	c.VolShortN = int(math.Round(float64(c.VolShortN) * scale))
	c.VolLongN = int(math.Round(float64(c.VolLongN) * scale))
	c.DrawdownN = int(math.Round(float64(c.DrawdownN) * scale))
	return ClampChromosome(c)
}

type SpawnPoint struct {
	Symbol               string             `json:"symbol"`
	QuoteAsset           string             `json:"quote_asset"`
	BaseInterval         string             `json:"base_interval"`
	InitialAvailableUSDT float64            `json:"initial_available_usdt"`
	InitialDeadBTC       float64            `json:"initial_dead_btc"`
	InitialFloatBTC      float64            `json:"initial_float_btc"`
	InitialColdSealedBTC float64            `json:"initial_cold_sealed_btc"`
	Policy               SpawnPolicy        `json:"policy"`
	Risk                 SpawnRisk          `json:"risk"`
	Precision            TradingConstraints `json:"precision"`
	StartTime            int64              `json:"start_time"`
	EvaluationWindows    []EvaluationWindow `json:"evaluation_windows,omitempty"`
}

type SpawnPolicy struct {
	MonthlyInjectUSDT    float64 `json:"monthly_inject_usdt"`
	DeadlineSpendablePct float64 `json:"deadline_spendable_pct"`
	SoftReleaseAgeMonths int     `json:"soft_release_age_months"`
	SoftReleaseMaxPct    float64 `json:"soft_release_max_pct"`
	DeadBTCFloor         float64 `json:"dead_btc_floor"`
	ReleaseTargetGapBTC  float64 `json:"release_target_gap_btc"`
	MacroDeadlineDays    int     `json:"macro_deadline_days"`
}

type SpawnRisk struct {
	FeeRate              float64 `json:"fee_rate"`
	SlippagePct          float64 `json:"slippage_pct"`
	GlobalStopLossPct    float64 `json:"global_stop_loss_pct"`
	FatalMaxDrawdownPct  float64 `json:"fatal_max_drawdown_pct"`
	MaxOrderNotionalUSDT float64 `json:"max_order_notional_usdt"`
}

type EvaluationWindow struct {
	Name        string  `json:"name"`
	Weight      float64 `json:"weight"`
	WarmupStart int64   `json:"warmup_start"`
	ScoreStart  int64   `json:"score_start"`
	ScoreEnd    int64   `json:"score_end"`
}

func clampByKey(key string, value float64) float64 {
	bound := HardBounds[key]
	return ClipFloat64(value, bound.Min, bound.Max)
}

func clampIntByKey(key string, value int) int {
	bound := HardBounds[key]
	if value < int(bound.Min) {
		return int(bound.Min)
	}
	if value > int(bound.Max) {
		return int(bound.Max)
	}
	return value
}
