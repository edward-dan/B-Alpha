package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"bian-trade-go/internal/saas/store"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	auditAgentDeltaReport = "AGENT_DELTA_REPORT_PROCESSED"
	auditAgentSnapshot    = "AGENT_BALANCE_SNAPSHOT_PROCESSED"
)

type reportAck struct {
	Type          string `json:"type"`
	ReportID      string `json:"report_id"`
	ClientOrderID string `json:"client_order_id,omitempty"`
	Success       bool   `json:"success"`
	ServerTimeMS  int64  `json:"server_time_ms"`
}

func (h *Hub) processDeltaReport(ctx context.Context, conn *AgentConn, report DeltaReport) error {
	if h == nil || h.db == nil {
		return ErrHubNotReady
	}
	if conn == nil {
		return ErrAgentNotConnected
	}
	report.ReportID = strings.TrimSpace(report.ReportID)
	if report.ReportID == "" {
		return errors.New("delta_report report_id is required")
	}

	clientOrderID := report.clientOrderID()
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		duplicate, err := reportAlreadyProcessed(tx, report.ReportID)
		if err != nil {
			return err
		}
		if duplicate {
			return nil
		}
		if clientOrderID == "" {
			return h.processBalanceSnapshot(tx, conn, report)
		}
		return h.processExecutionReport(tx, conn, clientOrderID, report)
	})
	if err != nil {
		return err
	}
	return conn.writeJSON(reportAck{
		Type:          "report_ack",
		ReportID:      report.ReportID,
		ClientOrderID: clientOrderID,
		Success:       true,
		ServerTimeMS:  h.now().UnixMilli(),
	})
}

func (h *Hub) processExecutionReport(tx *gorm.DB, conn *AgentConn, clientOrderID string, report DeltaReport) error {
	var execution store.SpotExecution
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("client_order_id = ?", clientOrderID).
		First(&execution).Error; err != nil {
		return fmt.Errorf("load spot execution %s: %w", clientOrderID, err)
	}

	var instance store.StrategyInstance
	if err := tx.Select("id", "user_id").First(&instance, execution.StrategyInstanceID).Error; err != nil {
		return fmt.Errorf("load strategy instance %d: %w", execution.StrategyInstanceID, err)
	}
	if instance.UserID != conn.UserID {
		return fmt.Errorf("delta_report user mismatch for %s", clientOrderID)
	}

	processedNow := false
	if execution.Status == store.SpotExecutionPending {
		status, err := reportExecutionStatus(report)
		if err != nil {
			return err
		}
		payload, err := jsonb(report)
		if err != nil {
			return err
		}
		updates := map[string]any{
			"status":            status,
			"execution_payload": payload,
			"error_message":     report.errorMessage(),
		}
		if status == store.SpotExecutionFilled {
			filledAt := reportTime(report, h.now)
			updates["filled_at"] = &filledAt
		} else {
			failedAt := reportTime(report, h.now)
			updates["failed_at"] = &failedAt
		}
		if err := tx.Model(&execution).Updates(updates).Error; err != nil {
			return fmt.Errorf("update spot execution %s: %w", clientOrderID, err)
		}
		execution.Status = status
		if status == store.SpotExecutionFilled {
			if err := applyFilledExecution(tx, &execution, report); err != nil {
				return err
			}
			if err := createTradeRecord(tx, execution, report); err != nil {
				return err
			}
		}
		processedNow = true
	}

	if err := updatePortfolioBalanceSnapshot(tx, execution.StrategyInstanceID, report); err != nil {
		return err
	}
	return writeAudit(tx, auditAgentDeltaReport, &execution.StrategyInstanceID, &conn.UserID, report.ReportID, map[string]any{
		"report_id":       report.ReportID,
		"client_order_id": clientOrderID,
		"status":          strings.ToUpper(strings.TrimSpace(report.Status)),
		"processed_now":   processedNow,
		"agent_id":        conn.AgentID,
		"agent_time_ms":   report.AgentTimeMS,
	})
}

