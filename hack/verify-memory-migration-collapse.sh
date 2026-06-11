#!/usr/bin/env bash
#
# Verify that the collapsed memory migration (000001_initial_schema, current
# working tree) produces a schema byte-identical to running the original
# 000001..000012 chain (from git HEAD) and then dropping the two embedding
# columns (which are now reconciler-owned, see #1309).
#
# Requires: docker, psql, pg_dump (libpq), git.
# Usage:    bash hack/verify-memory-migration-collapse.sh
# Exit 0 => schemas identical. Non-zero => differences (printed) or error.

set -euo pipefail

IMAGE="pgvector/pgvector:pg16"
MIG_DIR="internal/memory/postgres/migrations"
WORKDIR="$(mktemp -d)"
CA="verify-collapse-a-$$"
CB="verify-collapse-b-$$"

# Original chain, in order, as it exists at git HEAD.
ORIGINALS=(
  000001_initial_schema
  000002_consent_category
  000003_observation_fts
  000004_stateful_memory
  000005_about_nulls_not_distinct
  000006_drop_dead_columns
  000007_hnsw_embedding
  000008_workspace_index
  000009_consolidation_columns
  000010_audit_log
  000011_observation_importance
  000012_consolidation_runs
)

cleanup() {
  docker rm -f "$CA" "$CB" >/dev/null 2>&1 || true
  rm -rf "$WORKDIR"
}
trap cleanup EXIT

start_pg() {
  local name="$1"
  docker run -d --name "$name" -e POSTGRES_PASSWORD=postgres "$IMAGE" >/dev/null
  # Wait for readiness.
  for _ in $(seq 1 60); do
    if docker exec "$name" pg_isready -U postgres -d postgres >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "postgres container $name did not become ready" >&2
  return 1
}

apply_sql() {
  local name="$1"
  docker exec -i "$name" psql -U postgres -d postgres -v ON_ERROR_STOP=1 -q
}

echo "Starting postgres containers..."
start_pg "$CA"
start_pg "$CB"

echo "Side A: applying original 000001..000012 then dropping embedding columns..."
for m in "${ORIGINALS[@]}"; do
  git show "HEAD:$MIG_DIR/${m}.up.sql" | apply_sql "$CA"
done
# Drop the now-reconciler-owned vector columns (indexes drop with them).
printf 'ALTER TABLE memory_entities DROP COLUMN embedding;\nALTER TABLE memory_observations DROP COLUMN embedding;\n' | apply_sql "$CA"

echo "Side B: applying collapsed 000001 from working tree..."
apply_sql "$CB" < "$MIG_DIR/000001_initial_schema.up.sql"

echo "Dumping schemas..."
# Strip per-dump random security tokens (\restrict/\unrestrict) which differ
# on every run and are not schema.
dump() {
  docker exec "$1" pg_dump -U postgres --schema-only --no-owner --no-privileges -d postgres \
    | grep -vE '^\\(un)?restrict '
}
dump "$CA" >"$WORKDIR/a.sql"
dump "$CB" >"$WORKDIR/b.sql"

if diff -u "$WORKDIR/a.sql" "$WORKDIR/b.sql"; then
  echo "Schemas identical ✅"
  exit 0
else
  echo "Schemas differ ❌ (see diff above; a=original-chain, b=collapsed)" >&2
  exit 1
fi
