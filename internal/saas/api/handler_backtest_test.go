package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bian-trade-go/internal/adapters/backtest"
	"bian-trade-go/internal/saas/auth"
	"bian-trade-go/internal/saas/config"
	"bian-trade-go/internal/saas/store"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCreateBacktestFailureIncludesTopLevelError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&store.User{}, &store.BacktestRun{}, &store.KLine{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	if err := db.Create(&store.User{
		ID:                 7,
		Email:              "backtest@example.com",
		PasswordHash:       "hash",
		Role:               "user",
		SubscriptionPlan:   "free",
		SubscriptionStatus: "active",
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	router := NewRouter(RouterConfig{
		DB:           db,
		TokenService: fakeTokenService{claims: &auth.Claims{UserID: 7, Role: "user"}},
		AppRole:      config.AppRoleSaaS,
	})
	body := bytes.NewBufferString(`{"strategy_id":"example_sigmoid_dca_v1","symbol":"BTCUSDT","interval":"1h","limit":1}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backtests", body)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing market data, got %d: %s", w.Code, w.Body.String())
	}
	var got struct {
		Error    string `json:"error"`
		Backtest struct {
			Error  string `json:"error"`
			Status string `json:"status"`
		} `json:"backtest"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(got.Error, "load backtest klines: no bars for BTCUSDT 1h") {
		t.Fatalf("expected actionable top-level error, got %q", got.Error)
	}
	if got.Backtest.Error != got.Error {
		t.Fatalf("expected run error to match top-level error, got run=%q top=%q", got.Backtest.Error, got.Error)
	}
	if got.Backtest.Status != string(store.BacktestRunFailed) {
		t.Fatalf("expected failed run status, got %q", got.Backtest.Status)
	}
	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode raw response: %v", err)
	}
	backtest, ok := raw["backtest"].(map[string]any)
	if !ok {
		t.Fatalf("expected backtest object, got %#v", raw["backtest"])
	}
	for _, key := range []string{"id", "strategy_id", "symbol", "interval", "status", "request", "result", "started_at", "finished_at"} {
		if _, ok := backtest[key]; !ok {
			t.Fatalf("expected API response to include %q, got keys %#v", key, backtest)
		}
	}
	for _, key := range []string{"ID", "StrategyID", "Symbol", "Interval", "Status", "Request", "Result", "StartedAt", "FinishedAt"} {
		if _, ok := backtest[key]; ok {
			t.Fatalf("expected API response not to expose GORM field %q, got keys %#v", key, backtest)
		}
	}
}

func TestPersistBacktestArtifactsBatchesLargeOrderSets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&store.BTAccount{},
		&store.BTOrder{},
		&store.BTPosition{},
		&store.BTTradeLog{},
		&store.BTReport{},
	); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}

	orders := make([]backtest.SimulatedOrder, backtestArtifactBatchSize*9)
	for i := range orders {
		orders[i] = backtest.SimulatedOrder{
			Symbol:         "BTCUSDT",
			Side:           "BUY",
			Offset:         "OPEN",
			PositionSide:   "LONG",
			Leverage:       3,
			OrderPrice:     100 + float64(i%10),
			ExecutedPrice:  100 + float64(i%10),
			ExecutedQty:    0.01,
			Fee:            0.01,
			Status:         "FILLED",
			OrderedAtMs:    time.Date(2026, time.May, 20, 0, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Minute).UnixMilli(),
			ReasonCode:     fmt.Sprintf("TEST_%d", i),
			RealizedPnL:    0.1,
			MarginReleased: 1,
		}
	}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/backtests", nil)
	startedAt := time.Date(2026, time.May, 20, 0, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Second)
	handler := NewBacktestHandler(db)

	err = handler.persistBacktestArtifacts(c, 42, createBacktestRequest{
		Symbols:       []string{"BTCUSDT"},
		DirectionMode: string(backtest.DirectionLongShort),
		Leverage:      3,
	}, backtestResultPayload{
		Metrics: backtest.Result{
			FinalEquity:     9000,
			TotalInjected:   10000,
			ROI:             -0.1,
			MaxDrawdown:     0.2,
			RealizedPnL:     -1000,
			TradeCount:      len(orders) / 2,
			WinRate:         0.4,
			ProfitLossRatio: 0.8,
			Sharpe:          -0.1,
			Orders:          orders,
		},
		BarsCount: 120,
	}, startedAt, finishedAt)
	if err != nil {
		t.Fatalf("persist large backtest artifacts: %v", err)
	}

	var count int64
	if err := db.Model(&store.BTOrder{}).Where("backtest_run_id = ?", 42).Count(&count).Error; err != nil {
		t.Fatalf("count bt_orders: %v", err)
	}
	if count != int64(len(orders)) {
		t.Fatalf("persisted orders = %d, want %d", count, len(orders))
	}
}
