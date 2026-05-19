package api

import (
	"fmt"
	"net/http"
	"time"

	saasws "bian-trade-go/internal/saas/ws"
	"github.com/gin-gonic/gin"
)

type SystemHandler struct {
	hub     *saasws.Hub
	appRole string
}

func NewSystemHandler(hub *saasws.Hub, appRole string) *SystemHandler {
	return &SystemHandler{hub: hub, appRole: normalizeAppRole(appRole)}
}

func (h *SystemHandler) RegisterRoutes(router gin.IRouter) {
	router.GET("/system/status", h.GetStatus)
}

func (h *SystemHandler) GetStatus(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authenticated user"})
		return
	}
	agentID := fmt.Sprintf("user:%d", userID)
	connected := false
	if h.hub != nil {
		connected = h.hub.IsAgentConnected(agentID)
	}
	c.JSON(http.StatusOK, gin.H{
		"engine_state":          "running",
		"app_role":              h.appRole,
		"features":              featuresForSaaS(),
		"agent_connected":       connected,
		"api_connected":         connected,
		"api_configured":        connected,
		"agent_version":         "",
		"last_heartbeat_at":     time.Now().UTC().Format(time.RFC3339),
		"needs_reconciliation":  false,
		"reconciliation_reason": "",
		"checked_at":            time.Now().UTC().Format(time.RFC3339),
	})
}

func featuresForSaaS() gin.H {
	return gin.H{
		"dashboard":   true,
		"strategies":  true,
		"agents":      true,
		"risk":        true,
		"backtesting": true,
		"settings":    true,
	}
}
