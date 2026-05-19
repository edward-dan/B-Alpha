package quant

import (
	"sort"
	"time"
)

type SpotLot struct {
	LotType      LotType   `json:"lot_type"`
	Amount       float64   `json:"amount"`
	CostPrice    float64   `json:"cost_price"`
	CreatedAt    time.Time `json:"created_at"`
	IsColdSealed bool      `json:"is_cold_sealed"`
}

// TotalDeadBTC sums non-cold DEAD_STACK lots.
func TotalDeadBTC(lots []SpotLot) float64 {
	var total float64
	for _, lot := range lots {
		if releasableDeadLot(lot) {
			total += lot.Amount
		}
	}
	return total
}

// TotalFloatBTC sums FLOATING lots.
func TotalFloatBTC(lots []SpotLot) float64 {
	var total float64
	for _, lot := range lots {
		if lot.LotType == LotTypeFloating {
			total += lot.Amount
		}
	}
	return total
}

// TotalColdSealedBTC sums lots that are cold sealed by flag or lot type.
func TotalColdSealedBTC(lots []SpotLot) float64 {
	var total float64
	for _, lot := range lots {
		if lot.IsColdSealed || lot.LotType == LotTypeColdSealed {
			total += lot.Amount
		}
	}
	return total
}

// SoftReleaseDeadBTC releases aged, non-cold DEAD_STACK lots into FLOATING lots.
func SoftReleaseDeadBTC(lots []SpotLot, now time.Time, minAgeMonths int, maxReleasePct, sellableGapBTC float64) ([]SpotLot, float64) {
	releaseCap := TotalDeadBTC(lots) * ClipFloat64(maxReleasePct, 0, 1)
	targetRelease := minPositive(releaseCap, sellableGapBTC)
	if targetRelease <= 0 {
		return cloneLots(lots), 0
	}

	cutoff := now.AddDate(0, -minAgeMonths, 0)
	if minAgeMonths <= 0 {
		cutoff = now
	}

	indices := make([]int, 0, len(lots))
	for i, lot := range lots {
		if !releasableDeadLot(lot) {
			continue
		}
		if lot.CreatedAt.After(cutoff) {
			continue
		}
		indices = append(indices, i)
	}
	sortLotIndicesByAge(lots, indices)

	return releaseDeadLots(lots, indices, targetRelease)
}

// HardReleaseDeadBTC releases non-cold DEAD_STACK lots when required sell inventory exceeds FLOATING inventory.
func HardReleaseDeadBTC(lots []SpotLot, requiredSellBTC float64) ([]SpotLot, float64) {
	gap := requiredSellBTC - TotalFloatBTC(lots)
	if gap <= 0 {
		return cloneLots(lots), 0
	}

	indices := make([]int, 0, len(lots))
	for i, lot := range lots {
		if releasableDeadLot(lot) {
			indices = append(indices, i)
		}
	}
	sortLotIndicesByAge(lots, indices)

	return releaseDeadLots(lots, indices, gap)
}

func releasableDeadLot(lot SpotLot) bool {
	return lot.LotType == LotTypeDeadStack && !lot.IsColdSealed && lot.Amount > 0
}

func cloneLots(lots []SpotLot) []SpotLot {
	out := make([]SpotLot, len(lots))
	copy(out, lots)
	return out
}

func sortLotIndicesByAge(lots []SpotLot, indices []int) {
	sort.SliceStable(indices, func(i, j int) bool {
		return lots[indices[i]].CreatedAt.Before(lots[indices[j]].CreatedAt)
	})
}

func releaseDeadLots(lots []SpotLot, indices []int, targetRelease float64) ([]SpotLot, float64) {
	out := cloneLots(lots)
	remaining := targetRelease
	var released float64

	for _, idx := range indices {
		if remaining <= 0 {
			break
		}
		lot := out[idx]
		if !releasableDeadLot(lot) {
			continue
		}

		amount := lot.Amount
		if amount > remaining {
			amount = remaining
		}
		if amount <= 0 {
			continue
		}

		if amount >= lot.Amount {
			out[idx].LotType = LotTypeFloating
			out[idx].IsColdSealed = false
		} else {
			out[idx].Amount -= amount
			out = append(out, SpotLot{
				LotType:      LotTypeFloating,
				Amount:       amount,
				CostPrice:    lot.CostPrice,
				CreatedAt:    lot.CreatedAt,
				IsColdSealed: false,
			})
		}

		released += amount
		remaining -= amount
	}

	return out, released
}

func minPositive(a, b float64) float64 {
	if a <= 0 || b <= 0 {
		return 0
	}
	if a < b {
		return a
	}
	return b
}
