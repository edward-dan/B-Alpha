package instance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"bian-trade-go/internal/quant"
	"bian-trade-go/internal/saas/store"
	saasws "bian-trade-go/internal/saas/ws"
	"bian-trade-go/internal/strategies/example"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultMarketLookback     = 5000
	defaultCommandTTL         = 2 * time.Minute
	championCacheTTL          = 5 * time.Minute
	auditInstanceCreated      = "INSTANCE_CREATED"
	auditInstanceStarted      = "INSTANCE_STARTED"
	auditInstanceStopped      = "INSTANCE_STOPPED"
	auditInstanceDeleted      = "INSTANCE_DELETED"
	auditInstanceError        = "INSTANCE_ERROR"
	auditLedgerConversion     = "LEDGER_CONVERSION_APPLIED"
	auditAgentDisconnected    = "AGENT_DISCONNECTED"
	auditCommandDispatchError = "TRADE_COMMAND_DISPATCH_FAILED"
)

var (
	ErrInvalidTransition = errors.New("invalid strategy instance state transition")
	ErrManagerNotReady   = errors.New("instance manager is not initialized")
)

// MarketDataClient is a public-market-data boundary. Implementations must only
// return fully closed bars and must not require exchange private credentials.
type MarketDataClient interface {
	LatestClosedBars(ctx context.Context, symbol string, interval string, limit int) ([]quant.Bar, error)
}

type ChampionCache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
}

type CommandHub interface {
	IsAgentConnected(agentID string) bool
	SendTradeCommand(ctx context.Context, agentID string, command TradeCommand) error
}

type ManagerConfig struct {
	DB         *gorm.DB
	MarketData MarketDataClient
	Hub        CommandHub
	Cache      ChampionCache
	Logger     *zap.Logger
	Now        func() time.Time
}

type Manager struct {
	db         *gorm.DB
	marketData MarketDataClient
	hub        CommandHub
	cache      ChampionCache
	logger     *zap.Logger
	now        func() time.Time
	active     sync.Map // strategy instance ID uint -> struct{}
	restoredMu sync.RWMutex
	restored   bool
}

type CreateRequest struct {
	UserID           uint            `json:"user_id"`
	TemplateID       uint            `json:"template_id"`
	Name             string          `json:"name"`
	Symbol           string          `json:"symbol"`
	Interval         string          `json:"interval"`
	Config           json.RawMessage `json:"config"`
	InitialPortfolio PortfolioSeed   `json:"initial_portfolio"`
	InitialCostPrice float64         `json:"initial_cost_price"`
}

type PortfolioSeed struct {
	USDTBalance   float64 `json:"usdt_balance"`
	DeadBTC       float64 `json:"dead_btc"`
	FloatBTC      float64 `json:"float_btc"`
	ColdSealedBTC float64 `json:"cold_sealed_btc"`
	TotalEquity   float64 `json:"total_equity"`
}

type RuntimeConfig struct {
	AgentID           string                   `json:"agent_id"`
	StrategyID        string                   `json:"strategy_id"`
	TMicro            string                   `json:"t_micro"`
	Interval          string                   `json:"interval"`
	QuoteAsset        string                   `json:"quote_asset"`
	MarketLookback    int                      `json:"market_lookback"`
	CommandTTLSeconds int64                    `json:"command_ttl_seconds"`
	SpendableUSDT     *float64                 `json:"spendable_usdt"`
	Constraints       quant.TradingConstraints `json:"constraints"`
}

type TradeCommand = saasws.TradeCommand

type tickDispatch struct {
	AgentID string
	Command TradeCommand
}

func NewManager(cfg ManagerConfig) *Manager {
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	now := cfg.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Manager{
		db:         cfg.DB,
		marketData: cfg.MarketData,
		hub:        cfg.Hub,
		cache:      cfg.Cache,
		logger:     logger,
		now:        now,
	}
}

