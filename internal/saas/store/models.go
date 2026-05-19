package store

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

type JSONB []byte

func (j JSONB) Value() (driver.Value, error) {
	if len(j) == 0 {
		return "{}", nil
	}
	if !json.Valid(j) {
		return nil, errors.New("invalid jsonb value")
	}
	return string(j), nil
}

func (j *JSONB) Scan(value any) error {
	switch v := value.(type) {
	case nil:
		*j = nil
		return nil
	case []byte:
		return j.scanBytes(v)
	case string:
		return j.scanBytes([]byte(v))
	default:
		return fmt.Errorf("scan jsonb from %T", value)
	}
}

func (j JSONB) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("{}"), nil
	}
	if !json.Valid(j) {
		return nil, errors.New("invalid jsonb value")
	}
	return []byte(j), nil
}

func (j *JSONB) UnmarshalJSON(data []byte) error {
	return j.scanBytes(data)
}

func (j *JSONB) scanBytes(data []byte) error {
	if len(data) == 0 {
		*j = nil
		return nil
	}
	if !json.Valid(data) {
		return errors.New("invalid jsonb value")
	}
	*j = append((*j)[:0], data...)
	return nil
}

type Decimal string

func (d Decimal) Value() (driver.Value, error) {
	if d == "" {
		return "0", nil
	}
	return string(d), nil
}

func (d *Decimal) Scan(value any) error {
	switch v := value.(type) {
	case nil:
		*d = ""
	case []byte:
		*d = Decimal(string(v))
	case string:
		*d = Decimal(v)
	case int64:
		*d = Decimal(strconv.FormatInt(v, 10))
	case float64:
		*d = Decimal(strconv.FormatFloat(v, 'f', -1, 64))
	default:
		return fmt.Errorf("scan decimal from %T", value)
	}
	return nil
}

func (d Decimal) MarshalJSON() ([]byte, error) {
	if d == "" {
		return []byte(`"0"`), nil
	}
	return json.Marshal(string(d))
}

func (d *Decimal) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err == nil {
		*d = Decimal(value)
		return nil
	}

	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return err
	}
	*d = Decimal(number.String())
	return nil
}

type StrategyInstanceStatus string

const (
	StrategyInstanceRunning StrategyInstanceStatus = "RUNNING"
	StrategyInstanceStopped StrategyInstanceStatus = "STOPPED"
	StrategyInstanceError   StrategyInstanceStatus = "ERROR"
	StrategyInstanceDeleted StrategyInstanceStatus = "DELETED"
)

type SpotLotType string

const (
	SpotLotDeadStack  SpotLotType = "DEAD_STACK"
	SpotLotFloating   SpotLotType = "FLOATING"
	SpotLotColdSealed SpotLotType = "COLD_SEALED"
)

type SpotExecutionStatus string

const (
	SpotExecutionPending SpotExecutionStatus = "pending"
	SpotExecutionFilled  SpotExecutionStatus = "filled"
	SpotExecutionFailed  SpotExecutionStatus = "failed"
)

type GeneRole string

const (
	GeneRoleChallenger GeneRole = "challenger"
	GeneRoleChampion   GeneRole = "champion"
	GeneRoleRetired    GeneRole = "retired"
)

type EvolutionTaskStatus string

const (
	EvolutionTaskQueued    EvolutionTaskStatus = "queued"
	EvolutionTaskRunning   EvolutionTaskStatus = "running"
	EvolutionTaskSucceeded EvolutionTaskStatus = "succeeded"
	EvolutionTaskFailed    EvolutionTaskStatus = "failed"
	EvolutionTaskCanceled  EvolutionTaskStatus = "canceled"
)

type BacktestRunStatus string

const (
	BacktestRunRunning   BacktestRunStatus = "running"
	BacktestRunSucceeded BacktestRunStatus = "succeeded"
	BacktestRunFailed    BacktestRunStatus = "failed"
)

type BacktestPositionSide string

const (
	BacktestPositionLong  BacktestPositionSide = "LONG"
	BacktestPositionShort BacktestPositionSide = "SHORT"
)

type BacktestOffset string

