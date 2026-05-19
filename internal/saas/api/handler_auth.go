package api

import (
	"net/http"
	"strings"
	"time"

	"bian-trade-go/internal/saas/store"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AuthHandler struct {
	db           *gorm.DB
	tokenService TokenService
}

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	Token string       `json:"token"`
	User  userResponse `json:"user"`
}

type userResponse struct {
	ID                    uint       `json:"id"`
	Email                 string     `json:"email"`
	Role                  string     `json:"role"`
	SubscriptionPlan      string     `json:"subscription_plan"`
	SubscriptionStatus    string     `json:"subscription_status"`
	SubscriptionExpiresAt *time.Time `json:"subscription_expires_at,omitempty"`
}

func NewAuthHandler(db *gorm.DB, tokenService TokenService) *AuthHandler {
	return &AuthHandler{db: db, tokenService: tokenService}
}

func (h *AuthHandler) RegisterRoutes(router gin.IRouter) {
	router.POST("/register", h.Register)
	router.POST("/login", h.Login)
	router.GET("/me", JWTMiddleware(h.tokenService), h.Me)
}

func (h *AuthHandler) Register(c *gin.Context) {
	if !requireDB(c, h.db) {
		return
	}
	if h.tokenService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "jwt service is not initialized"})
		return
	}

	var req authRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	email := normalizeEmail(req.Email)
	if !validEmail(email) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email is required"})
		return
	}
	if len(req.Password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 8 characters"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "hash password failed"})
		return
	}
	user := store.User{
		Email:              email,
		PasswordHash:       string(hash),
		Role:               "user",
		SubscriptionPlan:   "free",
		SubscriptionStatus: "active",
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&user).Error; err != nil {
		if duplicateKey(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.writeAuthResponse(c, http.StatusCreated, user)
}

func (h *AuthHandler) Login(c *gin.Context) {
	if !requireDB(c, h.db) {
		return
	}
	if h.tokenService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "jwt service is not initialized"})
		return
	}

	var req authRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user store.User
	err := h.db.WithContext(c.Request.Context()).
		Where("email = ?", normalizeEmail(req.Email)).
		First(&user).Error
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
		return
	}

	h.writeAuthResponse(c, http.StatusOK, user)
}

func (h *AuthHandler) Me(c *gin.Context) {
	if !requireDB(c, h.db) {
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authenticated user"})
		return
	}
	var user store.User
	if err := h.db.WithContext(c.Request.Context()).First(&user, userID).Error; err != nil {
		c.JSON(statusForDBError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": toUserResponse(user)})
}

func (h *AuthHandler) writeAuthResponse(c *gin.Context, status int, user store.User) {
	token, err := h.tokenService.SignToken(user.ID, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(status, authResponse{Token: token, User: toUserResponse(user)})
}

func toUserResponse(user store.User) userResponse {
	return userResponse{
		ID:                    user.ID,
		Email:                 user.Email,
		Role:                  user.Role,
		SubscriptionPlan:      user.SubscriptionPlan,
		SubscriptionStatus:    user.SubscriptionStatus,
		SubscriptionExpiresAt: user.SubscriptionExpiresAt,
	}
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validEmail(email string) bool {
	return strings.Contains(email, "@") && strings.Contains(email, ".")
}

func duplicateKey(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate")
}
