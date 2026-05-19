package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAgentConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.agent.yaml")
	content := []byte(`
saas_url: "http://localhost:8080/"
email: "agent@example.com"
password: "secret"
exchange:
  name: "Bitget"
  api_key: "key"
  secret_key: "secret"
  passphrase: "pass"
  sandbox: true
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.SaaSURL != "http://localhost:8080" {
		t.Fatalf("SaaSURL = %q", cfg.SaaSURL)
	}
	if cfg.Exchange.Name != "bitget" {
		t.Fatalf("exchange name = %q", cfg.Exchange.Name)
	}
	if !cfg.Exchange.Sandbox {
		t.Fatal("sandbox should be true")
	}
}

func TestAgentConfigValidateRequiresSecrets(t *testing.T) {
	cfg := AgentConfig{
		SaaSURL:  "http://localhost:8080",
		Email:    "agent@example.com",
		Password: "secret",
		Exchange: ExchangeConfig{Name: "bitget", APIKey: "key", Passphrase: "pass"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing secret_key validation error")
	}
}
