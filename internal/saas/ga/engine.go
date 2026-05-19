package ga

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"

	"bian-trade-go/internal/quant"
	"bian-trade-go/internal/saas/store"
	"gorm.io/gorm"
)

type GenomeStore interface {
	LoadChampion(ctx context.Context, strategyID string, symbol string) ([]byte, error)
	LoadElites(ctx context.Context, strategyID string, symbol string, limit int) ([][]byte, error)
	SaveChallenger(ctx context.Context, record ChallengerRecord) (uint, error)
}

type ChallengerRecord struct {
	StrategyID  string
	Symbol      string
	ParamPack   []byte
	ScoreTotal  float64
	MaxDrawdown float64
	ScoreReport []byte
}

type EvolutionEngine struct {
	evolvable   EvolvableStrategy
	genomeStore GenomeStore
	db          *gorm.DB

	PopSize                int
	MaxGenerations         int
	EliteCount             int
	MutationProbability    float64
	MutationScale          float64
	MutationProbabilityMax float64
	MutationScaleMax       float64
	MutationRampFactor     float64
	EarlyStopPatience      int
	EarlyStopMinDelta      float64
	TournamentSize         int
	WarmupDays             int
}

type EpochConfig struct {
	PopSize            int
	MaxGenerations     int
	LotStepSize        float64
	LotMinQty          float64
	OnProgress         func(EpochProgress)
	SpawnPointOverride *quant.SpawnPoint
	Symbol             string
	BaseInterval       string
	Seed               int64
}

type EpochProgress struct {
	Generation          int     `json:"generation"`
	BestScore           float64 `json:"best_score"`
	MutationProbability float64 `json:"mutation_probability"`
	MutationScale       float64 `json:"mutation_scale"`
}

type EpochResult struct {
	BestGeneID   uint                   `json:"best_gene_id"`
	BestScore    float64                `json:"best_score"`
	MaxDrawdown  float64                `json:"max_drawdown"`
	Generations  int                    `json:"generations"`
	StoppedEarly bool                   `json:"stopped_early"`
	StopReason   string                 `json:"stop_reason,omitempty"`
	ParamPack    []byte                 `json:"param_pack,omitempty"`
	Windows      []quant.CrucibleResult `json:"windows"`
}

type gormGenomeStore struct {
	db *gorm.DB
}

type cachedFitness struct {
	score  float64
	result FitnessResult
}

func NewEvolutionEngine(db *gorm.DB, evolvable EvolvableStrategy) *EvolutionEngine {
	engine := &EvolutionEngine{
		evolvable:              evolvable,
		db:                     db,
		PopSize:                300,
		MaxGenerations:         25,
		EliteCount:             8,
		MutationProbability:    0.15,
		MutationScale:          1.0,
		MutationProbabilityMax: 0.55,
		MutationScaleMax:       3.0,
		MutationRampFactor:     1.25,
		EarlyStopPatience:      5,
		EarlyStopMinDelta:      0.001,
		TournamentSize:         3,
		WarmupDays:             1200,
	}
	if db != nil {
		engine.genomeStore = &gormGenomeStore{db: db}
	}
	return engine
}

func (e *EvolutionEngine) SetGenomeStore(genomeStore GenomeStore) {
	e.genomeStore = genomeStore
}

