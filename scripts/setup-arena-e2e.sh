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
ARENA_DEV_CONSOLE_IMAGE="omnia-arena-dev-console-dev:latest"
SESSION_API_IMAGE="omnia-session-api-dev:latest"
MEMORY_API_IMAGE="omnia-memory-api-dev:latest"
EVAL_WORKER_IMAGE="omnia-eval-worker-dev:latest"

cd "$PROJECT_ROOT"

log_info() { echo -e "\033[0;32m[INFO]\033[0m $1"; }
log_warn() { echo -e "\033[1;33m[WARN]\033[0m $1"; }
log_error() { echo -e "\033[0;31m[ERROR]\033[0m $1"; }

retry() {
    local retries=${1}; shift
    local delay=${1}; shift
    local attempt=0
    until "$@"; do
        attempt=$((attempt + 1))
        if [ "$attempt" -ge "$retries" ]; then
            log_error "Command failed after $retries attempts: $*"
            return 1
        fi
        log_warn "Attempt $attempt/$retries failed. Retrying in ${delay}s..."
        sleep "$delay"
    done
}

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
    kind create cluster --name "$KIND_CLUSTER" --wait 60s
fi

# Build images (unless skipped)
#
# Strategy: build all binaries natively via `go build` in one pass — this shares
# the module cache and the build cache across binaries. Then wrap each binary
# in a thin Dockerfile that only COPYs the pre-built binary (no Go toolchain
# in the image). This replaces 8 independent multi-stage docker builds (each
# re-downloading modules and re-compiling shared packages) and cuts BeforeSuite
# time from ~15-18m to a few minutes. Mirrors test/e2e/e2e_suite_test.go which
# uses the same pattern for Core E2E. See #732.
if [[ "${SKIP_BUILD:-false}" != "true" ]]; then
    DIST_DIR="$PROJECT_ROOT/dist/arena-e2e"
    log_info "Building binaries natively (shared Go cache)..."
    rm -rf "$DIST_DIR"
    mkdir -p "$DIST_DIR"

    # name|package|image
    BUILD_SPECS="
manager|./cmd|$OPERATOR_IMAGE
agent|./cmd/agent|$FACADE_IMAGE
runtime|./cmd/runtime|$RUNTIME_IMAGE
session-api|./cmd/session-api|$SESSION_API_IMAGE
memory-api|./cmd/memory-api|omnia-memory-api-dev:latest
arena-controller|./ee/cmd/omnia-arena-controller|$ARENA_CONTROLLER_IMAGE
arena-worker|./ee/cmd/arena-worker|$ARENA_WORKER_IMAGE
arena-dev-console|./ee/cmd/arena-dev-console|$ARENA_DEV_CONSOLE_IMAGE
arena-eval-worker|./ee/cmd/arena-eval-worker|$EVAL_WORKER_IMAGE
"

    build_one() {
        local name=$1 pkg=$2
        local out_dir="$DIST_DIR/$name"
        mkdir -p "$out_dir"
        # CGO_ENABLED=0 for static binaries (distroless has no libc);
        # -ldflags="-w -s" strips debug info to shrink the binary;
        # GOWORK=off ignores any local go.work file (e.g. promptkit-local
        # overrides) so the build uses the published SDK from go.mod, matching
        # the in-docker build path. Without this the build fails locally when
        # the developer has a promptkit-local checkout.
        env GOWORK=off CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
            go build -ldflags="-w -s" -o "$out_dir/$name" "$pkg"
    }

    # Parallel native build. Binaries share Go's build cache so shared
    # packages (internal/, api/, pkg/) only compile once across the suite.
    build_pids=()
    while IFS='|' read -r name pkg _; do
        [ -z "$name" ] && continue
        ( build_one "$name" "$pkg" ) &
        build_pids+=($!)
    done <<< "$BUILD_SPECS"
    build_fail=0
    for pid in "${build_pids[@]}"; do
        if ! wait "$pid"; then
            build_fail=1
        fi
    done
    if [ "$build_fail" -ne 0 ]; then
        log_error "One or more native builds failed"
        exit 1
    fi
    log_info "All binaries built natively"

    log_info "Packaging binaries into thin container images in parallel..."
    pkg_pids=()
    while IFS='|' read -r name pkg image; do
        [ -z "$name" ] && continue
        ctx="$DIST_DIR/$name"
        cat > "$ctx/Dockerfile" <<DOCKEREOF
