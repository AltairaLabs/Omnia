# Session API — Dev Instructions

## Running Locally

go run ./cmd/session-api/ --postgres-conn="postgres://user:pass@localhost:5432/sessions"

## Running in K8s Mode

go run ./cmd/session-api/ --workspace=dev --service-group=default

Requires K8s access and a Workspace CRD with matching service group.

## Testing

go test ./cmd/session-api/... -count=1 -v
go test ./internal/session/... -count=1 -v

## Migrations

Session-api runs its own migrations at startup from `internal/session/postgres/migrations/`.