func (m *Manager) Create(ctx context.Context, req CreateRequest) (*store.StrategyInstance, error) {
	if err := m.requireDB(); err != nil {
		return nil, err
	}
	if req.UserID == 0 {
		return nil, errors.New("create strategy instance: user_id is required")
	}
	if req.TemplateID == 0 {
		return nil, errors.New("create strategy instance: template_id is required")
	}
	if strings.TrimSpace(req.Symbol) == "" {
		return nil, errors.New("create strategy instance: symbol is required")
	}
	if strings.TrimSpace(req.Interval) == "" {
		return nil, errors.New("create strategy instance: interval is required")
	}

	configJSON, err := normalizeJSON(req.Config)
	if err != nil {
		return nil, fmt.Errorf("create strategy instance: invalid config: %w", err)
	}

	var created store.StrategyInstance
	err = m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var template store.StrategyTemplate
		if err := tx.First(&template, req.TemplateID).Error; err != nil {
			return fmt.Errorf("create strategy instance: load template: %w", err)
		}

		created = store.StrategyInstance{
			UserID:     req.UserID,
			TemplateID: req.TemplateID,
			Name:       strings.TrimSpace(req.Name),
			Symbol:     strings.ToUpper(strings.TrimSpace(req.Symbol)),
			Interval:   strings.TrimSpace(req.Interval),
			Status:     store.StrategyInstanceStopped,
			Config:     configJSON,
		}
		if created.Name == "" {
			created.Name = created.Symbol + "-" + created.Interval
		}
		if err := tx.Create(&created).Error; err != nil {
			return fmt.Errorf("create strategy instance: %w", err)
		}

		portfolio := store.PortfolioState{
			StrategyInstanceID: created.ID,
			USDTBalance:        decimal(req.InitialPortfolio.USDTBalance),
			DeadBTC:            decimal(req.InitialPortfolio.DeadBTC),
			FloatBTC:           decimal(req.InitialPortfolio.FloatBTC),
			ColdSealedBTC:      decimal(req.InitialPortfolio.ColdSealedBTC),
			TotalEquity:        decimal(req.InitialPortfolio.TotalEquity),
		}
		if err := tx.Create(&portfolio).Error; err != nil {
			return fmt.Errorf("create strategy instance portfolio: %w", err)
		}
		runtimeState, err := jsonb(quant.RuntimeState{})
		if err != nil {
			return err
		}
		runtime := store.RuntimeState{
			StrategyInstanceID: created.ID,
			State:              runtimeState,
		}
		if err := tx.Create(&runtime).Error; err != nil {
			return fmt.Errorf("create strategy instance runtime: %w", err)
		}
		if err := createInitialLots(tx, created.ID, req.InitialPortfolio, req.InitialCostPrice); err != nil {
			return err
		}
		if err := writeAudit(tx, auditInstanceCreated, &created.ID, &created.UserID, map[string]any{
			"status":   created.Status,
			"symbol":   created.Symbol,
			"interval": created.Interval,
		}); err != nil {
			return err
		}
		created.Portfolio = &portfolio
		created.RuntimeState = &runtime
		created.Template = &template
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
}

func (m *Manager) Start(ctx context.Context, instanceID uint) error {
	if err := m.transition(ctx, instanceID, store.StrategyInstanceStopped, store.StrategyInstanceRunning, auditInstanceStarted, ""); err != nil {
		return err
	}
	m.active.Store(instanceID, struct{}{})
	return nil
}

func (m *Manager) Stop(ctx context.Context, instanceID uint) error {
	if err := m.transition(ctx, instanceID, store.StrategyInstanceRunning, store.StrategyInstanceStopped, auditInstanceStopped, ""); err != nil {
		return err
	}
	m.active.Delete(instanceID)
	return nil
}

func (m *Manager) Delete(ctx context.Context, instanceID uint) error {
	if err := m.requireDB(); err != nil {
		return err
	}
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var inst store.StrategyInstance
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&inst, instanceID).Error; err != nil {
			return err
		}
		if inst.Status == store.StrategyInstanceDeleted {
			return nil
		}
		if err := tx.Model(&inst).Updates(map[string]any{
			"status":     store.StrategyInstanceDeleted,
			"last_error": "",
		}).Error; err != nil {
			return err
		}
		return writeAudit(tx, auditInstanceDeleted, &inst.ID, &inst.UserID, map[string]any{
			"from": inst.Status,
			"to":   store.StrategyInstanceDeleted,
		})
	})
	if err != nil {
		return err
	}
	m.active.Delete(instanceID)
	return nil
}

func (m *Manager) MarkError(ctx context.Context, instanceID uint, cause error) error {
	if cause == nil {
		cause = errors.New("unknown instance error")
	}
	return m.markError(ctx, instanceID, cause)
}

func (m *Manager) RunningInstances(ctx context.Context) ([]store.StrategyInstance, error) {
	if err := m.requireDB(); err != nil {
		return nil, err
	}
	instances, err := m.loadRunningInstances(ctx)
	if err != nil {
		return nil, err
	}
	if m.isRestored() {
		m.syncActive(instances)
	}
	return instances, nil
}

func (m *Manager) RestoreRunning(ctx context.Context) (int, error) {
	instances, err := m.loadRunningInstances(ctx)
	if err != nil {
		return 0, err
	}
	m.active.Range(func(key any, _ any) bool {
		m.active.Delete(key)
		return true
	})
	for _, inst := range instances {
		m.active.Store(inst.ID, struct{}{})
	}
	m.setRestored()
	return len(instances), nil
}

func (m *Manager) PersistActiveRuntimeSnapshots(ctx context.Context) error {
	if err := m.requireDB(); err != nil {
		return err
	}
	ids := m.activeIDs()
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		var runtime store.RuntimeState
		err := m.db.WithContext(ctx).Where("strategy_instance_id = ?", id).First(&runtime).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			state, stateErr := jsonb(quant.RuntimeState{})
			if stateErr != nil {
				return stateErr
			}
			runtime = store.RuntimeState{
				StrategyInstanceID: id,
				State:              state,
			}
			if createErr := m.db.WithContext(ctx).Create(&runtime).Error; createErr != nil {
				return fmt.Errorf("persist active runtime snapshot %d: %w", id, createErr)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("load active runtime snapshot %d: %w", id, err)
		}
		if _, parseErr := parseRuntime(runtime.State); parseErr != nil {
			return fmt.Errorf("parse active runtime snapshot %d: %w", id, parseErr)
		}
		if updateErr := m.db.WithContext(ctx).Model(&runtime).Update("state", runtime.State).Error; updateErr != nil {
			return fmt.Errorf("persist active runtime snapshot %d: %w", id, updateErr)
		}
	}
	return nil
}