FROM gcr.io/distroless/static:nonroot
COPY $name /$name
USER 65532:65532
ENTRYPOINT ["/$name"]
DOCKEREOF
        docker build -q -t "$image" "$ctx" &
        pkg_pids+=($!)
    done <<< "$BUILD_SPECS"
    pkg_fail=0
    for pid in "${pkg_pids[@]}"; do
        if ! wait "$pid"; then
            pkg_fail=1
        fi
    done
    if [ "$pkg_fail" -ne 0 ]; then
        log_error "One or more image packagings failed"
        exit 1
    fi
    log_info "All images built"
else
    log_info "Skipping image builds"
fi

# Load images into kind
log_info "Loading images into kind..."
for img in "$OPERATOR_IMAGE" "$FACADE_IMAGE" "$RUNTIME_IMAGE" "$ARENA_CONTROLLER_IMAGE" "$ARENA_WORKER_IMAGE" "$ARENA_DEV_CONSOLE_IMAGE" "$SESSION_API_IMAGE" "$MEMORY_API_IMAGE" "$EVAL_WORKER_IMAGE"; do
    kind load docker-image "$img" --name "$KIND_CLUSTER" &
done
wait
log_info "Images loaded"

# Pull and load third-party images that kind nodes can't pull (Docker Hub rate limits in CI)
REDIS_IMAGE="docker.io/bitnami/redis:latest"
log_info "Pulling third-party images..."
docker pull "$REDIS_IMAGE"
kind load docker-image "$REDIS_IMAGE" --name "$KIND_CLUSTER"
log_info "Third-party images loaded"

# Deploy with Helm (matching Tilt enterprise setup - NO --wait flag)
# Download each Helm dependency individually with retries. This prevents a
# transient CDN failure for one disabled subchart (e.g. 502 from Grafana's
# GitHub-hosted repo) from blocking the entire E2E setup.
log_info "Building Helm dependencies individually..."
mkdir -p charts/omnia/charts

# Download each dependency individually with retries. If a disabled dependency
# fails (e.g. 502 from Grafana's GitHub CDN), create a stub chart so Helm's
# dependency check passes — we don't need its contents since it won't be rendered.
#
# Format: name|repo|version|required (1=required, 0=disabled in E2E)
DEPS="
prometheus|https://prometheus-community.github.io/helm-charts|27.0.0|0
grafana|https://grafana.github.io/helm-charts|10.0.0|0
loki|https://grafana.github.io/helm-charts|6.0.0|0
alloy|https://grafana.github.io/helm-charts|0.10.1|0
tempo|https://grafana.github.io/helm-charts|1.0.3|0
keda|https://kedacore.github.io/charts|2.16.1|0
redis|https://charts.bitnami.com/bitnami|24.0.9|1
csi-driver-nfs|https://raw.githubusercontent.com/kubernetes-csi/csi-driver-nfs/master/charts|v4.9.0|0
"

create_stub_chart() {
    local name=$1 version=$2
    local stub_dir
    stub_dir=$(mktemp -d)
    mkdir -p "$stub_dir/$name"
    cat > "$stub_dir/$name/Chart.yaml" <<STUBEOF
apiVersion: v2
name: $name
version: $version
description: stub chart for E2E
STUBEOF
    tar -czf "charts/omnia/charts/${name}-${version}.tgz" -C "$stub_dir" "$name"
    rm -rf "$stub_dir"
}

while IFS='|' read -r name repo version required; do
    # Skip blank lines
    [ -z "$name" ] && continue

    if [ -f "charts/omnia/charts/${name}-${version}.tgz" ]; then
        log_info "  $name-$version already exists, skipping"
        continue
    fi

    if retry 3 10 helm pull "$name" --repo "$repo" --version "$version" --destination charts/omnia/charts; then
        log_info "  Downloaded $name-$version"
    elif [ "$required" = "0" ]; then
        log_warn "  Failed to download $name-$version (disabled dep), creating stub"
        create_stub_chart "$name" "$version"
    else
        log_error "  Failed to download required dependency: $name-$version"
        exit 1
    fi
done <<< "$DEPS"

log_info "Deploying via Helm..."