func (h *Hub) processBalanceSnapshot(tx *gorm.DB, conn *AgentConn, report DeltaReport) error {
	instanceID, hasInstanceID := instanceIDFromValue(report.InstanceID)
	if hasInstanceID {
		var instance store.StrategyInstance
		if err := tx.Select("id", "user_id").First(&instance, instanceID).Error; err != nil {
			return fmt.Errorf("load snapshot strategy instance %d: %w", instanceID, err)
		}
		if instance.UserID != conn.UserID {
			return fmt.Errorf("delta_report snapshot user mismatch for instance %d", instanceID)
		}
		if err := updatePortfolioBalanceSnapshot(tx, instanceID, report); err != nil {
			return err
		}
		return writeAudit(tx, auditAgentSnapshot, &instanceID, &conn.UserID, report.ReportID, map[string]any{
			"report_id":     report.ReportID,
			"status":        strings.ToUpper(strings.TrimSpace(report.Status)),
			"agent_id":      conn.AgentID,
			"agent_time_ms": report.AgentTimeMS,
		})
	}

	var instances []store.StrategyInstance
	if err := tx.Select("id", "user_id").
		Where("user_id = ? AND status <> ?", conn.UserID, store.StrategyInstanceDeleted).
		Find(&instances).Error; err != nil {
		return fmt.Errorf("load user snapshot strategy instances: %w", err)
	}
	for _, instance := range instances {
		if err := updatePortfolioBalanceSnapshot(tx, instance.ID, report); err != nil {
			return err
		}
	}
	return writeAudit(tx, auditAgentSnapshot, nil, &conn.UserID, report.ReportID, map[string]any{
		"report_id":         report.ReportID,
		"status":            strings.ToUpper(strings.TrimSpace(report.Status)),
		"agent_id":          conn.AgentID,
		"agent_time_ms":     report.AgentTimeMS,
		"updated_instances": len(instances),
		"snapshot_scope":    "user",
	})
}

func reportAlreadyProcessed(tx *gorm.DB, reportID string) (bool, error) {
	var count int64
	if err := tx.Model(&store.AuditLog{}).
		Where("trace_id = ? AND event_type IN ?", reportID, []string{auditAgentDeltaReport, auditAgentSnapshot}).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("check duplicate delta_report %s: %w", reportID, err)
	}
	return count > 0, nil
}

func applyFilledExecution(tx *gorm.DB, execution *store.SpotExecution, report DeltaReport) error {
	qty, err := report.filledQty()
	if err != nil {
		return err
	}
	price, err := report.filledPrice()
	if err != nil {
		return err
	}
	lotType := store.SpotLotType(strings.ToUpper(strings.TrimSpace(executionLotType(*execution))))
	if lotType != store.SpotLotDeadStack && lotType != store.SpotLotFloating {
		return fmt.Errorf("unsupported filled lot_type %q", lotType)
	}

	var portfolio store.PortfolioState
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("strategy_instance_id = ?", execution.StrategyInstanceID).
		First(&portfolio).Error; err != nil {
		return fmt.Errorf("load portfolio state: %w", err)
	}

	action := strings.ToUpper(strings.TrimSpace(execution.Action))
	switch action {
	case "BUY":
		if err := addSpotLot(tx, execution.StrategyInstanceID, lotType, qty, price); err != nil {
			return err
		}
		return applyPortfolioBucketDelta(tx, &portfolio, lotType, qty)
	case "SELL":
		if err := reduceSpotLots(tx, execution.StrategyInstanceID, lotType, qty); err != nil {
			return err
		}
		return applyPortfolioBucketDelta(tx, &portfolio, lotType, -qty)
	default:
		return fmt.Errorf("unsupported spot execution action %q", execution.Action)
	}
}

func addSpotLot(tx *gorm.DB, instanceID uint, lotType store.SpotLotType, qty float64, price float64) error {
	row := store.SpotLot{
		StrategyInstanceID: instanceID,
		LotType:            lotType,
		Amount:             decimal(qty),
		CostPrice:          decimal(price),
		IsColdSealed:       false,
	}
	return tx.Create(&row).Error
}

func reduceSpotLots(tx *gorm.DB, instanceID uint, lotType store.SpotLotType, qty float64) error {
	remaining := qty
	var lots []store.SpotLot
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("strategy_instance_id = ? AND lot_type = ? AND is_cold_sealed = ? AND amount > 0", instanceID, lotType, false).
		Order("created_at ASC").
		Find(&lots).Error; err != nil {
		return fmt.Errorf("load sellable lots: %w", err)
	}
	for _, lot := range lots {
		if remaining <= 0 {
			break
		}
		amount, err := decimalFloat(lot.Amount)
		if err != nil {
			return fmt.Errorf("parse lot %d amount: %w", lot.ID, err)
		}
		used := math.Min(amount, remaining)
		if used <= 0 {
			continue
		}
		if nearlyEqual(used, amount) {
			if err := tx.Model(&lot).Update("amount", decimal(0)).Error; err != nil {
				return fmt.Errorf("clear lot %d amount: %w", lot.ID, err)
			}
		} else if err := tx.Model(&lot).Update("amount", decimal(amount-used)).Error; err != nil {
			return fmt.Errorf("reduce lot %d amount: %w", lot.ID, err)
		}
		remaining -= used
	}
	if remaining > 1e-12 {
		return fmt.Errorf("insufficient %s lots for sell qty %.12f", lotType, qty)
	}
	return nil
}

