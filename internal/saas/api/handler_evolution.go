package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"bian-trade-go/internal/saas/epoch"
	"bian-trade-go/internal/saas/store"
	"bian-trade-go/internal/strategies/example"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type EvolutionHandler struct {
	db           *gorm.DB
	redis        *store.Redis
	epochService *epoch.EpochService
}

type promoteRequest struct {
	GeneID         uint   `json:"gene_id"`
	PromotedByUser *uint  `json:"promoted_by_user"`
	Reason         string `json:"reason"`
}

func NewEvolutionHandler(db *gorm.DB, redis *store.Redis, epochService *epoch.EpochService) *EvolutionHandler {
	return &EvolutionHandler{
		db:           db,
		redis:        redis,
		epochService: epochService,
	}
}

func (h *EvolutionHandler) RegisterRoutes(router gin.IRouter) {
	router.POST("/evolution/tasks", h.CreateEvolutionTask)
	router.GET("/evolution/tasks", h.ListEvolutionTasks)
	router.POST("/evolution/tasks/:id/cancel", h.CancelEvolutionTask)
	router.POST("/evolution/tasks/:id/promote", h.PromoteEvolutionTask)
	router.GET("/evolution/genomes", h.ListGenomes)
	router.GET("/genome/champion", h.GetChampionGenome)
	router.GET("/genome/challengers", h.ListChallengerGenomes)
}

func (h *EvolutionHandler) CreateEvolutionTask(c *gin.Context) {
	if h.epochService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "evolution service is not initialized"})
		return
	}
	var req epoch.CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	task, err := h.epochService.CreateAndRunTask(c.Request.Context(), req)
	if err != nil {
		c.JSON(httpStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, task)
}

func (h *EvolutionHandler) ListEvolutionTasks(c *gin.Context) {
	if h.epochService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "evolution service is not initialized"})
		return
	}
	status, err := h.epochService.Status(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, status)
}

func (h *EvolutionHandler) CancelEvolutionTask(c *gin.Context) {
	if h.epochService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "evolution service is not initialized"})
		return
	}
	taskID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	task, err := h.epochService.CancelTask(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(httpStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"task": task})
}

func (h *EvolutionHandler) PromoteEvolutionTask(c *gin.Context) {
	if !requireDB(c, h.db) {
		return
	}
	taskID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req promoteRequest
	if c.Request.Body != nil {
		_ = c.ShouldBindJSON(&req)
	}
	if req.PromotedByUser == nil {
		if userID, ok := currentUserID(c); ok {
			req.PromotedByUser = &userID
		}
	}

	ctx := c.Request.Context()
	var promoted store.GeneRecord
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task store.EvolutionTask
		taskErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&task, taskID).Error
		geneID := req.GeneID
		if errors.Is(taskErr, gorm.ErrRecordNotFound) && geneID == 0 {
			geneID = taskID
		} else if taskErr != nil {
			return taskErr
		}
		if geneID == 0 && task.BestGeneID != "" {
			parsed, err := strconv.ParseUint(task.BestGeneID, 10, 64)
			if err != nil {
				return err
			}
			geneID = uint(parsed)
		}
		if geneID == 0 {
			return errors.New("promote evolution task: gene_id is required")
		}

		var challenger store.GeneRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&challenger, geneID).Error; err != nil {
			return err
		}
		if challenger.Role != store.GeneRoleChallenger {
			return errors.New("promote evolution task: selected gene is not a challenger")
		}
		if task.ID != 0 && (challenger.StrategyID != task.StrategyID || challenger.Symbol != task.Symbol) {
			return errors.New("promote evolution task: challenger does not belong to task strategy and symbol")
		}

		now := time.Now().UTC()
		if err := tx.Model(&store.GeneRecord{}).
			Where("strategy_id = ? AND symbol = ? AND role = ?", challenger.StrategyID, challenger.Symbol, store.GeneRoleChampion).
			Updates(map[string]any{
				"role":       store.GeneRoleRetired,
				"retired_at": &now,
			}).Error; err != nil {
			return err
		}

		updates := map[string]any{
			"role":        store.GeneRoleChampion,
			"promoted_at": &now,
		}
		if req.PromotedByUser != nil {
			updates["promoted_by_user"] = req.PromotedByUser
		}
		if err := tx.Model(&challenger).Updates(updates).Error; err != nil {
			return err
		}
		promoted = challenger
		promoted.Role = store.GeneRoleChampion
		promoted.PromotedAt = &now
		return nil
	})
	if err != nil {
		c.JSON(httpStatusForError(err), gin.H{"error": err.Error()})
		return
	}

	if h.redis != nil {
		if err := h.redis.Del(ctx, ChampionCacheKey(promoted.StrategyID, promoted.Symbol)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "promoted but failed to invalidate champion cache"})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"champion": promoted})
}

