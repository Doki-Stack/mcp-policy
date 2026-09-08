// Package config loads mcp-policy's runtime configuration from the environment.
package config

import (
	"time"

	sharedconfig "github.com/doki-stack/shared-go/config"
)

// Config holds all runtime configuration for the Policy MCP.
type Config struct {
	Port int `env:"PORT" default:"8080"`

	DatabaseURL  string `env:"DATABASE_URL" required:"true"`
	DragonflyURL string `env:"DRAGONFLY_URL" required:"true"`

	QdrantURL    string `env:"QDRANT_URL" required:"true"`
	QdrantAPIKey string `env:"QDRANT_API_KEY"`

	OllamaBaseURL      string `env:"OLLAMA_BASE_URL" required:"true"`
	EmbeddingModel     string `env:"EMBEDDING_MODEL" default:"nomic-embed-text"`
	EmbeddingTimeoutMs int    `env:"EMBEDDING_TIMEOUT_MS" default:"10000"`

	CircuitBreakerThreshold int `env:"CIRCUIT_BREAKER_THRESHOLD" default:"5"`
	CircuitBreakerTimeoutMs int `env:"CIRCUIT_BREAKER_TIMEOUT_MS" default:"60000"`

	LogLevel             string `env:"LOG_LEVEL" default:"info"`
	Environment          string `env:"ENVIRONMENT" default:"development"`
	OTelExporterEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
}

// EmbeddingTimeout returns the embedding call timeout as a time.Duration.
func (c *Config) EmbeddingTimeout() time.Duration {
	return time.Duration(c.EmbeddingTimeoutMs) * time.Millisecond
}

// CircuitBreakerTimeout returns the circuit breaker open-state timeout as a time.Duration.
func (c *Config) CircuitBreakerTimeout() time.Duration {
	return time.Duration(c.CircuitBreakerTimeoutMs) * time.Millisecond
}

// Load reads configuration from the environment. Returns an error if a
// required variable is missing or a value fails to parse.
func Load() (*Config, error) {
	var cfg Config
	if err := sharedconfig.Load(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
