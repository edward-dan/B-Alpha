package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"bian-trade-go/internal/saas/auth"
	"bian-trade-go/internal/saas/config"
	"bian-trade-go/internal/saas/epoch"
	"bian-trade-go/internal/saas/instance"
	"bian-trade-go/internal/saas/store"
	saasws "bian-trade-go/internal/saas/ws"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type TokenService interface {
	SignToken(userID uint, role string) (string, error)
	ParseToken(tokenStr string) (*auth.Claims, error)
}

type RouterConfig struct {
	DB              *gorm.DB
	Redis           *store.Redis
	TokenService    TokenService
	InstanceManager *instance.Manager
	EpochService    *epoch.EpochService
	Hub             *saasws.Hub
	AppRole         string
}

func NewRouter(cfg RouterConfig) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	RegisterRoutes(router, cfg)
	RegisterStaticFrontend(router, filepath.Join("web-frontend", "dist"))
	return router
}

func RegisterRoutes(router gin.IRouter, cfg RouterConfig) {
	appRole := normalizeAppRole(cfg.AppRole)

	api := router.Group("/api/v1")
	NewAuthHandler(cfg.DB, cfg.TokenService).RegisterRoutes(api.Group("/auth"))

	protected := api.Group("", JWTMiddleware(cfg.TokenService))
	NewStrategyHandler(cfg.DB).RegisterRoutes(protected)
	NewInstanceHandler(cfg.DB, cfg.InstanceManager).RegisterRoutes(protected)
	NewDashboardHandler(cfg.DB).RegisterRoutes(protected)
	NewAgentHandler(cfg.Hub).RegisterRoutes(protected)
	NewSystemHandler(cfg.Hub, appRole).RegisterRoutes(protected)

	labOnly := protected.Group("", RequireAppRole(appRole, config.AppRoleLab, config.AppRoleDev))
	NewEvolutionHandler(cfg.DB, cfg.Redis, cfg.EpochService, appRole).RegisterRoutes(labOnly)
	NewBacktestHandler(cfg.DB, appRole).RegisterRoutes(labOnly)

	if cfg.Hub != nil {
		cfg.Hub.RegisterRoutes(router)
		return
	}
	router.GET("/ws/agent", func(c *gin.Context) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "websocket hub is not initialized"})
	})
}

func RegisterStaticFrontend(router *gin.Engine, distDir string) {
	indexPath := filepath.Join(distDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		return
	}
	assetsPath := filepath.Join(distDir, "assets")
	if _, err := os.Stat(assetsPath); err == nil {
		router.Static("/assets", assetsPath)
	}
	router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/ws/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.File(indexPath)
	})
}
