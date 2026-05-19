package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/goccy/go-yaml"
)

type AgentConfig struct {
	SaaSURL  string         `yaml:"saas_url" json:"saas_url"`
	Email    string         `yaml:"email" json:"email"`
	Password string         `yaml:"password" json:"password"`
	Exchange ExchangeConfig `yaml:"exchange" json:"exchange"`
}

type ExchangeConfig struct {
	Name       string `yaml:"name" json:"name"`
	APIKey     string `yaml:"api_key" json:"api_key"`
	SecretKey  string `yaml:"secret_key" json:"secret_key"`
	Passphrase string `yaml:"passphrase" json:"passphrase"`
	Sandbox    bool   `yaml:"sandbox" json:"sandbox"`
}

func Load(path string) (*AgentConfig, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read agent config %q: %w", path, err)
	}

	var cfg AgentConfig
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return nil, fmt.Errorf("parse agent config %q: %w", path, err)
	}
	cfg.normalize()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *AgentConfig) Validate() error {
	if c == nil {
		return errors.New("agent config is nil")
	}
	if c.SaaSURL == "" {
		return errors.New("agent config saas_url is required")
	}
	if c.Email == "" {
		return errors.New("agent config email is required")
	}
	if c.Password == "" {
		return errors.New("agent config password is required")
	}
	if c.Exchange.Name == "" {
		return errors.New("agent config exchange.name is required")
	}
	if c.Exchange.APIKey == "" {
		return errors.New("agent config exchange.api_key is required")
	}
	if c.Exchange.SecretKey == "" {
		return errors.New("agent config exchange.secret_key is required")
	}
	if c.Exchange.Passphrase == "" {
		return errors.New("agent config exchange.passphrase is required")
	}
	return nil
}

func (c *AgentConfig) normalize() {
	c.SaaSURL = strings.TrimRight(strings.TrimSpace(c.SaaSURL), "/")
	c.Email = strings.TrimSpace(c.Email)
	c.Exchange.Name = strings.ToLower(strings.TrimSpace(c.Exchange.Name))
	c.Exchange.APIKey = strings.TrimSpace(c.Exchange.APIKey)
	c.Exchange.SecretKey = strings.TrimSpace(c.Exchange.SecretKey)
	c.Exchange.Passphrase = strings.TrimSpace(c.Exchange.Passphrase)
}
