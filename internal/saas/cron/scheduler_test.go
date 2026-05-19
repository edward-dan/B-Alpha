package cron

import (
	"context"
	"testing"

	"bian-trade-go/internal/saas/store"
)

type fakeInstanceTicker struct{}

func (fakeInstanceTicker) RunningInstances(context.Context) ([]store.StrategyInstance, error) {
	return nil, nil
}

func (fakeInstanceTicker) Tick(context.Context, uint) error {
	return nil
}

func TestSchedulerShutdownIsIdempotent(t *testing.T) {
	scheduler := NewScheduler(fakeInstanceTicker{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if err := scheduler.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
	if err := scheduler.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown returned error: %v", err)
	}
}
