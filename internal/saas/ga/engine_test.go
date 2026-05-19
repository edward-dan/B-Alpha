package ga

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"hash/fnv"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"bian-trade-go/internal/quant"
	"bian-trade-go/internal/saas/store"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEvolutionEngineRetainsPreviousGenerationElites(t *testing.T) {
	evolvable := newRecordingEvolvable()
	evolvable.sampleScores = []float64{70, 60, 50, 40, 30, 20, 10}
	engine := NewEvolutionEngine(newEpochTestDB(t), evolvable)
	engine.SetGenomeStore(&fakeGenomeStore{})
	engine.EliteCount = 3
	engine.WarmupDays = 0

	_, err := engine.RunEpoch(context.Background(), EpochConfig{
		PopSize:        8,
		MaxGenerations: 1,
		Symbol:         "BTCUSDT",
		BaseInterval:   "1d",
		Seed:           11,
	})
	if err != nil {
		t.Fatalf("run epoch: %v", err)
	}

	fingerprinted := evolvable.fingerprintedGenes()
	if len(fingerprinted) < 16 {
		t.Fatalf("expected two evaluated populations, got %d fingerprint calls: %#v", len(fingerprinted), fingerprinted)
	}
	initial := append([]testGene(nil), fingerprinted[:8]...)
	next := fingerprinted[8:16]
	sort.SliceStable(initial, func(i, j int) bool {
		return initial[i].Score > initial[j].Score
	})

	nextIDs := make(map[string]struct{}, len(next))
	for _, gene := range next {
		nextIDs[gene.ID] = struct{}{}
	}
	for _, elite := range initial[:engine.EliteCount] {
		if _, ok := nextIDs[elite.ID]; !ok {
			t.Fatalf("previous top elite %s was not retained in next population: initial=%#v next=%#v", elite.ID, initial, next)
		}
	}
}

func TestEvolutionEngineMutationRampAfterPatienceWithoutImprovement(t *testing.T) {
	evolvable := newRecordingEvolvable()
	evolvable.constantScore = true
	engine := NewEvolutionEngine(newEpochTestDB(t), evolvable)
	engine.SetGenomeStore(&fakeGenomeStore{})
	engine.EliteCount = 1
	engine.MutationProbability = 0.20
	engine.MutationScale = 1.0
	engine.MutationRampFactor = 1.5
	engine.MutationProbabilityMax = 1.0
	engine.MutationScaleMax = 10.0
	engine.EarlyStopPatience = 2
	engine.EarlyStopMinDelta = 0.001
	engine.WarmupDays = 0

	var progress []EpochProgress
	_, err := engine.RunEpoch(context.Background(), EpochConfig{
		PopSize:        6,
		MaxGenerations: 3,
		Symbol:         "BTCUSDT",
		BaseInterval:   "1d",
		Seed:           17,
		OnProgress: func(update EpochProgress) {
			progress = append(progress, update)
		},
	})
	if err != nil {
		t.Fatalf("run epoch: %v", err)
	}
	if len(progress) != 3 {
		t.Fatalf("expected 3 progress updates, got %#v", progress)
	}
	want := engine.MutationProbability * engine.MutationRampFactor
	if math.Abs(progress[2].MutationProbability-want) > 1e-12 {
		t.Fatalf("mutation probability after ramp = %.12f, want %.12f; progress=%#v", progress[2].MutationProbability, want, progress)
	}
}

func TestTournamentSelectAlmostNeverSelectsFatalFitness(t *testing.T) {
	population := make([]Gene, 100)
	fitness := make([]float64, len(population))
	population[0] = testGene{ID: "fatal"}
	fitness[0] = FatalFitnessScore
	for i := 1; i < len(population); i++ {
		population[i] = testGene{ID: "ok"}
		fitness[i] = 1
	}

	engine := &EvolutionEngine{TournamentSize: 3}
	rng := rand.New(rand.NewSource(99))
	selectedFatal := 0
	for i := 0; i < 1000; i++ {
		selected := engine.tournamentSelect(population, fitness, rng).(testGene)
		if selected.ID == "fatal" {
			selectedFatal++
		}
	}
	if selectedFatal >= 50 {
		t.Fatalf("fatal gene selected %d times out of 1000", selectedFatal)
	}
}

type testGene struct {
	ID    string
	Score float64
}

