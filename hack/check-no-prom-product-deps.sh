#!/usr/bin/env bash
# check-no-prom-product-deps.sh
#
# Enforces the "Prometheus is for operational signals, structured tables
# are for product data" classification from CLAUDE.md → Observability
# Boundaries. The dashboard currently has two read paths; this lint
# prevents *new* product-class hooks from being added against Prometheus
# while the existing prom-coupled product hooks are migrated piecemeal.
#
# Product-class file patterns (any one of these flags the file):
#   dashboard/src/hooks/use-eval-*.ts(x)
#   dashboard/src/hooks/use-*-cost.ts(x)         (also matches use-*-costs.ts(x))
#   dashboard/src/hooks/use-provider-*.ts(x)
#
# A flagged file violates the rule if it imports any of:
#   @/lib/prometheus
#   @/lib/prometheus-proxy
#   @/lib/prometheus-queries
#   @/lib/data/prometheus-service
#
# Operational hooks (use-stats, use-system-metrics, use-agent-activity,
# use-agent-metrics) are intentionally NOT in the product pattern set —
# they may keep reading Prometheus.
#
# Exit codes:
#   0  no new violations
#   1  one or more new product-class files import a Prometheus client
#
# Allowlist: existing violations pinned in `hack/no-prom-product-deps.allowlist`.
# Removing a file from that list (by migrating it to a session-api endpoint)
# requires deleting the line below — the lint will tell you to.
#
# Usage:
#   bash hack/check-no-prom-product-deps.sh

set -u

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_root"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

allowlist_file="hack/no-prom-product-deps.allowlist"

allowlist="$tmp/allowed"
if [ -f "$allowlist_file" ]; then
    sed -e 's/#.*//' -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' "$allowlist_file" \
        | grep -v '^$' \
        | LC_ALL=C sort -u >"$allowlist"
else
    : >"$allowlist"
fi

# Build the set of files that match a product-class pattern AND import a
# Prometheus client. We scan everything that matches the patterns then
# grep their imports — fewer than 20 files total, no perf concern.
current="$tmp/current"
: >"$current"

# Patterns: hooks/use-eval-*.{ts,tsx}, use-*-cost{,s}.{ts,tsx}, use-provider-*.{ts,tsx}
shopt -s nullglob
candidates=(
    dashboard/src/hooks/use-eval-*.ts
    dashboard/src/hooks/use-eval-*.tsx
    dashboard/src/hooks/use-*-cost.ts
    dashboard/src/hooks/use-*-cost.tsx
    dashboard/src/hooks/use-*-costs.ts
    dashboard/src/hooks/use-*-costs.tsx
    dashboard/src/hooks/use-provider-*.ts
    dashboard/src/hooks/use-provider-*.tsx
)
shopt -u nullglob

for f in "${candidates[@]}"; do
    # Skip test files — they may import Prom helpers to mock them.
    case "$f" in
        *.test.ts|*.test.tsx) continue ;;
    esac
    if grep -qE 'from "@/lib/(data/)?prometheus(-proxy|-queries|-service)?"' "$f"; then
        echo "$f" >>"$current"
    fi
done
LC_ALL=C sort -o "$current" "$current"

new_violations="$(comm -23 "$current" "$allowlist")"
stale_allowlist="$(comm -13 "$current" "$allowlist")"

fail=0
if [ -n "$new_violations" ]; then
    fail=1
    echo "✗ new product-class hooks importing a Prometheus client (not in $allowlist_file):"
    while IFS= read -r f; do
        echo "  - $f"
    done <<<"$new_violations"
    echo ""
    echo "Product data (eval results, cost, per-tenant usage) should read from session-api,"
    echo "not Prometheus. See CLAUDE.md → 'Observability Boundaries' for the classification."
    echo ""
    echo "If you genuinely need an operational signal and the file name happens to match a"
    echo "product pattern, rename the hook to something that doesn't match (e.g. use-eval-rate"
    echo "→ use-eval-request-rate) or split the file."
fi

if [ -n "$stale_allowlist" ]; then
    fail=1
    echo "✗ stale entries in $allowlist_file (these no longer import Prom — remove them):"
    while IFS= read -r f; do
        echo "  - $f"
    done <<<"$stale_allowlist"
fi

if [ $fail -eq 0 ]; then
    current_count=$(wc -l <"$current" | tr -d '[:space:]')
    if [ "$current_count" = "0" ]; then
        echo "✓ no product-class hooks import Prometheus"
    else
        echo "✓ no new prom-product violations ($current_count pre-existing, all allowlisted)"
    fi
    exit 0
fi

exit 1
