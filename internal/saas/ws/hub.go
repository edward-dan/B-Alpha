package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"bian-trade-go/internal/saas/auth"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	defaultAuthTimeout  = 10 * time.Second
	defaultWriteTimeout = 10 * time.Second
)

var (
	ErrAgentNotConnected = errors.New("agent is not connected")
	ErrHubNotReady       = errors.New("websocket hub is not initialized")
)

type TokenParser interface {
	ParseToken(tokenStr string) (*auth.Claims, error)
}

type tokenParserFunc func(string) (*auth.Claims, error)

func (f tokenParserFunc) ParseToken(tokenStr string) (*auth.Claims, error) {
	return f(tokenStr)
}

type Config struct {
	DB           *gorm.DB
	TokenParser  TokenParser
	Upgrader     *websocket.Upgrader
	Logger       *zap.Logger
	Now          func() time.Time
	AuthTimeout  time.Duration
	WriteTimeout time.Duration
}

type Hub struct {
	db           *gorm.DB
	tokenParser  TokenParser
	upgrader     websocket.Upgrader
	logger       *zap.Logger
	now          func() time.Time
	authTimeout  time.Duration
	writeTimeout time.Duration
	agents       sync.Map // userID uint -> *AgentConn
}

type AgentConn struct {
	UserID  uint
	AgentID string
	Version string
	conn    *websocket.Conn
	hub     *Hub
	mu      sync.Mutex
}

type TradeCommand struct {
	ClientOrderID string  `json:"client_order_id"`
	InstanceID    uint    `json:"instance_id"`
	Symbol        string  `json:"symbol"`
	Action        string  `json:"action"`
	Engine        string  `json:"engine"`
	LotType       string  `json:"lot_type"`
	AmountUSDT    string  `json:"amount_usdt,omitempty"`
	QtyAsset      string  `json:"qty_asset,omitempty"`
	LimitPrice    *string `json:"limit_price"`
	CreatedAtMS   int64   `json:"created_at_ms"`
	ExpiresAtMS   int64   `json:"expires_at_ms"`
	ReasonCode    string  `json:"reason_code"`
}

type Execution struct {
	OrderID        string          `json:"order_id,omitempty"`
	ClientOrderID  string          `json:"client_order_id"`
	Status         string          `json:"status"`
	FilledQty      string          `json:"filled_qty,omitempty"`
	FilledPrice    string          `json:"filled_price,omitempty"`
	FilledAmount   string          `json:"filled_amount,omitempty"`
	Fee            string          `json:"fee,omitempty"`
	ExchangeTimeMS int64           `json:"exchange_time_ms,omitempty"`
	Raw            json.RawMessage `json:"raw,omitempty"`
}