func (m *Manager) Tick(ctx context.Context, instanceID uint) error {
	if err := m.requireTickReady(); err != nil {
		return err
	}
	err := m.tick(ctx, instanceID)
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		if markErr := m.markError(context.Background(), instanceID, err); markErr != nil {
			m.logger.Warn("mark strategy instance error failed", zap.Uint("instance_id", instanceID), zap.Error(markErr))
		}
	}
	return err
}

func (m *Manager) DispatchPending(ctx context.Context, instanceID uint) error {
	if err := m.requireDB(); err != nil {
		return err
	}
	if m.hub == nil {
		return nil
	}

	var inst store.StrategyInstance
	if err := m.db.WithContext(ctx).Preload("Template").First(&inst, instanceID).Error; err != nil {
		return err
	}
	cfg, err := decodeRuntimeConfig(inst.Config)
	if err != nil {
		return err
	}
	var executions []store.SpotExecution
	if err := m.db.WithContext(ctx).
		Where("strategy_instance_id = ? AND status = ?", instanceID, store.SpotExecutionPending).
		Order("created_at ASC").
		Find(&executions).Error; err != nil {
		return err
	}
	if len(executions) == 0 {
		return nil
	}

	agentID := cfg.agentID(inst.UserID)
	if agentID == "" || !m.hub.IsAgentConnected(agentID) {
		return m.auditWarning(ctx, auditAgentDisconnected, inst.ID, inst.UserID, map[string]any{
			"agent_id": agentID,
			"reason":   "pending command dispatch skipped",
		})
	}
	for _, execution := range executions {
		var command TradeCommand
		if err := json.Unmarshal(execution.CommandPayload, &command); err != nil {
			return fmt.Errorf("dispatch pending %s: %w", execution.ClientOrderID, err)
		}
		if err := m.hub.SendTradeCommand(ctx, agentID, command); err != nil {
			m.logger.Warn("dispatch pending trade command failed",
				zap.Uint("instance_id", instanceID),
				zap.String("client_order_id", execution.ClientOrderID),
				zap.Error(err),
			)
			_ = m.auditWarning(ctx, auditCommandDispatchError, inst.ID, inst.UserID, map[string]any{
				"agent_id":        agentID,
				"client_order_id": execution.ClientOrderID,
				"error":           err.Error(),
			})
		}
	}
	return nil
}

