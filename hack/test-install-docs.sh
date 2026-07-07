#!/usr/bin/env bash
# Smoke-test the documented Omnia install + getting-started walkthrough on k3d.
#
# Env:
#   CHART_REF                  default oci://ghcr.io/altairalabs/charts/omnia
#   CHART_VERSION              if set: install --version "$CHART_VERSION"; else --devel
#   CLUSTER_NAME               default omnia-install-test
#   INSTALL_TEST_SKIP_CLEANUP  if set (non-empty): leave the k3d cluster on exit
set -euo pipefail

CHART_REF="${CHART_REF:-oci://ghcr.io/altairalabs/charts/omnia}"
CLUSTER_NAME="${CLUSTER_NAME:-omnia-install-test}"
# Timeouts are env-overridable so slower environments (e.g. emulated nodes) can
# extend them without editing the script. Defaults suit a native CI runner.
HELM_TIMEOUT="${HELM_TIMEOUT:-5m}"
WAIT_TIMEOUT="${WAIT_TIMEOUT:-300s}"
NS_SYSTEM="omnia-system"
NS_APP="default"
SESSION_SECRET="smoke-test-only-not-a-real-secret-0000000"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PF_PID=""

log() { printf '\n=== %s ===\n' "$*"; }

cleanup() {
	[ -n "${PF_PID}" ] && kill "${PF_PID}" 2>/dev/null || true
	if [ -n "${INSTALL_TEST_SKIP_CLEANUP:-}" ]; then
		log "INSTALL_TEST_SKIP_CLEANUP set — leaving cluster ${CLUSTER_NAME}"
		return
	fi
	log "Deleting k3d cluster ${CLUSTER_NAME}"
	k3d cluster delete "${CLUSTER_NAME}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if [ -n "${CHART_VERSION:-}" ]; then
	VERSION_ARGS=(--version "${CHART_VERSION}")
else
	VERSION_ARGS=(--devel)
fi

log "Creating k3d cluster ${CLUSTER_NAME}"
k3d cluster create "${CLUSTER_NAME}" --wait --timeout 180s
kubectl config use-context "k3d-${CLUSTER_NAME}"

if [ -n "${CHART_VERSION:-}" ]; then
	log "Asserting documented --devel command resolves to ${CHART_VERSION}"
	resolved="$(helm show chart "${CHART_REF}" --devel | awk '/^version:/ {print $2}')"
	if [ "${resolved}" != "${CHART_VERSION}" ]; then
		echo "ERROR: --devel resolved '${resolved}', expected '${CHART_VERSION}'" >&2
		exit 1
	fi
fi

log "Installing operator from ${CHART_REF}"
helm install omnia "${CHART_REF}" \
	"${VERSION_ARGS[@]}" \
	--namespace "${NS_SYSTEM}" --create-namespace \
	--set dashboard.auth.mode=builtin \
	--set dashboard.auth.sessionSecret="${SESSION_SECRET}" \
	--wait --timeout "${HELM_TIMEOUT}"

log "Waiting for omnia-system deployments + CRDs"
kubectl -n "${NS_SYSTEM}" wait --for=condition=Available deployment --all --timeout="${WAIT_TIMEOUT}"
for crd in agentruntimes promptpacks providers; do
	kubectl wait --for=condition=Established "crd/${crd}.omnia.altairalabs.ai" --timeout=60s
done

log "Applying walkthrough resources (handler: demo)"
kubectl apply -f "${ROOT}/hack/testdata/install-docs/"

log "Waiting for Provider ready + agent pod running"
# Provider exposes status.phase=Ready; CR condition names vary, so the agent pod
# reaching Ready + the WebSocket roundtrip are the authoritative gates.
kubectl -n "${NS_APP}" wait --for=jsonpath='{.status.phase}'=Ready provider/my-provider --timeout=120s ||
	kubectl -n "${NS_APP}" get provider my-provider -o yaml
kubectl -n "${NS_APP}" wait --for=condition=Ready pod \
	-l app.kubernetes.io/instance=my-assistant --timeout="${WAIT_TIMEOUT}"

log "Port-forward + WebSocket roundtrip"
kubectl -n "${NS_APP}" port-forward svc/my-assistant 8080:8080 >/tmp/omnia-pf.log 2>&1 &
PF_PID=$!
for _ in $(seq 1 30); do
	if (echo >/dev/tcp/localhost/8080) 2>/dev/null; then
		break
	fi
	sleep 1
done
env GOWORK=off go -C "${ROOT}" run ./hack/wsprobe \
	--url "ws://localhost:8080/ws?agent=my-assistant" \
	--message "Hello, who are you?" \
	--timeout 30s

log "Install smoke test PASSED"