const (
	BacktestOffsetOpen  BacktestOffset = "OPEN"
	BacktestOffsetClose BacktestOffset = "CLOSE"
)

type User struct {
	ID                    uint       `gorm:"primaryKey"`
	CreatedAt             time.Time  `gorm:"not null"`
	UpdatedAt             time.Time  `gorm:"not null"`
	Email                 string     `gorm:"size:320;not null;uniqueIndex"`
	PasswordHash          string     `gorm:"size:255;not null"`
	Role                  string     `gorm:"size:64;not null;default:user"`
	SubscriptionPlan      string     `gorm:"size:64;not null;default:free"`
	SubscriptionStatus    string     `gorm:"size:64;not null;default:active"`
	SubscriptionExpiresAt *time.Time `gorm:"index"`
	StrategyInstances     []StrategyInstance
}

type StrategyTemplate struct {
	ID        uint               `gorm:"primaryKey"`
	CreatedAt time.Time          `gorm:"not null"`
	UpdatedAt time.Time          `gorm:"not null"`
	Name      string             `gorm:"size:128;not null;uniqueIndex:idx_strategy_templates_name_version"`
	Version   string             `gorm:"size:64;not null;uniqueIndex:idx_strategy_templates_name_version"`
	IsSpot    bool               `gorm:"not null;default:true"`
	Manifest  JSONB              `gorm:"type:jsonb;not null"`
	Instances []StrategyInstance `gorm:"foreignKey:TemplateID"`
}

type StrategyInstance struct {
	ID           uint                   `gorm:"primaryKey"`
	CreatedAt    time.Time              `gorm:"not null"`
	UpdatedAt    time.Time              `gorm:"not null"`
	UserID       uint                   `gorm:"not null;index"`
	User         *User                  `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	TemplateID   uint                   `gorm:"not null;index"`
	Template     *StrategyTemplate      `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Name         string                 `gorm:"size:128;not null"`
	Symbol       string                 `gorm:"size:32;not null;index"`
	Interval     string                 `gorm:"size:16;not null"`
	Status       StrategyInstanceStatus `gorm:"type:varchar(16);not null;default:STOPPED;index;check:strategy_instance_status_check,status = 'RUNNING' OR status = 'STOPPED' OR status = 'ERROR' OR status = 'DELETED'"`
	Config       JSONB                  `gorm:"type:jsonb;not null"`
	LastError    string                 `gorm:"type:text"`
	Portfolio    *PortfolioState
	RuntimeState *RuntimeState
}

type PortfolioState struct {
	ID                   uint      `gorm:"primaryKey"`
	CreatedAt            time.Time `gorm:"not null"`
	UpdatedAt            time.Time `gorm:"not null"`
	StrategyInstanceID   uint      `gorm:"not null;uniqueIndex"`
	StrategyInstance     *StrategyInstance
	USDTBalance          Decimal    `gorm:"type:numeric(36,18);not null;default:0"`
	DeadBTC              Decimal    `gorm:"type:numeric(36,18);not null;default:0"`
	FloatBTC             Decimal    `gorm:"type:numeric(36,18);not null;default:0"`
	ColdSealedBTC        Decimal    `gorm:"type:numeric(36,18);not null;default:0"`
	TotalEquity          Decimal    `gorm:"type:numeric(36,18);not null;default:0"`
	LastProcessedBarTime *time.Time `gorm:"index"`
}

type RuntimeState struct {
	ID                 uint              `gorm:"primaryKey"`
	CreatedAt          time.Time         `gorm:"not null"`
	UpdatedAt          time.Time         `gorm:"not null"`
	StrategyInstanceID uint              `gorm:"not null;uniqueIndex"`
	StrategyInstance   *StrategyInstance `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	State              JSONB             `gorm:"type:jsonb;not null"`
}

type SpotLot struct {
	ID                 uint              `gorm:"primaryKey"`
	CreatedAt          time.Time         `gorm:"not null"`
	UpdatedAt          time.Time         `gorm:"not null"`
	StrategyInstanceID uint              `gorm:"not null;index"`
	StrategyInstance   *StrategyInstance `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	LotType            SpotLotType       `gorm:"type:varchar(16);not null;index;check:spot_lot_type_check,lot_type = 'DEAD_STACK' OR lot_type = 'FLOATING' OR lot_type = 'COLD_SEALED'"`
	Amount             Decimal           `gorm:"type:numeric(36,18);not null;default:0"`
	CostPrice          Decimal           `gorm:"type:numeric(36,18);not null;default:0"`
	IsColdSealed       bool              `gorm:"not null;default:false"`
}

