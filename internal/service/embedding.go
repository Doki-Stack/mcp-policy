// Package service implements the Policy MCP's business logic: embedding
// generation, policy evaluation (Qdrant search + re-ranking), and cost
// checking.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// EmbeddingDimensions is the vector size produced by nomic-embed-text, and
// the dimensionality configured on the Qdrant `policies` collection.
const EmbeddingDimensions = 768

// EmbeddingService generates text embeddings via an Ollama-compatible HTTP API.
type EmbeddingService struct {
	baseURL string
	model   string
	timeout time.Duration
	client  *http.Client
}

// NewEmbeddingService creates an EmbeddingService targeting baseURL (e.g.
// http://ollama:11434) using the given model and per-request timeout.
func NewEmbeddingService(baseURL, model string, timeout time.Duration) *EmbeddingService {
	return &EmbeddingService{
		baseURL: baseURL,
		model:   model,
		timeout: timeout,
		client:  &http.Client{},
	}
}

type embeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type embeddingResponse struct {
	Embedding []float32 `json:"embedding"`
}

// Embed returns the embedding vector for text. Fails if the call exceeds the
// configured timeout, the server errors, or the returned vector isn't
// EmbeddingDimensions long.
func (e *EmbeddingService) Embed(ctx context.Context, text string) ([]float32, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	body, err := json.Marshal(embeddingRequest{Model: e.model, Prompt: text})
	if err != nil {
		return nil, fmt.Errorf("embedding: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embedding: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding: unexpected status %d", resp.StatusCode)
	}

	var out embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("embedding: decode response: %w", err)
	}
	if len(out.Embedding) != EmbeddingDimensions {
		return nil, fmt.Errorf("embedding: expected %d dimensions, got %d", EmbeddingDimensions, len(out.Embedding))
	}
	return out.Embedding, nil
}