func (e *EvolutionEngine) RunEpoch(ctx context.Context, cfg EpochConfig) (EpochResult, error) {
	if e == nil || e.evolvable == nil {
		return EpochResult{}, errors.New("run epoch: evolvable strategy is required")
	}
	if e.db == nil {
		return EpochResult{}, errors.New("run epoch: database is required")
	}
	if e.genomeStore == nil {
		e.genomeStore = &gormGenomeStore{db: e.db}
	}

	popSize := cfg.PopSize
	if popSize <= 0 {
		popSize = e.PopSize
	}
	maxGenerations := cfg.MaxGenerations
	if maxGenerations <= 0 {
		maxGenerations = e.MaxGenerations
	}
	if popSize < 2 {
		return EpochResult{}, errors.New("run epoch: PopSize must be at least 2")
	}

	seed := cfg.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))

	plan, err := e.buildEvaluablePlan(ctx, cfg)
	if err != nil {
		return EpochResult{}, err
	}
	population, err := e.initializePopulation(ctx, plan.Spawn.Symbol, popSize, rng)
	if err != nil {
		return EpochResult{}, err
	}

	var cache sync.Map
	fitness, results, err := e.evaluatePopulation(ctx, population, plan, &cache)
	if err != nil {
		return EpochResult{}, err
	}

	mutProb := e.MutationProbability
	mutScale := e.MutationScale
	historyBest := math.Inf(-1)
	patienceCount := 0
	generationsRun := 0
	stoppedEarly := false
	stopReason := ""

	for generation := 0; generation < maxGenerations; generation++ {
		if err := ctx.Err(); err != nil {
			return EpochResult{}, err
		}
		sortPopulation(population, fitness, results)
		generationsRun = generation + 1

		currentBest := fitness[0]
		if currentBest-historyBest < e.EarlyStopMinDelta {
			patienceCount++
		} else {
			historyBest = currentBest
			patienceCount = 0
		}

		if patienceCount >= e.EarlyStopPatience {
			mutProb = math.Min(e.MutationProbabilityMax, mutProb*e.MutationRampFactor)
			mutScale = math.Min(e.MutationScaleMax, mutScale*e.MutationRampFactor)
			if mutProb >= e.MutationProbabilityMax && mutScale >= e.MutationScaleMax {
				stoppedEarly = true
				stopReason = "mutation ramp reached max without improvement"
				break
			}
			patienceCount = 0
		}

		if cfg.OnProgress != nil {
			cfg.OnProgress(EpochProgress{
				Generation:          generation,
				BestScore:           currentBest,
				MutationProbability: mutProb,
				MutationScale:       mutScale,
			})
		}

		next := make([]Gene, 0, popSize)
		eliteCount := e.EliteCount
		if eliteCount >= popSize {
			eliteCount = popSize - 1
		}
		for i := 0; i < eliteCount; i++ {
			next = append(next, population[i])
		}
		for len(next) < popSize {
			parentA := e.tournamentSelect(population, fitness, rng)
			parentB := e.tournamentSelect(population, fitness, rng)
			child := e.evolvable.Crossover(parentA, parentB, rng)
			child = e.evolvable.Mutate(child, mutProb, mutScale, rng)
			next = append(next, child)
		}
		population = next
		fitness, results, err = e.evaluatePopulation(ctx, population, plan, &cache)
		if err != nil {
			return EpochResult{}, err
		}
	}

	sortPopulation(population, fitness, results)
	bestGene := population[0]
	bestResult := results[0]
	paramPack, err := e.evolvable.EncodeResult(bestGene, plan.Spawn)
	if err != nil {
		return EpochResult{}, err
	}
	scoreReport, err := json.Marshal(bestResult)
	if err != nil {
		return EpochResult{}, err
	}

	geneID, err := e.genomeStore.SaveChallenger(ctx, ChallengerRecord{
		StrategyID:  e.evolvable.StrategyID(),
		Symbol:      plan.Spawn.Symbol,
		ParamPack:   paramPack,
		ScoreTotal:  bestResult.ScoreTotal,
		MaxDrawdown: bestResult.MaxDrawdown,
		ScoreReport: scoreReport,
	})
	if err != nil {
		return EpochResult{}, err
	}

	return EpochResult{
		BestGeneID:   geneID,
		BestScore:    bestResult.ScoreTotal,
		MaxDrawdown:  bestResult.MaxDrawdown,
		Generations:  generationsRun,
		StoppedEarly: stoppedEarly,
		StopReason:   stopReason,
		ParamPack:    paramPack,
		Windows:      bestResult.Windows,
	}, nil
}