type TradeRecord struct {
	ID                 uint              `gorm:"primaryKey"`
	CreatedAt          time.Time         `gorm:"not null"`
	UpdatedAt          time.Time         `gorm:"not null"`
	StrategyInstanceID uint              `gorm:"not null;index"`
	StrategyInstance   *StrategyInstance `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	ClientOrderID      string            `gorm:"size:128;not null;uniqueIndex"`
	Action             string            `gorm:"size:16;not null;index"`
	Engine             string            `gorm:"size:16;not null;index"`
	Symbol             string            `gorm:"size:32;not null;index"`
	FilledQty          Decimal           `gorm:"type:numeric(36,18);not null;default:0"`
	FilledPrice        Decimal           `gorm:"type:numeric(36,18);not null;default:0"`
	Fee                Decimal           `gorm:"type:numeric(36,18);not null;default:0"`
	ExecutedAt         *time.Time        `gorm:"index"`
}

type SpotExecution struct {
	ID                 uint                `gorm:"primaryKey"`
	CreatedAt          time.Time           `gorm:"not null"`
	UpdatedAt          time.Time           `gorm:"not null"`
	StrategyInstanceID uint                `gorm:"not null;index"`
	StrategyInstance   *StrategyInstance   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	ClientOrderID      string              `gorm:"size:128;not null;uniqueIndex"`
	Action             string              `gorm:"size:16;not null;index"`
	Engine             string              `gorm:"size:16;not null;index"`
	Symbol             string              `gorm:"size:32;not null;index"`
	Status             SpotExecutionStatus `gorm:"type:varchar(16);not null;default:pending;index;check:spot_execution_status_check,status = 'pending' OR status = 'filled' OR status = 'failed'"`
	CommandPayload     JSONB               `gorm:"type:jsonb;not null"`
	ExecutionPayload   JSONB               `gorm:"type:jsonb;not null"`
	ErrorMessage       string              `gorm:"type:text"`
	FilledAt           *time.Time          `gorm:"index"`
	FailedAt           *time.Time          `gorm:"index"`
}

type AuditLog struct {
	ID                 uint      `gorm:"primaryKey"`
	CreatedAt          time.Time `gorm:"not null;index"`
	UpdatedAt          time.Time `gorm:"not null"`
	EventType          string    `gorm:"size:128;not null;index"`
	Payload            JSONB     `gorm:"type:jsonb;not null"`
	UserID             *uint     `gorm:"index"`
	StrategyInstanceID *uint     `gorm:"index"`
	TraceID            string    `gorm:"size:128;index"`
}

type GeneRecord struct {
	ID             uint      `gorm:"primaryKey"`
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`
	StrategyID     string    `gorm:"size:128;not null;index:idx_gene_strategy_symbol_role"`
	Symbol         string    `gorm:"size:32;not null;index:idx_gene_strategy_symbol_role"`
	Role           GeneRole  `gorm:"type:varchar(16);not null;index:idx_gene_strategy_symbol_role;check:gene_role_check,role = 'challenger' OR role = 'champion' OR role = 'retired'"`
	ParamPack      JSONB     `gorm:"type:jsonb;not null"`
	ScoreTotal     Decimal   `gorm:"type:numeric(36,18);not null;default:0"`
	MaxDrawdown    Decimal   `gorm:"type:numeric(18,12);not null;default:0"`
	ScoreReport    JSONB     `gorm:"type:jsonb;not null"`
	PromotedByUser *uint     `gorm:"index"`
	PromotedAt     *time.Time
	RetiredAt      *time.Time
}

