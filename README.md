# mcp-policy

Policy MCP server for the Doki Stack platform. Enforces security, compliance, and cost policies using Qdrant-based RAG retrieval. **Fail-closed by design** — if policy context is unavailable, the system blocks.

## Purpose

The Policy MCP is the guardrails layer of the platform. Before any infrastructure change is planned or applied, the agents consult this service to evaluate the request against organizational policies. If the Policy MCP or its dependencies (Qdrant, embeddings) are unavailable, the entire system blocks — it never proceeds without policy context.

## Technology Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.22+ |
| Router | github.com/go-chi/chi/v5 |
| Database | github.com/jackc/pgx/v5 (PostgreSQL) |
| Vector DB | github.com/qdrant/go-client (Qdrant) |
| Cache | github.com/redis/go-redis/v9 (Dragonfly) |
| Embeddings | nomic-embed-text via Ollama/vLLM HTTP API |
| Observability | go.opentelemetry.io/otel |
| Logging | go.uber.org/zap |
| Shared Module | github.com/doki-stack/shared-go |

## MCP Tools

| Tool | Description |
|------|-------------|
| `evaluate-policy` | Evaluate a proposed plan against all applicable policies |
| `get-policies` | Retrieve policies matching a query (semantic search via Qdrant) |
| `ingest-policy` | Ingest new policy documents into the vector store |
| `check-cost` | Check an estimated cost against the org's configured budget for a resource type |

## Key Behaviors

- **Fail-closed**: If Qdrant or embedding service is unavailable → BLOCK (not proceed)
- **Circuit breaker**: 5 consecutive Qdrant failures → circuit open → fail closed
- **Cache**: Frequently queried policies cached in Dragonfly with TTL
- **Multi-tenant**: All queries scoped by `org_id`

## Implementation Phase

**Phase 1** (Weeks 9-14) — Built after shared-go and db-schemas are stable.

## License

Apache License 2.0 — see [LICENSE](LICENSE)
