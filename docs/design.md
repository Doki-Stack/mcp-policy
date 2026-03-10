# mcp-policy — High-Level Design

## Overview

The Policy MCP server is the security and compliance enforcement layer. It uses Qdrant as a vector database to store and retrieve organizational policies via semantic search (RAG). Agents query this service before generating or executing any infrastructure plan.

## Architecture

```
mcp-policy/
├── cmd/
│   └── server/
│       └── main.go            # Entry point, chi router, graceful shutdown
├── internal/
│   ├── handler/               # HTTP handlers for MCP tools
│   │   ├── evaluate.go
│   │   ├── policies.go
│   │   └── ingest.go
│   ├── service/               # Business logic
│   │   ├── policy.go          # Policy evaluation engine
│   │   └── embedding.go       # Embedding generation via Ollama
│   ├── repository/            # Data access
│   │   ├── qdrant.go          # Qdrant vector operations
│   │   ├── postgres.go        # Policy metadata in PostgreSQL
│   │   └── cache.go           # Dragonfly cache layer
│   └── model/                 # Domain types
│       └── policy.go
├── go.mod
├── go.sum
├── Dockerfile
├── openapi.yaml
└── README.md
```

## Data Flow

```
Agent request → evaluate-policy tool
  → Embed query via nomic-embed-text
  → Search Qdrant for relevant policies (filtered by org_id)
  → Score and rank policy matches
  → Return policy evaluation result (pass/fail/warn with details)
```

## Fail-Closed Behavior

| Dependency Down | System Behavior |
|----------------|-----------------|
| Qdrant | BLOCK — return `POLICY_MCP_UNAVAILABLE` |
| Embedding service | BLOCK — cannot evaluate without embeddings |
| PostgreSQL | BLOCK — cannot verify policy metadata |
| Dragonfly (cache) | PROCEED — higher latency, but policy still queryable |

## Dependencies

| Dependency | Type |
|-----------|------|
| `shared-go` | Go module (error envelopes, logger, OTel, middleware) |
| `db-schemas` | SQL migrations (policy tables) |
| Qdrant | Vector database (runtime) |
| PostgreSQL | Relational database (runtime) |
| Dragonfly | Cache (runtime, optional) |
| Ollama/vLLM | Embedding generation (runtime) |
