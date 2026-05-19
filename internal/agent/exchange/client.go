package exchange

import (
	"fmt"
	"strings"

	"bian-trade-go/internal/agent/config"
)

type Client interface {
	PlaceOrder(cmd TradeCommand) (Execution, error)
	GetBalances() ([]Balance, error)
}

func NewClient(cfg config.ExchangeConfig) (Client, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Name)) {
	case "bitget":
		return NewBitgetClient(cfg)
	case "binance":
		return NewBinanceClient(cfg)
	default:
		return nil, fmt.Errorf("new exchange client: unsupported exchange %q", cfg.Name)
	}
}
