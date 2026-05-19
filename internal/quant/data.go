package quant

const (
	ActionBuy  TradeAction = "BUY"
	ActionSell TradeAction = "SELL"

	EngineMacro EngineType = "MACRO"
	EngineMicro EngineType = "MICRO"

	LotTypeDeadStack  LotType = "DEAD_STACK"
	LotTypeFloating   LotType = "FLOATING"
	LotTypeColdSealed LotType = "COLD_SEALED"
)

type TradeAction string

type EngineType string

type LotType string

// Bar is one fully closed OHLCV candle.
type Bar struct {
	OpenTime int64   `json:"open_time"`
	Open     float64 `json:"open"`
	High     float64 `json:"high"`
	Low      float64 `json:"low"`
	Close    float64 `json:"close"`
	Volume   float64 `json:"volume"`
}

// PortfolioSnapshot is the strategy-visible account and semantic ledger snapshot.
type PortfolioSnapshot struct {
	USDTBalance   float64 `json:"usdt_balance"`
	DeadBTC       float64 `json:"dead_btc"`
	FloatBTC      float64 `json:"float_btc"`
	ColdSealedBTC float64 `json:"cold_sealed_btc"`
}

// RuntimeState is the explicit state carried between Step calls.
type RuntimeState struct {
	LastMacroBuyAt       int64    `json:"last_macro_buy_at"`
	LastProcessedBucket  int64    `json:"last_processed_bucket"`
	LastReasonCodes      []string `json:"last_reason_codes,omitempty"`
	LastMarketState      string   `json:"last_market_state,omitempty"`
	LastMicroTargetBasis float64  `json:"last_micro_target_basis,omitempty"`
}

// TradingConstraints contains execution limits supplied by SaaS or backtest adapters.
type TradingConstraints struct {
	MinOrderUSDT float64 `json:"min_order_usdt"`
	LotStep      float64 `json:"lot_step"`
	LotMin       float64 `json:"lot_min"`
	PriceTick    float64 `json:"price_tick"`
	FeeRate      float64 `json:"fee_rate"`
	SlippagePct  float64 `json:"slippage_pct"`
}

// StrategyInput is the pure strategy snapshot consumed by Step.
type StrategyInput struct {
	Symbol            string             `json:"symbol"`
	QuoteAsset        string             `json:"quote_asset"`
	Closes            []float64          `json:"closes"`
	Timestamps        []int64            `json:"timestamps"`
	CurrentPrice      float64            `json:"current_price"`
	Portfolio         PortfolioSnapshot  `json:"portfolio"`
	Runtime           RuntimeState       `json:"runtime"`
	SpotLots          []SpotLot          `json:"spot_lots,omitempty"`
	Chromosome        Chromosome         `json:"chromosome"`
	SpawnPoint        SpawnPoint         `json:"spawn_point"`
	Constraints       TradingConstraints `json:"constraints"`
	CurrentBucketTime int64              `json:"current_bucket_time"`
	TotalEquity       float64            `json:"total_equity"`
	SpendableUSDT     float64            `json:"spendable_usdt"`
}

// TradeIntent is a strategy output intent. It does not execute I/O.
type TradeIntent struct {
	Action     TradeAction `json:"action"`
	Engine     EngineType  `json:"engine"`
	LotType    LotType     `json:"lot_type"`
	AmountUSDT float64     `json:"amount_usdt,omitempty"`
	QtyAsset   float64     `json:"qty_asset,omitempty"`
	ReasonCode string      `json:"reason_code"`
}

// LedgerConversion expresses a semantic ledger conversion such as DeadBTC to FloatBTC.
type LedgerConversion struct {
	FromLotType LotType `json:"from_lot_type"`
	ToLotType   LotType `json:"to_lot_type"`
	Amount      float64 `json:"amount"`
	ReasonCode  string  `json:"reason_code"`
}

// StrategyOutput is the Step output intent set.
type StrategyOutput struct {
	MacroIntents      []TradeIntent      `json:"macro_intents,omitempty"`
	MicroIntents      []TradeIntent      `json:"micro_intents,omitempty"`
	LedgerConversions []LedgerConversion `json:"ledger_conversions,omitempty"`
	NextRuntime       RuntimeState       `json:"next_runtime"`
	ReasonCodes       []string           `json:"reason_codes,omitempty"`
	SkipTrading       bool               `json:"skip_trading"`
	SkipReason        string             `json:"skip_reason,omitempty"`
}