func applyPortfolioBucketDelta(tx *gorm.DB, portfolio *store.PortfolioState, lotType store.SpotLotType, delta float64) error {
	switch lotType {
	case store.SpotLotDeadStack:
		current, err := decimalFloat(portfolio.DeadBTC)
		if err != nil {
			return err
		}
		portfolio.DeadBTC = decimal(math.Max(0, current+delta))
		return tx.Model(portfolio).Update("dead_btc", portfolio.DeadBTC).Error
	case store.SpotLotFloating:
		current, err := decimalFloat(portfolio.FloatBTC)
		if err != nil {
			return err
		}
		portfolio.FloatBTC = decimal(math.Max(0, current+delta))
		return tx.Model(portfolio).Update("float_btc", portfolio.FloatBTC).Error
	default:
		return fmt.Errorf("unsupported portfolio lot_type %q", lotType)
	}
}

func updatePortfolioBalanceSnapshot(tx *gorm.DB, instanceID uint, report DeltaReport) error {
	if instanceID == 0 {
		return nil
	}
	usdt, ok, err := availableBalance(report.Balances, "USDT")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	var portfolio store.PortfolioState
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("strategy_instance_id = ?", instanceID).
		First(&portfolio).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load portfolio balance snapshot: %w", err)
	}
	updates := map[string]any{
		"usdt_balance": decimal(usdt),
	}
	if price, err := report.filledPrice(); err == nil && price > 0 {
		dead, _ := decimalFloat(portfolio.DeadBTC)
		floatBTC, _ := decimalFloat(portfolio.FloatBTC)
		cold, _ := decimalFloat(portfolio.ColdSealedBTC)
		updates["total_equity"] = decimal(usdt + (dead+floatBTC+cold)*price)
	}
	return tx.Model(&portfolio).Updates(updates).Error
}

func createTradeRecord(tx *gorm.DB, execution store.SpotExecution, report DeltaReport) error {
	qty, err := report.filledQty()
	if err != nil {
		return err
	}
	price, err := report.filledPrice()
	if err != nil {
		return err
	}
	fee, err := decimalStringFloat(report.feeString())
	if err != nil {
		return err
	}
	executedAt := reportTime(report, nil)
	record := store.TradeRecord{
		StrategyInstanceID: execution.StrategyInstanceID,
		ClientOrderID:      execution.ClientOrderID,
		Action:             execution.Action,
		Engine:             execution.Engine,
		Symbol:             execution.Symbol,
		FilledQty:          decimal(qty),
		FilledPrice:        decimal(price),
		Fee:                decimal(fee),
		ExecutedAt:         &executedAt,
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&record).Error
}

func reportExecutionStatus(report DeltaReport) (store.SpotExecutionStatus, error) {
	switch strings.ToUpper(strings.TrimSpace(report.Status)) {
	case "FILLED", "PARTIALLY_FILLED":
		return store.SpotExecutionFilled, nil
	case "REJECTED", "EXPIRED", "FAILED":
		return store.SpotExecutionFailed, nil
	default:
		return "", fmt.Errorf("unsupported delta_report status %q", report.Status)
	}
}

func (r DeltaReport) clientOrderID() string {
	if r.ClientOrderID == nil {
		return ""
	}
	return strings.TrimSpace(*r.ClientOrderID)
}

func (r DeltaReport) errorMessage() string {
	if r.ErrorMessage != nil && strings.TrimSpace(*r.ErrorMessage) != "" {
		return strings.TrimSpace(*r.ErrorMessage)
	}
	if r.ErrorCode != nil {
		return strings.TrimSpace(*r.ErrorCode)
	}
	return ""
}