func (m *Manager) tick(ctx context.Context, instanceID uint) error {
	inst, cfg, strategyID, err := m.loadInstanceEnvelope(ctx, instanceID)
	if err != nil {
		return err
	}
	if inst.Status != store.StrategyInstanceRunning {
		return nil
	}
	if err := m.DispatchPending(ctx, instanceID); err != nil {
		m.logger.Warn("pending command dispatch scan failed", zap.Uint("instance_id", instanceID), zap.Error(err))
	}

	paramPack, err := m.loadChampionParamPack(ctx, strategyID, inst.Symbol)
	if err != nil {
		return err
	}
	params, err := parseExampleParams(strategyID, paramPack)
	if err != nil {
		return err
	}
	interval := cfg.interval(inst.Interval, params.SpawnPoint.BaseInterval)
	bars, err := m.marketData.LatestClosedBars(ctx, inst.Symbol, interval, cfg.marketLookback())
	if err != nil {
		return fmt.Errorf("load latest closed bars: %w", err)
	}
	bars = normalizeBars(bars)
	if len(bars) == 0 {
		return fmt.Errorf("load latest closed bars: no fully closed bars for %s %s", inst.Symbol, interval)
	}
	latest := bars[len(bars)-1]
	latestTime := time.UnixMilli(latest.OpenTime).UTC()
	closes, timestamps := extractACLSeries(bars)

	var dispatches []tickDispatch
	skipped := false
	err = m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked store.StrategyInstance
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Template").First(&locked, instanceID).Error; err != nil {
			return err
		}
		if locked.Status != store.StrategyInstanceRunning {
			skipped = true
			return nil
		}

		var portfolio store.PortfolioState
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("strategy_instance_id = ?", instanceID).
			First(&portfolio).Error; err != nil {
			return fmt.Errorf("load portfolio state: %w", err)
		}
		if portfolio.LastProcessedBarTime != nil && !latestTime.After(portfolio.LastProcessedBarTime.UTC()) {
			skipped = true
			return nil
		}

		var runtime store.RuntimeState
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("strategy_instance_id = ?", instanceID).
			First(&runtime).Error; err != nil {
			return fmt.Errorf("load runtime state: %w", err)
		}
		runtimeSnapshot, err := parseRuntime(runtime.State)
		if err != nil {
			return err
		}

		var lotRows []store.SpotLot
		if err := tx.Where("strategy_instance_id = ?", instanceID).Order("created_at ASC").Find(&lotRows).Error; err != nil {
			return fmt.Errorf("load spot lots: %w", err)
		}
		spotLots, err := quantLots(lotRows)
		if err != nil {
			return err
		}
		portfolioSnapshot, err := portfolioSnapshot(portfolio)
		if err != nil {
			return err
		}
		constraints := mergeConstraints(params.SpawnPoint, cfg.Constraints)
		totalEquity := totalEquity(portfolioSnapshot, latest.Close)
		spendableUSDT := spendableUSDT(portfolioSnapshot, latest.Close, params.Chromosome)
		if cfg.SpendableUSDT != nil && *cfg.SpendableUSDT >= 0 {
			spendableUSDT = *cfg.SpendableUSDT
		}

		input := quant.StrategyInput{
			Symbol:            locked.Symbol,
			QuoteAsset:        quoteAsset(cfg, params),
			Closes:            closes,
			Timestamps:        timestamps,
			CurrentPrice:      latest.Close,
			Portfolio:         portfolioSnapshot,
			Runtime:           runtimeSnapshot,
			SpotLots:          spotLots,
			Chromosome:        params.Chromosome,
			SpawnPoint:        params.SpawnPoint,
			Constraints:       constraints,
			CurrentBucketTime: latest.OpenTime,
			TotalEquity:       totalEquity,
			SpendableUSDT:     spendableUSDT,
		}
		output := example.Step(input, params)

		nextRuntime, err := jsonb(output.NextRuntime)
		if err != nil {
			return err
		}
		if err := tx.Model(&runtime).Update("state", nextRuntime).Error; err != nil {
			return fmt.Errorf("persist runtime state: %w", err)
		}

		for _, conversion := range output.LedgerConversions {
			applied, err := applyLedgerConversion(tx, &portfolio, conversion)
			if err != nil {
				return err
			}
			if applied <= 0 {
				continue
			}
			if err := writeAudit(tx, auditLedgerConversion, &locked.ID, &locked.UserID, map[string]any{
				"from_lot_type": conversion.FromLotType,
				"to_lot_type":   conversion.ToLotType,
				"requested":     conversion.Amount,
				"applied":       applied,
				"reason_code":   conversion.ReasonCode,
				"bucket_time":   latest.OpenTime,
			}); err != nil {
				return err
			}
		}

		agentID := cfg.agentID(locked.UserID)
		createdAt := m.now()
		commands, err := buildTradeCommands(locked.ID, locked.Symbol, latest.OpenTime, createdAt, cfg.commandTTL(createdAt), output)
		if err != nil {
			return err
		}
		for _, command := range commands {
			payload, err := jsonb(command)
			if err != nil {
				return err
			}
			execution := store.SpotExecution{
				StrategyInstanceID: locked.ID,
				ClientOrderID:      command.ClientOrderID,
				Action:             command.Action,
				Engine:             command.Engine,
				Symbol:             command.Symbol,
				Status:             store.SpotExecutionPending,
				CommandPayload:     payload,
				ExecutionPayload:   store.JSONB([]byte("{}")),
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&execution).Error; err != nil {
				return fmt.Errorf("create pending spot execution: %w", err)
			}
			if m.hub == nil || agentID == "" || !m.hub.IsAgentConnected(agentID) {
				if err := writeAudit(tx, auditAgentDisconnected, &locked.ID, &locked.UserID, map[string]any{
					"agent_id":        agentID,
					"client_order_id": command.ClientOrderID,
					"bucket_time":     latest.OpenTime,
					"reason":          "trade command persisted but not dispatched",
				}); err != nil {
					return err
				}
				continue
			}
			dispatches = append(dispatches, tickDispatch{AgentID: agentID, Command: command})
		}

		if err := tx.Model(&portfolio).Update("last_processed_bar_time", latestTime).Error; err != nil {
			return fmt.Errorf("update last processed bar time: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if skipped {
		return nil
	}

	for _, dispatch := range dispatches {
		if err := m.hub.SendTradeCommand(ctx, dispatch.AgentID, dispatch.Command); err != nil {
			m.logger.Warn("dispatch trade command failed",
				zap.Uint("instance_id", dispatch.Command.InstanceID),
				zap.String("client_order_id", dispatch.Command.ClientOrderID),
				zap.Error(err),
			)
			_ = m.auditWarning(ctx, auditCommandDispatchError, dispatch.Command.InstanceID, inst.UserID, map[string]any{
				"agent_id":        dispatch.AgentID,
				"client_order_id": dispatch.Command.ClientOrderID,
				"error":           err.Error(),
			})
		}
	}
	return nil
}

