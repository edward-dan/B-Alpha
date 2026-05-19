package api

import (
	"net/http"
	"strings"

	"bian-trade-go/internal/saas/auth"
	"bian-trade-go/internal/saas/config"
	"github.com/gin-gonic/gin"
)

const (
	contextClaimsKey = "auth_claims"
	contextUserIDKey = "auth_user_id"
	contextRoleKey   = "auth_role"
)

func JWTMiddleware(tokenService TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if tokenService == nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "jwt service is not initialized"})
			return
		}

		token := bearerToken(c.GetHeader("Authorization"))
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}

		claims, err := tokenService.ParseToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid bearer token"})
			return
		}
		if claims.UserID == 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "jwt user_id is required"})
			return
		}

		c.Set(contextClaimsKey, claims)
		c.Set(contextUserIDKey, claims.UserID)
		c.Set(contextRoleKey, claims.Role)
		c.Next()
	}
}

func RequireAppRole(appRole string, allowed ...string) gin.HandlerFunc {
	normalized := normalizeAppRole(appRole)
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, role := range allowed {
		role = normalizeAppRole(role)
		if role != "" {
			allowedSet[role] = struct{}{}
		}
	}

	return func(c *gin.Context) {
		if _, ok := allowedSet[normalized]; ok {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "route is only available in lab/dev mode"})
	}
}

func currentUserID(c *gin.Context) (uint, bool) {
	value, ok := c.Get(contextUserIDKey)
	if !ok {
		return 0, false
	}
	id, ok := value.(uint)
	return id, ok && id > 0
}

func currentClaims(c *gin.Context) (*auth.Claims, bool) {
	value, ok := c.Get(contextClaimsKey)
	if !ok {
		return nil, false
	}
	claims, ok := value.(*auth.Claims)
	return claims, ok && claims != nil
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	parts := strings.Fields(header)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func normalizeAppRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		return config.AppRoleDev
	}
	return role
}
