package ws

import (
	"encoding/json"
	"math"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bian-trade-go/internal/saas/auth"
	"bian-trade-go/internal/saas/store"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fakeTokenParser struct {
	claims *auth.Claims
	err    error
}

func (p fakeTokenParser) ParseToken(string) (*auth.Claims, error) {
	return p.claims, p.err
}

func TestHandleConnectionAuthHeartbeatAndSend(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hub := NewHub(Config{
		TokenParser: fakeTokenParser{claims: &auth.Claims{UserID: 42, Role: "user"}},
		Now:         func() time.Time { return time.Now().UTC() },
	})
	router := gin.New()
	hub.RegisterRoutes(router)
	server := httptest.NewServer(router)
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(authRequest{
		Type:    "auth",
		JWT:     "token",
		AgentID: "agent:test@example.com",
		Version: "test",
	}); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	setWSReadDeadline(t, conn)
	var authResp authResult
	if err := conn.ReadJSON(&authResp); err != nil {
		t.Fatalf("read auth_result: %v", err)
	}
	if !authResp.Success || authResp.Type != "auth_result" || authResp.UserID != 42 {
		t.Fatalf("unexpected auth_result: %#v", authResp)
	}
	if !hub.IsAgentConnected("user:42") || !hub.IsAgentConnected("agent:test@example.com") {
		t.Fatal("expected authenticated agent to be connected")
	}

	if err := conn.WriteJSON(heartbeatMessage{Type: "heartbeat", AgentTimeMS: 77}); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}
	var heartbeat heartbeatAck
	if err := conn.ReadJSON(&heartbeat); err != nil {
		t.Fatalf("read heartbeat_ack: %v", err)
	}
	if heartbeat.Type != "heartbeat_ack" || heartbeat.AgentTimeMS != 77 {
		t.Fatalf("unexpected heartbeat_ack: %#v", heartbeat)
	}

	if err := hub.SendToAgent(42, TradeCommand{
		ClientOrderID: "inst7-MACRO-123",
		InstanceID:    7,
		Symbol:        "BTCUSDT",
		Action:        "BUY",
		Engine:        "MACRO",
		LotType:       "DEAD_STACK",
		AmountUSDT:    "25",
	}); err != nil {
		t.Fatalf("send to agent: %v", err)
	}
	var command commandMessage
	if err := conn.ReadJSON(&command); err != nil {
		t.Fatalf("read command: %v", err)
	}
	if command.Type != "command" || command.Payload.ClientOrderID != "inst7-MACRO-123" {
		t.Fatalf("unexpected command: %#v", command)
	}
}

func TestHandleConnectionRejectsNonAuthFirstMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hub := NewHub(Config{
		TokenParser: fakeTokenParser{claims: &auth.Claims{UserID: 42, Role: "user"}},
		Now:         func() time.Time { return time.Now().UTC() },
	})
	router := gin.New()
	hub.RegisterRoutes(router)
	server := httptest.NewServer(router)
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(heartbeatMessage{Type: "heartbeat"}); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}
	var authResp authResult
	if err := conn.ReadJSON(&authResp); err != nil {
		t.Fatalf("read auth_result: %v", err)
	}
	if authResp.Success || authResp.Code != "invalid_auth" {
		t.Fatalf("expected invalid auth response, got %#v", authResp)
	}
}

func TestCloseAllDisconnectsAuthenticatedAgents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hub := NewHub(Config{
		TokenParser: fakeTokenParser{claims: &auth.Claims{UserID: 42, Role: "user"}},
		Now:         func() time.Time { return time.Now().UTC() },
	})
	router := gin.New()
	hub.RegisterRoutes(router)
	server := httptest.NewServer(router)
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(authRequest{
		Type:    "auth",
		JWT:     "token",
		AgentID: "agent:test@example.com",
		Version: "test",
	}); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	var authResp authResult
	if err := conn.ReadJSON(&authResp); err != nil {
		t.Fatalf("read auth_result: %v", err)
	}
	if !authResp.Success {
		t.Fatalf("unexpected auth_result: %#v", authResp)
	}

	hub.CloseAll()
	if hub.IsAgentConnected("user:42") || hub.IsAgentConnected("agent:test@example.com") {
		t.Fatal("expected CloseAll to remove authenticated agent connection")
	}
	if err := hub.SendToAgent(42, TradeCommand{ClientOrderID: "inst7-MACRO-123"}); err == nil {
		t.Fatal("expected SendToAgent to fail after CloseAll")
	}
}

func TestHandleConnectionUnauthenticatedConnectionClosesAfterAuthTimeout(t *testing.T) {
	if defaultAuthTimeout != 10*time.Second {
		t.Fatalf("default auth timeout = %s, want 10s", defaultAuthTimeout)
	}

	gin.SetMode(gin.TestMode)
	hub := NewHub(Config{
		TokenParser: fakeTokenParser{claims: &auth.Claims{UserID: 42, Role: "user"}},
		AuthTimeout: 30 * time.Millisecond,
		Now:         func() time.Time { return time.Now().UTC() },
	})
	router := gin.New()
	hub.RegisterRoutes(router)
	server := httptest.NewServer(router)
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("expected unauthenticated connection to close after auth timeout")
	}
}

