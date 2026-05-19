package epoch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"bian-trade-go/internal/quant"
	"bian-trade-go/internal/saas/ga"
	"bian-trade-go/internal/saas/store"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type EpochService struct {
	db          *gorm.DB
	engine      *ga.EvolutionEngine
	logger      *zap.Logger
	mu          sync.Mutex
	currentTask *store.EvolutionTask
	currentStop context.CancelFunc
}

type CreateTaskRequest struct {
	StrategyID     string            `json:"strategy_id"`
	Symbol         string            `json:"symbol"`
	PopSize        int               `json:"pop_size"`
	MaxGenerations int               `json:"max_generations"`
	SpawnMode      string            `json:"spawn_mode"`
	SpawnPoint     *quant.SpawnPoint `json:"spawn_point"`
	TestMode       bool              `json:"test_mode"`
	Seed           *int64            `json:"seed"`
	Notes          string            `json:"notes"`
}

type ChallengerSummary struct {
	ID          uint            `json:"id"`
	CreatedAt   time.Time       `json:"created_at"`
	StrategyID  string          `json:"strategy_id"`
	Symbol      string          `json:"symbol"`
	Role        store.GeneRole  `json:"role"`
	ScoreTotal  store.Decimal   `json:"score_total"`
	MaxDrawdown store.Decimal   `json:"max_drawdown"`
	ScoreReport json.RawMessage `json:"score_report"`
}

type TaskStatus struct {
	CurrentTask *store.EvolutionTask `json:"current_task"`
	Challengers []ChallengerSummary  `json:"challengers"`
}

func NewEpochService(db *gorm.DB, engine *ga.EvolutionEngine, logger *zap.Logger) *EpochService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &EpochService{
		db:     db,
		engine: engine,
		logger: logger,
	}
}

func (s *EpochService) CreateAndRunTask(ctx context.Context, req CreateTaskRequest) (*store.EvolutionTask, error) {
	if s == nil || s.db == nil || s.engine == nil {
		return nil, errors.New("create evolution task: service is not initialized")
	}
	if req.StrategyID == "" {
		return nil, errors.New("create evolution task: strategy_id is required")
	}
	if req.Symbol == "" {
		return nil, errors.New("create evolution task: symbol is required")
	}

	s.mu.Lock()
	if s.currentTask != nil && s.currentTask.Status == store.EvolutionTaskRunning {
		s.mu.Unlock()
		return nil, errors.New("create evolution task: another evolution task is running")
	}
	spawn, err := s.resolveSpawnPoint(ctx, req)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	popSize, maxGenerations := normalizeEpochSize(req)
	seed := int64(0)
	if req.Seed != nil {
		seed = *req.Seed
	}

	configBlob, err := json.Marshal(map[string]any{
		"strategy_id":     req.StrategyID,
		"symbol":          req.Symbol,
		"pop_size":        popSize,
		"max_generations": maxGenerations,
		"spawn_mode":      spawnModeOrDefault(req.SpawnMode),
		"spawn_point":     spawn,
		"test_mode":       req.TestMode,
		"seed":            seed,
		"notes":           req.Notes,
	})
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}

	now := time.Now().UTC()
	task := store.EvolutionTask{
		StrategyID:        req.StrategyID,
		Symbol:            req.Symbol,
		Status:            store.EvolutionTaskRunning,
		Progress:          0,
		Config:            store.JSONB(configBlob),
		CurrentGeneration: 0,
		StartedAt:         &now,
	}
	if err := s.db.WithContext(ctx).Create(&task).Error; err != nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("create evolution task: %w", err)
	}

	runCtx, stop := context.WithCancel(context.Background())
	s.currentTask = &task
	s.currentStop = stop
	s.mu.Unlock()

	go s.runEpoch(runCtx, task.ID, ga.EpochConfig{
		PopSize:            popSize,
		MaxGenerations:     maxGenerations,
		LotStepSize:        spawn.Precision.LotStep,
		LotMinQty:          spawn.Precision.LotMin,
		SpawnPointOverride: &spawn,
		Symbol:             req.Symbol,
		BaseInterval:       spawn.BaseInterval,
		Seed:               seed,
		OnProgress: func(progress ga.EpochProgress) {
			percent := int(float64(progress.Generation+1) / float64(maxGenerations) * 100)
			if percent > 99 {
				percent = 99
			}
			s.mu.Lock()
			if s.currentTask != nil && s.currentTask.ID == task.ID {
				s.currentTask.CurrentGeneration = progress.Generation
				s.currentTask.BestScore = decimal(progress.BestScore)
				s.currentTask.Progress = percent
			}
			s.mu.Unlock()
			updates := map[string]any{
				"current_generation": progress.Generation,
				"best_score":         decimal(progress.BestScore),
				"progress":           percent,
			}
			if err := s.db.WithContext(context.Background()).Model(&store.EvolutionTask{}).Where("id = ?", task.ID).Updates(updates).Error; err != nil {
				s.logger.Warn("update evolution progress failed", zap.Uint("task_id", task.ID), zap.Error(err))
			}
		},
	})

	return &task, nil
}

