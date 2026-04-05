# Memory API — Dev Instructions

## Running Locally

go run ./cmd/memory-api/ --postgres-conn="postgres://user:pass@localhost:5432/memories"

With embeddings:
go run ./cmd/memory-api/ --postgres-conn="postgres://..." --embedding-provider=ollama-local --default-ttl=720h

## Running in K8s Mode

go run ./cmd/memory-api/ --workspace=dev --service-group=default

## Testing

go test ./cmd/memory-api/... -count=1 -v
go test ./internal/memory/... -count=1 -v

## Migrations

Memory-api runs its own migrations at startup from `internal/memory/postgres/migrations/`.
Separate from session-api — each service has its own database.
