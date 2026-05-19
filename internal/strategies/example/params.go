package example

import (
	"encoding/json"
	"reflect"

	"bian-trade-go/internal/quant"
)

type Params struct {
	Chromosome quant.Chromosome `json:"chromosome"`
	SpawnPoint quant.SpawnPoint `json:"spawn_point"`
}

func DefaultParams() Params {
	return Params{
		Chromosome: quant.DefaultSeedChromosome,
		SpawnPoint: quant.SpawnPoint{
			Symbol:               "BTCUSDT",
			QuoteAsset:           "USDT",
			BaseInterval:         "1h",
			InitialAvailableUSDT: 10000,
			Policy: quant.SpawnPolicy{
				MonthlyInjectUSDT:    0,
				DeadlineSpendablePct: 0.50,
				SoftReleaseMaxPct:    quant.DefaultSeedChromosome.ReleaseMaxPctPerStep,
				DeadBTCFloor:         0,
			},
			Risk: quant.SpawnRisk{
				FeeRate:             0.001,
				FatalMaxDrawdownPct: 0.88,
			},
			Precision: quant.TradingConstraints{
				MinOrderUSDT: quant.DefaultMinOrderUSDT,
			},
		},
	}
}

func ParseParamPack(raw []byte) (Params, error) {
	params := DefaultParams()
	if len(raw) == 0 {
		return params, nil
	}

	var decoded Params
	if err := json.Unmarshal(raw, &decoded); err == nil {
		if decoded.Chromosome != (quant.Chromosome{}) || !reflect.DeepEqual(decoded.SpawnPoint, quant.SpawnPoint{}) {
			params = mergeParams(params, decoded)
			params.Chromosome = quant.ClampChromosome(params.Chromosome)
			return params, nil
		}
	}

	var chromosome quant.Chromosome
	if err := json.Unmarshal(raw, &chromosome); err != nil {
		return params, err
	}
	params.Chromosome = quant.ClampChromosome(chromosome)
	return params, nil
}

func paramsOrDefault(params Params) Params {
	return mergeParams(DefaultParams(), params)
}

func mergeParams(base, override Params) Params {
	if override.Chromosome != (quant.Chromosome{}) {
		base.Chromosome = quant.ClampChromosome(override.Chromosome)
	}
	if !reflect.DeepEqual(override.SpawnPoint, quant.SpawnPoint{}) {
		base.SpawnPoint = override.SpawnPoint
		if base.SpawnPoint.Precision.MinOrderUSDT <= 0 {
			base.SpawnPoint.Precision.MinOrderUSDT = quant.DefaultMinOrderUSDT
		}
	}
	return base
}