type recordingEvolvable struct {
	mu            sync.Mutex
	sampleScores  []float64
	samples       int
	children      int
	fingerprinted []testGene
	constantScore bool
}

func newRecordingEvolvable() *recordingEvolvable {
	return &recordingEvolvable{
		sampleScores: []float64{5, 4, 3, 2, 1},
	}
}

func (e *recordingEvolvable) StrategyID() string {
	return "test-strategy"
}

func (e *recordingEvolvable) Sample(*rand.Rand) Gene {
	e.mu.Lock()
	defer e.mu.Unlock()
	score := float64(len(e.sampleScores) - e.samples)
	if e.samples < len(e.sampleScores) {
		score = e.sampleScores[e.samples]
	}
	gene := testGene{ID: "sample-" + strconvItoa(e.samples), Score: score}
	e.samples++
	return gene
}

func (e *recordingEvolvable) Mutate(gene Gene, _ float64, _ float64, _ *rand.Rand) Gene {
	return gene
}

func (e *recordingEvolvable) Crossover(parentA Gene, parentB Gene, _ *rand.Rand) Gene {
	a := parentA.(testGene)
	b := parentB.(testGene)
	e.mu.Lock()
	defer e.mu.Unlock()
	child := testGene{ID: "child-" + strconvItoa(e.children) + "-" + a.ID + "-" + b.ID, Score: -100}
	e.children++
	return child
}

func (e *recordingEvolvable) Fingerprint(gene Gene) uint64 {
	typed := gene.(testGene)
	e.mu.Lock()
	e.fingerprinted = append(e.fingerprinted, typed)
	e.mu.Unlock()

	h := fnv.New64a()
	_, _ = h.Write([]byte(typed.ID))
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], math.Float64bits(typed.Score))
	_, _ = h.Write(buf[:])
	return h.Sum64()
}

func (e *recordingEvolvable) Evaluate(_ context.Context, gene Gene, _ EvaluablePlan) (FitnessResult, error) {
	score := gene.(testGene).Score
	if e.constantScore {
		score = 1
	}
	return FitnessResult{ScoreTotal: score}, nil
}

func (e *recordingEvolvable) DecodeElite(raw []byte) Gene {
	if len(raw) == 0 {
		return testGene{ID: "seed", Score: 0}
	}
	var gene testGene
	if err := json.Unmarshal(raw, &gene); err != nil {
		return testGene{ID: "seed", Score: 0}
	}
	return gene
}

func (e *recordingEvolvable) EncodeResult(gene Gene, _ quant.SpawnPoint) ([]byte, error) {
	return json.Marshal(gene)
}

func (e *recordingEvolvable) fingerprintedGenes() []testGene {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]testGene(nil), e.fingerprinted...)
}

type fakeGenomeStore struct {
	champion []byte
	elites   [][]byte
}

func (s *fakeGenomeStore) LoadChampion(context.Context, string, string) ([]byte, error) {
	return s.champion, nil
}

func (s *fakeGenomeStore) LoadElites(context.Context, string, string, int) ([][]byte, error) {
	return s.elites, nil
}

func (s *fakeGenomeStore) SaveChallenger(context.Context, ChallengerRecord) (uint, error) {
	return 1, nil
}

func newEpochTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("unwrap sqlite test db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	if err := db.AutoMigrate(&store.KLine{}); err != nil {
		t.Fatalf("migrate kline: %v", err)
	}
	start := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	rows := make([]store.KLine, 1900)
	for i := range rows {
		ts := start.AddDate(0, 0, i)
		price := 100 + float64(i)*0.01 + 2*math.Sin(float64(i)/31)
		rows[i] = store.KLine{
			CreatedAt: ts,
			UpdatedAt: ts,
			Symbol:    "BTCUSDT",
			Interval:  "1d",
			OpenTime:  ts,
			Open:      store.Decimal(strconvFormat(price * 0.999)),
			High:      store.Decimal(strconvFormat(price * 1.002)),
			Low:       store.Decimal(strconvFormat(price * 0.998)),
			Close:     store.Decimal(strconvFormat(price)),
			Volume:    store.Decimal("100"),
		}
	}
	if err := db.CreateInBatches(rows, 250).Error; err != nil {
		t.Fatalf("seed kline rows: %v", err)
	}
	return db
}

func strconvItoa(value int) string {
	return strconv.FormatInt(int64(value), 10)
}

func strconvFormat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