func (m *Manager) transition(ctx context.Context, instanceID uint, from store.StrategyInstanceStatus, to store.StrategyInstanceStatus, auditType string, lastError string) error {
	if err := m.requireDB(); err != nil {
		return err
	}
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var inst store.StrategyInstance
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&inst, instanceID).Error; err != nil {
			return err
		}
		if inst.Status != from {
			return fmt.Errorf("%w: instance %d is %s, want %s", ErrInvalidTransition, instanceID, inst.Status, from)
		}
		if err := tx.Model(&inst).Updates(map[string]any{
			"status":     to,
			"last_error": lastError,
		}).Error; err != nil {
			return err
		}
		return writeAudit(tx, auditType, &inst.ID, &inst.UserID, map[string]any{
			"from": from,
			"to":   to,
		})
	})
}

func (m *Manager) markError(ctx context.Context, instanceID uint, cause error) error {
	if err := m.requireDB(); err != nil {
		return err
	}
	message := cause.Error()
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var inst store.StrategyInstance
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&inst, instanceID).Error; err != nil {
			return err
		}
		if inst.Status != store.StrategyInstanceRunning {
			return nil
		}
		if err := tx.Model(&inst).Updates(map[string]any{
			"status":     store.StrategyInstanceError,
			"last_error": message,
		}).Error; err != nil {
			return err
		}
		return writeAudit(tx, auditInstanceError, &inst.ID, &inst.UserID, map[string]any{
			"from":  store.StrategyInstanceRunning,
			"to":    store.StrategyInstanceError,
			"error": message,
		})
	})
	if err != nil {
		return err
	}
	m.active.Delete(instanceID)
	return nil
}

func (m *Manager) loadInstanceEnvelope(ctx context.Context, instanceID uint) (store.StrategyInstance, RuntimeConfig, string, error) {
	var inst store.StrategyInstance
	if err := m.db.WithContext(ctx).Preload("Template").First(&inst, instanceID).Error; err != nil {
		return inst, RuntimeConfig{}, "", err
	}
	cfg, err := decodeRuntimeConfig(inst.Config)
	if err != nil {
		return inst, RuntimeConfig{}, "", err
	}
	strategyID := cfg.StrategyID
	if inst.Template != nil {
		if manifestID := strategyIDFromManifest(inst.Template.Manifest); manifestID != "" {
			strategyID = manifestID
		} else if strategyID == "" && inst.Template.Name != "" {
			strategyID = inst.Template.Name
		}
	}
	if strategyID == "" {
		strategyID = example.StrategyID
	}
	return inst, cfg, strategyID, nil
}

func (m *Manager) loadChampionParamPack(ctx context.Context, strategyID string, symbol string) ([]byte, error) {
	key := championCacheKey(strategyID, symbol)
	if m.cache != nil {
		if cached, err := m.cache.Get(ctx, key); err == nil && json.Valid([]byte(cached)) {
			return []byte(cached), nil
		}
	}

	var champion store.GeneRecord
	err := m.db.WithContext(ctx).
		Where("strategy_id = ? AND symbol = ? AND role = ?", strategyID, symbol, store.GeneRoleChampion).
		Order("updated_at DESC").
		First(&champion).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load champion param pack: no champion for %s %s", strategyID, symbol)
	}
	if err != nil {
		return nil, fmt.Errorf("load champion param pack: %w", err)
	}
	if m.cache != nil {
		_ = m.cache.Set(ctx, key, string(champion.ParamPack), championCacheTTL)
	}
	return []byte(champion.ParamPack), nil
}

func (m *Manager) auditWarning(ctx context.Context, eventType string, instanceID uint, userID uint, payload any) error {
	if m.db == nil {
		return nil
	}
	return writeAudit(m.db.WithContext(ctx), eventType, &instanceID, &userID, payload)
}

func (m *Manager) requireDB() error {
	if m == nil || m.db == nil {
		return ErrManagerNotReady
	}
	return nil
}

func (m *Manager) loadRunningInstances(ctx context.Context) ([]store.StrategyInstance, error) {
	if err := m.requireDB(); err != nil {
		return nil, err
	}
	var instances []store.StrategyInstance
	if err := m.db.WithContext(ctx).
		Where("status = ?", store.StrategyInstanceRunning).
		Order("id ASC").
		Find(&instances).Error; err != nil {
		return nil, err
	}
	return instances, nil
}

