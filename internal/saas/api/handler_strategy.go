package api

import (
	"net/http"

	"bian-trade-go/internal/saas/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type StrategyHandler struct {
	db *gorm.DB
}

func NewStrategyHandler(db *gorm.DB) *StrategyHandler {
	return &StrategyHandler{db: db}
}

func (h *StrategyHandler) RegisterRoutes(router gin.IRouter) {
	router.GET("/strategies", h.ListStrategies)
	router.GET("/strategies/:id", h.GetStrategy)
}

func (h *StrategyHandler) ListStrategies(c *gin.Context) {
	if !requireDB(c, h.db) {
		return
	}
	var templates []store.StrategyTemplate
	if err := h.db.WithContext(c.Request.Context()).
		Order("id ASC").
		Find(&templates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"strategies": templates})
}

func (h *StrategyHandler) GetStrategy(c *gin.Context) {
	if !requireDB(c, h.db) {
		return
	}
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var template store.StrategyTemplate
	if err := h.db.WithContext(c.Request.Context()).First(&template, id).Error; err != nil {
		c.JSON(statusForDBError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"strategy": template})
}