func (e *EvolutionEngine) buildEvaluablePlan(ctx context.Context, cfg EpochConfig) (EvaluablePlan, error) {
	spawn := defaultSpawnPoint(cfg.Symbol, cfg.BaseInterval)
	if cfg.SpawnPointOverride != nil {
		spawn = *cfg.SpawnPointOverride
	}
	spawn = normalizeSpawnPoint(spawn, cfg)

	var rows []store.KLine
	if err := e.db.WithContext(ctx).
		Where("symbol = ? AND interval = ?", spawn.Symbol, spawn.BaseInterval).
		Order("open_time ASC").
		Find(&rows).Error; err != nil {
		return EvaluablePlan{}, fmt.Errorf("load klines: %w", err)
	}
	if len(rows) == 0 {
		return EvaluablePlan{}, fmt.Errorf("load klines: no bars for %s %s", spawn.Symbol, spawn.BaseInterval)
	}

	bars := make([]quant.Bar, 0, len(rows))
	for _, row := range rows {
		bars = append(bars, quant.Bar{
			OpenTime: row.OpenTime.UnixMilli(),
			Open:     decimalFloat(row.Open),
			High:     decimalFloat(row.High),
			Low:      decimalFloat(row.Low),
			Close:    decimalFloat(row.Close),
			Volume:   decimalFloat(row.Volume),
		})
	}

	windows, err := quant.BuildCrucibleWindows(bars, e.WarmupDays)
	if err != nil {
		return EvaluablePlan{}, err
	}

	baselines := make([]DCABaseline, 0, len(windows))
	aggregate := make(map[string]float64, len(windows))
	spawn.EvaluationWindows = make([]quant.EvaluationWindow, 0, len(windows))
	for _, window := range windows {
		evalBars := barsAtOrAfter(window.Bars, window.EvalStartMs)
		if len(evalBars) == 0 {
			return EvaluablePlan{}, fmt.Errorf("build DCA baseline %s: no evaluation bars", window.Label)
		}
		initialCapital := initialCapitalAt(spawn, evalBars[0].Close)
		dca := quant.SimulateGhostDCA(evalBars, quant.GhostDCAConfig{
			InitialCapital: initialCapital,
			MonthlyInject:  spawn.Policy.MonthlyInjectUSDT,
		})
		baselines = append(baselines, DCABaseline{
			FinalEquity:   dca.FinalEquity,
			TotalInjected: dca.TotalInjected,
			MaxDrawdown:   dca.MaxDrawdown,
		})
		aggregate["dca_roi:"+window.Label] = dca.ROI
		spawn.EvaluationWindows = append(spawn.EvaluationWindows, quant.EvaluationWindow{
			Name:        window.Label,
			Weight:      window.Weight,
			WarmupStart: window.Bars[0].OpenTime,
			ScoreStart:  window.EvalStartMs,
			ScoreEnd:    window.Bars[len(window.Bars)-1].OpenTime,
		})
	}

	return EvaluablePlan{
		Pair:           spawn.Symbol,
		TemplateName:   e.evolvable.StrategyID(),
		Spawn:          spawn,
		LotStep:        spawn.Precision.LotStep,
		LotMin:         spawn.Precision.LotMin,
		Windows:        windows,
		DCABaselines:   baselines,
		AggregateCache: aggregate,
	}, nil
}

