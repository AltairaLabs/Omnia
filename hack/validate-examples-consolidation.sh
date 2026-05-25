#!/usr/bin/env bash
#
# Server-side dry-run validation of the example consolidation packs
# against the live AgentRuntime + PromptPack CRDs.
#
# Requires:
#   - A kubectl context pointing at a cluster with omnia CRDs installed
#   - The `omnia-functions` namespace to exist (the packs reference it)
#
# Exits 0 when both packs validate; non-zero on any rejection.

set -euo pipefail
cd "$(dirname "$0")/.."

kubectl create namespace omnia-functions --dry-run=client -o yaml | kubectl apply -f - >/dev/null

for pack in examples/consolidation/demo-rescope examples/consolidation/demo-merge-entities; do
  echo "validating $pack ..."
  kubectl apply --dry-run=server -f "$pack/" >/dev/null
done
echo OK