retry 2 15 helm upgrade --install omnia charts/omnia \
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
    --set enterprise.arena.devConsole.image.repository=omnia-arena-dev-console-dev \
    --set enterprise.arena.devConsole.image.tag=latest \
    --set enterprise.arena.devConsole.image.pullPolicy=Never \
    --set workspaceServices.sessionApi.image.repository=omnia-session-api-dev \
    --set workspaceServices.sessionApi.image.tag=latest \
    --set workspaceServices.sessionApi.image.pullPolicy=Never \
    --set workspaceServices.memoryApi.image.repository=omnia-memory-api-dev \
    --set workspaceServices.memoryApi.image.tag=latest \
    --set workspaceServices.memoryApi.image.pullPolicy=Never \
    --set postgres.dev.enabled=true \
    --set enterprise.evalWorker.image.repository=omnia-eval-worker-dev \
    --set enterprise.evalWorker.image.tag=latest \
    --set enterprise.evalWorker.image.pullPolicy=Never \
    --set enterprise.evalWorker.workspaceNamespace=dev-agents \
    --set enterprise.arena.queue.type=redis \
    --set enterprise.arena.queue.redis.host=omnia-redis-master \
    --set enterprise.arena.queue.redis.port=6379 \
    --set redis.enabled=true \
    --set redis.architecture=standalone \
    --set redis.auth.enabled=false \
    --set redis.image.tag=latest \
    --set redis.master.persistence.enabled=false \
    --set redis.master.podSecurityContext.enabled=false \
    --set redis.master.containerSecurityContext.enabled=false \
    --set nfs.server.enabled=false \
    --set nfs.csiDriver.enabled=false \
    --set workspaceContent.persistence.accessModes[0]=ReadWriteOnce \
    --set workspaceContent.persistence.storageClass=standard \
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
kubectl rollout status statefulset/omnia-postgres -n "$NAMESPACE" --timeout=3m

# Create separate databases for session-api and memory-api. They cannot share
# a database because their schema migrations use the same tracking table and
# collide. The dev postgres initializes with a single "omnia" database.
log_info "Creating per-service databases in postgres..."
kubectl exec -n "$NAMESPACE" omnia-postgres-0 -- psql -U omnia -d omnia -c "CREATE DATABASE omnia_sessions;" 2>/dev/null || true
kubectl exec -n "$NAMESPACE" omnia-postgres-0 -- psql -U omnia -d omnia -c "CREATE DATABASE omnia_memory;" 2>/dev/null || true

# Create Workspace CRD so the operator provisions per-workspace session-api
# and memory-api instances in the dev-agents namespace.
log_info "Creating dev-agents Workspace..."
kubectl apply -f - <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: dev-agents
---
apiVersion: v1
kind: Secret
metadata:
  name: omnia-postgres
  namespace: dev-agents
type: Opaque
stringData:
  POSTGRES_CONN: "postgres://omnia:omnia@omnia-postgres.omnia-system.svc.cluster.local:5432/omnia_sessions?sslmode=disable"
---
apiVersion: v1
kind: Secret
metadata:
  name: omnia-postgres-memory
  namespace: dev-agents
type: Opaque
stringData:
  POSTGRES_CONN: "postgres://omnia:omnia@omnia-postgres.omnia-system.svc.cluster.local:5432/omnia_memory?sslmode=disable"
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Workspace
metadata:
  name: dev-agents
spec:
  displayName: Dev Agents
  environment: development
  namespace:
    name: dev-agents
  anonymousAccess:
    enabled: true
    role: editor
  services:
    - name: default
      mode: managed
      session:
        database:
          secretRef:
            name: omnia-postgres
      memory:
        database:
          secretRef:
            name: omnia-postgres-memory
EOF

log_info "Waiting for per-workspace services..."
kubectl wait --for=condition=Available --timeout=3m \
  deployment/session-dev-agents-default -n dev-agents || true
kubectl wait --for=condition=Available --timeout=3m \
  deployment/memory-dev-agents-default -n dev-agents || true

log_info "Arena E2E environment ready!"
echo ""
echo "Cluster: kind-${KIND_CLUSTER}"
echo "Context: kubectl config use-context kind-${KIND_CLUSTER}"
echo ""
kubectl get pods -n "$NAMESPACE"
echo ""
echo "To clean up: ./scripts/setup-arena-e2e.sh clean"
