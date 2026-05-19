package ga

import (
	"context"
	"math/rand"

	"bian-trade-go/internal/quant"
)

const FatalFitnessScore = -99999.0

// Gene is intentionally opaque to EvolutionEngine.
type Gene = any

type DCABaseline struct {
	FinalEquity   float64 `json:"final_equity"`
	TotalInjected float64 `json:"total_injected"`
	MaxDrawdown   float64 `json:"max_drawdown"`
}

type EvaluablePlan struct {
	Pair           string                 `json:"pair"`
	TemplateName   string                 `json:"template_name"`
	Spawn          quant.SpawnPoint       `json:"spawn"`
	LotStep        float64                `json:"lot_step"`
	LotMin         float64                `json:"lot_min"`
	Windows        []quant.CrucibleWindow `json:"windows"`
	DCABaselines   []DCABaseline          `json:"dca_baselines"`
	AggregateCache map[string]float64     `json:"aggregate_cache,omitempty"`
}

type FitnessResult struct {
	ScoreTotal  float64                `json:"score_total"`
	MaxDrawdown float64                `json:"max_drawdown"`
	Windows     []quant.CrucibleResult `json:"windows"`
	Fatal       bool                   `json:"fatal"`
	FatalWindow string                 `json:"fatal_window,omitempty"`
	CacheHit    bool                   `json:"cache_hit"`
}

type EvolvableStrategy interface {
	StrategyID() string
	Sample(rng *rand.Rand) Gene
	Mutate(gene Gene, prob float64, scale float64, rng *rand.Rand) Gene
	Crossover(parentA Gene, parentB Gene, rng *rand.Rand) Gene
	Fingerprint(gene Gene) uint64
	Evaluate(ctx context.Context, gene Gene, plan EvaluablePlan) (FitnessResult, error)
	DecodeElite(paramPackJSON []byte) Gene
	EncodeResult(gene Gene, spawnPoint quant.SpawnPoint) ([]byte, error)
}
