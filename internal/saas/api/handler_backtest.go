package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"bian-trade-go/internal/adapters/backtest"
	"bian-trade-go/internal/quant"
	"bian-trade-go/internal/saas/config"
	"bian-trade-go/internal/saas/marketdata"
	"bian-trade-go/internal/saas/store"
	"bian-trade-go/internal/strategies/example"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type BacktestHandler struct {
	db      *gorm.DB
	appRole string
}

type createBacktestRequest struct {
	StrategyID    string                   `json:"strategy_id"`
	Symbol        string                   `json:"symbol"`
	Symbols       []string                 `json:"symbols"`
	Interval      string                   `json:"interval"`
	Direction     string                   `json:"direction"`
	DirectionMode string                   `json:"direction_mode"`
	Leverage      int                      `json:"leverage"`
	ParamPack     json.RawMessage          `json:"param_pack"`
	GeneID        uint                     `json:"gene_id"`
	EvalStartMs   int64                    `json:"eval_start_ms"`
	StartTimeMs   *int64                   `json:"start_time_ms"`
	EndTimeMs     *int64                   `json:"end_time_ms"`
	Limit         int                      `json:"limit"`
	Constraints   quant.TradingConstraints `json:"constraints"`
}

type backtestResultPayload struct {
	Metrics        backtest.Result             `json:"metrics"`
	BarsCount      int                         `json:"bars_count"`
	DataCoverage   []dataCoverage              `json:"data_coverage"`
	MarketCoverage []marketdata.CoverageReport `json:"market_coverage,omitempty"`
	SymbolResults  map[string]backtest.Result  `json:"symbol_results,omitempty"`
	Direction      string                      `json:"direction"`
	Leverage       int                         `json:"leverage"`
}

type dataCoverage struct {
	StartOpenTimeMs int64  `json:"start_open_time_ms"`
	EndOpenTimeMs   int64  `json:"end_open_time_ms"`
	EvalStartMs     int64  `json:"eval_start_ms"`
	Symbol          string `json:"symbol"`
	Interval        string `json:"interval"`
}

func NewBacktestHandler(db *gorm.DB, appRole string) *BacktestHandler {
	return &BacktestHandler{db: db, appRole: normalizeAppRole(appRole)}
}

func (h *BacktestHandler) RegisterRoutes(router gin.IRouter) {
	router.POST("/backtests", h.CreateBacktest)
	router.GET("/backtests/:id", h.GetBacktest)
}

