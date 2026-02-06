#!/usr/bin/env bash
#
# Validate Helm templates
# - Runs helm lint with strict mode
# - Renders templates with common configurations
#
# Usage:
#   ./hack/validate-helm.sh
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CHART_DIR="$REPO_ROOT/charts/omnia"

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

print_info() { echo -e "${BLUE}ℹ ${NC}$1"; }
print_success() { echo -e "${GREEN}✓${NC} $1"; }
print_error() { echo -e "${RED}✗${NC} $1"; }

FAILED=0

#
# 1. Helm lint with strict mode
#
print_info "Running helm lint --strict..."
if helm lint "$CHART_DIR" --strict 2>&1; then
    print_success "Helm lint passed"
else
    print_error "Helm lint failed"
    FAILED=1
fi

#
# 2. Template rendering with different value combinations
#
print_info "Testing template rendering..."

# Test default values
if helm template omnia "$CHART_DIR" > /dev/null 2>&1; then
    print_success "Template renders with default values"
else
    print_error "Template failed with default values"
    helm template omnia "$CHART_DIR" 2>&1 | tail -20
    FAILED=1
fi

# Test with enterprise enabled
if helm template omnia "$CHART_DIR" \
    --set enterprise.enabled=true \
    --set enterprise.arena.controller.image.repository=test \
    --set enterprise.arena.worker.image.repository=test \
    > /dev/null 2>&1; then
    print_success "Template renders with enterprise enabled"
else
    print_error "Template failed with enterprise enabled"
    FAILED=1
fi

#
# Summary
#
if [[ $FAILED -eq 0 ]]; then
    print_success "All Helm validations passed"
    exit 0
else
    print_error "Helm validation failed"
    exit 1
fi
