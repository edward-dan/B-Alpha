package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"bian-trade-go/internal/agent/config"
	"bian-trade-go/internal/agent/exchange"
	agentws "bian-trade-go/internal/agent/ws"
	"go.uber.org/zap"
)

func main() {
	configPath := flag.String("config", "config.agent.yaml", "path to local Agent config")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load agent config: %v", err)
	}

	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("create logger: %v", err)
	}
	defer func() { _ = logger.Sync() }()

	exchangeClient, err := exchange.NewBitgetClient(cfg.Exchange)
	if err != nil {
		logger.Fatal("create exchange client", zap.Error(err))
	}

	client, err := agentws.NewAgentClient(agentws.Config{
		AgentConfig: *cfg,
		Exchange:    exchangeClient,
		Logger:      logger,
	})
	if err != nil {
		logger.Fatal("create agent websocket client", zap.Error(err))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := client.Run(ctx); err != nil && err != context.Canceled {
		logger.Fatal("run agent", zap.Error(err))
	}
	logger.Info("agent shutdown complete")
}
