package ga

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash"
	"hash/fnv"
	"math"
	"math/rand"
	"reflect"
	"strings"

	"bian-trade-go/internal/adapters/backtest"
	"bian-trade-go/internal/quant"
	"bian-trade-go/internal/strategies/example"
)

type ExampleEvolvable struct{}

func NewExampleEvolvable() ExampleEvolvable {
	return ExampleEvolvable{}
}

func (ExampleEvolvable) StrategyID() string {
	return example.StrategyID
}

func (ExampleEvolvable) Sample(rng *rand.Rand) Gene {
	if rng == nil {
		rng = rand.New(rand.NewSource(1))
	}
	var chromosome quant.Chromosome
	value := reflect.ValueOf(&chromosome).Elem()
	typ := value.Type()
	for i := 0; i < value.NumField(); i++ {
		key := chromosomeFieldKey(typ.Field(i))
		bound, ok := quant.HardBounds[key]
		if !ok {
			continue
		}
		field := value.Field(i)
		if bound.IsInt {
			minValue := int(bound.Min)
			maxValue := int(bound.Max)
			field.SetInt(int64(minValue + rng.Intn(maxValue-minValue+1)))
			continue
		}
		field.SetFloat(bound.Min + rng.Float64()*(bound.Max-bound.Min))
	}
	return quant.ClampChromosome(chromosome)
}

func (ExampleEvolvable) Mutate(gene Gene, prob float64, scale float64, rng *rand.Rand) Gene {
	if rng == nil {
		rng = rand.New(rand.NewSource(1))
	}
	prob = quant.ClipFloat64(prob, 0, 1)
	if scale < 0 {
		scale = 0
	}
	chromosome := chromosomeFromGene(gene)
	value := reflect.ValueOf(&chromosome).Elem()
	typ := value.Type()
	for i := 0; i < value.NumField(); i++ {
		if rng.Float64() >= prob {
			continue
		}
		key := chromosomeFieldKey(typ.Field(i))
		bound, ok := quant.HardBounds[key]
		if !ok {
			continue
		}
		field := value.Field(i)
		step := geneStep(bound)
		if bound.IsInt {
			next := float64(field.Int()) + rng.NormFloat64()*step*scale
			field.SetInt(int64(math.Round(next)))
			continue
		}
		field.SetFloat(field.Float() + rng.NormFloat64()*step*scale)
	}
	return quant.ClampChromosome(chromosome)
}

func (ExampleEvolvable) Crossover(parentA Gene, parentB Gene, rng *rand.Rand) Gene {
	if rng == nil {
		rng = rand.New(rand.NewSource(1))
	}
	a := chromosomeFromGene(parentA)
	b := chromosomeFromGene(parentB)
	child := a

	childValue := reflect.ValueOf(&child).Elem()
	bValue := reflect.ValueOf(b)
	for i := 0; i < childValue.NumField(); i++ {
		if rng.Float64() < 0.5 {
			childValue.Field(i).Set(bValue.Field(i))
		}
	}
	return quant.ClampChromosome(child)
}

func (ExampleEvolvable) Fingerprint(gene Gene) uint64 {
	chromosome := chromosomeFromGene(gene)
	h := fnv.New64a()
	value := reflect.ValueOf(chromosome)
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		switch field.Kind() {
		case reflect.Int:
			writeInt64(h, field.Int())
		case reflect.Float64:
			writeInt64(h, int64(math.Round(field.Float()*1_000_000)))
		}
	}
	return h.Sum64()
}