func (h *BacktestHandler) CreateBacktest(c *gin.Context) {
	if !h.labOrDev(c) {
		return
	}
	if !requireDB(c, h.db) {
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authenticated user"})
		return
	}

	var req createBacktestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	params, req, err := h.resolveBacktestRequest(c, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	requestBlob, err := mustJSONB(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	startedAt := time.Now().UTC()
	run := store.BacktestRun{
		UserID:     userID,
		StrategyID: req.StrategyID,
		Symbol:     req.Symbol,
		Interval:   req.Interval,
		Status:     store.BacktestRunRunning,
		Request:    requestBlob,
		Result:     store.JSONB([]byte("{}")),
		StartedAt:  &startedAt,
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&run).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	payload, err := h.runBacktest(c, req, params)
	finishedAt := time.Now().UTC()
	if err != nil {
		_ = h.db.WithContext(c.Request.Context()).
			Model(&run).
			Updates(map[string]any{
				"status":      store.BacktestRunFailed,
				"error":       err.Error(),
				"finished_at": &finishedAt,
			}).Error
		run.Status = store.BacktestRunFailed
		run.Error = err.Error()
		run.FinishedAt = &finishedAt
		c.JSON(http.StatusBadRequest, gin.H{"backtest": run})
		return
	}

	resultBlob, err := mustJSONB(payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.WithContext(c.Request.Context()).
		Model(&run).
		Updates(map[string]any{
			"status":      store.BacktestRunSucceeded,
			"result":      resultBlob,
			"finished_at": &finishedAt,
		}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	run.Status = store.BacktestRunSucceeded
	run.Result = resultBlob
	run.FinishedAt = &finishedAt
	if err := h.persistBacktestArtifacts(c, run.ID, req, payload, startedAt, finishedAt); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"backtest": run})
}

func (h *BacktestHandler) GetBacktest(c *gin.Context) {
	if !h.labOrDev(c) {
		return
	}
	if !requireDB(c, h.db) {
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authenticated user"})
		return
	}
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var run store.BacktestRun
	err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND user_id = ?", id, userID).
		First(&run).Error
	if err != nil {
		c.JSON(statusForDBError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"backtest": run})
}

func (h *BacktestHandler) resolveBacktestRequest(c *gin.Context, req createBacktestRequest) (example.Params, createBacktestRequest, error) {
	req.StrategyID = strings.TrimSpace(req.StrategyID)
	req.Symbol = marketdata.NormalizeSymbol(req.Symbol)
	req.Interval = marketdata.NormalizeInterval(req.Interval)
	if req.DirectionMode == "" {
		req.DirectionMode = req.Direction
	}
	req.DirectionMode = strings.ToLower(strings.TrimSpace(req.DirectionMode))
	if req.DirectionMode == "" {
		req.DirectionMode = string(backtest.DirectionLong)
	}
	switch backtest.DirectionMode(req.DirectionMode) {
	case backtest.DirectionLong, backtest.DirectionShort, backtest.DirectionLongShort:
	default:
		return example.Params{}, req, fmt.Errorf("unsupported direction_mode %q", req.DirectionMode)
	}
	leverage, err := marketdata.ValidateLeverage(req.Leverage)
	if err != nil {
		return example.Params{}, req, err
	}
	req.Leverage = leverage
	if len(req.Symbols) > 0 {
		seen := make(map[string]struct{}, len(req.Symbols))
		normalized := make([]string, 0, len(req.Symbols))
		for _, symbol := range req.Symbols {
			symbol = marketdata.NormalizeSymbol(symbol)
			if symbol == "" {
				continue
			}
			if _, ok := seen[symbol]; ok {
				continue
			}
			seen[symbol] = struct{}{}
			normalized = append(normalized, symbol)
		}
		req.Symbols = normalized
	}

	if req.GeneID != 0 {
		var gene store.GeneRecord
		if err := h.db.WithContext(c.Request.Context()).First(&gene, req.GeneID).Error; err != nil {
			return example.Params{}, req, fmt.Errorf("load gene_id %d: %w", req.GeneID, err)
		}
		if req.StrategyID != "" && req.StrategyID != gene.StrategyID {
			return example.Params{}, req, errors.New("gene_id strategy_id does not match request")
		}
		if req.Symbol != "" && req.Symbol != gene.Symbol {
			return example.Params{}, req, errors.New("gene_id symbol does not match request")
		}
		req.StrategyID = gene.StrategyID
		req.Symbol = gene.Symbol
		req.ParamPack = append(req.ParamPack[:0], gene.ParamPack...)
	}

	if req.StrategyID == "" {
		req.StrategyID = example.StrategyID
	}
	if req.StrategyID != example.StrategyID {
		return example.Params{}, req, fmt.Errorf("unsupported strategy %q: only %q is registered", req.StrategyID, example.StrategyID)
	}
	if len(req.ParamPack) == 0 {
		return example.Params{}, req, errors.New("param_pack or gene_id is required")
	}
	if !json.Valid(req.ParamPack) {
		return example.Params{}, req, errors.New("param_pack must be valid JSON")
	}

	params, err := example.ParseParamPack(req.ParamPack)
	if err != nil {
		return example.Params{}, req, fmt.Errorf("parse param_pack: %w", err)
	}
	if req.Symbol == "" {
		req.Symbol = marketdata.NormalizeSymbol(params.SpawnPoint.Symbol)
	}
	if req.Symbol == "" && len(req.Symbols) > 0 {
		req.Symbol = req.Symbols[0]
	}
	if req.Symbol == "" {
		return example.Params{}, req, errors.New("symbol is required")
	}
	if len(req.Symbols) == 0 {
		req.Symbols = []string{req.Symbol}
	}
	if req.Interval == "" {
		req.Interval = marketdata.NormalizeInterval(params.SpawnPoint.BaseInterval)
	}
	if req.Interval == "" {
		req.Interval = "1h"
	}
	if _, ok := marketdata.IntervalDuration(req.Interval); !ok {
		return example.Params{}, req, fmt.Errorf("unsupported interval %q", req.Interval)
	}
	params.SpawnPoint.Symbol = req.Symbol
	params.SpawnPoint.BaseInterval = req.Interval
	if params.SpawnPoint.QuoteAsset == "" {
		params.SpawnPoint.QuoteAsset = "USDT"
	}
	return params, req, nil
}

func (h *BacktestHandler) runBacktest(c *gin.Context, req createBacktestRequest, params example.Params) (backtestResultPayload, error) {
	coverageReports, err := h.ensureMarketData(c, req)
	if err != nil {
		return backtestResultPayload{}, err
	}
	symbolResults := make(map[string]backtest.Result, len(req.Symbols))
	dataCoverages := make([]dataCoverage, 0, len(req.Symbols))
	totalBars := 0
	var combined backtest.Result
	for _, symbol := range req.Symbols {
		symbolReq := req
		symbolReq.Symbol = symbol
		symbolParams := params
		symbolParams.SpawnPoint.Symbol = symbol
		bars, err := h.loadBacktestBars(c, symbolReq)
		if err != nil {
			return backtestResultPayload{}, err
		}
		evalStartMs := req.EvalStartMs
		if evalStartMs == 0 {
			evalStartMs = bars[0].OpenTime
		}
		result, err := backtest.RunBacktest(c.Request.Context(), backtest.Config{
			Bars:        bars,
			EvalStartMs: evalStartMs,
			Chromosome:  symbolParams.Chromosome,
			SpawnPoint:  symbolParams.SpawnPoint,
			Constraints: req.Constraints,
			Leverage:    req.Leverage,
			Direction:   backtest.DirectionMode(req.DirectionMode),
			Step: func(input quant.StrategyInput) quant.StrategyOutput {
				return example.Step(input, symbolParams)
			},
		})
		if err != nil {
			return backtestResultPayload{}, err
		}
		symbolResults[symbol] = result
		combined = combineBacktestResults(combined, result)
		totalBars += len(bars)
		dataCoverages = append(dataCoverages, dataCoverage{
			StartOpenTimeMs: bars[0].OpenTime,
			EndOpenTimeMs:   bars[len(bars)-1].OpenTime,
			EvalStartMs:     evalStartMs,
			Symbol:          symbol,
			Interval:        req.Interval,
		})
	}
	if len(symbolResults) > 0 && combined.TotalInjected > 0 {
		combined.ROI = (combined.FinalEquity - combined.TotalInjected) / combined.TotalInjected
	}
	return backtestResultPayload{
		Metrics:        combined,
		BarsCount:      totalBars,
		DataCoverage:   dataCoverages,
		MarketCoverage: coverageReports,
		SymbolResults:  symbolResults,
		Direction:      req.DirectionMode,
		Leverage:       req.Leverage,
	}, nil
}

func combineBacktestResults(left backtest.Result, right backtest.Result) backtest.Result {
	left.FinalEquity += right.FinalEquity
	left.TotalInjected += right.TotalInjected
	if right.MaxDrawdown > left.MaxDrawdown {
		left.MaxDrawdown = right.MaxDrawdown
	}
	left.RealizedPnL += right.RealizedPnL
	left.Fees += right.Fees
	left.TradeCount += right.TradeCount
	left.Orders = append(left.Orders, right.Orders...)
	left.Positions = append(left.Positions, right.Positions...)
	if left.TradeCount > 0 {
		weightedWins := left.WinRate*float64(left.TradeCount-right.TradeCount) + right.WinRate*float64(right.TradeCount)
		left.WinRate = weightedWins / float64(left.TradeCount)
	}
	if right.ProfitLossRatio > left.ProfitLossRatio {
		left.ProfitLossRatio = right.ProfitLossRatio
	}
	if right.Sharpe > left.Sharpe {
		left.Sharpe = right.Sharpe
	}
	return left
}

func (h *BacktestHandler) ensureMarketData(c *gin.Context, req createBacktestRequest) ([]marketdata.CoverageReport, error) {
	if req.StartTimeMs == nil || req.EndTimeMs == nil {
		return nil, nil
	}
	start := time.UnixMilli(*req.StartTimeMs).UTC()
	end := time.UnixMilli(*req.EndTimeMs).UTC()
	reports := make([]marketdata.CoverageReport, 0, len(req.Symbols))
	for _, symbol := range req.Symbols {
		report, err := marketdata.EnsureClosedKLines(c.Request.Context(), h.db, marketdata.EnsureOptions{
			Symbol:   symbol,
			Interval: req.Interval,
			Start:    start,
			End:      end,
		})
		if err != nil {
			return reports, err
		}
		reports = append(reports, report)
	}
	return reports, nil
}

func (h *BacktestHandler) persistBacktestArtifacts(c *gin.Context, runID uint, req createBacktestRequest, payload backtestResultPayload, startedAt time.Time, finishedAt time.Time) error {
	if h.db == nil {
		return nil
	}
	paramSnap, err := mustJSONB(req)
	if err != nil {
		return err
	}
	frozenMargin := 0.0
	for _, position := range payload.Metrics.Positions {
		frozenMargin += position.UsedMargin
	}
	return h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		account := store.BTAccount{
			BacktestRunID:    runID,
			InitialEquity:    decimalFromFloat(payload.Metrics.TotalInjected),
			CurrentEquity:    decimalFromFloat(payload.Metrics.FinalEquity),
			AvailableBalance: decimalFromFloat(payload.Metrics.FinalEquity - frozenMargin),
			FrozenMargin:     decimalFromFloat(frozenMargin),
			RealizedPnL:      decimalFromFloat(payload.Metrics.RealizedPnL),
		}
		if err := tx.Create(&account).Error; err != nil {
			return err
		}
		if len(payload.Metrics.Orders) > 0 {
			orders := make([]store.BTOrder, 0, len(payload.Metrics.Orders))
			for _, order := range payload.Metrics.Orders {
				orders = append(orders, store.BTOrder{
					BacktestRunID:  runID,
					Symbol:         order.Symbol,
					Side:           order.Side,
					OffsetFlag:     store.BacktestOffset(order.Offset),
					PositionSide:   store.BacktestPositionSide(order.PositionSide),
					Leverage:       order.Leverage,
					OrderPrice:     decimalFromFloat(order.OrderPrice),
					ExecutedPrice:  decimalFromFloat(order.ExecutedPrice),
					ExecutedQty:    decimalFromFloat(order.ExecutedQty),
					Fee:            decimalFromFloat(order.Fee),
					Status:         order.Status,
					OrderedAt:      time.UnixMilli(order.OrderedAtMs).UTC(),
					ReasonCode:     order.ReasonCode,
					RealizedPnL:    decimalFromFloat(order.RealizedPnL),
					MarginReleased: decimalFromFloat(order.MarginReleased),
				})
			}
			if err := tx.Create(&orders).Error; err != nil {
				return err
			}
		}
		if len(payload.Metrics.Positions) > 0 {
			positions := make([]store.BTPosition, 0, len(payload.Metrics.Positions))
			for _, position := range payload.Metrics.Positions {
				positions = append(positions, store.BTPosition{
					BacktestRunID:   runID,
					Symbol:          position.Symbol,
					PositionSide:    store.BacktestPositionSide(position.PositionSide),
					AverageEntry:    decimalFromFloat(position.AverageEntry),
					Quantity:        decimalFromFloat(position.Quantity),
					Leverage:        position.Leverage,
					UsedMargin:      decimalFromFloat(position.UsedMargin),
					UnrealizedPnL:   decimalFromFloat(position.UnrealizedPnL),
					LiquidationHint: decimalFromFloat(position.LiquidationHint),
				})
			}
			if err := tx.Create(&positions).Error; err != nil {
				return err
			}
		}
		logRow := store.BTTradeLog{
			BacktestRunID: runID,
			Timestamp:     finishedAt,
			Symbol:        strings.Join(req.Symbols, ","),
			Level:         "info",
			TriggerDetail: fmt.Sprintf("backtest completed: symbols=%d bars=%d direction=%s leverage=%dx", len(req.Symbols), payload.BarsCount, req.DirectionMode, req.Leverage),
		}
		if err := tx.Create(&logRow).Error; err != nil {
			return err
		}
		report := store.BTReport{
			BacktestRunID:   runID,
			ParameterSnap:   paramSnap,
			TotalReturn:     decimalFromFloat(payload.Metrics.ROI),
			MaxDrawdown:     decimalFromFloat(payload.Metrics.MaxDrawdown),
			WinRate:         decimalFromFloat(payload.Metrics.WinRate),
			ProfitLossRatio: decimalFromFloat(payload.Metrics.ProfitLossRatio),
			SharpeRatio:     decimalFromFloat(payload.Metrics.Sharpe),
			TradeCount:      payload.Metrics.TradeCount,
			ExecutionMs:     finishedAt.Sub(startedAt).Milliseconds(),
		}
		return tx.Create(&report).Error
	})
}

