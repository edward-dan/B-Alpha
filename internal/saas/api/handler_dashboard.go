package api

import (
	"net/http"
	"strconv"
	"time"

	"bian-trade-go/internal/saas/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type DashboardHandler struct {
	db *gorm.DB
}

func NewDashboardHandler(db *gorm.DB) *DashboardHandler {
	return &DashboardHandler{db: db}
}

func (h *DashboardHandler) RegisterRoutes(router gin.IRouter) {
	router.GET("/dashboard", h.GetDashboard)
	router.GET("/dashboard/equity-snapshots", h.GetEquitySnapshots)
}

func (h *DashboardHandler) GetDashboard(c *gin.Context) {
	if !requireDB(c, h.db) {
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authenticated user"})
		return
	}

	var instances []store.StrategyInstance
	if err := h.db.WithContext(c.Request.Context()).
		Preload("Portfolio").
		Where("user_id = ? AND status <> ?", userID, store.StrategyInstanceDeleted).
		Order("id ASC").
		Find(&instances).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	overview := gin.H{
		"user_id":          userID,
		"instances_total":  len(instances),
		"running_count":    0,
		"stopped_count":    0,
		"error_count":      0,
		"total_equity":     0.0,
		"usdt_balance":     0.0,
		"dead_btc":         0.0,
		"float_btc":        0.0,
		"cold_sealed_btc":  0.0,
		"pending_commands": int64(0),
	}

	instanceIDs := make([]uint, 0, len(instances))
	for _, inst := range instances {
		instanceIDs = append(instanceIDs, inst.ID)
		switch inst.Status {
		case store.StrategyInstanceRunning:
			overview["running_count"] = overview["running_count"].(int) + 1
		case store.StrategyInstanceStopped:
			overview["stopped_count"] = overview["stopped_count"].(int) + 1
		case store.StrategyInstanceError:
			overview["error_count"] = overview["error_count"].(int) + 1
		}
		if inst.Portfolio == nil {
			continue
		}
		addDecimal(overview, "total_equity", inst.Portfolio.TotalEquity)
		addDecimal(overview, "usdt_balance", inst.Portfolio.USDTBalance)
		addDecimal(overview, "dead_btc", inst.Portfolio.DeadBTC)
		addDecimal(overview, "float_btc", inst.Portfolio.FloatBTC)
		addDecimal(overview, "cold_sealed_btc", inst.Portfolio.ColdSealedBTC)
	}

	var recentTrades []store.TradeRecord
	if len(instanceIDs) > 0 {
		var pending int64
		if err := h.db.WithContext(c.Request.Context()).
			Model(&store.SpotExecution{}).
			Where("strategy_instance_id IN ? AND status = ?", instanceIDs, store.SpotExecutionPending).
			Count(&pending).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		overview["pending_commands"] = pending

		if err := h.db.WithContext(c.Request.Context()).
			Where("strategy_instance_id IN ?", instanceIDs).
			Order("executed_at DESC NULLS LAST, created_at DESC").
			Limit(20).
			Find(&recentTrades).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"overview":      overview,
		"instances":     instances,
		"recent_trades": recentTrades,
	})
}

func (h *DashboardHandler) GetEquitySnapshots(c *gin.Context) {
	if !requireDB(c, h.db) {
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authenticated user"})
		return
	}
	instanceID, err := strconv.ParseUint(c.Query("instance_id"), 10, 64)
	if err != nil || instanceID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "instance_id is required"})
		return
	}
	days := 30
	if rawDays := c.Query("days"); rawDays != "" {
		parsed, parseErr := strconv.Atoi(rawDays)
		if parseErr != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid days"})
			return
		}
		if parsed < days {
			days = parsed
		}
		if parsed > 90 {
			days = 90
		}
	}

	var inst store.StrategyInstance
	err = h.db.WithContext(c.Request.Context()).
		Preload("Portfolio").
		Where("id = ? AND user_id = ? AND status <> ?", uint(instanceID), userID, store.StrategyInstanceDeleted).
		First(&inst).Error
	if err != nil {
		c.JSON(statusForDBError(err), gin.H{"error": err.Error()})
		return
	}
	equity := 0.0
	if inst.Portfolio != nil {
		equity, _ = decimalToFloat(inst.Portfolio.TotalEquity)
	}
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -days+1)
	snapshots := []gin.H{
		{"time": start.Format(time.RFC3339), "total_equity": equity},
		{"time": now.Format(time.RFC3339), "total_equity": equity},
	}
	c.JSON(http.StatusOK, gin.H{"snapshots": snapshots})
}

func addDecimal(target gin.H, key string, value store.Decimal) {
	parsed, err := decimalToFloat(value)
	if err != nil {
		return
	}
	current, _ := target[key].(float64)
	target[key] = current + parsed
}
