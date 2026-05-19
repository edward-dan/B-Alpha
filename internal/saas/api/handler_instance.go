package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"bian-trade-go/internal/saas/instance"
	"bian-trade-go/internal/saas/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type InstanceHandler struct {
	db      *gorm.DB
	manager *instance.Manager
}

var errResponseAlreadyWritten = errors.New("response already written")

type createInstanceRequest struct {
	TemplateID       uint                   `json:"template_id"`
	Name             string                 `json:"name"`
	Symbol           string                 `json:"symbol"`
	Interval         string                 `json:"interval"`
	Config           json.RawMessage        `json:"config"`
	InitialPortfolio instance.PortfolioSeed `json:"initial_portfolio"`
	InitialCostPrice float64                `json:"initial_cost_price"`
}

type updateInstanceRequest struct {
	Status string `json:"status"`
}

func NewInstanceHandler(db *gorm.DB, manager *instance.Manager) *InstanceHandler {
	return &InstanceHandler{db: db, manager: manager}
}

func (h *InstanceHandler) RegisterRoutes(router gin.IRouter) {
	router.GET("/instances", h.ListInstances)
	router.POST("/instances", h.CreateInstance)
	router.PATCH("/instances/:id", h.UpdateInstance)
	router.POST("/instances/:id/start", h.StartInstance)
	router.POST("/instances/:id/stop", h.StopInstance)
	router.DELETE("/instances/:id", h.DeleteInstance)
	router.GET("/instances/:id/lots", h.GetInstanceLots)
	router.GET("/instances/:id/trades", h.GetInstanceTrades)
}

func (h *InstanceHandler) ListInstances(c *gin.Context) {
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
		Preload("Template").
		Preload("Portfolio").
		Preload("RuntimeState").
		Where("user_id = ? AND status <> ?", userID, store.StrategyInstanceDeleted).
		Order("id ASC").
		Find(&instances).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"instances": instances})
}

func (h *InstanceHandler) CreateInstance(c *gin.Context) {
	if !requireDB(c, h.db) {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "instance manager is not initialized"})
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authenticated user"})
		return
	}

	var req createInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if containsForbiddenSecretKey(req.Config) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "instance config must not contain exchange API credentials"})
		return
	}
	if ok := h.checkCreateQuota(c, userID); !ok {
		return
	}

	created, err := h.manager.Create(c.Request.Context(), instance.CreateRequest{
		UserID:           userID,
		TemplateID:       req.TemplateID,
		Name:             req.Name,
		Symbol:           req.Symbol,
		Interval:         req.Interval,
		Config:           req.Config,
		InitialPortfolio: req.InitialPortfolio,
		InitialCostPrice: req.InitialCostPrice,
	})
	if err != nil {
		c.JSON(statusForInstanceError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"instance": created})
}

func (h *InstanceHandler) StartInstance(c *gin.Context) {
	h.transitionOwned(c, func(ctxUserID uint, id uint) error {
		if ok := h.checkStartQuota(c, ctxUserID); !ok {
			return errResponseAlreadyWritten
		}
		return h.manager.Start(c.Request.Context(), id)
	})
}

func (h *InstanceHandler) UpdateInstance(c *gin.Context) {
	var req updateInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	switch strings.ToLower(strings.TrimSpace(req.Status)) {
	case "running", "run", "start":
		h.StartInstance(c)
	case "stopped", "paused", "pause", "stop":
		h.StopInstance(c)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "status must be running or stopped"})
	}
}

func (h *InstanceHandler) StopInstance(c *gin.Context) {
	h.transitionOwned(c, func(_ uint, id uint) error {
		return h.manager.Stop(c.Request.Context(), id)
	})
}

func (h *InstanceHandler) DeleteInstance(c *gin.Context) {
	h.transitionOwned(c, func(_ uint, id uint) error {
		return h.manager.Delete(c.Request.Context(), id)
	})
}

func (h *InstanceHandler) GetInstanceLots(c *gin.Context) {
	if !requireDB(c, h.db) {
		return
	}
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if _, ok := h.loadOwnedInstance(c, id); !ok {
		return
	}
	var lots []store.SpotLot
	if err := h.db.WithContext(c.Request.Context()).
		Where("strategy_instance_id = ?", id).
		Order("created_at ASC").
		Find(&lots).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"lots": lots})
}

