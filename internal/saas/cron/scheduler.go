package cron

import (
	"context"
	"sync"

	"bian-trade-go/internal/saas/store"
	robfigcron "github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

type InstanceTicker interface {
	RunningInstances(ctx context.Context) ([]store.StrategyInstance, error)
	Tick(ctx context.Context, instanceID uint) error
}

type Scheduler struct {
	manager InstanceTicker
	logger  *zap.Logger
	cron    *robfigcron.Cron
	mu      sync.Mutex
	running bool
}

func NewScheduler(manager InstanceTicker, logger *zap.Logger) *Scheduler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Scheduler{
		manager: manager,
		logger:  logger,
		cron:    robfigcron.New(),
	}
}

func (s *Scheduler) Start(ctx context.Context) error {
	if s == nil || s.manager == nil {
		return nil
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	if _, err := s.cron.AddFunc("@every 1m", func() {
		s.Scan(context.Background())
	}); err != nil {
		s.mu.Unlock()
		return err
	}
	s.running = true
	s.cron.Start()
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.Stop()
	}()
	return nil
}

func (s *Scheduler) Stop() {
	_ = s.Shutdown(context.Background())
}

func (s *Scheduler) Shutdown(ctx context.Context) error {
	if s == nil || s.cron == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	stopCtx := s.cron.Stop()
	s.running = false
	s.mu.Unlock()
	select {
	case <-stopCtx.Done():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Scheduler) Scan(ctx context.Context) {
	if s == nil || s.manager == nil {
		return
	}
	instances, err := s.manager.RunningInstances(ctx)
	if err != nil {
		s.logger.Warn("load running strategy instances failed", zap.Error(err))
		return
	}

	var wg sync.WaitGroup
	for _, inst := range instances {
		instanceID := inst.ID
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.manager.Tick(ctx, instanceID); err != nil {
				s.logger.Warn("strategy instance tick failed", zap.Uint("instance_id", instanceID), zap.Error(err))
			}
		}()
	}
	wg.Wait()
}
