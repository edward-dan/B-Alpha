package auth

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"bian-trade-go/internal/saas/config"
	"github.com/golang-jwt/jwt/v5"
)

var ErrMissingJWTSecret = errors.New("jwt secret is required")

type Claims struct {
	UserID uint   `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

type Service struct {
	secret    []byte
	issuer    string
	expiresIn time.Duration
}

func NewService(cfg config.JWTConfig) (*Service, error) {
	if cfg.Secret == "" {
		return nil, ErrMissingJWTSecret
	}
	if cfg.ExpiresInSeconds <= 0 {
		return nil, errors.New("jwt expiration must be positive")
	}

	return &Service{
		secret:    []byte(cfg.Secret),
		issuer:    cfg.Issuer,
		expiresIn: time.Duration(cfg.ExpiresInSeconds) * time.Second,
	}, nil
}

func (s *Service) SignToken(userID uint, role string) (string, error) {
	if len(s.secret) == 0 {
		return "", ErrMissingJWTSecret
	}
	if userID == 0 {
		return "", errors.New("userID must be positive")
	}
	if role == "" {
		return "", errors.New("role is required")
	}

	now := time.Now()
	claims := Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatUint(uint64(userID), 10),
			Issuer:    s.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.expiresIn)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

func (s *Service) ParseToken(tokenStr string) (*Claims, error) {
	if len(s.secret) == 0 {
		return nil, ErrMissingJWTSecret
	}

	claims := &Claims{}
	options := []jwt.ParserOption{}
	if s.issuer != "" {
		options = append(options, jwt.WithIssuer(s.issuer))
	}

	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected jwt signing method %v", token.Header["alg"])
		}
		return s.secret, nil
	}, options...)
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid jwt token")
	}

	return claims, nil
}

func SignToken(userID uint, role string) (string, error) {
	service, err := serviceFromEnv()
	if err != nil {
		return "", err
	}
	return service.SignToken(userID, role)
}

func ParseToken(tokenStr string) (*Claims, error) {
	service, err := serviceFromEnv()
	if err != nil {
		return nil, err
	}
	return service.ParseToken(tokenStr)
}

func serviceFromEnv() (*Service, error) {
	expiresInSeconds := int64(86400)
	if value, ok := os.LookupEnv("B_ALPHA_JWT_EXPIRES_IN_SECONDS"); ok {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse B_ALPHA_JWT_EXPIRES_IN_SECONDS: %w", err)
		}
		expiresInSeconds = parsed
	}

	issuer := os.Getenv("B_ALPHA_JWT_ISSUER")
	if issuer == "" {
		issuer = "b-alpha"
	}

	return NewService(config.JWTConfig{
		Secret:           os.Getenv("B_ALPHA_JWT_SECRET"),
		Issuer:           issuer,
		ExpiresInSeconds: expiresInSeconds,
	})
}
