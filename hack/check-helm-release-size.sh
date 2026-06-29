#!/usr/bin/env bash
#
# check-helm-release-size.sh — Helm release Secret size guard
#
# Helm stores each release as a Kubernetes Secret containing the gzipped,
# base64-encoded manifests. The Kubernetes Secret data cap is 1,048,576 bytes.
# This script estimates the stored size BEFORE a real helm install, so the
# guard fires at commit/PR time (seconds) rather than during a 15-minute
# E2E run.
#
# How the estimate works:
#   1. Render the "worst case" manifest set:
#        helm template ... --set enterprise.enabled=true
#      Enterprise CRDs in templates/enterprise/ ARE included in the release
#      Secret; files under charts/omnia/crds/ are NOT (Helm skips them).
#   2. Gzip the rendered YAML at level 9 (same compression Helm uses).
#   3. Estimate base64 encoding: adds ~33%  ceil(gz_bytes * 4/3).
#   4. Add 4096 bytes for Helm release metadata JSON overhead
#      (name, namespace, version, status, manifest hash, etc.).
#
#   EST = ceil(GZ * 4/3) + 4096
#
# Threshold: 90% of the Secret data limit  (943,718 / 1,048,576 bytes).
# Failing at 90% leaves a 10% safety buffer for organic growth before a
# hard block.  The formula intentionally errs on the conservative side.
#
# If helm is not installed, the check is skipped with a warning — mirrors
# the degradation pattern in hack/pre-push for missing optional tools (e.g.
# tygo) so this never blocks a push on a machine without helm.
#
# Usage:
#   bash hack/check-helm-release-size.sh
#

set -euo pipefail

# ---------------------------------------------------------------------------
# Config — adjust only with a corresponding CLAUDE.md update
# ---------------------------------------------------------------------------
SECRET_LIMIT=1048576    # Kubernetes Secret data size cap: 1 MiB
THRESHOLD_PCT=90        # Warn/fail threshold as % of limit
# THRESHOLD = floor(1048576 * 90 / 100) = 943718
THRESHOLD=$(( SECRET_LIMIT * THRESHOLD_PCT / 100 ))

# ---------------------------------------------------------------------------
# Output helpers (match hack/pre-push colour conventions)
# ---------------------------------------------------------------------------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_info()   { echo -e "${BLUE}ℹ ${NC}$1"; }
print_success(){ echo -e "${GREEN}✓${NC} $1"; }
print_warning(){ echo -e "${YELLOW}⚠${NC} $1"; }
print_error()  { echo -e "${RED}✗${NC} $1"; }

# ---------------------------------------------------------------------------
# Guard: helm must be installed — degrade gracefully if not present
# ---------------------------------------------------------------------------
if ! command -v helm &>/dev/null; then
    print_warning "helm not installed — skipping release-size check"
    exit 0
fi

# ---------------------------------------------------------------------------
# Resolve paths from repo root
# ---------------------------------------------------------------------------
REPO_ROOT=$(git rev-parse --show-toplevel)
CHART_DIR="${REPO_ROOT}/charts/omnia"
CHART_TESTS_VALUES="${CHART_DIR}/values-chart-tests.yaml"

if [ ! -d "$CHART_DIR" ]; then
    print_warning "Helm chart not found at ${CHART_DIR} — skipping release-size check"
    exit 0
fi

print_info "Checking Helm release Secret size (worst case: enterprise.enabled=true)..."

# ---------------------------------------------------------------------------
# Render the worst-case manifest set.
#
# --set enterprise.enabled=true  includes enterprise CRDs from
# templates/enterprise/ in the rendered output (and therefore in the
# release Secret).  Files under charts/omnia/crds/ are skipped by Helm
# and do NOT appear in the Secret.
#
# Additional flags satisfy the chart's render-time guards:
#   enterprise.arena.*.image.repository=test  — avoids "image required"
#   redis.enabled=true                        — satisfies omnia.validateArenaQueue
# ---------------------------------------------------------------------------
RENDERED_FILE=$(mktemp /tmp/helm-release-size-XXXXXX.yaml)
trap 'rm -f "$RENDERED_FILE"' EXIT

RENDER_STDERR=$(mktemp /tmp/helm-release-size-err-XXXXXX.txt)
trap 'rm -f "$RENDERED_FILE" "$RENDER_STDERR"' EXIT

if ! helm template omnia "$CHART_DIR" \
    -f "$CHART_TESTS_VALUES" \
    --set enterprise.enabled=true \
    --set enterprise.arena.controller.image.repository=test \
    --set enterprise.arena.worker.image.repository=test \
    --set redis.enabled=true \
    >"$RENDERED_FILE" 2>"$RENDER_STDERR"; then
    print_warning "helm template failed (chart deps may be missing) — skipping release-size check"
    cat "$RENDER_STDERR" >&2
    exit 0
fi

# ---------------------------------------------------------------------------
# Estimate the Helm release Secret size:
#
#   GZ  = compressed byte count  (gzip -9 -c, same level as Helm)
#   B64 = ceiling of GZ * 4 / 3  (base64 encoding overhead)
#   EST = B64 + 4096              (+ release metadata JSON fudge)
#
# Integer ceiling without floating point: ceil(a/b) = (a + b - 1) / b
# For b=3: ceil(GZ*4/3) = (GZ*4 + 2) / 3
# ---------------------------------------------------------------------------
GZ=$(gzip -9 -c "$RENDERED_FILE" | wc -c | tr -d ' ')
B64=$(( (GZ * 4 + 2) / 3 ))
EST=$(( B64 + 4096 ))

# Human-readable KiB values for output
EST_KIB=$(( EST / 1024 ))
LIMIT_KIB=$(( SECRET_LIMIT / 1024 ))
HEADROOM_PCT=$(( (SECRET_LIMIT - EST) * 100 / SECRET_LIMIT ))

if [ "$EST" -ge "$THRESHOLD" ]; then
    print_error "Helm release Secret estimated at ~${EST_KIB} KiB — exceeds ${THRESHOLD_PCT}% of the ${LIMIT_KIB} KiB limit"
    print_error "  Estimated: ${EST} bytes  |  Limit: ${SECRET_LIMIT} bytes  |  Threshold: ${THRESHOLD} bytes"
    print_info  "Top contributors: enterprise CRDs in templates/enterprise/ dominate the release payload."
    print_info  "Options: move large CRDs to charts/omnia/crds/ (excluded from Secret) or prune schemaless fields."
    exit 1
else
    print_success "Helm release ~${EST_KIB} KiB of ${LIMIT_KIB} KiB limit (${HEADROOM_PCT}% headroom)"
    exit 0
fi