func (m *Manager) activeIDs() []uint {
	if m == nil {
		return nil
	}
	ids := make([]uint, 0)
	m.active.Range(func(key any, _ any) bool {
		id, ok := key.(uint)
		if ok && id > 0 {
			ids = append(ids, id)
		}
		return true
	})
	sort.Slice(ids, func(i int, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (m *Manager) setRestored() {
	if m == nil {
		return
	}
	m.restoredMu.Lock()
	m.restored = true
	m.restoredMu.Unlock()
}

func (m *Manager) isRestored() bool {
	if m == nil {
		return false
	}
	m.restoredMu.RLock()
	defer m.restoredMu.RUnlock()
	return m.restored
}

func (m *Manager) syncActive(instances []store.StrategyInstance) {
	if m == nil {
		return
	}
	current := make(map[uint]struct{}, len(instances))
	for _, inst := range instances {
		current[inst.ID] = struct{}{}
		m.active.Store(inst.ID, struct{}{})
	}
	m.active.Range(func(key any, _ any) bool {
		id, ok := key.(uint)
		if !ok {
			m.active.Delete(key)
			return true
		}
		if _, ok := current[id]; !ok {
			m.active.Delete(id)
		}
		return true
	})
}

func (m *Manager) requireTickReady() error {
	if err := m.requireDB(); err != nil {
		return err
	}
	if m.marketData == nil {
		return fmt.Errorf("%w: market data client is required", ErrManagerNotReady)
	}
	return nil
}

func decodeRuntimeConfig(raw store.JSONB) (RuntimeConfig, error) {
	var cfg RuntimeConfig
	if len(raw) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("decode instance config: %w", err)
	}
	return cfg, nil
}

func (c RuntimeConfig) interval(instanceInterval string, spawnInterval string) string {
	switch {
	case c.TMicro != "":
		return c.TMicro
	case c.Interval != "":
		return c.Interval
	case instanceInterval != "":
		return instanceInterval
	case spawnInterval != "":
		return spawnInterval
	default:
		return "1h"
	}
}

func (c RuntimeConfig) marketLookback() int {
	if c.MarketLookback > 0 {
		return c.MarketLookback
	}
	return defaultMarketLookback
}

func (c RuntimeConfig) commandTTL(now time.Time) time.Time {
	ttl := defaultCommandTTL
	if c.CommandTTLSeconds > 0 {
		ttl = time.Duration(c.CommandTTLSeconds) * time.Second
	}
	return now.UTC().Add(ttl)
}

func (c RuntimeConfig) agentID(userID uint) string {
	if c.AgentID != "" {
		return c.AgentID
	}
	if userID == 0 {
		return ""
	}
	return fmt.Sprintf("user:%d", userID)
}

func parseExampleParams(strategyID string, raw []byte) (example.Params, error) {
	if strategyID != example.StrategyID {
		return example.Params{}, fmt.Errorf("unsupported strategy %q: only %q is registered", strategyID, example.StrategyID)
	}
	params, err := example.ParseParamPack(raw)
	if err != nil {
		return example.Params{}, fmt.Errorf("parse champion param pack: %w", err)
	}
	return params, nil
}

func strategyIDFromManifest(raw store.JSONB) string {
	if len(raw) == 0 || !json.Valid(raw) {
		return ""
	}
	var manifest struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return ""
	}
	return manifest.ID
}

func championCacheKey(strategyID string, symbol string) string {
	return "champion:" + strategyID + ":" + symbol
}

func normalizeJSON(raw json.RawMessage) (store.JSONB, error) {
	if len(raw) == 0 {
		return store.JSONB([]byte("{}")), nil
	}
	if !json.Valid(raw) {
		return nil, errors.New("config must be valid JSON")
	}
	out := append([]byte(nil), raw...)
	return store.JSONB(out), nil
}

func createInitialLots(tx *gorm.DB, instanceID uint, seed PortfolioSeed, costPrice float64) error {
	lots := []store.SpotLot{}
	if seed.DeadBTC > 0 {
		lots = append(lots, store.SpotLot{
			StrategyInstanceID: instanceID,
			LotType:            store.SpotLotDeadStack,
			Amount:             decimal(seed.DeadBTC),
			CostPrice:          decimal(costPrice),
		})
	}
	if seed.FloatBTC > 0 {
		lots = append(lots, store.SpotLot{
			StrategyInstanceID: instanceID,
			LotType:            store.SpotLotFloating,
			Amount:             decimal(seed.FloatBTC),
			CostPrice:          decimal(costPrice),
		})
	}
	if seed.ColdSealedBTC > 0 {
		lots = append(lots, store.SpotLot{
			StrategyInstanceID: instanceID,
			LotType:            store.SpotLotColdSealed,
			Amount:             decimal(seed.ColdSealedBTC),
			CostPrice:          decimal(costPrice),
			IsColdSealed:       true,
		})
	}
	if len(lots) == 0 {
		return nil
	}
	return tx.Create(&lots).Error
}

func normalizeBars(bars []quant.Bar) []quant.Bar {
	out := make([]quant.Bar, 0, len(bars))
	for _, bar := range bars {
		if bar.OpenTime <= 0 || bar.Close <= 0 {
			continue
		}
		out = append(out, bar)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].OpenTime < out[j].OpenTime
	})
	return out
}

func extractACLSeries(bars []quant.Bar) ([]float64, []int64) {
	closes := make([]float64, 0, len(bars))
	timestamps := make([]int64, 0, len(bars))
	for _, bar := range bars {
		closes = append(closes, bar.Close)
		timestamps = append(timestamps, bar.OpenTime)
	}
	return closes, timestamps
}

func parseRuntime(raw store.JSONB) (quant.RuntimeState, error) {
	var runtime quant.RuntimeState
	if len(raw) == 0 {
		return runtime, nil
	}
	if err := json.Unmarshal(raw, &runtime); err != nil {
		return runtime, fmt.Errorf("parse runtime state: %w", err)
	}
	return runtime, nil
}

