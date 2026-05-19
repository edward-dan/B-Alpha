package example

const (
	StrategyID          = "example_sigmoid_dca_v1"
	StrategyName        = "Dynamic Balance Spot"
	StrategyVersion     = "0.1.0"
	StrategyDescription = "Spot strategy combining macro DCA, Sigmoid micro rebalancing, and semantic DeadBTC release."
)

type Manifest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	IsSpot      bool   `json:"is_spot"`
	Description string `json:"description"`
}

func GetManifest() Manifest {
	return Manifest{
		ID:          StrategyID,
		Name:        StrategyName,
		Version:     StrategyVersion,
		IsSpot:      true,
		Description: StrategyDescription,
	}
}
