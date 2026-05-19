package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"bian-trade-go/internal/adapters/backtest"
	"bian-trade-go/internal/quant"
	"bian-trade-go/internal/saas/config"
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
	StrategyID  string                   `json:"strategy_id"`
	Symbol      string                   `json:"symbol"`
	Interval    string                   `json:"interval"`
	ParamPack   json.RawMessage          `json:"param_pack"`
	GeneID      uint                     `json:"gene_id"`
	EvalStartMs int64                    `json:"eval_start_ms"`
	StartTimeMs *int64                   `json:"start_time_ms"`
	EndTimeMs   *int64                   `json:"end_time_ms"`
	Limit       int                      `json:"limit"`
	Constraints quant.TradingConstraints `json:"constraints"`
}

type backtestResultPayload struct {
	Metrics      backtest.Result `json:"metrics"`
	BarsCount    int             `json:"bars_count"`
	DataCoverage dataCoverage    `json:"data_coverage"`
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
	req.Symbol = strings.ToUpper(strings.TrimSpace(req.Symbol))
	req.Interval = strings.TrimSpace(req.Interval)

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
		req.Symbol = strings.ToUpper(strings.TrimSpace(params.SpawnPoint.Symbol))
	}
	if req.Symbol == "" {
		return example.Params{}, req, errors.New("symbol is required")
	}
	if req.Interval == "" {
		req.Interval = strings.TrimSpace(params.SpawnPoint.BaseInterval)
	}
	if req.Interval == "" {
		req.Interval = "1h"
	}
	params.SpawnPoint.Symbol = req.Symbol
	params.SpawnPoint.BaseInterval = req.Interval
	if params.SpawnPoint.QuoteAsset == "" {
		params.SpawnPoint.QuoteAsset = "USDT"
	}
	return params, req, nil
}

func (h *BacktestHandler) runBacktest(c *gin.Context, req createBacktestRequest, params example.Params) (backtestResultPayload, error) {
	bars, err := h.loadBacktestBars(c, req)
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
		Chromosome:  params.Chromosome,
		SpawnPoint:  params.SpawnPoint,
		Constraints: req.Constraints,
		Step: func(input quant.StrategyInput) quant.StrategyOutput {
			return example.Step(input, params)
		},
	})
	if err != nil {
		return backtestResultPayload{}, err
	}
	return backtestResultPayload{
		Metrics:   result,
		BarsCount: len(bars),
		DataCoverage: dataCoverage{
			StartOpenTimeMs: bars[0].OpenTime,
			EndOpenTimeMs:   bars[len(bars)-1].OpenTime,
			EvalStartMs:     evalStartMs,
			Symbol:          req.Symbol,
			Interval:        req.Interval,
		},
	}, nil
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
