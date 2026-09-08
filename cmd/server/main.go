// Command server runs the Policy MCP HTTP server.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/doki-stack/mcp-policy/internal/config"
	"github.com/doki-stack/mcp-policy/internal/handler"
	"github.com/doki-stack/mcp-policy/internal/repository"
	"github.com/doki-stack/mcp-policy/internal/service"
	"github.com/doki-stack/shared-go/health"
	sharedlog "github.com/doki-stack/shared-go/logger"
	sharedmw "github.com/doki-stack/shared-go/middleware"
	"github.com/doki-stack/shared-go/otel"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log, err := sharedlog.New("mcp-policy",
		sharedlog.WithLevel(cfg.LogLevel),
		sharedlog.WithDevelopment(cfg.Environment == "development"),
	)
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer log.Sync() //nolint:errcheck

	ctx := context.Background()
	shutdownOTel, err := otel.Init(ctx, "mcp-policy",
		otel.WithExporterEndpoint(cfg.OTelExporterEndpoint),
		otel.WithEnvironment(cfg.Environment),
	)
	if err != nil {
		return fmt.Errorf("init otel: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownOTel(shutdownCtx)
	}()

	store, err := repository.NewStore(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer store.Close()

	qdrantRepo, err := repository.NewQdrantRepo(repository.QdrantConfig{
		URL:              cfg.QdrantURL,
		APIKey:           cfg.QdrantAPIKey,
		BreakerThreshold: cfg.CircuitBreakerThreshold,
		BreakerTimeout:   cfg.CircuitBreakerTimeout(),
	})
	if err != nil {
		return fmt.Errorf("connect qdrant: %w", err)
	}
	defer qdrantRepo.Close() //nolint:errcheck

	if err := qdrantRepo.EnsureCollection(ctx); err != nil {
		return fmt.Errorf("ensure qdrant collection: %w", err)
	}

	cache, err := repository.NewCacheRepo(cfg.DragonflyURL)
	if err != nil {
		return fmt.Errorf("configure cache: %w", err)
	}
	defer cache.Close() //nolint:errcheck

	embedder := service.NewEmbeddingService(cfg.OllamaBaseURL, cfg.EmbeddingModel, cfg.EmbeddingTimeout())
	engine := service.NewPolicyEngine(embedder, qdrantRepo, cache)
	costChecker := service.NewCostChecker(store)
	policyHandler := handler.NewPolicyHandler(engine, costChecker, log)

	r := chi.NewRouter()
	r.Use(chimw.RealIP)
	r.Use(sharedmw.RequestID)
	r.Use(sharedmw.Recovery(log))
	r.Use(sharedmw.Logger(log))

	// Mounts GET /healthz (liveness) and GET /readyz (readiness). All three
	// dependencies are fail-closed (ADR-005): readyz reports unhealthy if
	// any is down, since evaluate-policy cannot function without them.
	r.Mount("/", health.Handler(
		health.NewCheck("postgres", store.Ping),
		health.NewCheck("qdrant", qdrantRepo.Ping),
		health.HTTPCheck("ollama", cfg.OllamaBaseURL+"/api/tags"),
	))

	r.Post("/mcp/v1/tools/evaluate-policy", policyHandler.EvaluatePolicy)
	r.Post("/mcp/v1/tools/ingest-policy", policyHandler.IngestPolicy)
	r.Post("/mcp/v1/tools/get-policies", policyHandler.GetPolicies)
	r.Post("/mcp/v1/tools/check-cost", policyHandler.CheckCost)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("mcp-policy listening", zap.Int("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case <-stop:
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