func quantLots(rows []store.SpotLot) ([]quant.SpotLot, error) {
	lots := make([]quant.SpotLot, 0, len(rows))
	for _, row := range rows {
		amount, err := decimalFloat(row.Amount)
		if err != nil {
			return nil, fmt.Errorf("parse spot lot amount %d: %w", row.ID, err)
		}
		costPrice, err := decimalFloat(row.CostPrice)
		if err != nil {
			return nil, fmt.Errorf("parse spot lot cost price %d: %w", row.ID, err)
		}
		lots = append(lots, quant.SpotLot{
			LotType:      quant.LotType(row.LotType),
			Amount:       amount,
			CostPrice:    costPrice,
			CreatedAt:    row.CreatedAt,
			IsColdSealed: row.IsColdSealed,
		})
	}
	return lots, nil
}

func portfolioSnapshot(row store.PortfolioState) (quant.PortfolioSnapshot, error) {
	usdt, err := decimalFloat(row.USDTBalance)
	if err != nil {
		return quant.PortfolioSnapshot{}, fmt.Errorf("parse portfolio USDT balance: %w", err)
	}
	dead, err := decimalFloat(row.DeadBTC)
	if err != nil {
		return quant.PortfolioSnapshot{}, fmt.Errorf("parse portfolio DeadBTC: %w", err)
	}
	floatBTC, err := decimalFloat(row.FloatBTC)
	if err != nil {
		return quant.PortfolioSnapshot{}, fmt.Errorf("parse portfolio FloatBTC: %w", err)
	}
	cold, err := decimalFloat(row.ColdSealedBTC)
	if err != nil {
		return quant.PortfolioSnapshot{}, fmt.Errorf("parse portfolio ColdSealedBTC: %w", err)
	}
	return quant.PortfolioSnapshot{
		USDTBalance:   usdt,
		DeadBTC:       dead,
		FloatBTC:      floatBTC,
		ColdSealedBTC: cold,
	}, nil
}

func mergeConstraints(spawn quant.SpawnPoint, override quant.TradingConstraints) quant.TradingConstraints {
	out := spawn.Precision
	if out.MinOrderUSDT <= 0 {
		out.MinOrderUSDT = quant.DefaultMinOrderUSDT
	}
	if out.FeeRate <= 0 {
		out.FeeRate = spawn.Risk.FeeRate
	}
	if out.SlippagePct <= 0 {
		out.SlippagePct = spawn.Risk.SlippagePct
	}
	if override.MinOrderUSDT > 0 {
		out.MinOrderUSDT = override.MinOrderUSDT
	}
	if override.LotStep > 0 {
		out.LotStep = override.LotStep
	}
	if override.LotMin > 0 {
		out.LotMin = override.LotMin
	}
	if override.PriceTick > 0 {
		out.PriceTick = override.PriceTick
	}
	if override.FeeRate > 0 {
		out.FeeRate = override.FeeRate
	}
	if override.SlippagePct > 0 {
		out.SlippagePct = override.SlippagePct
	}
	return out
}

func quoteAsset(cfg RuntimeConfig, params example.Params) string {
	if cfg.QuoteAsset != "" {
		return cfg.QuoteAsset
	}
	if params.SpawnPoint.QuoteAsset != "" {
		return params.SpawnPoint.QuoteAsset
	}
	return "USDT"
}

func totalEquity(portfolio quant.PortfolioSnapshot, price float64) float64 {
	return portfolio.USDTBalance + (portfolio.DeadBTC+portfolio.FloatBTC+portfolio.ColdSealedBTC)*price
}

func spendableUSDT(portfolio quant.PortfolioSnapshot, price float64, chromosome quant.Chromosome) float64 {
	equity := totalEquity(portfolio, price)
	return math.Max(0, portfolio.USDTBalance-equity*chromosome.MicroReservePct)
}

