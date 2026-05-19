package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"bian-trade-go/internal/saas/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func requireDB(c *gin.Context, db *gorm.DB) bool {
	if db != nil {
		return true
	}
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database is not initialized"})
	return false
}

func parseIDParam(c *gin.Context, name string) (uint, bool) {
	value := c.Param(name)
	if value == "" && name == "id" {
		value = c.Param("taskID")
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid %s", name)})
		return 0, false
	}
	return uint(parsed), true
}

func statusForDBError(err error) int {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

func decimalToFloat(value store.Decimal) (float64, error) {
	raw := strings.TrimSpace(string(value))
	if raw == "" {
		return 0, nil
	}
	return strconv.ParseFloat(raw, 64)
}

func mustJSONB(value any) (store.JSONB, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return store.JSONB(raw), nil
}

func rawJSONB(raw json.RawMessage) (store.JSONB, error) {
	if len(raw) == 0 {
		return store.JSONB([]byte("{}")), nil
	}
	if !json.Valid(raw) {
		return nil, errors.New("json value must be valid")
	}
	out := append([]byte(nil), raw...)
	return store.JSONB(out), nil
}

func containsForbiddenSecretKey(raw json.RawMessage) bool {
	if len(raw) == 0 || !json.Valid(raw) {
		return false
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	return containsForbiddenSecretValue(value)
}

func containsForbiddenSecretValue(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if isForbiddenSecretKey(key) {
				return true
			}
			if containsForbiddenSecretValue(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if containsForbiddenSecretValue(nested) {
				return true
			}
		}
	}
	return false
}

func isForbiddenSecretKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))

	if normalized == "secret" || normalized == "pass"+"phrase" {
		return true
	}

	parts := strings.Split(normalized, "_")
	compact := strings.Join(parts, "")
	if compact == "apikey" || compact == "apisecret" || compact == "accesskey" {
		return true
	}

	hasPart := func(want string) bool {
		for _, part := range parts {
			if part == want {
				return true
			}
		}
		return false
	}
	return hasPart("key") && (hasPart("api") || hasPart("secret") || hasPart("access"))
}