func decimalFromFloat(value float64) store.Decimal {
	if !isFinite(value) {
		return store.Decimal("0")
	}
	return store.Decimal(fmt.Sprintf("%.12f", value))
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func (h *BacktestHandler) loadBacktestBars(c *gin.Context, req createBacktestRequest) ([]quant.Bar, error) {
	query := h.db.WithContext(c.Request.Context()).
		Where("symbol = ? AND interval = ?", req.Symbol, req.Interval)
	if req.StartTimeMs != nil {
		query = query.Where("open_time >= ?", time.UnixMilli(*req.StartTimeMs).UTC())
	}
	if req.EndTimeMs != nil {
		query = query.Where("open_time <= ?", time.UnixMilli(*req.EndTimeMs).UTC())
	}

	var rows []store.KLine
	if req.Limit > 0 {
		limit := req.Limit
		if limit > 200000 {
			limit = 200000
		}
		query = query.Order("open_time DESC").Limit(limit)
	} else {
		query = query.Order("open_time ASC")
	}
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load backtest klines: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("load backtest klines: no bars for %s %s", req.Symbol, req.Interval)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].OpenTime.Before(rows[j].OpenTime) })

	bars := make([]quant.Bar, 0, len(rows))
	for _, row := range rows {
		open, err := decimalToFloat(row.Open)
		if err != nil {
			return nil, fmt.Errorf("parse open for kline %d: %w", row.ID, err)
		}
		high, err := decimalToFloat(row.High)
		if err != nil {
			return nil, fmt.Errorf("parse high for kline %d: %w", row.ID, err)
		}
		low, err := decimalToFloat(row.Low)
		if err != nil {
			return nil, fmt.Errorf("parse low for kline %d: %w", row.ID, err)
		}
		closePrice, err := decimalToFloat(row.Close)
		if err != nil {
			return nil, fmt.Errorf("parse close for kline %d: %w", row.ID, err)
		}
		volume, err := decimalToFloat(row.Volume)
		if err != nil {
			return nil, fmt.Errorf("parse volume for kline %d: %w", row.ID, err)
		}
		if row.OpenTime.IsZero() || closePrice <= 0 {
			continue
		}
		bars = append(bars, quant.Bar{
			OpenTime: row.OpenTime.UnixMilli(),
			Open:     open,
			High:     high,
			Low:      low,
			Close:    closePrice,
			Volume:   volume,
		})
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("load backtest klines: no usable closed bars for %s %s", req.Symbol, req.Interval)
	}
	return bars, nil
}

func (h *BacktestHandler) labOrDev(c *gin.Context) bool {
	switch normalizeAppRole(h.appRole) {
	case config.AppRoleLab, config.AppRoleDev:
		return true
	default:
		c.JSON(http.StatusForbidden, gin.H{"error": "backtest writes are only available in lab/dev mode"})
		return false
	}
}