type EvolutionTask struct {
	ID                uint                `gorm:"primaryKey"`
	CreatedAt         time.Time           `gorm:"not null"`
	UpdatedAt         time.Time           `gorm:"not null"`
	StrategyID        string              `gorm:"size:128;not null;index"`
	Symbol            string              `gorm:"size:32;not null;index"`
	Status            EvolutionTaskStatus `gorm:"type:varchar(16);not null;default:queued;index;check:evolution_task_status_check,status = 'queued' OR status = 'running' OR status = 'succeeded' OR status = 'failed' OR status = 'canceled'"`
	Progress          int                 `gorm:"not null;default:0"`
	Config            JSONB               `gorm:"type:jsonb;not null"`
	CurrentGeneration int                 `gorm:"not null;default:0"`
	BestScore         Decimal             `gorm:"type:numeric(36,18);not null;default:0"`
	BestGeneID        string              `gorm:"size:128;index"`
	StartedAt         *time.Time          `gorm:"index"`
	FinishedAt        *time.Time          `gorm:"index"`
	Error             string              `gorm:"type:text"`
}

type BacktestRun struct {
	ID         uint              `gorm:"primaryKey"`
	CreatedAt  time.Time         `gorm:"not null"`
	UpdatedAt  time.Time         `gorm:"not null"`
	UserID     uint              `gorm:"not null;index"`
	User       *User             `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	StrategyID string            `gorm:"size:128;not null;index"`
	Symbol     string            `gorm:"size:32;not null;index"`
	Interval   string            `gorm:"size:16;not null;index"`
	Status     BacktestRunStatus `gorm:"type:varchar(16);not null;default:running;index;check:backtest_run_status_check,status = 'running' OR status = 'succeeded' OR status = 'failed'"`
	Request    JSONB             `gorm:"type:jsonb;not null"`
	Result     JSONB             `gorm:"type:jsonb;not null"`
	Error      string            `gorm:"type:text"`
	StartedAt  *time.Time        `gorm:"index"`
	FinishedAt *time.Time        `gorm:"index"`
}

type KLine struct {
	ID        uint      `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
	Symbol    string    `gorm:"size:32;not null;uniqueIndex:idx_klines_symbol_interval_open_time;uniqueIndex:idx_market_klines_symbol_interval_open_time_unique"`
	Interval  string    `gorm:"size:16;not null;uniqueIndex:idx_klines_symbol_interval_open_time;uniqueIndex:idx_market_klines_symbol_interval_open_time_unique"`
	OpenTime  time.Time `gorm:"not null;uniqueIndex:idx_klines_symbol_interval_open_time;uniqueIndex:idx_market_klines_symbol_interval_open_time_unique"`
	Open      Decimal   `gorm:"type:numeric(36,18);not null"`
	High      Decimal   `gorm:"type:numeric(36,18);not null"`
	Low       Decimal   `gorm:"type:numeric(36,18);not null"`
	Close     Decimal   `gorm:"type:numeric(36,18);not null"`
	Volume    Decimal   `gorm:"type:numeric(36,18);not null"`
	CloseTime time.Time `gorm:"index"`
}

func (KLine) TableName() string {
	return "market_klines"
}

type BTAccount struct {
	ID               uint      `gorm:"primaryKey"`
	CreatedAt        time.Time `gorm:"not null"`
	UpdatedAt        time.Time `gorm:"not null"`
	BacktestRunID    uint      `gorm:"not null;index"`
	InitialEquity    Decimal   `gorm:"type:numeric(36,18);not null;default:0"`
	CurrentEquity    Decimal   `gorm:"type:numeric(36,18);not null;default:0"`
	AvailableBalance Decimal   `gorm:"type:numeric(36,18);not null;default:0"`
	FrozenMargin     Decimal   `gorm:"type:numeric(36,18);not null;default:0"`
	RealizedPnL      Decimal   `gorm:"type:numeric(36,18);not null;default:0"`
}

func (BTAccount) TableName() string {
	return "bt_account"
}