func (r DeltaReport) filledQty() (float64, error) {
	if r.Execution == nil {
		return 0, errors.New("filled delta_report execution is required")
	}
	qty, err := decimalStringFloat(r.Execution.FilledQty)
	if err == nil && qty > 0 {
		return qty, nil
	}
	amount, amountErr := decimalStringFloat(r.Execution.FilledAmount)
	price, priceErr := decimalStringFloat(r.Execution.FilledPrice)
	if amountErr == nil && priceErr == nil && amount > 0 && price > 0 {
		return amount / price, nil
	}
	if err != nil {
		return 0, fmt.Errorf("parse filled qty: %w", err)
	}
	return 0, errors.New("filled qty must be positive")
}

func (r DeltaReport) filledPrice() (float64, error) {
	if r.Execution == nil {
		return 0, errors.New("filled delta_report execution is required")
	}
	price, err := decimalStringFloat(r.Execution.FilledPrice)
	if err == nil && price > 0 {
		return price, nil
	}
	amount, amountErr := decimalStringFloat(r.Execution.FilledAmount)
	qty, qtyErr := decimalStringFloat(r.Execution.FilledQty)
	if amountErr == nil && qtyErr == nil && amount > 0 && qty > 0 {
		return amount / qty, nil
	}
	if err != nil {
		return 0, fmt.Errorf("parse filled price: %w", err)
	}
	return 0, errors.New("filled price must be positive")
}

func (r DeltaReport) feeString() string {
	if r.Execution == nil {
		return ""
	}
	return r.Execution.Fee
}

func reportTime(report DeltaReport, now func() time.Time) time.Time {
	if report.ExchangeTimeMS != nil && *report.ExchangeTimeMS > 0 {
		return time.UnixMilli(*report.ExchangeTimeMS).UTC()
	}
	if report.Execution != nil && report.Execution.ExchangeTimeMS > 0 {
		return time.UnixMilli(report.Execution.ExchangeTimeMS).UTC()
	}
	if report.AgentTimeMS > 0 {
		return time.UnixMilli(report.AgentTimeMS).UTC()
	}
	if now == nil {
		now = time.Now
	}
	return now().UTC()
}

func availableBalance(balances []Balance, asset string) (float64, bool, error) {
	asset = strings.ToUpper(strings.TrimSpace(asset))
	for _, balance := range balances {
		if strings.ToUpper(strings.TrimSpace(balance.Asset)) != asset {
			continue
		}
		value, err := decimalStringFloat(balance.Available)
		if err != nil {
			return 0, false, fmt.Errorf("parse %s available balance: %w", asset, err)
		}
		return value, true, nil
	}
	return 0, false, nil
}

func instanceIDFromValue(value any) (uint, bool) {
	switch v := value.(type) {
	case nil:
		return 0, false
	case uint:
		return v, v > 0
	case uint64:
		return uint(v), v > 0
	case int:
		return uint(v), v > 0
	case int64:
		return uint(v), v > 0
	case float64:
		if v <= 0 || math.Trunc(v) != v {
			return 0, false
		}
		return uint(v), true
	case json.Number:
		parsed, err := strconv.ParseUint(v.String(), 10, 64)
		return uint(parsed), err == nil && parsed > 0
	case string:
		parsed, err := strconv.ParseUint(strings.TrimSpace(v), 10, 64)
		return uint(parsed), err == nil && parsed > 0
	default:
		return 0, false
	}
}

func jsonb(value any) (store.JSONB, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || !json.Valid(raw) {
		return nil, errors.New("jsonb payload is not valid JSON")
	}
	return store.JSONB(raw), nil
}

func writeAudit(tx *gorm.DB, eventType string, instanceID *uint, userID *uint, traceID string, payload any) error {
	body, err := jsonb(payload)
	if err != nil {
		return err
	}
	return tx.Create(&store.AuditLog{
		EventType:          eventType,
		Payload:            body,
		UserID:             userID,
		StrategyInstanceID: instanceID,
		TraceID:            traceID,
	}).Error
}

func decimal(value float64) store.Decimal {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value) < 1e-18 {
		return store.Decimal("0")
	}
	return store.Decimal(strconv.FormatFloat(value, 'f', -1, 64))
}

func decimalFloat(value store.Decimal) (float64, error) {
	return decimalStringFloat(string(value))
}

func decimalStringFloat(value string) (float64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, fmt.Errorf("invalid decimal %q", value)
	}
	return parsed, nil
}

func nearlyEqual(a float64, b float64) bool {
	return math.Abs(a-b) <= 1e-12
}

func executionLotType(e store.SpotExecution) string {
	var command TradeCommand
	if len(e.CommandPayload) == 0 {
		return ""
	}
	if err := json.Unmarshal(e.CommandPayload, &command); err != nil {
		return ""
	}
	return command.LotType
}