func (s *EpochService) CancelTask(ctx context.Context, taskID uint) (*store.EvolutionTask, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("cancel evolution task: service is not initialized")
	}
	finishedAt := time.Now().UTC()
	s.mu.Lock()
	if s.currentTask != nil && s.currentTask.ID == taskID {
		if s.currentStop != nil {
			s.currentStop()
		}
		s.currentTask.Status = store.EvolutionTaskCanceled
		s.currentTask.FinishedAt = &finishedAt
		s.currentTask = nil
		s.currentStop = nil
	}
	s.mu.Unlock()

	err := s.db.WithContext(ctx).
		Model(&store.EvolutionTask{}).
		Where("id = ? AND status = ?", taskID, store.EvolutionTaskRunning).
		Updates(map[string]any{
			"status":      store.EvolutionTaskCanceled,
			"finished_at": &finishedAt,
		}).Error
	if err != nil {
		return nil, err
	}
	var task store.EvolutionTask
	if err := s.db.WithContext(ctx).First(&task, taskID).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func (s *EpochService) Status(ctx context.Context) (TaskStatus, error) {
	var current *store.EvolutionTask
	s.mu.Lock()
	if s.currentTask != nil {
		copyTask := *s.currentTask
		current = &copyTask
	}
	s.mu.Unlock()

	var records []store.GeneRecord
	if err := s.db.WithContext(ctx).
		Where("role = ?", store.GeneRoleChallenger).
		Order("created_at DESC").
		Limit(50).
		Find(&records).Error; err != nil {
		return TaskStatus{}, err
	}
	challengers := make([]ChallengerSummary, 0, len(records))
	for _, record := range records {
		challengers = append(challengers, ChallengerSummary{
			ID:          record.ID,
			CreatedAt:   record.CreatedAt,
			StrategyID:  record.StrategyID,
			Symbol:      record.Symbol,
			Role:        record.Role,
			ScoreTotal:  record.ScoreTotal,
			MaxDrawdown: record.MaxDrawdown,
			ScoreReport: json.RawMessage(record.ScoreReport),
		})
	}
	return TaskStatus{CurrentTask: current, Challengers: challengers}, nil
}

func (s *EpochService) runEpoch(ctx context.Context, taskID uint, cfg ga.EpochConfig) {
	defer func() {
		s.mu.Lock()
		if s.currentTask != nil && s.currentTask.ID == taskID {
			s.currentTask = nil
			s.currentStop = nil
		}
		s.mu.Unlock()
	}()

	result, err := s.engine.RunEpoch(ctx, cfg)
	finishedAt := time.Now().UTC()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			_ = s.db.WithContext(context.Background()).
				Model(&store.EvolutionTask{}).
				Where("id = ? AND status = ?", taskID, store.EvolutionTaskRunning).
				Updates(map[string]any{
					"status":      store.EvolutionTaskCanceled,
					"finished_at": &finishedAt,
				}).Error
			return
		}
		s.logger.Error("evolution epoch failed", zap.Uint("task_id", taskID), zap.Error(err))
		_ = s.db.WithContext(context.Background()).Model(&store.EvolutionTask{}).
			Where("id = ? AND status = ?", taskID, store.EvolutionTaskRunning).
			Updates(map[string]any{
				"status":      store.EvolutionTaskFailed,
				"error":       err.Error(),
				"finished_at": &finishedAt,
			}).Error
		return
	}

	updates := map[string]any{
		"status":             store.EvolutionTaskSucceeded,
		"progress":           100,
		"current_generation": result.Generations,
		"best_score":         decimal(result.BestScore),
		"best_gene_id":       strconv.FormatUint(uint64(result.BestGeneID), 10),
		"finished_at":        &finishedAt,
	}
	if err := s.db.WithContext(context.Background()).Model(&store.EvolutionTask{}).
		Where("id = ? AND status = ?", taskID, store.EvolutionTaskRunning).
		Updates(updates).Error; err != nil {
		s.logger.Warn("mark evolution task succeeded failed", zap.Uint("task_id", taskID), zap.Error(err))
	}
}

