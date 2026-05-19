package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"bian-trade-go/internal/saas/auth"
	"bian-trade-go/internal/saas/config"
	"github.com/gin-gonic/gin"
)

type fakeTokenService struct {
	claims *auth.Claims
	err    error
}

func (s fakeTokenService) SignToken(userID uint, role string) (string, error) {
	return "token", nil
}

func (s fakeTokenService) ParseToken(string) (*auth.Claims, error) {
	return s.claims, s.err
}

func TestProtectedAPIRoutesRequireJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(RouterConfig{
		TokenService: fakeTokenService{claims: &auth.Claims{UserID: 7, Role: "user"}},
		AppRole:      config.AppRoleSaaS,
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/strategies", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for protected route without JWT, got %d", w.Code)
	}
}

func TestEvolutionRoutesUseSaaSMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(RouterConfig{
		TokenService: fakeTokenService{claims: &auth.Claims{UserID: 7, Role: "user"}},
		AppRole:      config.AppRoleSaaS,
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/genome/champion?strategy_id=x&symbol=BTCUSDT", nil)
	req.Header.Set("Authorization", "Bearer token")
	router.ServeHTTP(w, req)

	if w.Code == http.StatusForbidden {
		t.Fatalf("expected evolution route to be available in saas app_role, got %d", w.Code)
	}
}