func (e *EvolutionEngine) initializePopulation(ctx context.Context, symbol string, popSize int, rng *rand.Rand) ([]Gene, error) {
	championRaw, err := e.genomeStore.LoadChampion(ctx, e.evolvable.StrategyID(), symbol)
	if err != nil {
		return nil, err
	}
	seedGene := e.evolvable.DecodeElite(championRaw)

	elitesRaw, err := e.genomeStore.LoadElites(ctx, e.evolvable.StrategyID(), symbol, popSize)
	if err != nil {
		return nil, err
	}
	elites := make([]Gene, 0, len(elitesRaw))
	for _, raw := range elitesRaw {
		elites = append(elites, e.evolvable.DecodeElite(raw))
	}

	population := make([]Gene, 0, popSize)
	population = append(population, seedGene)
	remaining := popSize - 1
	if len(elites) == 0 {
		for i := 0; i < remaining; i++ {
			population = append(population, e.evolvable.Sample(rng))
		}
		return population, nil
	}

	inheritCount := int(math.Round(float64(remaining) * 0.10))
	mutatedCount := int(math.Round(float64(remaining) * 0.40))
	if inheritCount+mutatedCount > remaining {
		mutatedCount = remaining - inheritCount
	}
	randomCount := remaining - inheritCount - mutatedCount
	for i := 0; i < inheritCount; i++ {
		population = append(population, elites[i%len(elites)])
	}
	for i := 0; i < mutatedCount; i++ {
		base := elites[i%len(elites)]
		population = append(population, e.evolvable.Mutate(base, 0.15, 1.5, rng))
	}
	for i := 0; i < randomCount; i++ {
		population = append(population, e.evolvable.Sample(rng))
	}
	return population, nil
}

func (e *EvolutionEngine) evaluatePopulation(ctx context.Context, population []Gene, plan EvaluablePlan, cache *sync.Map) ([]float64, []FitnessResult, error) {
	if len(population) == 0 {
		return nil, nil, errors.New("evaluate population: empty population")
	}
	workers := runtime.NumCPU()
	if workers > len(population) {
		workers = len(population)
	}
	if workers < 1 {
		workers = 1
	}

	fitness := make([]float64, len(population))
	results := make([]FitnessResult, len(population))
	jobs := make(chan int, len(population))
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				if err := ctx.Err(); err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMu.Unlock()
					continue
				}
				gene := population[index]
				fingerprint := e.evolvable.Fingerprint(gene)
				if cached, ok := cache.Load(fingerprint); ok {
					entry := cached.(cachedFitness)
					entry.result.CacheHit = true
					fitness[index] = entry.score
					results[index] = entry.result
					continue
				}

				result, err := e.evolvable.Evaluate(ctx, gene, plan)
				if err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMu.Unlock()
					continue
				}
				fitness[index] = result.ScoreTotal
				results[index] = result
				cache.Store(fingerprint, cachedFitness{score: result.ScoreTotal, result: result})
			}
		}()
	}
	for i := range population {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	if firstErr != nil {
		return nil, nil, firstErr
	}
	return fitness, results, nil
}

func (e *EvolutionEngine) tournamentSelect(population []Gene, fitness []float64, rng *rand.Rand) Gene {
	size := e.TournamentSize
	if size <= 0 {
		size = 3
	}
	if size > len(population) {
		size = len(population)
	}
	seen := make(map[int]struct{}, size)
	bestIndex := -1
	for len(seen) < size {
		index := rng.Intn(len(population))
		if _, ok := seen[index]; ok {
			continue
		}
		seen[index] = struct{}{}
		if bestIndex < 0 || fitness[index] > fitness[bestIndex] {
			bestIndex = index
		}
	}
	return population[bestIndex]
}

func (s *gormGenomeStore) LoadChampion(ctx context.Context, strategyID string, symbol string) ([]byte, error) {
	var record store.GeneRecord
	err := s.db.WithContext(ctx).
		Where("strategy_id = ? AND symbol = ? AND role = ?", strategyID, symbol, store.GeneRoleChampion).
		Order("updated_at DESC").
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load champion: %w", err)
	}
	return []byte(record.ParamPack), nil
}

func (s *gormGenomeStore) LoadElites(ctx context.Context, strategyID string, symbol string, limit int) ([][]byte, error) {
	if limit <= 0 {
		limit = 32
	}
	var records []store.GeneRecord
	if err := s.db.WithContext(ctx).
		Where("strategy_id = ? AND symbol = ? AND role IN ?", strategyID, symbol, []store.GeneRole{store.GeneRoleChampion, store.GeneRoleChallenger, store.GeneRoleRetired}).
		Order("CASE WHEN role = 'champion' THEN 0 ELSE 1 END").
		Order("score_total DESC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("load elites: %w", err)
	}
	out := make([][]byte, 0, len(records))
	for _, record := range records {
		out = append(out, []byte(record.ParamPack))
	}
	return out, nil
}