func (s *EpochService) resolveSpawnPoint(ctx context.Context, req CreateTaskRequest) (quant.SpawnPoint, error) {
	switch spawnModeOrDefault(req.SpawnMode) {
	case "inherit":
		return s.inheritSpawnPoint(ctx, req.StrategyID, req.Symbol)
	case "random_once":
		return RandomSpawnPoint(req.Symbol), nil
	case "manual":
		if req.SpawnPoint == nil {
			return quant.SpawnPoint{}, errors.New("manual spawn_mode requires spawn_point")
		}
		spawn := *req.SpawnPoint
		return normalizeSpawn(spawn, req.Symbol), nil
	default:
		return quant.SpawnPoint{}, fmt.Errorf("unsupported spawn_mode %q", req.SpawnMode)
	}
}

func (s *EpochService) inheritSpawnPoint(ctx context.Context, strategyID string, symbol string) (quant.SpawnPoint, error) {
	var record store.GeneRecord
	err := s.db.WithContext(ctx).
		Where("strategy_id = ? AND symbol = ? AND role = ?", strategyID, symbol, store.GeneRoleChampion).
		Order("updated_at DESC").
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DefaultSpawnPoint(symbol), nil
	}
	if err != nil {
		return quant.SpawnPoint{}, err
	}

	var decoded struct {
		SpawnPoint quant.SpawnPoint `json:"spawn_point"`
	}
	if err := json.Unmarshal(record.ParamPack, &decoded); err != nil {
		return DefaultSpawnPoint(symbol), nil
	}
	return normalizeSpawn(decoded.SpawnPoint, symbol), nil
}

func DefaultSpawnPoint(symbol string) quant.SpawnPoint {
	if symbol == "" {
		symbol = "BTCUSDT"
	}
	return quant.SpawnPoint{
		Symbol:               symbol,
		QuoteAsset:           "USDT",
		BaseInterval:         "1h",
		InitialAvailableUSDT: 10000,
		Policy: quant.SpawnPolicy{
			MonthlyInjectUSDT:    0,
			DeadlineSpendablePct: 0.50,
			SoftReleaseMaxPct:    quant.DefaultSeedChromosome.ReleaseMaxPctPerStep,
		},
		Risk: quant.SpawnRisk{
			FeeRate:             0.001,
			FatalMaxDrawdownPct: 0.88,
		},
		Precision: quant.TradingConstraints{
			MinOrderUSDT: quant.DefaultMinOrderUSDT,
		},
	}
}

func RandomSpawnPoint(symbol string) quant.SpawnPoint {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	spawn := DefaultSpawnPoint(symbol)
	spawn.InitialAvailableUSDT = 5000 + rng.Float64()*45000
	spawn.Policy.MonthlyInjectUSDT = rng.Float64() * 1000
	spawn.Risk.SlippagePct = rng.Float64() * 0.001
	return spawn
}

func normalizeEpochSize(req CreateTaskRequest) (int, int) {
	if req.TestMode {
		return 10, 3
	}
	popSize := req.PopSize
	if popSize <= 0 {
		popSize = 300
	}
	if popSize < 10 {
		popSize = 10
	}
	if popSize > 500 {
		popSize = 500
	}
	maxGenerations := req.MaxGenerations
	if maxGenerations <= 0 {
		maxGenerations = 25
	}
	if maxGenerations < 5 {
		maxGenerations = 5
	}
	if maxGenerations > 50 {
		maxGenerations = 50
	}
	return popSize, maxGenerations
}

func spawnModeOrDefault(mode string) string {
	if mode == "" {
		return "inherit"
	}
	return mode
}

func normalizeSpawn(spawn quant.SpawnPoint, symbol string) quant.SpawnPoint {
	if spawn.Symbol == "" {
		spawn.Symbol = symbol
	}
	if spawn.Symbol == "" {
		spawn.Symbol = "BTCUSDT"
	}
	if spawn.QuoteAsset == "" {
		spawn.QuoteAsset = "USDT"
	}
	if spawn.BaseInterval == "" {
		spawn.BaseInterval = "1h"
	}
	if spawn.Precision.MinOrderUSDT <= 0 {
		spawn.Precision.MinOrderUSDT = quant.DefaultMinOrderUSDT
	}
	if spawn.Risk.FeeRate <= 0 {
		spawn.Risk.FeeRate = 0.001
	}
	if spawn.Risk.FatalMaxDrawdownPct <= 0 {
		spawn.Risk.FatalMaxDrawdownPct = 0.88
	}
	return spawn
}

func decimal(value float64) store.Decimal {
	return store.Decimal(strconv.FormatFloat(value, 'f', -1, 64))
}
