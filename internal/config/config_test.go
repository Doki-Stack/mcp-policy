package config

import (
	"os"
	"testing"
	"time"
)

func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

func TestLoad_Defaults(t *testing.T) {
	setEnv(t, map[string]string{
		"DATABASE_URL":    "postgres://localhost/policy",
		"DRAGONFLY_URL":   "dragonfly:6379",
		"QDRANT_URL":      "http://qdrant:6333",
		"OLLAMA_BASE_URL": "http://ollama:11434",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.EmbeddingModel != "nomic-embed-text" {
		t.Errorf("EmbeddingModel = %q, want nomic-embed-text", cfg.EmbeddingModel)
	}
	if cfg.EmbeddingTimeoutMs != 10000 {
		t.Errorf("EmbeddingTimeoutMs = %d, want 10000", cfg.EmbeddingTimeoutMs)
	}
	if got := cfg.EmbeddingTimeout(); got != 10*time.Second {
		t.Errorf("EmbeddingTimeout() = %v, want 10s", got)
	}
	if cfg.CircuitBreakerThreshold != 5 {
		t.Errorf("CircuitBreakerThreshold = %d, want 5", cfg.CircuitBreakerThreshold)
	}
	if got := cfg.CircuitBreakerTimeout(); got != 60*time.Second {
		t.Errorf("CircuitBreakerTimeout() = %v, want 60s", got)
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("DRAGONFLY_URL")
	os.Unsetenv("QDRANT_URL")
	os.Unsetenv("OLLAMA_BASE_URL")

	if _, err := Load(); err == nil {
		t.Fatal("Load should fail when required config is missing")
	}
}

func TestLoad_Overrides(t *testing.T) {
	setEnv(t, map[string]string{
		"PORT":                      "9091",
		"DATABASE_URL":              "postgres://localhost/policy",
		"DRAGONFLY_URL":             "dragonfly:6379",
		"QDRANT_URL":                "http://qdrant:6333",
		"QDRANT_API_KEY":            "secret",
		"OLLAMA_BASE_URL":           "http://ollama:11434",
		"EMBEDDING_TIMEOUT_MS":      "5000",
		"CIRCUIT_BREAKER_THRESHOLD": "3",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Port != 9091 {
		t.Errorf("Port = %d, want 9091", cfg.Port)
	}
	if cfg.QdrantAPIKey != "secret" {
		t.Errorf("QdrantAPIKey = %q, want secret", cfg.QdrantAPIKey)
	}
	if cfg.EmbeddingTimeoutMs != 5000 {
		t.Errorf("EmbeddingTimeoutMs = %d, want 5000", cfg.EmbeddingTimeoutMs)
	}
	if cfg.CircuitBreakerThreshold != 3 {
		t.Errorf("CircuitBreakerThreshold = %d, want 3", cfg.CircuitBreakerThreshold)
	}
}