type Balance struct {
	Asset     string `json:"asset"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
	Locked    string `json:"locked,omitempty"`
}

type DeltaReport struct {
	Type           string     `json:"type"`
	ReportID       string     `json:"report_id"`
	ClientOrderID  *string    `json:"client_order_id"`
	InstanceID     any        `json:"instance_id,omitempty"`
	Symbol         string     `json:"symbol"`
	Status         string     `json:"status"`
	Execution      *Execution `json:"execution"`
	Balances       []Balance  `json:"balances"`
	ExchangeTimeMS *int64     `json:"exchange_time_ms"`
	AgentTimeMS    int64      `json:"agent_time_ms"`
	ErrorCode      *string    `json:"error_code"`
	ErrorMessage   *string    `json:"error_message"`
}

type wireMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type authRequest struct {
	Type    string `json:"type"`
	JWT     string `json:"jwt"`
	Token   string `json:"token"`
	AgentID string `json:"agent_id"`
	Version string `json:"version"`
}

type authResult struct {
	Type         string `json:"type"`
	Success      bool   `json:"success"`
	Code         string `json:"code,omitempty"`
	Error        string `json:"error,omitempty"`
	ServerTime   int64  `json:"server_time_ms"`
	UserID       uint   `json:"user_id,omitempty"`
	AgentID      string `json:"agent_id,omitempty"`
	AgentVersion string `json:"agent_version,omitempty"`
}

type heartbeatMessage struct {
	Type        string `json:"type"`
	AgentID     string `json:"agent_id,omitempty"`
	AgentTimeMS int64  `json:"agent_time_ms,omitempty"`
	Version     string `json:"version,omitempty"`
}

type heartbeatAck struct {
	Type         string `json:"type"`
	AgentTimeMS  int64  `json:"agent_time_ms,omitempty"`
	ServerTimeMS int64  `json:"server_time_ms"`
}

type commandMessage struct {
	Type    string       `json:"type"`
	Payload TradeCommand `json:"payload"`
}

func NewHub(cfg Config) *Hub {
	parser := cfg.TokenParser
	if parser == nil {
		parser = tokenParserFunc(auth.ParseToken)
	}
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	if cfg.Upgrader != nil {
		upgrader = *cfg.Upgrader
	}
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	now := cfg.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	authTimeout := cfg.AuthTimeout
	if authTimeout <= 0 {
		authTimeout = defaultAuthTimeout
	}
	writeTimeout := cfg.WriteTimeout
	if writeTimeout <= 0 {
		writeTimeout = defaultWriteTimeout
	}
	return &Hub{
		db:           cfg.DB,
		tokenParser:  parser,
		upgrader:     upgrader,
		logger:       logger,
		now:          now,
		authTimeout:  authTimeout,
		writeTimeout: writeTimeout,
	}
}

func (h *Hub) RegisterRoutes(router gin.IRouter) {
	router.GET("/ws/agent", h.HandleConnection)
}

func (h *Hub) IsAgentConnected(agentID string) bool {
	if h == nil {
		return false
	}
	if userID, ok := userIDFromAgentID(agentID); ok {
		_, ok := h.agents.Load(userID)
		return ok
	}
	_, ok := h.findConnByAgentID(agentID)
	return ok
}

func (h *Hub) SendTradeCommand(ctx context.Context, agentID string, command TradeCommand) error {
	if h == nil {
		return ErrHubNotReady
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if userID, ok := userIDFromAgentID(agentID); ok {
		return h.SendToAgent(userID, command)
	}
	conn, ok := h.findConnByAgentID(agentID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrAgentNotConnected, agentID)
	}
	return conn.writeJSON(commandMessage{Type: "command", Payload: command})
}

func (h *Hub) SendToAgent(userID uint, cmd TradeCommand) error {
	if h == nil {
		return ErrHubNotReady
	}
	if userID == 0 {
		return errors.New("send to agent: userID is required")
	}
	value, ok := h.agents.Load(userID)
	if !ok {
		return fmt.Errorf("%w: user:%d", ErrAgentNotConnected, userID)
	}
	conn, ok := value.(*AgentConn)
	if !ok || conn == nil {
		h.agents.Delete(userID)
		return fmt.Errorf("%w: user:%d", ErrAgentNotConnected, userID)
	}
	return conn.writeJSON(commandMessage{Type: "command", Payload: cmd})
}

func (h *Hub) CloseAll() {
	if h == nil {
		return
	}
	h.agents.Range(func(key any, value any) bool {
		if conn, ok := value.(*AgentConn); ok && conn != nil {
			_ = conn.Close()
		}
		h.agents.Delete(key)
		return true
	})
}

func (h *Hub) HandleConnection(c *gin.Context) {
	if h == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": ErrHubNotReady.Error()})
		return
	}
	wsConn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Warn("upgrade agent websocket failed", zap.Error(err))
		return
	}
	defer wsConn.Close()

	if err := wsConn.SetReadDeadline(h.now().Add(h.authTimeout)); err != nil {
		h.logger.Warn("set websocket auth deadline failed", zap.Error(err))
		return
	}
	_, raw, err := wsConn.ReadMessage()
	if err != nil {
		h.logger.Warn("read agent websocket auth failed", zap.Error(err))
		return
	}
	req, err := decodeAuth(raw)
	if err != nil {
		_ = writeAuthFailure(wsConn, h.now, h.writeTimeout, "invalid_auth", err.Error())
		return
	}
	claims, err := h.tokenParser.ParseToken(req.token())
	if err != nil {
		_ = writeAuthFailure(wsConn, h.now, h.writeTimeout, "auth_failed", err.Error())
		return
	}
	if claims.UserID == 0 {
		_ = writeAuthFailure(wsConn, h.now, h.writeTimeout, "auth_failed", "jwt user_id is required")
		return
	}
	if err := wsConn.SetReadDeadline(time.Time{}); err != nil {
		h.logger.Warn("clear websocket auth deadline failed", zap.Error(err))
		return
	}

	agentConn := &AgentConn{
		UserID:  claims.UserID,
		AgentID: strings.TrimSpace(req.AgentID),
		Version: strings.TrimSpace(req.Version),
		conn:    wsConn,
		hub:     h,
	}
	h.register(agentConn)
	defer h.unregister(agentConn)

	if err := agentConn.writeJSON(authResult{
		Type:         "auth_result",
		Success:      true,
		Code:         "OK",
		ServerTime:   h.now().UnixMilli(),
		UserID:       claims.UserID,
		AgentID:      agentConn.AgentID,
		AgentVersion: agentConn.Version,
	}); err != nil {
		h.logger.Warn("write agent websocket auth_result failed", zap.Uint("user_id", claims.UserID), zap.Error(err))
		return
	}
	h.messageLoop(c.Request.Context(), agentConn)
}

func (h *Hub) messageLoop(ctx context.Context, conn *AgentConn) {
	for {
		_, raw, err := conn.conn.ReadMessage()
		if err != nil {
			h.logger.Info("agent websocket disconnected", zap.Uint("user_id", conn.UserID), zap.String("agent_id", conn.AgentID), zap.Error(err))
			return
		}
		var msg wireMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			h.logger.Warn("decode agent websocket message failed", zap.Uint("user_id", conn.UserID), zap.Error(err))
			continue
		}
		switch msg.Type {
		case "heartbeat":
			var heartbeat heartbeatMessage
			if err := decodePayload(raw, msg.Payload, &heartbeat); err != nil {
				h.logger.Warn("decode agent heartbeat failed", zap.Uint("user_id", conn.UserID), zap.Error(err))
				continue
			}
			if err := conn.writeJSON(heartbeatAck{
				Type:         "heartbeat_ack",
				AgentTimeMS:  heartbeat.AgentTimeMS,
				ServerTimeMS: h.now().UnixMilli(),
			}); err != nil {
				h.logger.Warn("write agent heartbeat_ack failed", zap.Uint("user_id", conn.UserID), zap.Error(err))
				return
			}
		case "delta_report":
			var report DeltaReport
			if err := decodePayload(raw, msg.Payload, &report); err != nil {
				h.logger.Warn("decode agent delta_report failed", zap.Uint("user_id", conn.UserID), zap.Error(err))
				continue
			}
			if err := h.processDeltaReport(ctx, conn, report); err != nil {
				h.logger.Warn("process agent delta_report failed",
					zap.Uint("user_id", conn.UserID),
					zap.String("agent_id", conn.AgentID),
					zap.String("report_id", report.ReportID),
					zap.Error(err),
				)
			}
		case "command_ack":
			h.logger.Debug("agent command ack received", zap.Uint("user_id", conn.UserID))
		default:
			h.logger.Debug("unknown agent websocket message ignored", zap.Uint("user_id", conn.UserID), zap.String("type", msg.Type))
		}
	}
}

func (h *Hub) register(conn *AgentConn) {
	if conn == nil {
		return
	}
	if old, loaded := h.agents.Swap(conn.UserID, conn); loaded {
		if oldConn, ok := old.(*AgentConn); ok && oldConn != nil && oldConn != conn {
			_ = oldConn.Close()
		}
	}
}

func (h *Hub) unregister(conn *AgentConn) {
	if conn == nil {
		return
	}
	value, ok := h.agents.Load(conn.UserID)
	if !ok {
		return
	}
	if current, ok := value.(*AgentConn); ok && current == conn {
		h.agents.Delete(conn.UserID)
	}
}

func (h *Hub) findConnByAgentID(agentID string) (*AgentConn, bool) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, false
	}
	var found *AgentConn
	h.agents.Range(func(_, value any) bool {
		conn, ok := value.(*AgentConn)
		if !ok || conn == nil {
			return true
		}
		if conn.AgentID == agentID {
			found = conn
			return false
		}
		return true
	})
	return found, found != nil
}

func (c *AgentConn) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *AgentConn) writeJSON(value any) error {
	if c == nil || c.conn == nil || c.hub == nil {
		return ErrAgentNotConnected
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.conn.SetWriteDeadline(c.hub.now().Add(c.hub.writeTimeout)); err != nil {
		return err
	}
	return c.conn.WriteJSON(value)
}

func decodeAuth(raw []byte) (authRequest, error) {
	var wire wireMessage
	if err := json.Unmarshal(raw, &wire); err != nil {
		return authRequest{}, err
	}
	if wire.Type != "auth" {
		return authRequest{}, fmt.Errorf("first websocket message must be auth, got %q", wire.Type)
	}
	var req authRequest
	if err := decodePayload(raw, wire.Payload, &req); err != nil {
		return authRequest{}, err
	}
	req.Type = "auth"
	if req.token() == "" {
		return authRequest{}, errors.New("auth jwt is required")
	}
	if strings.TrimSpace(req.AgentID) == "" {
		return authRequest{}, errors.New("auth agent_id is required")
	}
	return req, nil
}

func decodePayload(raw []byte, payload json.RawMessage, out any) error {
	if len(payload) > 0 {
		return json.Unmarshal(payload, out)
	}
	return json.Unmarshal(raw, out)
}

func (r authRequest) token() string {
	if strings.TrimSpace(r.JWT) != "" {
		return strings.TrimSpace(r.JWT)
	}
	return strings.TrimSpace(r.Token)
}

func writeAuthFailure(conn *websocket.Conn, now func() time.Time, timeout time.Duration, code string, message string) error {
	if conn == nil {
		return nil
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	if timeout <= 0 {
		timeout = defaultWriteTimeout
	}
	if err := conn.SetWriteDeadline(now().Add(timeout)); err != nil {
		return err
	}
	return conn.WriteJSON(authResult{
		Type:       "auth_result",
		Success:    false,
		Code:       code,
		Error:      message,
		ServerTime: now().UnixMilli(),
	})
}

func userIDFromAgentID(agentID string) (uint, bool) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return 0, false
	}
	if strings.HasPrefix(agentID, "user:") {
		agentID = strings.TrimPrefix(agentID, "user:")
	}
	id, err := strconv.ParseUint(agentID, 10, 64)
	if err != nil || id == 0 {
		return 0, false
	}
	return uint(id), true
}
