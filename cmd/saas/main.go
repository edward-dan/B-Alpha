package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"bian-trade-go/internal/quant"
	"bian-trade-go/internal/saas/api"
	"bian-trade-go/internal/saas/auth"
	"bian-trade-go/internal/saas/config"
	"bian-trade-go/internal/saas/cron"
	"bian-trade-go/internal/saas/epoch"
	"bian-trade-go/internal/saas/ga"
	"bian-trade-go/internal/saas/instance"
	"bian-trade-go/internal/saas/store"
	saasws "bian-trade-go/internal/saas/ws"
	"bian-trade-go/internal/strategies/example"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const shutdownTimeout = 30 * time.Second

func main() {
	configPath := flag.String("config", "config.yaml", "path to SaaS config")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load SaaS config: %v", err)
	}

	logger, err := newLogger(cfg.Server.Mode)
	if err != nil {
		log.Fatalf("create logger: %v", err)
	}
	defer func() { _ = logger.Sync() }()

	if err := run(cfg, logger); err != nil {
		logger.Error("SaaS process exited with error", zap.Error(err))
		os.Exit(1)
	}
}

func run(cfg *config.Config, logger *zap.Logger) error {
	gin.SetMode(ginMode(cfg.Server.Mode))

	dbStore, err := store.NewDB(cfg.Database)
	if err != nil {
		return err
	}
	if err := seedStrategyTemplates(dbStore.DB); err != nil {
		return err
	}
	sqlDB, err := dbStore.DB.DB()
	if err != nil {
		return fmt.Errorf("unwrap postgres db: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()

	redisCtx, cancelRedis := context.WithTimeout(context.Background(), shutdownTimeout)
	redisStore, err := store.NewRedis(redisCtx, cfg.Redis)
	cancelRedis()
	if err != nil {
		return err
	}
	defer func() { _ = redisStore.Client().Close() }()

	tokenService, err := auth.NewService(cfg.JWT)
	if err != nil {
		return fmt.Errorf("initialize JWT service: %w", err)
	}

	hub := saasws.NewHub(saasws.Config{
		DB:          dbStore.DB,
		TokenParser: tokenService,
		Logger:      logger,
	})

	manager := instance.NewManager(instance.ManagerConfig{
		DB:         dbStore.DB,
		MarketData: &klineMarketDataClient{db: dbStore.DB},
		Hub:        hub,
		Cache:      redisStore,
		Logger:     logger,
	})
	restoreCtx, cancelRestore := context.WithTimeout(context.Background(), shutdownTimeout)
	restored, err := manager.RestoreRunning(restoreCtx)
	cancelRestore()
	if err != nil {
		return fmt.Errorf("restore running instances: %w", err)
	}
	logger.Info("restored running strategy instances", zap.Int("count", restored))

	evolutionEngine := ga.NewEvolutionEngine(dbStore.DB, ga.NewExampleEvolvable())
	epochService := epoch.NewEpochService(dbStore.DB, evolutionEngine, logger)

	scheduler := cron.NewScheduler(manager, logger)
	cronCtx, cancelCron := context.WithCancel(context.Background())
	defer cancelCron()
	if err := scheduler.Start(cronCtx); err != nil {
		return fmt.Errorf("start cron scheduler: %w", err)
	}

	router := api.NewRouter(api.RouterConfig{
		DB:              dbStore.DB,
		Redis:           redisStore,
		TokenService:    tokenService,
		InstanceManager: manager,
		EpochService:    epochService,
		Hub:             hub,
		AppRole:         cfg.AppRole,
	})

	server := &http.Server{
		Addr:    net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port)),
		Handler: router,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("starting SaaS HTTP server",
			zap.String("addr", server.Addr),
			zap.String("app_role", cfg.AppRole),
		)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	select {
	case <-signalCtx.Done():
		stopSignals()
		logger.Info("shutdown signal received")
	case err := <-serverErr:
		if err != nil {
			cancelCron()
			stopCtx, cancelStop := context.WithTimeout(context.Background(), shutdownTimeout)
			_ = scheduler.Shutdown(stopCtx)
			cancelStop()
			hub.CloseAll()
			return fmt.Errorf("run HTTP server: %w", err)
		}
		return nil
	}

	return shutdown(server, scheduler, manager, hub, redisStore, sqlDB, cancelCron, logger)
}

func seedStrategyTemplates(db *gorm.DB) error {
	manifest, err := json.Marshal(example.GetManifest())
	if err != nil {
		return fmt.Errorf("marshal strategy manifest: %w", err)
	}
	template := store.StrategyTemplate{
		Name:     example.StrategyName,
		Version:  example.StrategyVersion,
		IsSpot:   true,
		Manifest: store.JSONB(manifest),
	}
	if err := db.Where("name = ? AND version = ?", template.Name, template.Version).
		FirstOrCreate(&template).Error; err != nil {
		return fmt.Errorf("seed strategy template: %w", err)
	}
	return nil
}

func shutdown(
	server *http.Server,
	scheduler *cron.Scheduler,
	manager *instance.Manager,
	hub *saasws.Hub,
	redisStore *store.Redis,
	sqlDB interface{ Close() error },
	cancelCron context.CancelFunc,
	logger *zap.Logger,
) error {
	var errs []error

	httpCtx, cancelHTTP := context.WithTimeout(context.Background(), shutdownTimeout)
	httpDone := make(chan error, 1)
	go func() {
		httpDone <- server.Shutdown(httpCtx)
	}()

	cronCtx, cancelCronTimeout := context.WithTimeout(context.Background(), shutdownTimeout)
	if err := scheduler.Shutdown(cronCtx); err != nil {
		errs = append(errs, fmt.Errorf("shutdown cron scheduler: %w", err))
		logger.Warn("shutdown cron scheduler timed out or failed", zap.Error(err))
	}
	cancelCronTimeout()
	cancelCron()

	if err := <-httpDone; err != nil {
		errs = append(errs, fmt.Errorf("shutdown HTTP server: %w", err))
		logger.Warn("shutdown HTTP server failed", zap.Error(err))
	}
	cancelHTTP()

	snapshotCtx, cancelSnapshots := context.WithTimeout(context.Background(), shutdownTimeout)
	if err := manager.PersistActiveRuntimeSnapshots(snapshotCtx); err != nil {
		errs = append(errs, fmt.Errorf("persist active runtime snapshots: %w", err))
		logger.Warn("persist active runtime snapshots failed", zap.Error(err))
	}
	cancelSnapshots()

	hub.CloseAll()

	if redisStore != nil {
		if err := redisStore.Client().Close(); err != nil {
			errs = append(errs, fmt.Errorf("close redis: %w", err))
		}
	}
	if sqlDB != nil {
		if err := sqlDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close postgres: %w", err))
		}
	}
	logger.Info("SaaS shutdown complete")
	return errors.Join(errs...)
}

