#!/usr/bin/env bash
#
# Verify RBAC sync between kustomize and Helm.
#
# Why this exists:
#   #731 fixed two RBAC regressions where the kustomize manifests (under
#   config/rbac/) and the Helm chart (charts/omnia/templates/*.yaml) had
#   drifted silently. Both tree are maintained by hand — kubebuilder markers
#   update config/rbac/role.yaml, hand edits update the Helm chart, and
#   nothing ensures the two stay in sync. The two specific regressions were:
#
#     1. `clusterrolebindings` list permission was in Helm but missing from
#        kustomize (because a +kubebuilder:rbac marker was attached to a
#        helper function, which controller-gen does not scan).
#     2. The `agent-workspace-reader` ClusterRole existed only in the Helm
#        chart, so kustomize-based deployments (including Core E2E) had a
#        dangling ClusterRoleBinding reference.
#
#   Both bugs silently broke Core E2E for ~3 weeks before anyone noticed.
#
# What this script does:
#   Renders both sources, picks the ClusterRoles that appear in *both*
#   (Helm intentionally omits kubebuilder's CRD editor/viewer aggregation
#   roles), and diffs the normalized (apiGroup, resource, verb) triples.
#   Fails with a human-readable diff if the two drift.
#
# Tracked by: #733
#
# Usage:
#   ./hack/verify-rbac-sync.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()    { echo -e "${BLUE}ℹ${NC}  $*"; }
ok()      { echo -e "${GREEN}✓${NC}  $*"; }
fail()    { echo -e "${RED}✗${NC}  $*"; }

for cmd in kustomize helm yq; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
        fail "$cmd not found in PATH"
        exit 2
    fi
done

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

KUSTOMIZE_OUT="$TMP_DIR/kustomize.yaml"
HELM_OUT="$TMP_DIR/helm.yaml"

info "Rendering kustomize (config/default)..."
kustomize build "$REPO_ROOT/config/default" > "$KUSTOMIZE_OUT"

info "Rendering Helm chart (charts/omnia)..."
# Render with defaults. RBAC is controlled by .Values.rbac.create (default true).
helm template omnia "$REPO_ROOT/charts/omnia" > "$HELM_OUT"

# Extract normalized rule triples for a given ClusterRole name from a source.
# Each rule expands to the cartesian product (apiGroup, resource, verb), one
# per line, sorted and deduplicated. "" (core API group) is preserved so
# missing-vs-explicit-empty-string isn't flagged as drift.
extract_rules() {
    local source_file=$1
    local role_name=$2
    yq eval-all "
        select(.kind == \"ClusterRole\" and .metadata.name == \"$role_name\") |
        .rules[] |
        (.apiGroups // [\"\"]) as \$groups |
        (.resources // []) as \$resources |
        (.verbs // []) as \$verbs |
        \$groups[] as \$g |
        \$resources[] as \$r |
        \$verbs[] as \$v |
        \$g + \"|\" + \$r + \"|\" + \$v
    " "$source_file" 2>/dev/null | sort -u
}

# List of ClusterRole names present in both sources — these are the ones we
# enforce sync on. Kustomize emits extra kubebuilder-generated CRD editor /
# viewer aggregation roles that Helm intentionally omits; those are ignored.
# yq emits "---" between matching documents when multiple docs match a
# select() — filter it out so it doesn't show up as a fake role name.
kustomize_roles=$(yq eval-all 'select(.kind == "ClusterRole") | .metadata.name' "$KUSTOMIZE_OUT" | grep -v '^---$' | sort -u)
helm_roles=$(yq eval-all 'select(.kind == "ClusterRole") | .metadata.name' "$HELM_OUT" | grep -v '^---$' | sort -u)
shared_roles=$(comm -12 <(echo "$kustomize_roles") <(echo "$helm_roles"))

if [ -z "$shared_roles" ]; then
    fail "No ClusterRoles found in both sources — is kustomize/helm rendering broken?"
    exit 2
fi

info "Checking shared ClusterRoles:"
while IFS= read -r r; do
    [ -n "$r" ] && echo "    - $r"
done <<< "$shared_roles"
echo ""

DRIFT=0
while IFS= read -r role; do
    [ -z "$role" ] && continue

    kustomize_rules=$(extract_rules "$KUSTOMIZE_OUT" "$role")
    helm_rules=$(extract_rules "$HELM_OUT" "$role")

    only_in_kustomize=$(comm -23 <(echo "$kustomize_rules") <(echo "$helm_rules"))
    only_in_helm=$(comm -13 <(echo "$kustomize_rules") <(echo "$helm_rules"))

    if [ -z "$only_in_kustomize" ] && [ -z "$only_in_helm" ]; then
        ok "$role: in sync"
        continue
    fi

    DRIFT=1
    fail "$role: drift detected"
    if [ -n "$only_in_kustomize" ]; then
        echo -e "    ${YELLOW}Missing from Helm${NC} (present in kustomize):"
        echo "$only_in_kustomize" | awk -F'|' '{printf "      - apiGroup=%s resource=%s verb=%s\n", $1, $2, $3}'
    fi
    if [ -n "$only_in_helm" ]; then
        echo -e "    ${YELLOW}Missing from kustomize${NC} (present in Helm):"
        echo "$only_in_helm" | awk -F'|' '{printf "      - apiGroup=%s resource=%s verb=%s\n", $1, $2, $3}'
    fi
done <<< "$shared_roles"

echo ""
if [ "$DRIFT" -eq 0 ]; then
    ok "All shared ClusterRoles in sync between kustomize and Helm"
    exit 0
fi

fail "RBAC drift detected between kustomize and Helm"
echo ""
echo "To fix:"
echo "  - If a rule comes from a +kubebuilder:rbac marker: run 'make manifests'"
echo "    to regenerate config/rbac/role.yaml, then hand-edit the matching"
echo "    charts/omnia/templates/clusterrole.yaml to match."
echo "  - If a rule is hand-added to Helm: add a matching kubebuilder marker"
echo "    (above a reconcile method that can see it) or a hand-edited entry"
echo "    under config/rbac/."
echo ""
echo "See issue #733 for context on why these drift silently."
exit 1