func (s *gormGenomeStore) SaveChallenger(ctx context.Context, record ChallengerRecord) (uint, error) {
	gene := store.GeneRecord{
		StrategyID:  record.StrategyID,
		Symbol:      record.Symbol,
		Role:        store.GeneRoleChallenger,
		ParamPack:   store.JSONB(record.ParamPack),
		ScoreTotal:  decimal(record.ScoreTotal),
		MaxDrawdown: decimal(record.MaxDrawdown),
		ScoreReport: store.JSONB(record.ScoreReport),
	}
	if err := s.db.WithContext(ctx).Create(&gene).Error; err != nil {
		return 0, fmt.Errorf("save challenger: %w", err)
	}
	return gene.ID, nil
}

func sortPopulation(population []Gene, fitness []float64, results []FitnessResult) {
	indices := make([]int, len(population))
	for i := range indices {
		indices[i] = i
	}
	sort.SliceStable(indices, func(i, j int) bool {
		return fitness[indices[i]] > fitness[indices[j]]
	})
	popCopy := append([]Gene(nil), population...)
	fitCopy := append([]float64(nil), fitness...)
	resultCopy := append([]FitnessResult(nil), results...)
	for i, oldIndex := range indices {
		population[i] = popCopy[oldIndex]
		fitness[i] = fitCopy[oldIndex]
		results[i] = resultCopy[oldIndex]
	}
}

func defaultSpawnPoint(symbol string, interval string) quant.SpawnPoint {
	if symbol == "" {
		symbol = "BTCUSDT"
	}
	if interval == "" {
		interval = "1h"
	}
	return quant.SpawnPoint{
		Symbol:               symbol,
		QuoteAsset:           "USDT",
		BaseInterval:         interval,
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

func normalizeSpawnPoint(spawn quant.SpawnPoint, cfg EpochConfig) quant.SpawnPoint {
	if spawn.Symbol == "" {
		spawn.Symbol = cfg.Symbol
	}
	if spawn.Symbol == "" {
		spawn.Symbol = "BTCUSDT"
	}
	if spawn.QuoteAsset == "" {
		spawn.QuoteAsset = "USDT"
	}
	if spawn.BaseInterval == "" {
		spawn.BaseInterval = cfg.BaseInterval
	}
	if spawn.BaseInterval == "" {
		spawn.BaseInterval = "1h"
	}
	if spawn.Precision.MinOrderUSDT <= 0 {
		spawn.Precision.MinOrderUSDT = quant.DefaultMinOrderUSDT
	}
	if cfg.LotStepSize > 0 {
		spawn.Precision.LotStep = cfg.LotStepSize
	}
	if cfg.LotMinQty > 0 {
		spawn.Precision.LotMin = cfg.LotMinQty
	}
	if spawn.Risk.FeeRate <= 0 {
		spawn.Risk.FeeRate = 0.001
	}
	if spawn.Risk.FatalMaxDrawdownPct <= 0 {
		spawn.Risk.FatalMaxDrawdownPct = 0.88
	}
	return spawn
}

func barsAtOrAfter(bars []quant.Bar, timestamp int64) []quant.Bar {
	for i, bar := range bars {
		if bar.OpenTime >= timestamp {
			return bars[i:]
		}
	}
	return nil
}

func initialCapitalAt(spawn quant.SpawnPoint, price float64) float64 {
	return math.Max(spawn.InitialAvailableUSDT, 0) +
		math.Max(spawn.InitialDeadBTC, 0)*price +
		math.Max(spawn.InitialFloatBTC, 0)*price +
		math.Max(spawn.InitialColdSealedBTC, 0)*price
}

func decimalFloat(value store.Decimal) float64 {
	parsed, err := strconv.ParseFloat(string(value), 64)
	if err != nil {
		return 0
	}
	return parsed
}

func decimal(value float64) store.Decimal {
	return store.Decimal(strconv.FormatFloat(value, 'f', -1, 64))
}
