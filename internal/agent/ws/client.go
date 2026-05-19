package ws

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"bian-trade-go/internal/agent/config"
	"bian-trade-go/internal/agent/exchange"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	agentVersion         = "0.1.0"
	initialReconnectWait = time.Second
	maxReconnectWait     = 5 * time.Minute
	heartbeatInterval    = 30 * time.Second
)

type Exchange interface {
	PlaceOrder(cmd exchange.TradeCommand) (exchange.Execution, error)
	GetBalances() ([]exchange.Balance, error)
}

type AgentClient struct {
	cfg        config.AgentConfig
	exchange   Exchange
	httpClient *http.Client
	dialer     *websocket.Dialer
	logger     *zap.Logger
	now        func() time.Time
	seen       sync.Map
}

type Config struct {
	AgentConfig config.AgentConfig
	Exchange    Exchange
	HTTPClient  *http.Client
	Dialer      *websocket.Dialer
	Logger      *zap.Logger
	Now         func() time.Time
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token       string `json:"token"`
	AccessToken string `json:"access_token"`
	JWT         string `json:"jwt"`
	Data        struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
		JWT         string `json:"jwt"`
	} `json:"data"`
}

type wireMessage struct {
	Type          string          `json:"type"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	Command       json.RawMessage `json:"command,omitempty"`
	ClientOrderID string          `json:"client_order_id,omitempty"`
	Success       *bool           `json:"success,omitempty"`
	Status        string          `json:"status,omitempty"`
	Code          string          `json:"code,omitempty"`
	Error         string          `json:"error,omitempty"`
	ErrorMessage  string          `json:"error_message,omitempty"`
}

type authMessage struct {
	Type    string `json:"type"`
	JWT     string `json:"jwt"`
	AgentID string `json:"agent_id"`
	Version string `json:"version"`
}

type commandAck struct {
	Type          string `json:"type"`
	ClientOrderID string `json:"client_order_id"`
	InstanceID    any    `json:"instance_id,omitempty"`
	Accepted      bool   `json:"accepted"`
	Duplicate     bool   `json:"duplicate"`
	AgentTimeMS   int64  `json:"agent_time_ms"`
}

type heartbeatMessage struct {
	Type        string `json:"type"`
	AgentID     string `json:"agent_id"`
	AgentTimeMS int64  `json:"agent_time_ms"`
	Version     string `json:"version"`
}

type DeltaReport struct {
	Type           string              `json:"type"`
	ReportID       string              `json:"report_id"`
	ClientOrderID  *string             `json:"client_order_id"`
	InstanceID     any                 `json:"instance_id,omitempty"`
	Symbol         string              `json:"symbol"`
	Status         string              `json:"status"`
	Execution      *exchange.Execution `json:"execution"`
	Balances       []exchange.Balance  `json:"balances"`
	ExchangeTimeMS *int64              `json:"exchange_time_ms"`
	AgentTimeMS    int64               `json:"agent_time_ms"`
	ErrorCode      *string             `json:"error_code"`
	ErrorMessage   *string             `json:"error_message"`
}

func NewAgentClient(cfg Config) (*AgentClient, error) {
	if cfg.Exchange == nil {
		return nil, errors.New("new agent client: exchange is required")
	}
	if err := cfg.AgentConfig.Validate(); err != nil {
		return nil, err
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	dialer := cfg.Dialer
	if dialer == nil {
		dialer = websocket.DefaultDialer
	}
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	now := cfg.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &AgentClient{
		cfg:        cfg.AgentConfig,
		exchange:   cfg.Exchange,
		httpClient: httpClient,
		dialer:     dialer,
		logger:     logger,
		now:        now,
	}, nil
}

func (c *AgentClient) Run(ctx context.Context) error {
	if c == nil {
		return errors.New("run agent client: client is nil")
	}
	wait := initialReconnectWait
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := c.runSession(ctx); err != nil {
			c.logger.Warn("agent websocket session ended", zap.Error(err), zap.Duration("retry_after", wait))
		} else {
			wait = initialReconnectWait
			continue
		}

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		wait *= 2
		if wait > maxReconnectWait {
			wait = maxReconnectWait
		}
	}
}

func (c *AgentClient) runSession(ctx context.Context) error {
	token, err := c.login(ctx)
	if err != nil {
		return err
	}

	conn, _, err := c.dialer.DialContext(ctx, c.wsURL(), nil)
	if err != nil {
		return fmt.Errorf("connect agent websocket: %w", err)
	}
	defer conn.Close()

	writer := &safeWriter{conn: conn}
	if err := writer.writeJSON(authMessage{
		Type:    "auth",
		JWT:     token,
		AgentID: c.agentID(),
		Version: agentVersion,
	}); err != nil {
		return fmt.Errorf("send auth message: %w", err)
	}
	if err := c.waitAuthResult(conn); err != nil {
		return err
	}
	if err := c.sendSnapshot(writer); err != nil {
		return err
	}
	return c.messageLoop(ctx, conn, writer)
}

func (c *AgentClient) login(ctx context.Context) (string, error) {
	body, err := json.Marshal(loginRequest{
		Email:    c.cfg.Email,
		Password: c.cfg.Password,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.SaaSURL+"/api/v1/auth/login", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("login SaaS: %w", err)
	}
	defer resp.Body.Close()
	rawResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("login SaaS returned HTTP %d: %s", resp.StatusCode, string(rawResp))
	}

	var decoded loginResponse
	if err := json.Unmarshal(rawResp, &decoded); err != nil {
		return "", err
	}
	token := decoded.token()
	if token == "" {
		return "", errors.New("login SaaS response did not include JWT token")
	}
	return token, nil
}

func (r loginResponse) token() string {
	for _, candidate := range []string{
		r.Token,
		r.AccessToken,
		r.JWT,
		r.Data.Token,
		r.Data.AccessToken,
		r.Data.JWT,
	} {
		if strings.TrimSpace(candidate) != "" {
			return strings.TrimSpace(candidate)
		}
	}
	return ""
}

func (c *AgentClient) waitAuthResult(conn *websocket.Conn) error {
	var msg wireMessage
	if err := conn.ReadJSON(&msg); err != nil {
		return fmt.Errorf("read auth_result: %w", err)
	}
	if msg.Type != "auth_result" {
		return fmt.Errorf("read auth_result: unexpected message type %q", msg.Type)
	}
	if msg.Error != "" || msg.ErrorMessage != "" {
		if msg.ErrorMessage != "" {
			return fmt.Errorf("agent auth failed: %s", msg.ErrorMessage)
		}
		return fmt.Errorf("agent auth failed: %s", msg.Error)
	}
	if msg.Success != nil {
		if *msg.Success {
			return nil
		}
		return errors.New("agent auth failed")
	}
	switch strings.ToLower(msg.Status) {
	case "ok", "success", "authenticated":
		return nil
	}
	switch msg.Code {
	case "00000", "OK":
		return nil
	}
	return errors.New("agent auth_result missing success confirmation")
}

func (c *AgentClient) sendSnapshot(writer *safeWriter) error {
	balances, err := c.exchange.GetBalances()
	if err != nil {
		return fmt.Errorf("load initial exchange balance snapshot: %w", err)
	}
	report := DeltaReport{
		Type:        "delta_report",
		ReportID:    newReportID("snapshot"),
		Symbol:      "ALL",
		Status:      "SNAPSHOT",
		Execution:   nil,
		Balances:    balances,
		AgentTimeMS: c.now().UnixMilli(),
	}
	return writer.writeJSON(report)
}

func (c *AgentClient) messageLoop(ctx context.Context, conn *websocket.Conn, writer *safeWriter) error {
	done := make(chan error, 1)
	go func() {
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				done <- err
				return
			}
			if err := c.handleMessage(ctx, writer, raw); err != nil {
				c.logger.Warn("handle agent websocket message failed", zap.Error(err))
			}
		}
	}()

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-done:
			return fmt.Errorf("agent websocket read loop: %w", err)
		case <-ticker.C:
			if err := writer.writeJSON(heartbeatMessage{
				Type:        "heartbeat",
				AgentID:     c.agentID(),
				AgentTimeMS: c.now().UnixMilli(),
				Version:     agentVersion,
			}); err != nil {
				return fmt.Errorf("send heartbeat: %w", err)
			}
		}
	}
}

func (c *AgentClient) handleMessage(ctx context.Context, writer *safeWriter, raw []byte) error {
	var msg wireMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return err
	}
	switch msg.Type {
	case "heartbeat_ack", "report_ack":
		return nil
	case "command":
		cmd, err := decodeCommand(raw, msg)
		if err != nil {
			return err
		}
		duplicate := false
		if _, loaded := c.seen.LoadOrStore(cmd.ClientOrderID, struct{}{}); loaded {
			duplicate = true
		}
		if err := writer.writeJSON(commandAck{
			Type:          "command_ack",
			ClientOrderID: cmd.ClientOrderID,
			InstanceID:    cmd.InstanceID,
			Accepted:      !duplicate,
			Duplicate:     duplicate,
			AgentTimeMS:   c.now().UnixMilli(),
		}); err != nil {
			return err
		}
		if duplicate {
			return nil
		}
		go c.executeCommand(ctx, writer, cmd)
		return nil
	default:
		return nil
	}
}

func (c *AgentClient) executeCommand(ctx context.Context, writer *safeWriter, cmd exchange.TradeCommand) {
	execution, err := c.exchange.PlaceOrder(cmd)
	balances, balanceErr := c.exchange.GetBalances()
	status := "FILLED"
	var errorCode *string
	var errorMessage *string
	var executionPtr *exchange.Execution
	var exchangeTimeMS *int64
	if err != nil {
		status = "FAILED"
		code := "EXCHANGE_PLACE_ORDER_FAILED"
		message := err.Error()
		errorCode = &code
		errorMessage = &message
	} else {
		executionPtr = &execution
		if execution.ExchangeTimeMS > 0 {
			exchangeTimeMS = &execution.ExchangeTimeMS
		}
	}
	if balanceErr != nil && errorMessage == nil {
		status = "FAILED"
		code := "EXCHANGE_BALANCE_SNAPSHOT_FAILED"
		message := balanceErr.Error()
		errorCode = &code
		errorMessage = &message
	}

	clientOrderID := cmd.ClientOrderID
	report := DeltaReport{
		Type:           "delta_report",
		ReportID:       newReportID("exec"),
		ClientOrderID:  &clientOrderID,
		InstanceID:     cmd.InstanceID,
		Symbol:         cmd.Symbol,
		Status:         status,
		Execution:      executionPtr,
		Balances:       balances,
		ExchangeTimeMS: exchangeTimeMS,
		AgentTimeMS:    c.now().UnixMilli(),
		ErrorCode:      errorCode,
		ErrorMessage:   errorMessage,
	}
	if writeErr := writer.writeJSON(report); writeErr != nil && !errors.Is(ctx.Err(), context.Canceled) {
		c.logger.Warn("send delta_report failed", zap.String("client_order_id", cmd.ClientOrderID), zap.Error(writeErr))
	}
}

func decodeCommand(raw []byte, msg wireMessage) (exchange.TradeCommand, error) {
	payload := msg.Payload
	if len(payload) == 0 {
		payload = msg.Command
	}
	if len(payload) == 0 {
		payload = raw
	}
	var cmd exchange.TradeCommand
	if err := json.Unmarshal(payload, &cmd); err != nil {
		return cmd, err
	}
	if cmd.ClientOrderID == "" {
		return cmd, errors.New("command message missing client_order_id")
	}
	return cmd, nil
}

func (c *AgentClient) wsURL() string {
	u, err := url.Parse(c.cfg.SaaSURL)
	if err != nil {
		return c.cfg.SaaSURL + "/ws/agent"
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	default:
		u.Scheme = "ws"
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/ws/agent"
	u.RawQuery = ""
	return u.String()
}

func (c *AgentClient) agentID() string {
	return "agent:" + c.cfg.Email
}

type safeWriter struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (w *safeWriter) writeJSON(value any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteJSON(value)
}

func newReportID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}