func (h *EvolutionHandler) ListGenomes(c *gin.Context) {
	if !requireDB(c, h.db) {
		return
	}
	query := h.db.WithContext(c.Request.Context())
	if instanceRaw := c.Query("instance_id"); instanceRaw != "" {
		instanceID, err := strconv.ParseUint(instanceRaw, 10, 64)
		if err != nil || instanceID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid instance_id"})
			return
		}
		userID, ok := currentUserID(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authenticated user"})
			return
		}
		var inst store.StrategyInstance
		if err := h.db.WithContext(c.Request.Context()).
			Preload("Template").
			Where("id = ? AND user_id = ? AND status <> ?", uint(instanceID), userID, store.StrategyInstanceDeleted).
			First(&inst).Error; err != nil {
			c.JSON(statusForDBError(err), gin.H{"error": err.Error()})
			return
		}
		strategyID := strategyIDFromTemplate(inst.Template)
		query = query.Where("strategy_id = ? AND symbol = ?", strategyID, inst.Symbol)
	}
	var genomes []store.GeneRecord
	if err := query.Order("created_at DESC").Limit(200).Find(&genomes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"genomes": genomes})
}

func (h *EvolutionHandler) GetChampionGenome(c *gin.Context) {
	if !requireDB(c, h.db) {
		return
	}
	strategyID := c.Query("strategy_id")
	symbol := c.Query("symbol")
	if strategyID == "" || symbol == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "strategy_id and symbol are required"})
		return
	}

	ctx := c.Request.Context()
	cacheKey := ChampionCacheKey(strategyID, symbol)
	if h.redis != nil {
		if cached, err := h.redis.Get(ctx, cacheKey); err == nil && json.Valid([]byte(cached)) {
			c.JSON(http.StatusOK, gin.H{
				"strategy_id": strategyID,
				"symbol":      symbol,
				"cached":      true,
				"param_pack":  json.RawMessage(cached),
			})
			return
		}
	}

	var champion store.GeneRecord
	err := h.db.WithContext(ctx).
		Where("strategy_id = ? AND symbol = ? AND role = ?", strategyID, symbol, store.GeneRoleChampion).
		Order("updated_at DESC").
		First(&champion).Error
	if err != nil {
		c.JSON(httpStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	if h.redis != nil {
		_ = h.redis.Set(ctx, cacheKey, string(champion.ParamPack), 5*time.Minute)
	}
	c.JSON(http.StatusOK, gin.H{
		"strategy_id": strategyID,
		"symbol":      symbol,
		"cached":      false,
		"param_pack":  json.RawMessage(champion.ParamPack),
	})
}

func (h *EvolutionHandler) ListChallengerGenomes(c *gin.Context) {
	if !requireDB(c, h.db) {
		return
	}
	query := h.db.WithContext(c.Request.Context()).
		Where("role = ?", store.GeneRoleChallenger)
	if strategyID := c.Query("strategy_id"); strategyID != "" {
		query = query.Where("strategy_id = ?", strategyID)
	}
	if symbol := c.Query("symbol"); symbol != "" {
		query = query.Where("symbol = ?", symbol)
	}
	limit := 100
	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return
		}
		if parsed < limit {
			limit = parsed
		}
	}
	var challengers []store.GeneRecord
	if err := query.Order("created_at DESC").Limit(limit).Find(&challengers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"challengers": challengers})
}

func ChampionCacheKey(strategyID string, symbol string) string {
	return "champion:" + strategyID + ":" + symbol
}

func strategyIDFromTemplate(template *store.StrategyTemplate) string {
	if template != nil {
		var manifest struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(template.Manifest, &manifest); err == nil && manifest.ID != "" {
			return manifest.ID
		}
	}
	return example.StrategyID
}

func httpStatusForError(err error) int {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}
