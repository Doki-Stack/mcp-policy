package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func vector(n int) []float32 {
	v := make([]float32, n)
	for i := range v {
		v[i] = float32(i) / float32(n)
	}
	return v
}

func TestEmbed_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embeddings" {
			t.Errorf("path = %q, want /api/embeddings", r.URL.Path)
		}
		var req embeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "nomic-embed-text" {
			t.Errorf("model = %q, want nomic-embed-text", req.Model)
		}
		json.NewEncoder(w).Encode(embeddingResponse{Embedding: vector(EmbeddingDimensions)})
	}))
	defer srv.Close()

	svc := NewEmbeddingService(srv.URL, "nomic-embed-text", 5*time.Second)
	got, err := svc.Embed(context.Background(), "S3 buckets must not be public")
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	if len(got) != EmbeddingDimensions {
		t.Errorf("len(embedding) = %d, want %d", len(got), EmbeddingDimensions)
	}
}

func TestEmbed_WrongDimensions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(embeddingResponse{Embedding: vector(128)})
	}))
	defer srv.Close()

	svc := NewEmbeddingService(srv.URL, "nomic-embed-text", 5*time.Second)
	if _, err := svc.Embed(context.Background(), "query"); err == nil {
		t.Fatal("Embed should fail on wrong dimension count")
	} else if !strings.Contains(err.Error(), "768") {
		t.Errorf("error = %v, want mention of expected dimensions", err)
	}
}

func TestEmbed_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	svc := NewEmbeddingService(srv.URL, "nomic-embed-text", 5*time.Second)
	if _, err := svc.Embed(context.Background(), "query"); err == nil {
		t.Fatal("Embed should fail on server error")
	}
}

func TestEmbed_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		json.NewEncoder(w).Encode(embeddingResponse{Embedding: vector(EmbeddingDimensions)})
	}))
	defer srv.Close()

	svc := NewEmbeddingService(srv.URL, "nomic-embed-text", 5*time.Millisecond)
	if _, err := svc.Embed(context.Background(), "query"); err == nil {
		t.Fatal("Embed should fail when the call exceeds the configured timeout")
	}
}