func (ExampleEvolvable) Evaluate(ctx context.Context, gene Gene, plan EvaluablePlan) (FitnessResult, error) {
	chromosome := chromosomeFromGene(gene)
	if len(plan.Windows) == 0 {
		return FitnessResult{}, fmt.Errorf("evaluate %s: no crucible windows", example.StrategyID)
	}
	if len(plan.DCABaselines) != len(plan.Windows) {
		return FitnessResult{}, fmt.Errorf("evaluate %s: DCA baseline count mismatch", example.StrategyID)
	}

	params := example.Params{
		Chromosome: chromosome,
		SpawnPoint: plan.Spawn,
	}
	fatalMaxDrawdown := plan.Spawn.Risk.FatalMaxDrawdownPct
	if fatalMaxDrawdown <= 0 {
		fatalMaxDrawdown = 0.88
	}

	results := make([]quant.CrucibleResult, 0, len(plan.Windows))
	scoreTotal := 0.0
	maxDrawdown := 0.0
	for i, window := range plan.Windows {
		backtestResult, err := backtest.RunBacktest(ctx, backtest.Config{
			Bars:        window.Bars,
			EvalStartMs: window.EvalStartMs,
			Chromosome:  chromosome,
			SpawnPoint:  plan.Spawn,
			Constraints: quant.TradingConstraints{
				LotStep: plan.LotStep,
				LotMin:  plan.LotMin,
			},
			Step: func(input quant.StrategyInput) quant.StrategyOutput {
				return example.Step(input, params)
			},
		})
		if err != nil {
			return FitnessResult{}, err
		}

		baseline := plan.DCABaselines[i]
		dcaROI := dcaROIForWindow(plan, window.Label, baseline)
		alpha := backtestResult.ROI - dcaROI
		score := alpha - 1.5*math.Max(0, backtestResult.MaxDrawdown-baseline.MaxDrawdown)
		if backtestResult.MaxDrawdown >= fatalMaxDrawdown {
			score = FatalFitnessScore
		}
		result := quant.CrucibleResult{
			Window: window.Label,
			Score:  score,
			ROI:    backtestResult.ROI,
			MaxDD:  backtestResult.MaxDrawdown,
			Alpha:  alpha,
		}
		results = append(results, result)
		if backtestResult.MaxDrawdown > maxDrawdown {
			maxDrawdown = backtestResult.MaxDrawdown
		}
		if score == FatalFitnessScore {
			return FitnessResult{
				ScoreTotal:  FatalFitnessScore,
				MaxDrawdown: maxDrawdown,
				Windows:     results,
				Fatal:       true,
				FatalWindow: window.Label,
			}, nil
		}
		scoreTotal += window.Weight * score
	}

	return FitnessResult{
		ScoreTotal:  scoreTotal,
		MaxDrawdown: maxDrawdown,
		Windows:     results,
	}, nil
}

func (ExampleEvolvable) DecodeElite(paramPackJSON []byte) Gene {
	if len(paramPackJSON) == 0 {
		return quant.DefaultSeedChromosome
	}
	params, err := example.ParseParamPack(paramPackJSON)
	if err != nil {
		return quant.DefaultSeedChromosome
	}
	return quant.ClampChromosome(params.Chromosome)
}

func (ExampleEvolvable) EncodeResult(gene Gene, spawnPoint quant.SpawnPoint) ([]byte, error) {
	params := example.DefaultParams()
	params.Chromosome = chromosomeFromGene(gene)
	params.SpawnPoint = spawnPoint
	return json.Marshal(params)
}

func chromosomeFromGene(gene Gene) quant.Chromosome {
	switch value := gene.(type) {
	case quant.Chromosome:
		return quant.ClampChromosome(value)
	case *quant.Chromosome:
		if value == nil {
			return quant.DefaultSeedChromosome
		}
		return quant.ClampChromosome(*value)
	default:
		return quant.DefaultSeedChromosome
	}
}

func chromosomeFieldKey(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" {
		return field.Name
	}
	key, _, _ := strings.Cut(tag, ",")
	return key
}

func geneStep(bound quant.HardBound) float64 {
	if bound.IsInt {
		return 1
	}
	step := (bound.Max - bound.Min) / 100
	if step <= 0 {
		return 1e-6
	}
	return step
}

func dcaROIForWindow(plan EvaluablePlan, label string, baseline DCABaseline) float64 {
	if plan.AggregateCache != nil {
		if roi, ok := plan.AggregateCache["dca_roi:"+label]; ok {
			return roi
		}
	}
	if baseline.TotalInjected <= 0 {
		return 0
	}
	return (baseline.FinalEquity - baseline.TotalInjected) / baseline.TotalInjected
}

func writeInt64(h hash.Hash64, value int64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(value))
	_, _ = h.Write(buf[:])
}