func newLogger(mode string) (*zap.Logger, error) {
	if ginMode(mode) == gin.ReleaseMode {
		return zap.NewProduction()
	}
	return zap.NewDevelopment()
}

func ginMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case gin.ReleaseMode:
		return gin.ReleaseMode
	case gin.TestMode:
		return gin.TestMode
	default:
		return gin.DebugMode
	}
}

type klineMarketDataClient struct {
	db  *gorm.DB
	now func() time.Time
}

func (c *klineMarketDataClient) LatestClosedBars(ctx context.Context, symbol string, interval string, limit int) ([]quant.Bar, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("latest closed bars: database is required")
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	interval = strings.ToLower(strings.TrimSpace(interval))
	if symbol == "" || interval == "" {
		return nil, errors.New("latest closed bars: symbol and interval are required")
	}
	if limit <= 0 {
		limit = 5000
	}
	duration, ok := intervalDuration(interval)
	if !ok {
		return nil, fmt.Errorf("latest closed bars: unsupported interval %q", interval)
	}
	now := time.Now().UTC()
	if c.now != nil {
		now = c.now().UTC()
	}

	var rows []store.KLine
	if err := c.db.WithContext(ctx).
		Where("symbol = ? AND interval = ? AND open_time <= ?", symbol, interval, now.Add(-duration)).
		Order("open_time DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}

	bars := make([]quant.Bar, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		bar, err := klineToBar(rows[i])
		if err != nil {
			return nil, err
		}
		bars = append(bars, bar)
	}
	return bars, nil
}

func intervalDuration(interval string) (time.Duration, bool) {
	interval = strings.TrimSpace(strings.ToLower(interval))
	if interval == "" {
		return 0, false
	}
	unit := interval[len(interval)-1]
	value := interval[:len(interval)-1]
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 0, false
	}
	switch unit {
	case 's':
		return time.Duration(n) * time.Second, true
	case 'm':
		return time.Duration(n) * time.Minute, true
	case 'h':
		return time.Duration(n) * time.Hour, true
	case 'd':
		return time.Duration(n) * 24 * time.Hour, true
	case 'w':
		return time.Duration(n) * 7 * 24 * time.Hour, true
	default:
		return 0, false
	}
}

func klineToBar(row store.KLine) (quant.Bar, error) {
	open, err := decimalToFloat(row.Open)
	if err != nil {
		return quant.Bar{}, fmt.Errorf("parse kline open %d: %w", row.ID, err)
	}
	high, err := decimalToFloat(row.High)
	if err != nil {
		return quant.Bar{}, fmt.Errorf("parse kline high %d: %w", row.ID, err)
	}
	low, err := decimalToFloat(row.Low)
	if err != nil {
		return quant.Bar{}, fmt.Errorf("parse kline low %d: %w", row.ID, err)
	}
	closePrice, err := decimalToFloat(row.Close)
	if err != nil {
		return quant.Bar{}, fmt.Errorf("parse kline close %d: %w", row.ID, err)
	}
	volume, err := decimalToFloat(row.Volume)
	if err != nil {
		return quant.Bar{}, fmt.Errorf("parse kline volume %d: %w", row.ID, err)
	}
	return quant.Bar{
		OpenTime: row.OpenTime.UnixMilli(),
		Open:     open,
		High:     high,
		Low:      low,
		Close:    closePrice,
		Volume:   volume,
	}, nil
}

func decimalToFloat(value store.Decimal) (float64, error) {
	if value == "" {
		return 0, nil
	}
	return strconv.ParseFloat(string(value), 64)
}