func (h *InstanceHandler) GetInstanceTrades(c *gin.Context) {
	if !requireDB(c, h.db) {
		return
	}
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if _, ok := h.loadOwnedInstance(c, id); !ok {
		return
	}
	var trades []store.TradeRecord
	if err := h.db.WithContext(c.Request.Context()).
		Where("strategy_instance_id = ?", id).
		Order("executed_at DESC NULLS LAST, created_at DESC").
		Limit(200).
		Find(&trades).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"trades": trades})
}

func (h *InstanceHandler) transitionOwned(c *gin.Context, action func(userID uint, id uint) error) {
	if !requireDB(c, h.db) {
		return
	}
	if h.manager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "instance manager is not initialized"})
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
	if _, ok := h.loadOwnedInstance(c, id); !ok {
		return
	}
	if err := action(userID, id); err != nil {
		if errors.Is(err, errResponseAlreadyWritten) {
			return
		}
		c.JSON(statusForInstanceError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "status": "ok"})
}

func (h *InstanceHandler) loadOwnedInstance(c *gin.Context, id uint) (store.StrategyInstance, bool) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authenticated user"})
		return store.StrategyInstance{}, false
	}
	var inst store.StrategyInstance
	err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND user_id = ? AND status <> ?", id, userID, store.StrategyInstanceDeleted).
		First(&inst).Error
	if err != nil {
		c.JSON(statusForDBError(err), gin.H{"error": err.Error()})
		return store.StrategyInstance{}, false
	}
	return inst, true
}

func (h *InstanceHandler) checkCreateQuota(c *gin.Context, userID uint) bool {
	user, quota, count, ok := h.subscriptionQuota(c, userID)
	if !ok {
		return false
	}
	if count >= int64(quota) {
		c.JSON(http.StatusForbidden, gin.H{
			"error":             "subscription instance quota exceeded",
			"subscription_plan": user.SubscriptionPlan,
			"quota":             quota,
			"current_instances": count,
		})
		return false
	}
	return true
}

func (h *InstanceHandler) checkStartQuota(c *gin.Context, userID uint) bool {
	user, quota, count, ok := h.subscriptionQuota(c, userID)
	if !ok {
		return false
	}
	if count > int64(quota) {
		c.JSON(http.StatusForbidden, gin.H{
			"error":             "subscription instance quota exceeded",
			"subscription_plan": user.SubscriptionPlan,
			"quota":             quota,
			"current_instances": count,
		})
		return false
	}
	return true
}

func (h *InstanceHandler) subscriptionQuota(c *gin.Context, userID uint) (store.User, int, int64, bool) {
	var user store.User
	if err := h.db.WithContext(c.Request.Context()).First(&user, userID).Error; err != nil {
		c.JSON(statusForDBError(err), gin.H{"error": err.Error()})
		return user, 0, 0, false
	}
	quota := maxInstancesForUser(user, time.Now().UTC())
	if quota <= 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "subscription is not active"})
		return user, quota, 0, false
	}
	var count int64
	if err := h.db.WithContext(c.Request.Context()).
		Model(&store.StrategyInstance{}).
		Where("user_id = ? AND status <> ?", userID, store.StrategyInstanceDeleted).
		Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return user, quota, 0, false
	}
	return user, quota, count, true
}

func maxInstancesForUser(user store.User, now time.Time) int {
	if !subscriptionActive(user, now) {
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(user.SubscriptionPlan)) {
	case "", "free":
		return 1
	case "basic", "starter":
		return 3
	case "pro":
		return 10
	case "team":
		return 50
	case "enterprise":
		return 1000
	default:
		return 1
	}
}

func subscriptionActive(user store.User, now time.Time) bool {
	if strings.ToLower(strings.TrimSpace(user.SubscriptionStatus)) != "active" {
		return false
	}
	return user.SubscriptionExpiresAt == nil || user.SubscriptionExpiresAt.After(now)
}

func statusForInstanceError(err error) int {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return http.StatusNotFound
	case errors.Is(err, instance.ErrInvalidTransition):
		return http.StatusConflict
	case errors.Is(err, instance.ErrManagerNotReady):
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadRequest
	}
}