func applyLedgerConversion(tx *gorm.DB, portfolio *store.PortfolioState, conversion quant.LedgerConversion) (float64, error) {
	amount := math.Max(conversion.Amount, 0)
	if amount <= 0 {
		return 0, nil
	}
	from := store.SpotLotType(conversion.FromLotType)
	to := store.SpotLotType(conversion.ToLotType)
	if from == store.SpotLotColdSealed || to == store.SpotLotColdSealed {
		return 0, errors.New("ledger conversion cannot release or create ColdSealedBTC in instance tick")
	}
	if from != store.SpotLotDeadStack || to != store.SpotLotFloating {
		return 0, fmt.Errorf("unsupported ledger conversion %s -> %s", from, to)
	}

	var lots []store.SpotLot
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("strategy_instance_id = ? AND lot_type = ? AND is_cold_sealed = ? AND amount > 0", portfolio.StrategyInstanceID, from, false).
		Order("created_at ASC").
		Find(&lots).Error; err != nil {
		return 0, fmt.Errorf("load releasable lots: %w", err)
	}

	remaining := amount
	applied := 0.0
	for _, lot := range lots {
		if remaining <= 0 {
			break
		}
		lotAmount, err := decimalFloat(lot.Amount)
		if err != nil {
			return 0, fmt.Errorf("parse lot %d amount: %w", lot.ID, err)
		}
		move := math.Min(lotAmount, remaining)
		if move <= 0 {
			continue
		}
		if nearlyEqual(move, lotAmount) {
			if err := tx.Model(&lot).Updates(map[string]any{
				"lot_type":       to,
				"is_cold_sealed": false,
			}).Error; err != nil {
				return 0, fmt.Errorf("move lot %d: %w", lot.ID, err)
			}
		} else {
			if err := tx.Model(&lot).Update("amount", decimal(lotAmount-move)).Error; err != nil {
				return 0, fmt.Errorf("split lot %d: %w", lot.ID, err)
			}
			newLot := store.SpotLot{
				StrategyInstanceID: portfolio.StrategyInstanceID,
				LotType:            to,
				Amount:             decimal(move),
				CostPrice:          lot.CostPrice,
				IsColdSealed:       false,
			}
			if err := tx.Create(&newLot).Error; err != nil {
				return 0, fmt.Errorf("create released lot: %w", err)
			}
		}
		applied += move
		remaining -= move
	}
	if applied <= 0 {
		return 0, nil
	}

	dead, err := decimalFloat(portfolio.DeadBTC)
	if err != nil {
		return 0, err
	}
	floatBTC, err := decimalFloat(portfolio.FloatBTC)
	if err != nil {
		return 0, err
	}
	dead = math.Max(0, dead-applied)
	floatBTC += applied
	portfolio.DeadBTC = decimal(dead)
	portfolio.FloatBTC = decimal(floatBTC)
	if err := tx.Model(portfolio).Updates(map[string]any{
		"dead_btc":  portfolio.DeadBTC,
		"float_btc": portfolio.FloatBTC,
	}).Error; err != nil {
		return 0, fmt.Errorf("update portfolio ledger buckets: %w", err)
	}
	return applied, nil
}

func buildTradeCommands(instanceID uint, symbol string, bucketTime int64, createdAt time.Time, expiresAt time.Time, output quant.StrategyOutput) ([]TradeCommand, error) {
	nowMS := createdAt.UTC().UnixMilli()
	expiresAtMS := expiresAt.UTC().UnixMilli()
	intents := make([]quant.TradeIntent, 0, len(output.MacroIntents)+len(output.MicroIntents))
	intents = append(intents, output.MacroIntents...)
	intents = append(intents, output.MicroIntents...)

	seen := make(map[string]struct{}, len(intents))
	commands := make([]TradeCommand, 0, len(intents))
	for _, intent := range intents {
		if intent.AmountUSDT <= 0 && intent.QtyAsset <= 0 {
			continue
		}
		if intent.Engine == "" {
			return nil, errors.New("trade intent engine is required")
		}
		clientOrderID := fmt.Sprintf("inst%d-%s-%d", instanceID, intent.Engine, bucketTime)
		if _, ok := seen[clientOrderID]; ok {
			return nil, fmt.Errorf("duplicate trade command id %s", clientOrderID)
		}
		seen[clientOrderID] = struct{}{}
		command := TradeCommand{
			ClientOrderID: clientOrderID,
			InstanceID:    instanceID,
			Symbol:        symbol,
			Action:        string(intent.Action),
			Engine:        string(intent.Engine),
			LotType:       string(intent.LotType),
			LimitPrice:    nil,
			CreatedAtMS:   nowMS,
			ExpiresAtMS:   expiresAtMS,
			ReasonCode:    intent.ReasonCode,
		}
		if intent.AmountUSDT > 0 {
			command.AmountUSDT = decimalString(intent.AmountUSDT)
		}
		if intent.QtyAsset > 0 {
			command.QtyAsset = decimalString(intent.QtyAsset)
		}
		commands = append(commands, command)
	}
	return commands, nil
}

func writeAudit(tx *gorm.DB, eventType string, instanceID *uint, userID *uint, payload any) error {
	body, err := jsonb(payload)
	if err != nil {
		return err
	}
	return tx.Create(&store.AuditLog{
		EventType:          eventType,
		Payload:            body,
		UserID:             userID,
		StrategyInstanceID: instanceID,
	}).Error
}

func jsonb(value any) (store.JSONB, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || !json.Valid(raw) {
		return nil, errors.New("jsonb payload is not valid JSON")
	}
	return store.JSONB(raw), nil
}

func decimal(value float64) store.Decimal {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return store.Decimal("0")
	}
	return store.Decimal(decimalString(value))
}

func decimalString(value float64) string {
	if math.Abs(value) < 1e-18 {
		value = 0
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func decimalFloat(value store.Decimal) (float64, error) {
	raw := strings.TrimSpace(string(value))
	if raw == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func nearlyEqual(a float64, b float64) bool {
	return math.Abs(a-b) <= 1e-12
}