func TestHandleConnectionDeltaReportUpdatesPortfolioState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, instanceID, clientOrderID := newWSTestDB(t)
	hub := NewHub(Config{
		DB:          db,
		TokenParser: fakeTokenParser{claims: &auth.Claims{UserID: 42, Role: "user"}},
		Now:         func() time.Time { return time.Now().UTC() },
	})
	router := gin.New()
	hub.RegisterRoutes(router)
	server := httptest.NewServer(router)
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(server.URL), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(authRequest{
		Type:    "auth",
		JWT:     "token",
		AgentID: "agent:test@example.com",
		Version: "test",
	}); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	var authResp authResult
	if err := conn.ReadJSON(&authResp); err != nil {
		t.Fatalf("read auth_result: %v", err)
	}
	if !authResp.Success {
		t.Fatalf("unexpected auth_result: %#v", authResp)
	}

	now := time.Date(2026, time.May, 13, 12, 0, 0, 0, time.UTC).UnixMilli()
	if err := conn.WriteJSON(DeltaReport{
		Type:          "delta_report",
		ReportID:      "report-1",
		ClientOrderID: &clientOrderID,
		InstanceID:    instanceID,
		Symbol:        "BTCUSDT",
		Status:        "FILLED",
		Execution: &Execution{
			OrderID:        "exchange-order-1",
			ClientOrderID:  clientOrderID,
			Status:         "FILLED",
			FilledQty:      "0.01",
			FilledPrice:    "50000",
			FilledAmount:   "500",
			Fee:            "0.5",
			ExchangeTimeMS: now,
		},
		Balances: []Balance{
			{Asset: "USDT", Available: "499.5"},
			{Asset: "BTC", Available: "0.01"},
		},
		AgentTimeMS: now,
	}); err != nil {
		t.Fatalf("write delta_report: %v", err)
	}
	setWSReadDeadline(t, conn)
	var ack reportAck
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read report_ack: %v", err)
	}
	if !ack.Success || ack.ReportID != "report-1" || ack.ClientOrderID != clientOrderID {
		t.Fatalf("unexpected report_ack: %#v", ack)
	}

	var portfolio store.PortfolioState
	if err := db.Where("strategy_instance_id = ?", instanceID).First(&portfolio).Error; err != nil {
		t.Fatalf("load portfolio: %v", err)
	}
	if got := decimalTestFloat(t, portfolio.USDTBalance); !wsFloatEqual(got, 499.5) {
		t.Fatalf("USDTBalance = %.12f, want 499.5", got)
	}
	if got := decimalTestFloat(t, portfolio.FloatBTC); !wsFloatEqual(got, 0.01) {
		t.Fatalf("FloatBTC = %.12f, want 0.01", got)
	}
	if got := decimalTestFloat(t, portfolio.TotalEquity); !wsFloatEqual(got, 999.5) {
		t.Fatalf("TotalEquity = %.12f, want 999.5", got)
	}
}

func wsURL(httpURL string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http") + "/ws/agent"
}

func setWSReadDeadline(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set websocket read deadline: %v", err)
	}
}

func newWSTestDB(t *testing.T) (*gorm.DB, uint, string) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("unwrap sqlite test db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	if err := db.AutoMigrate(
		&store.User{},
		&store.StrategyTemplate{},
		&store.StrategyInstance{},
		&store.PortfolioState{},
		&store.SpotLot{},
		&store.TradeRecord{},
		&store.SpotExecution{},
		&store.AuditLog{},
	); err != nil {
		t.Fatalf("migrate ws test db: %v", err)
	}

	user := store.User{
		ID:           42,
		Email:        "agent@example.com",
		PasswordHash: "hash",
		Role:         "user",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	template := store.StrategyTemplate{
		Name:     "example",
		Version:  "test",
		IsSpot:   true,
		Manifest: store.JSONB(`{}`),
	}
	if err := db.Create(&template).Error; err != nil {
		t.Fatalf("seed strategy template: %v", err)
	}
	instance := store.StrategyInstance{
		UserID:     user.ID,
		TemplateID: template.ID,
		Name:       "test instance",
		Symbol:     "BTCUSDT",
		Interval:   "1h",
		Status:     store.StrategyInstanceRunning,
		Config:     store.JSONB(`{}`),
	}
	if err := db.Create(&instance).Error; err != nil {
		t.Fatalf("seed strategy instance: %v", err)
	}
	portfolio := store.PortfolioState{
		StrategyInstanceID: instance.ID,
		USDTBalance:        store.Decimal("1000"),
		DeadBTC:            store.Decimal("0"),
		FloatBTC:           store.Decimal("0"),
		ColdSealedBTC:      store.Decimal("0"),
		TotalEquity:        store.Decimal("1000"),
	}
	if err := db.Create(&portfolio).Error; err != nil {
		t.Fatalf("seed portfolio: %v", err)
	}

	clientOrderID := "inst1-MICRO-20260513"
	commandPayload, err := json.Marshal(TradeCommand{
		ClientOrderID: clientOrderID,
		InstanceID:    instance.ID,
		Symbol:        "BTCUSDT",
		Action:        "BUY",
		Engine:        "MICRO",
		LotType:       "FLOATING",
		AmountUSDT:    "500",
	})
	if err != nil {
		t.Fatalf("marshal command payload: %v", err)
	}
	execution := store.SpotExecution{
		StrategyInstanceID: instance.ID,
		ClientOrderID:      clientOrderID,
		Action:             "BUY",
		Engine:             "MICRO",
		Symbol:             "BTCUSDT",
		Status:             store.SpotExecutionPending,
		CommandPayload:     store.JSONB(commandPayload),
		ExecutionPayload:   store.JSONB(`{}`),
	}
	if err := db.Create(&execution).Error; err != nil {
		t.Fatalf("seed spot execution: %v", err)
	}
	return db, instance.ID, clientOrderID
}

func decimalTestFloat(t *testing.T, value store.Decimal) float64 {
	t.Helper()
	got, err := decimalFloat(value)
	if err != nil {
		t.Fatalf("parse decimal %q: %v", value, err)
	}
	return got
}

func wsFloatEqual(a, b float64) bool {
	return math.Abs(a-b) <= 1e-12
}
