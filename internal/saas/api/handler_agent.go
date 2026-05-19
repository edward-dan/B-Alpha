package api

import (
	"fmt"
	"net/http"
	"strings"

	saasws "bian-trade-go/internal/saas/ws"
	"github.com/gin-gonic/gin"
)

type AgentHandler struct {
	hub *saasws.Hub
}

func NewAgentHandler(hub *saasws.Hub) *AgentHandler {
	return &AgentHandler{hub: hub}
}

func (h *AgentHandler) RegisterRoutes(router gin.IRouter) {
	router.GET("/agents/status", h.GetStatus)
}

func (h *AgentHandler) GetStatus(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authenticated user"})
		return
	}
	agentID := strings.TrimSpace(c.Query("agent_id"))
	if agentID == "" {
		agentID = fmt.Sprintf("user:%d", userID)
	}
	connected := false
	if h.hub != nil {
		connected = h.hub.IsAgentConnected(agentID)
	}
	c.JSON(http.StatusOK, gin.H{
		"user_id":   userID,
		"agent_id":  agentID,
		"connected": connected,
	})
}
