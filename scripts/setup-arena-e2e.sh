#!/bin/bash
# Setup Arena E2E environment in Kind
# This mirrors the Tilt enterprise setup for comparison/debugging
#
# Usage:
#   ./scripts/setup-arena-e2e.sh        # Set up the environment
#   ./scripts/setup-arena-e2e.sh clean  # Tear down the environment
#
# Environment variables:
#   KIND_CLUSTER      - Name of the kind cluster (default: omnia-arena-e2e)
#   SKIP_BUILD        - Skip building images (default: false)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Configuration
KIND_CLUSTER="${KIND_CLUSTER:-omnia-arena-e2e}"
NAMESPACE="omnia-system"

# Image names (matching Tiltfile dev images)
OPERATOR_IMAGE="omnia-operator-dev:latest"
FACADE_IMAGE="omnia-facade-dev:latest"
RUNTIME_IMAGE="omnia-runtime-dev:latest"
ARENA_CONTROLLER_IMAGE="omnia-arena-controller-dev:latest"
ARENA_WORKER_IMAGE="omnia-arena-worker-dev:latest"

cd "$PROJECT_ROOT"

log_info() { echo -e "\033[0;32m[INFO]\033[0m $1"; }
log_warn() { echo -e "\033[1;33m[WARN]\033[0m $1"; }
log_error() { echo -e "\033[0;31m[ERROR]\033[0m $1"; }

# Clean up function
cleanup() {
    log_info "Cleaning up Arena E2E environment..."
    if kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER}$"; then
        kind delete cluster --name "$KIND_CLUSTER"
    fi
    log_info "Cleanup complete"
}

if [[ "$1" == "clean" ]]; then
    cleanup
    exit 0
fi

# Check prerequisites
log_info "Checking prerequisites..."
for cmd in kind kubectl helm docker; do
    command -v $cmd >/dev/null 2>&1 || { log_error "Missing: $cmd"; exit 1; }
done

# Create kind cluster
if kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER}$"; then
    log_info "Kind cluster '${KIND_CLUSTER}' already exists"
    kubectl config use-context "kind-${KIND_CLUSTER}"
else
    log_info "Creating kind cluster '${KIND_CLUSTER}'..."
    kind create cluster --name "$KIND_CLUSTER"
fi

# Build images (unless skipped)
if [[ "${SKIP_BUILD:-false}" != "true" ]]; then
    log_info "Building container images in parallel..."

    docker build -t "$OPERATOR_IMAGE" -f Dockerfile . &
    docker build -t "$FACADE_IMAGE" -f Dockerfile.agent . &
    docker build -t "$RUNTIME_IMAGE" -f Dockerfile.runtime . &
    docker build -t "$ARENA_CONTROLLER_IMAGE" -f ee/Dockerfile.arena-controller . &
    docker build -t "$ARENA_WORKER_IMAGE" -f ee/Dockerfile.arena-worker . &

    wait
    log_info "All images built"
else
    log_info "Skipping image builds"
fi

# Load images into kind
log_info "Loading images into kind..."
for img in "$OPERATOR_IMAGE" "$FACADE_IMAGE" "$RUNTIME_IMAGE" "$ARENA_CONTROLLER_IMAGE" "$ARENA_WORKER_IMAGE"; do
    kind load docker-image "$img" --name "$KIND_CLUSTER" &
done
wait
log_info "Images loaded"

# Deploy with Helm (matching Tilt enterprise setup - NO --wait flag)
log_info "Deploying via Helm..."

helm upgrade --install omnia charts/omnia \
    --namespace "$NAMESPACE" \
    --create-namespace \
    --set image.repository=omnia-operator-dev \
    --set image.tag=latest \
    --set image.pullPolicy=Never \
    --set dashboard.enabled=false \
    --set facade.image.repository=omnia-facade-dev \
    --set facade.image.tag=latest \
    --set facade.image.pullPolicy=Never \
    --set framework.image.repository=omnia-runtime-dev \
    --set framework.image.tag=latest \
    --set framework.image.pullPolicy=Never \
    --set enterprise.enabled=true \
    --set devMode=true \
    --set enterprise.arena.controller.image.repository=omnia-arena-controller-dev \
    --set enterprise.arena.controller.image.tag=latest \
    --set enterprise.arena.controller.image.pullPolicy=Never \
    --set enterprise.arena.worker.image.repository=omnia-arena-worker-dev \
    --set enterprise.arena.worker.image.tag=latest \
    --set enterprise.arena.worker.image.pullPolicy=Never \
    --set enterprise.arena.queue.type=redis \
    --set enterprise.arena.queue.redis.host=omnia-redis-master \
    --set enterprise.arena.queue.redis.port=6379 \
    --set redis.enabled=true \
    --set redis.architecture=standalone \
    --set redis.auth.enabled=false \
    --set redis.master.persistence.enabled=false \
    --set nfs.server.enabled=false \
    --set nfs.csiDriver.enabled=false \
    --set prometheus.enabled=false \
    --set grafana.enabled=false \
    --set loki.enabled=false \
    --set tempo.enabled=false \
    --set alloy.enabled=false \
    --set keda.enabled=false

# Create storage class for tests (uses kind's local-path)
log_info "Creating storage class..."
kubectl apply -f - <<EOF
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: omnia-nfs
provisioner: rancher.io/local-path
reclaimPolicy: Delete
volumeBindingMode: WaitForFirstConsumer
EOF

# Wait for critical deployments only
log_info "Waiting for deployments..."
kubectl rollout status deployment/omnia-controller-manager -n "$NAMESPACE" --timeout=3m
kubectl rollout status deployment/omnia-arena-controller -n "$NAMESPACE" --timeout=3m
kubectl rollout status statefulset/omnia-redis-master -n "$NAMESPACE" --timeout=3m

log_info "Arena E2E environment ready!"
echo ""
echo "Cluster: kind-${KIND_CLUSTER}"
echo "Context: kubectl config use-context kind-${KIND_CLUSTER}"
echo ""
kubectl get pods -n "$NAMESPACE"
echo ""
echo "To clean up: ./scripts/setup-arena-e2e.sh clean"