type BTOrder struct {
	ID             uint                 `gorm:"primaryKey"`
	CreatedAt      time.Time            `gorm:"not null"`
	UpdatedAt      time.Time            `gorm:"not null"`
	BacktestRunID  uint                 `gorm:"not null;index"`
	Symbol         string               `gorm:"size:32;not null;index"`
	Side           string               `gorm:"size:16;not null;index"`
	OffsetFlag     BacktestOffset       `gorm:"column:offset_flag;type:varchar(16);not null;index;check:bt_order_offset_check,offset_flag = 'OPEN' OR offset_flag = 'CLOSE'"`
	PositionSide   BacktestPositionSide `gorm:"type:varchar(16);not null;index;check:bt_order_position_side_check,position_side = 'LONG' OR position_side = 'SHORT'"`
	Leverage       int                  `gorm:"not null;default:1"`
	OrderPrice     Decimal              `gorm:"type:numeric(36,18);not null;default:0"`
	ExecutedPrice  Decimal              `gorm:"type:numeric(36,18);not null;default:0"`
	ExecutedQty    Decimal              `gorm:"type:numeric(36,18);not null;default:0"`
	Fee            Decimal              `gorm:"type:numeric(36,18);not null;default:0"`
	Status         string               `gorm:"size:32;not null;index"`
	OrderedAt      time.Time            `gorm:"not null;index"`
	ReasonCode     string               `gorm:"size:128;index"`
	RealizedPnL    Decimal              `gorm:"type:numeric(36,18);not null;default:0"`
	MarginReleased Decimal              `gorm:"type:numeric(36,18);not null;default:0"`
}

func (BTOrder) TableName() string {
	return "bt_orders"
}

type BTPosition struct {
	ID              uint                 `gorm:"primaryKey"`
	CreatedAt       time.Time            `gorm:"not null"`
	UpdatedAt       time.Time            `gorm:"not null"`
	BacktestRunID   uint                 `gorm:"not null;index:idx_bt_position_run_symbol_side"`
	Symbol          string               `gorm:"size:32;not null;index:idx_bt_position_run_symbol_side"`
	PositionSide    BacktestPositionSide `gorm:"type:varchar(16);not null;index:idx_bt_position_run_symbol_side;check:bt_position_side_check,position_side = 'LONG' OR position_side = 'SHORT'"`
	AverageEntry    Decimal              `gorm:"type:numeric(36,18);not null;default:0"`
	Quantity        Decimal              `gorm:"type:numeric(36,18);not null;default:0"`
	Leverage        int                  `gorm:"not null;default:1"`
	UsedMargin      Decimal              `gorm:"type:numeric(36,18);not null;default:0"`
	UnrealizedPnL   Decimal              `gorm:"type:numeric(36,18);not null;default:0"`
	LiquidationHint Decimal              `gorm:"type:numeric(36,18);not null;default:0"`
}

func (BTPosition) TableName() string {
	return "bt_positions"
}

type BTTradeLog struct {
	ID            uint      `gorm:"primaryKey"`
	CreatedAt     time.Time `gorm:"not null"`
	UpdatedAt     time.Time `gorm:"not null"`
	BacktestRunID uint      `gorm:"not null;index"`
	Timestamp     time.Time `gorm:"not null;index"`
	Symbol        string    `gorm:"size:32;not null;index"`
	Level         string    `gorm:"size:16;not null;index"`
	TriggerDetail string    `gorm:"type:text"`
	ErrorMessage  string    `gorm:"type:text"`
}

func (BTTradeLog) TableName() string {
	return "bt_trade_logs"
}

type BTReport struct {
	ID              uint      `gorm:"primaryKey"`
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
	BacktestRunID   uint      `gorm:"not null;uniqueIndex"`
	ParameterSnap   JSONB     `gorm:"type:jsonb;not null"`
	TotalReturn     Decimal   `gorm:"type:numeric(36,18);not null;default:0"`
	MaxDrawdown     Decimal   `gorm:"type:numeric(18,12);not null;default:0"`
	WinRate         Decimal   `gorm:"type:numeric(18,12);not null;default:0"`
	ProfitLossRatio Decimal   `gorm:"type:numeric(36,18);not null;default:0"`
	SharpeRatio     Decimal   `gorm:"type:numeric(36,18);not null;default:0"`
	TradeCount      int       `gorm:"not null;default:0"`
	ExecutionMs     int64     `gorm:"not null;default:0"`
}

func (BTReport) TableName() string {
	return "bt_reports"
}
