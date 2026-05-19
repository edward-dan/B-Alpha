package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestStaticFrontendCacheHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	distDir := t.TempDir()
	assetsDir := filepath.Join(distDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatalf("create assets dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "index.html"), []byte("<html><body><div id=\"root\"></div></body></html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "app.js"), []byte("console.log('ok');"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	router := gin.New()
	RegisterStaticFrontend(router, distDir)

	indexRecorder := httptest.NewRecorder()
	router.ServeHTTP(indexRecorder, httptest.NewRequest(http.MethodGet, "/backtesting", nil))
	if indexRecorder.Code != http.StatusOK {
		t.Fatalf("expected index response 200, got %d", indexRecorder.Code)
	}
	if got := indexRecorder.Header().Get("Cache-Control"); got != "no-store, no-cache, must-revalidate, max-age=0" {
		t.Fatalf("expected index no-cache header, got %q", got)
	}

	assetRecorder := httptest.NewRecorder()
	router.ServeHTTP(assetRecorder, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if assetRecorder.Code != http.StatusOK {
		t.Fatalf("expected asset response 200, got %d", assetRecorder.Code)
	}
	if got := assetRecorder.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("expected immutable asset cache header, got %q", got)
	}
}
