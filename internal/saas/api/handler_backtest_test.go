package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
}
