package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
)

const (
	AppRoleSaaS = "saas"
	AppRoleLab  = "lab"
	AppRoleDev  = "dev"
)

type Config struct {
	AppRole  string         `yaml:"app_role"`
	Database DatabaseConfig `yaml:"database"`
	Redis    RedisConfig    `yaml:"redis"`
	JWT      JWTConfig      `yaml:"jwt"`
	Server   ServerConfig   `yaml:"server"`
}

type DatabaseConfig struct {
	Host                   string `yaml:"host"`
	Port                   int    `yaml:"port"`
	User                   string `yaml:"user"`
	Password               string `yaml:"password"`
	Name                   string `yaml:"name"`
	SSLMode                string `yaml:"ssl_mode"`
	TimeZone               string `yaml:"timezone"`
	MaxOpenConns           int    `yaml:"max_open_conns"`
	MaxIdleConns           int    `yaml:"max_idle_conns"`
	ConnMaxLifetimeSeconds int    `yaml:"conn_max_lifetime_seconds"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type JWTConfig struct {
	Secret           string `yaml:"secret"`
	Issuer           string `yaml:"issuer"`
	ExpiresInSeconds int64  `yaml:"expires_in_seconds"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	Mode string `yaml:"mode"`
}

func Load(path string) (*Config, error) {
	cfg := defaultConfig()

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %q: %w", path, err)
	}
	if err := yaml.Unmarshal(content, cfg); err != nil {
		return nil, fmt.Errorf("parse config file %q: %w", path, err)
	}
	if err := applyEnv(cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	switch c.AppRole {
	case AppRoleSaaS, AppRoleLab, AppRoleDev:
	default:
		return fmt.Errorf("invalid app_role %q: allowed values are %q, %q, %q", c.AppRole, AppRoleSaaS, AppRoleLab, AppRoleDev)
	}

	if c.Database.Port <= 0 {
		return errors.New("database.port must be positive")
	}
	if c.Redis.Addr == "" {
		return errors.New("redis.addr is required")
	}
	if c.Server.Port <= 0 {
		return errors.New("server.port must be positive")
	}
	if c.JWT.ExpiresInSeconds <= 0 {
		return errors.New("jwt.expires_in_seconds must be positive")
	}

	return nil
}

func (c DatabaseConfig) DSN() string {
	parts := []string{
		pgConnParam("host", c.Host),
		pgConnParam("port", strconv.Itoa(c.Port)),
		pgConnParam("user", c.User),
		pgConnParam("dbname", c.Name),
	}
	if c.Password != "" {
		parts = append(parts, pgConnParam("password", c.Password))
	}
	if c.SSLMode != "" {
		parts = append(parts, pgConnParam("sslmode", c.SSLMode))
	}
	if c.TimeZone != "" {
		parts = append(parts, pgConnParam("TimeZone", c.TimeZone))
	}

	return strings.Join(parts, " ")
}

func pgConnParam(key, value string) string {
	return key + "=" + pgConnValue(value)
}

func pgConnValue(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.ContainsAny(value, " \t\r\n'\\") {
		return value
	}
	escaped := strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(value)
	return "'" + escaped + "'"
}

func defaultConfig() *Config {
	return &Config{
		AppRole: AppRoleDev,
		Database: DatabaseConfig{
			Host:                   "localhost",
			Port:                   5432,
			User:                   "b_alpha",
			Name:                   "b_alpha",
			SSLMode:                "disable",
			TimeZone:               "Asia/Shanghai",
			MaxOpenConns:           20,
			MaxIdleConns:           5,
			ConnMaxLifetimeSeconds: 1800,
		},
		Redis: RedisConfig{
			Addr: "localhost:6379",
		},
		JWT: JWTConfig{
			Issuer:           "b-alpha",
			ExpiresInSeconds: 86400,
		},
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
			Mode: "debug",
		},
	}
}

func applyEnv(cfg *Config) error {
	setString := func(name string, target *string) {
		if value, ok := os.LookupEnv(name); ok {
			*target = value
		}
	}
	setInt := func(name string, target *int) error {
		value, ok := os.LookupEnv(name)
		if !ok {
			return nil
		}
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse %s as int: %w", name, err)
		}
		*target = parsed
		return nil
	}
	setInt64 := func(name string, target *int64) error {
		value, ok := os.LookupEnv(name)
		if !ok {
			return nil
		}
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("parse %s as int64: %w", name, err)
		}
		*target = parsed
		return nil
	}

	setString("B_ALPHA_APP_ROLE", &cfg.AppRole)
	setString("B_ALPHA_DB_HOST", &cfg.Database.Host)
	setString("B_ALPHA_DB_USER", &cfg.Database.User)
	setString("B_ALPHA_DB_PASSWORD", &cfg.Database.Password)
	setString("B_ALPHA_DB_NAME", &cfg.Database.Name)
	setString("B_ALPHA_DB_SSL_MODE", &cfg.Database.SSLMode)
	setString("B_ALPHA_DB_TIMEZONE", &cfg.Database.TimeZone)
	setString("B_ALPHA_REDIS_ADDR", &cfg.Redis.Addr)
	setString("B_ALPHA_REDIS_USERNAME", &cfg.Redis.Username)
	setString("B_ALPHA_REDIS_PASSWORD", &cfg.Redis.Password)
	setString("B_ALPHA_JWT_SECRET", &cfg.JWT.Secret)
	setString("B_ALPHA_JWT_ISSUER", &cfg.JWT.Issuer)
	setString("B_ALPHA_SERVER_HOST", &cfg.Server.Host)
	setString("B_ALPHA_SERVER_MODE", &cfg.Server.Mode)

	for _, setter := range []func() error{
		func() error { return setInt("B_ALPHA_DB_PORT", &cfg.Database.Port) },
		func() error { return setInt("B_ALPHA_DB_MAX_OPEN_CONNS", &cfg.Database.MaxOpenConns) },
		func() error { return setInt("B_ALPHA_DB_MAX_IDLE_CONNS", &cfg.Database.MaxIdleConns) },
		func() error {
			return setInt("B_ALPHA_DB_CONN_MAX_LIFETIME_SECONDS", &cfg.Database.ConnMaxLifetimeSeconds)
		},
		func() error { return setInt("B_ALPHA_REDIS_DB", &cfg.Redis.DB) },
		func() error { return setInt64("B_ALPHA_JWT_EXPIRES_IN_SECONDS", &cfg.JWT.ExpiresInSeconds) },
		func() error { return setInt("B_ALPHA_SERVER_PORT", &cfg.Server.Port) },
	} {
		if err := setter(); err != nil {
			return err
		}
	}

	return nil
}
