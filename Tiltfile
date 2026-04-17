# Tiltfile for Omnia local development
#
# This enables hot-reload development of the dashboard and operator
# on a local Kubernetes cluster (kind, Docker Desktop, etc.)
#
# Usage:
#   tilt up                          # Start core development (dashboard + operator)
#   ENABLE_ENTERPRISE=true tilt up   # Enable enterprise features (Arena, NFS, Redis)
#   tilt down                        # Stop and clean up
#   tilt up --stream                 # Start with log streaming
#
# Environment variables:
#   ENABLE_ENTERPRISE  - Enable enterprise features (Arena controller, NFS, Redis)
#   ENABLE_DEMO        - Enable demo mode with Ollama
#   ENABLE_OBSERVABILITY - Enable Prometheus/Grafana (default: true)
#   ENABLE_FULL_STACK  - Enable full production-like stack (Istio, etc.)
#
# See docs/LOCAL_DEVELOPMENT.md for setup instructions.

load('ext://helm_resource', 'helm_resource', 'helm_repo')
load('ext://namespace', 'namespace_create')

# ============================================================================
# Configuration
# ============================================================================

# Set to True to enable Prometheus/Grafana for cost tracking development
ENABLE_OBSERVABILITY = True

# Set to True to enable Demo mode with Ollama + OPA model validation
# Requires: 8GB+ RAM, 10GB+ disk for llava:7b model
# Can be set via environment: ENABLE_DEMO=true tilt up
# Also supports legacy ENABLE_OLLAMA for backwards compatibility
ENABLE_DEMO = os.getenv('ENABLE_DEMO', os.getenv('ENABLE_OLLAMA', '')).lower() in ('true', '1', 'yes') or False

# Set to True to enable Audio Demo with Gemini (requires GEMINI_API_KEY)
# Can be set via environment: ENABLE_AUDIO_DEMO=true tilt up
# Note: You must create the gemini-credentials secret manually:
#   kubectl create secret generic gemini-credentials -n omnia-demo --from-literal=api-key=$GEMINI_API_KEY
ENABLE_AUDIO_DEMO = os.getenv('ENABLE_AUDIO_DEMO', '').lower() in ('true', '1', 'yes') or False

# Set to True to enable LangChain runtime demos alongside PromptKit demos
# Deploys vision-demo-langchain and tools-demo-langchain agents using the LangChain runtime
# Can be set via environment: ENABLE_LANGCHAIN=true tilt up
ENABLE_LANGCHAIN = os.getenv('ENABLE_LANGCHAIN', '').lower() in ('true', '1', 'yes') or False

# Path to the omnia-langchain-runtime repository for local builds
LANGCHAIN_RUNTIME_PATH = os.getenv('LANGCHAIN_RUNTIME_PATH', '../omnia-langchain-runtime')

# Set to True to build runtime with local PromptKit source for debugging
# Can be set via environment: USE_LOCAL_PROMPTKIT=true PROMPTKIT_PATH=/path/to/PromptKit tilt up
# This allows rapid iteration on PromptKit changes without publishing releases
# Auto-enables if promptkit-local/ has content (from rsync) or PROMPTKIT_PATH source exists
PROMPTKIT_PATH = os.getenv('PROMPTKIT_PATH', '../PromptKit')
_promptkit_local_exists = os.path.exists('./promptkit-local/tools/arena/cmd/promptarena/main.go')
_promptkit_source_exists = os.path.exists(PROMPTKIT_PATH + '/runtime')
USE_LOCAL_PROMPTKIT = os.getenv('USE_LOCAL_PROMPTKIT', '').lower() in ('true', '1', 'yes') or _promptkit_local_exists or _promptkit_source_exists

# Set to True to enable full production-like stack
# Includes: Istio service mesh, Tempo (tracing), Loki (logging), Alloy (collector),
#           Gateway API, and dashboard with builtin auth
# Requires: 16GB+ RAM recommended
# Can be set via environment: ENABLE_FULL_STACK=true tilt up
ENABLE_FULL_STACK = os.getenv('ENABLE_FULL_STACK', '').lower() in ('true', '1', 'yes') or False

# Set to True to enable Enterprise features (Arena, licensing)
# Includes: Arena controller, arena-worker for evaluations, NFS storage, VS Code server
# Can be set via environment: ENABLE_ENTERPRISE=true tilt up
ENABLE_ENTERPRISE = os.getenv('ENABLE_ENTERPRISE', '').lower() in ('true', '1', 'yes') or False

# Set to True to enable Entra ID (Azure AD) OAuth for the dashboard.
# Requires a one-time setup: run `./scripts/setup-entra-dev.sh --create-secret`
# to register the app, create the `dashboard-oauth` Secret, and generate
# `charts/omnia/values-dev-entra.yaml` (gitignored).
# Can be set via environment: ENABLE_ENTRA=true tilt up
ENABLE_ENTRA = os.getenv('ENABLE_ENTRA', '').lower() in ('true', '1', 'yes') or False

# Enable internal NFS server for workspace content storage
# Provides ReadWriteMany (RWX) storage for Arena and workspace content
# Auto-enabled when ENABLE_ENTERPRISE is true, can be explicitly controlled via ENABLE_NFS
# Can be set via environment: ENABLE_NFS=true/false tilt up
_nfs_env = os.getenv('ENABLE_NFS', '')
if _nfs_env:
    ENABLE_NFS = _nfs_env.lower() in ('true', '1', 'yes')
else:
    # Default: enabled when enterprise is enabled, disabled otherwise
    ENABLE_NFS = ENABLE_ENTERPRISE

# Allow deployment to local clusters only (safety check)
allow_k8s_contexts(['kind-omnia-dev', 'docker-desktop', 'minikube', 'kind-kind', 'orbstack'])

# Suppress warnings for images passed as CLI args to operator (not in K8s manifests)
# Also suppress langchain runtime which is referenced via Helm values, not directly in manifests
_suppress_images = ['omnia-facade-dev', 'omnia-runtime-dev', 'omnia-langchain-runtime-dev']
if ENABLE_ENTERPRISE:
    _suppress_images.extend(['omnia-arena-controller-dev', 'omnia-promptkit-lsp-dev', 'omnia-policy-proxy-dev', 'omnia-session-api-dev', 'omnia-memory-api-dev'])
update_settings(suppress_unused_image_warnings=_suppress_images)


# Create namespace if it doesn't exist
namespace_create('omnia-system')

# ============================================================================
# Helm Repositories (required for subcharts)
# ============================================================================

if ENABLE_OBSERVABILITY or ENABLE_FULL_STACK:
    helm_repo('prometheus-community', 'https://prometheus-community.github.io/helm-charts')
    helm_repo('grafana', 'https://grafana.github.io/helm-charts')

if ENABLE_FULL_STACK:
    helm_repo('istio', 'https://istio-release.storage.googleapis.com/charts')

# Bitnami charts for Redis
helm_repo('bitnami', 'https://charts.bitnami.com/bitnami')

# ============================================================================
# Full Stack Mode - Istio Installation via Helm
# ============================================================================

if ENABLE_FULL_STACK:
    # Create istio-system namespace
    namespace_create('istio-system')

    # Install Gateway API CRDs and Istio base CRDs (no workloads, just CRDs)
    # Using local_resource because helm_resource can't track CRD-only releases
    local_resource(
        'istio-crds',
        cmd='''
            kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.0/standard-install.yaml
            helm upgrade --install istio-base istio/base -n istio-system --wait
        ''',
        labels=['istio'],
    )

    # Install Istiod (control plane) with OpenTelemetry tracing to Tempo
    helm_resource(
        'istiod',
        'istio/istiod',
        namespace='istio-system',
        flags=['--wait', '-f', './charts/omnia/values-istiod.yaml'],
        labels=['istio'],
        resource_deps=['istio-crds'],
    )

    # Install Istio ingress gateway for agent traffic
    helm_resource(
        'istio-ingress',
        'istio/gateway',
        namespace='istio-system',
        flags=['--wait'],
        labels=['istio'],
        resource_deps=['istiod'],
    )

    # Enable sidecar injection for agent namespaces
    local_resource(
        'istio-inject-labels',
        cmd='''
            kubectl label namespace dev-agents istio-injection=enabled --overwrite 2>/dev/null || true
            kubectl label namespace omnia-demo istio-injection=enabled --overwrite 2>/dev/null || true
        ''',
        labels=['istio'],
        resource_deps=['istiod'],
    )

# ============================================================================
# Dashboard - Hot reload development
# ============================================================================

# Build dashboard with live_update for instant file sync
docker_build(
    'omnia-dashboard-dev',
    context='./dashboard',
    dockerfile='./dashboard/Dockerfile.dev',
    live_update=[
        # If package.json changes, need full rebuild (must be first)
        fall_back_on(['./dashboard/package.json', './dashboard/package-lock.json']),

        # Sync source files - triggers Next.js hot reload
        sync('./dashboard/src', '/app/src'),
        sync('./dashboard/public', '/app/public'),
    ],
)

# ============================================================================
# Operator - Rebuild on changes (Go doesn't hot reload)
# ============================================================================

# Build operator image
operator_only = [
    './cmd',
    './api',
    './internal',
    './pkg',
    './ee',
    './go.mod',
    './go.sum',
    # Embedded files for go:embed directives
    './pkg/license/keys',
]
if USE_LOCAL_PROMPTKIT:
    operator_only.append('./promptkit-local')

docker_build(
    'omnia-operator-dev',
    context='.',
    dockerfile='./Dockerfile',
    # Only rebuild when Go files change
    only=operator_only,
)

# ============================================================================
# Session API Server - Session history backend for the dashboard
# ============================================================================

docker_build(
    'omnia-session-api-dev',
    context='.',
    dockerfile='./Dockerfile.session-api',
    only=[
        './cmd/session-api',
        './api',
        './internal/session',
        './internal/pgutil',
        './internal/httputil',
        './internal/tracing',
        './ee/api',
        './ee/pkg/privacy',
        './ee/pkg/redaction',
        './ee/pkg/audit',
        './ee/pkg/encryption',
        './ee/pkg/metrics',
        './pkg',
        './go.mod',
        './go.sum',
    ],
)

# ============================================================================
# Memory API Server - Memory store backend for cross-session memory
# ============================================================================

docker_build(
    'omnia-memory-api-dev',
    context='.',
    dockerfile='./Dockerfile.memory-api',
    only=[
        './cmd/memory-api',
        './api',
        './ee',
        './internal/memory',
        './internal/session',
        './internal/pgutil',
        './internal/httputil',
        './internal/tracing',
        './pkg',
        './go.mod',
        './go.sum',
    ],
)

# ============================================================================
# Doctor - Cluster diagnostic service for Omnia health checks
# ============================================================================

docker_build(
    'omnia-doctor-dev',
    context='.',
    dockerfile='./Dockerfile.doctor',
    only=[
        './Dockerfile.doctor',
        './cmd/doctor',
        './internal',
        './api',
        './ee',
        './pkg',
        './go.mod',
        './go.sum',
    ],
)

# ============================================================================
# Dev Ollama — pre-loaded with a small model for tool-calling tests
# ============================================================================

docker_build(
    'ollama-preloaded',
    context='.',
    dockerfile='./hack/Dockerfile.ollama-preloaded',
)

k8s_yaml(blob("""
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: dev-ollama
  namespace: dev-agents
spec:
  serviceName: dev-ollama
  replicas: 1
  selector:
    matchLabels:
      app: dev-ollama
  template:
    metadata:
      labels:
        app: dev-ollama
    spec:
      containers:
        - name: ollama
          image: ollama-preloaded
          ports:
            - containerPort: 11434
          env:
            # Keep the model resident in memory indefinitely so every
            # request uses the warm model — no cold-start latency that
            # would blow past the pipeline idle timeout.
            - name: OLLAMA_KEEP_ALIVE
              value: "-1"
            # Only load one model at a time — the StatefulSet pre-pulls
            # a single model so there's no contention.
            - name: OLLAMA_MAX_LOADED_MODELS
              value: "1"
          # Basic TCP readiness — accepts connections as soon as the API
          # server is up. Warmup is handled by a dedicated sidecar below
          # that pre-compiles the model before agents start calling it.
          readinessProbe:
            tcpSocket:
              port: 11434
            initialDelaySeconds: 2
            periodSeconds: 5
        # Warmup sidecar: sends a chat completion with tools once the API
        # is reachable, forcing the model to JIT-compile against the
        # tool-calling code path. After it exits 0, subsequent requests
        # from agents hit a warm model and complete in single-digit seconds.
        - name: warmup
          image: curlimages/curl:8.10.1
          command:
            - /bin/sh
            - -c
            - |
              set -e
              echo "waiting for ollama API..."
              until curl -fsS -m 5 http://127.0.0.1:11434/api/tags > /dev/null 2>&1; do
                sleep 2
              done
              echo "sending warmup chat completion..."
              curl -fsS -m 180 -X POST http://127.0.0.1:11434/v1/chat/completions \\
                -H 'Content-Type: application/json' \\
                -d '{"model":"qwen2.5:14b","messages":[{"role":"user","content":"warmup"}],"max_tokens":4,"stream":false}' \\
                > /dev/null
              echo "warmup complete; sleeping"
              # Keep the sidecar running so we don't churn — Kubernetes
              # restarts completed containers under restartPolicy: Always.
              while true; do sleep 3600; done
          resources:
            requests:
              cpu: "2"
              memory: 2Gi
            limits:
              cpu: "8"
              memory: 4Gi
---
apiVersion: v1
kind: Service
metadata:
  name: dev-ollama
  namespace: dev-agents
spec:
  selector:
    app: dev-ollama
  ports:
    - port: 11434
      targetPort: 11434
"""))

k8s_resource(
    'dev-ollama',
    labels=['dev'],
    port_forwards=['11434:11434'],
    resource_deps=['sample-resources'],
)

# ============================================================================
# Local PromptKit Sync — rsync source into promptkit-local/ for Docker builds
# ============================================================================
# Docker COPY does not follow symlinks, so we rsync the actual PromptKit source
# into a real directory. This runs automatically when PromptKit source changes.

if USE_LOCAL_PROMPTKIT:
    local_resource(
        'sync-promptkit',
        cmd='rsync -a --delete ' +
            '--exclude="examples/" ' +
            '--include="runtime/***" ' +
            '--include="sdk/***" ' +
            '--include="server/***" ' +
            '--include="pkg/***" ' +
            '--include="tools/***" ' +
            '--include="go.work" ' +
            '--include="go.work.sum" ' +
            '--exclude="*" ' +
            PROMPTKIT_PATH + '/ ./promptkit-local/',
        deps=[
            PROMPTKIT_PATH + '/runtime',
            PROMPTKIT_PATH + '/sdk',
            PROMPTKIT_PATH + '/server',
            PROMPTKIT_PATH + '/pkg',
            PROMPTKIT_PATH + '/tools',
            PROMPTKIT_PATH + '/go.work',
        ],
        labels=['build'],
    )

# ============================================================================
# Agent Images - Facade and Runtime containers for AgentRuntime pods
# ============================================================================

# Facade image is built by auto-rebuild-facade local_resource (below).
# Do NOT add a docker_build here — it conflicts with the local_resource
# and causes Docker layer caching to mask source changes.

# Build runtime image (LLM interaction and tool execution)
# When USE_LOCAL_PROMPTKIT is enabled, includes local PromptKit source for development
# Runtime image is built by auto-rebuild-runtime local_resource (below).
# Do NOT add a docker_build here — it conflicts with the local_resource
# and causes Docker layer caching to mask source changes.

# ============================================================================
# Enterprise Features - Arena Controller and Worker
# ============================================================================

if ENABLE_ENTERPRISE:
    # Build arena-controller image (enterprise ArenaSource/ArenaJob controllers)
    arena_controller_only = [
        './ee/cmd/omnia-arena-controller',
        './ee/internal',
        './ee/pkg',
        './ee/api',
        './internal',
        './pkg',
        './api',
        './go.mod',
        './go.sum',
    ]
    if USE_LOCAL_PROMPTKIT:
        arena_controller_only.append('./promptkit-local')

    docker_build(
        'omnia-arena-controller-dev',
        context='.',
        dockerfile='./ee/Dockerfile.arena-controller',
        only=arena_controller_only,
    )

    # Build arena-worker image (evaluation job worker)
    # Uses local_resource because Tilt can't detect image refs in CRD fields —
    # the controller spawns worker pods dynamically at runtime.
    arena_worker_deps = [
        './ee/cmd/arena-worker',
        './ee/Dockerfile.arena-worker',
        './ee/internal',
        './ee/pkg',
        './ee/api',
        './go.mod',
        './go.sum',
    ]
    local_resource(
        'arena-worker-image',
        cmd='docker build -t omnia-arena-worker-dev:latest -f ./ee/Dockerfile.arena-worker .',
        deps=arena_worker_deps,
        labels=['arena'],
    )

    # Build promptkit-lsp image (LSP server for PromptKit YAML validation)
    docker_build(
        'omnia-promptkit-lsp-dev',
        context='.',
        dockerfile='./ee/Dockerfile.promptkit-lsp',
        only=[
            './ee/cmd/promptkit-lsp',
            './ee/internal',
            './ee/pkg',
            './ee/api',
            './internal',
            './pkg',
            './api',
            './go.mod',
            './go.sum',
        ],
    )

    # Build arena-dev-console image (interactive agent testing in project editor)
    # Uses local_resource — dynamically spawned by ArenaDevSession controller.
    arena_dev_console_deps = [
        './ee/cmd/arena-dev-console',
        './ee/Dockerfile.arena-dev-console',
        './ee/internal',
        './ee/pkg',
        './ee/api',
        './go.mod',
        './go.sum',
    ]
    local_resource(
        'arena-dev-console-image',
        cmd='docker build -t omnia-arena-dev-console-dev:latest -f ./ee/Dockerfile.arena-dev-console .',
        deps=arena_dev_console_deps,
        labels=['arena'],
    )

    # Build eval-worker image (non-PromptKit agent eval execution)
    # Uses local_resource — dynamically spawned by ArenaJob controller.
    eval_worker_deps = [
        './ee/cmd/arena-eval-worker',
        './ee/Dockerfile.eval-worker',
        './ee/internal',
        './ee/pkg',
        './ee/api',
        './internal',
        './pkg',
        './go.mod',
        './go.sum',
    ]
    if _promptkit_local_exists:
        eval_worker_deps.append('./promptkit-local')
    local_resource(
        'eval-worker-image',
        cmd='docker build -t omnia-eval-worker-dev:latest -f ./ee/Dockerfile.eval-worker .',
        deps=eval_worker_deps,
        labels=['arena'],
    )

    # Build policy-proxy sidecar image (ToolPolicy enforcement sidecar)
    docker_build(
        'omnia-policy-proxy-dev',
        context='.',
        dockerfile='./ee/Dockerfile.policy-proxy',
        only=[
            './ee/cmd/policy-proxy',
            './ee/api',
            './ee/pkg',
            './api',
            './internal',
            './pkg',
            './go.mod',
            './go.sum',
        ],
    )

# ============================================================================
# LangChain Runtime - Python-based agent framework
# ============================================================================

if ENABLE_LANGCHAIN:
    # Build LangChain runtime from external repo using local_resource
    # This always runs because Tilt can't detect image refs in CRD fields
    local_resource(
        'langchain-runtime-build',
        cmd='docker build -t omnia-langchain-runtime-dev:latest ' + LANGCHAIN_RUNTIME_PATH,
        deps=[LANGCHAIN_RUNTIME_PATH + '/src', LANGCHAIN_RUNTIME_PATH + '/Dockerfile', LANGCHAIN_RUNTIME_PATH + '/pyproject.toml'],
        labels=['langchain'],
    )

# ============================================================================
# Helm Deployment
# ============================================================================

# Build helm set values
helm_set = [
    # Use dev images for operator
    'image.repository=omnia-operator-dev',
    'image.tag=latest',
    'image.pullPolicy=Never',
    # Use dev images for dashboard
    'dashboard.image.repository=omnia-dashboard-dev',
    'dashboard.image.tag=latest',
    'dashboard.image.pullPolicy=Never',
    # Use dev images for agent containers (facade + framework)
    'facade.image.repository=omnia-facade-dev',
    'facade.image.tag=latest',
    'facade.image.pullPolicy=Never',
    'framework.image.repository=omnia-runtime-dev',
    'framework.image.tag=latest',
    'framework.image.pullPolicy=Never',
    # Enable dashboard
    'dashboard.enabled=true',
    # Per-workspace service images (operator creates Deployments from Workspace CRD)
    'workspaceServices.sessionApi.image.repository=omnia-session-api-dev',
    'workspaceServices.sessionApi.image.tag=latest',
    'workspaceServices.sessionApi.image.pullPolicy=Never',
    'workspaceServices.memoryApi.image.repository=omnia-memory-api-dev',
    'workspaceServices.memoryApi.image.tag=latest',
    'workspaceServices.memoryApi.image.pullPolicy=Never',
    # Dev Postgres for workspace services
    'postgres.dev.enabled=true',
    # Doctor
    'doctor.enabled=true',
    'doctor.image.repository=omnia-doctor-dev',
    'doctor.image.tag=latest',
    'doctor.image.pullPolicy=Never',
    'doctor.workspace=dev-agents',
    'doctor.serviceGroup=default',
    'doctor.agentNamespace=dev-agents',
    'doctor.agentName=ollama-agent',
    'doctor.ollamaService=dev-ollama',
    # LangChain runtime image (used when framework.type=langchain)
    'langchainRuntime.image.repository=omnia-langchain-runtime-dev',
    'langchainRuntime.image.tag=latest',
    'langchainRuntime.image.pullPolicy=Never',
    # Enable operator tool test API server
    'operator.apiBindAddress=:8083',
    # Increase dashboard resources for HMR compilation
    'dashboard.resources.limits.cpu=4000m',
    'dashboard.resources.limits.memory=4Gi',
    'dashboard.resources.requests.cpu=1000m',
    'dashboard.resources.requests.memory=2Gi',
    # Give dashboard more time to compile in dev mode
    'dashboard.startupProbe.httpGet.path=/api/health',
    'dashboard.startupProbe.httpGet.port=http',
    'dashboard.startupProbe.initialDelaySeconds=10',
    'dashboard.startupProbe.periodSeconds=10',
    'dashboard.startupProbe.failureThreshold=30',  # 30 * 10s = 5 minutes to start
    'dashboard.livenessProbe.initialDelaySeconds=60',
    'dashboard.livenessProbe.failureThreshold=6',
]

# Enterprise features configuration
if ENABLE_ENTERPRISE:
    helm_set.extend([
        # Enable enterprise features
        'enterprise.enabled=true',
        'devMode=true',  # Enable dev mode license for local development
        # Arena controller image
        'enterprise.arena.controller.image.repository=omnia-arena-controller-dev',
        'enterprise.arena.controller.image.tag=latest',
        'enterprise.arena.controller.image.pullPolicy=Never',
        # Arena worker image
        'enterprise.arena.worker.image.repository=omnia-arena-worker-dev',
        'enterprise.arena.worker.image.tag=latest',
        'enterprise.arena.worker.image.pullPolicy=Never',
        # Arena queue - use Redis when enterprise is enabled
        'enterprise.arena.queue.type=redis',
        'enterprise.arena.queue.redis.host=omnia-redis-master',
        'enterprise.arena.queue.redis.port=6379',
        # Enable Redis for Arena queue (Bitnami subchart)
        'redis.enabled=true',
        'redis.architecture=standalone',
        'redis.auth.enabled=false',
        'redis.master.persistence.enabled=false',
        # PromptKit LSP server for YAML validation
        'enterprise.promptkitLsp.enabled=true',
        'enterprise.promptkitLsp.image.repository=omnia-promptkit-lsp-dev',
        'enterprise.promptkitLsp.image.tag=latest',
        'enterprise.promptkitLsp.image.pullPolicy=Never',
        # Arena Dev Console image for ArenaDevSession controller (creates dynamic pods)
        'enterprise.arena.devConsole.image.repository=omnia-arena-dev-console-dev',
        'enterprise.arena.devConsole.image.tag=latest',
        'enterprise.arena.devConsole.image.pullPolicy=Never',
        # Eval worker for non-PromptKit agent eval execution
        'enterprise.evalWorker.image.repository=omnia-eval-worker-dev',
        'enterprise.evalWorker.image.tag=latest',
        'enterprise.evalWorker.image.pullPolicy=Never',
        # Watch all dev namespaces plus omnia-system for eval events (e2e tests publish there)
        'enterprise.evalWorker.namespaces={dev-agents,omnia-demo,omnia-system}',
        # Policy proxy sidecar for ToolPolicy enforcement
        'enterprise.policyProxy.image.repository=omnia-policy-proxy-dev',
        'enterprise.policyProxy.image.tag=latest',
        'enterprise.policyProxy.image.pullPolicy=Never',
    ])
else:
    # Disable enterprise features
    helm_set.extend([
        'enterprise.enabled=false',
        'redis.enabled=false',
    ])

if ENABLE_OBSERVABILITY:
    helm_set.extend([
        # Enable Prometheus for cost metrics
        'prometheus.enabled=true',
        'prometheus.server.persistentVolume.enabled=false',
        'prometheus.alertmanager.enabled=false',
        'prometheus.prometheus-pushgateway.enabled=false',
        'prometheus.prometheus-node-exporter.enabled=false',
        'prometheus.kube-state-metrics.enabled=false',
        # Disable TLS on operator metrics so Prometheus can scrape without certs
        'metrics.secure=false',
        'metrics.port=8080',
        # Enable Grafana for dashboards
        'grafana.enabled=true',
        'grafana.adminPassword=admin',
        'grafana.persistence.enabled=false',
        # Enable anonymous access for iframe embedding
        'grafana.grafana\\.ini.auth\\.anonymous.enabled=true',
        'grafana.grafana\\.ini.auth\\.anonymous.org_role=Viewer',
        'grafana.grafana\\.ini.security.allow_embedding=true',
        # Configure dashboard to use Prometheus (with /prometheus prefix from values.yaml)
        'dashboard.prometheus.url=http://omnia-prometheus-server:80/prometheus',
        # Use localhost URL for browser access to Grafana iframes
        'dashboard.grafana.url=http://localhost:3001',
        # Enable Loki + Alloy + Tempo for logs and traces (queryable in Grafana)
        'loki.enabled=true',
        'alloy.enabled=true',
        'tempo.enabled=true',
        # Enable tracing: facade/runtime → Alloy (OTLP) → Tempo
        'tracing.enabled=true',
        'tracing.endpoint=omnia-alloy.omnia-system.svc.cluster.local:4317',
    ])
else:
    helm_set.extend([
        # Disable observability
        'prometheus.enabled=false',
        'grafana.enabled=false',
        'loki.enabled=false',
    ])

# Demo mode namespace creation (demos are now in separate chart)
if ENABLE_DEMO or ENABLE_AUDIO_DEMO:
    namespace_create('omnia-demo')

if ENABLE_FULL_STACK:
    # Full stack mode overrides some observability settings
    # Generate a deterministic session secret for local dev (DO NOT use in production!)
    helm_set.extend([
        # Enable Istio integration (telemetry resources)
        'istio.enabled=true',
        'istio.tempoService=omnia-tempo.omnia-system.svc.cluster.local',
        'istio.tempoPort=4317',
        # Enable Gateway API for agent ingress
        'gateway.enabled=true',
        'gateway.className=istio',
        'internalGateway.enabled=true',
        'internalGateway.className=istio',
        # Tracing goes through Alloy (already set in ENABLE_OBSERVABILITY above)
        # Alloy fans out to Tempo + session-api
        # Configure dashboard with builtin auth
        'dashboard.auth.mode=builtin',
        'dashboard.auth.sessionSecret=dev-session-secret-do-not-use-in-prod',
        # Initial admin user for local development
        'dashboard.builtin.admin.username=admin',
        'dashboard.builtin.admin.email=admin@localhost',
        'dashboard.builtin.admin.password=admin123',
        'dashboard.persistence.enabled=true',
        'dashboard.persistence.size=1Gi',
        # Configure Grafana datasources for Tempo/Loki
        'grafana.grafana\\.ini.auth\\.anonymous.org_role=Admin',  # Admin for full access
    ])

    # Note: When full stack is enabled, OPA mode for demos is set in demos chart values

# Build values files list
helm_values = ['./charts/omnia/values-dev.yaml']
if ENABLE_NFS:
    helm_values.append('./charts/omnia/values-dev-enterprise.yaml')
if ENABLE_FULL_STACK:
    helm_values.append('./charts/omnia/values-istio-prometheus.yaml')
if ENABLE_ENTRA:
    _entra_values = './charts/omnia/values-dev-entra.yaml'
    if not os.path.exists(_entra_values):
        fail('ENABLE_ENTRA=true but %s is missing. Run ./scripts/setup-entra-dev.sh --create-secret first.' % _entra_values)
    helm_values.append(_entra_values)

# Deploy the Helm chart with development images
k8s_yaml(helm(
    './charts/omnia',
    name='omnia',
    namespace='omnia-system',
    values=helm_values,
    set=helm_set,
))

# ============================================================================
# Demo Charts (separate from main Omnia chart)
# ============================================================================

if ENABLE_DEMO or ENABLE_AUDIO_DEMO:
    # Build demo helm set values
    demo_helm_set = [
        'namespace=omnia-demo',
        # Use persistence for model cache
        'ollama.persistence.enabled=true',
        # Grant anonymous users owner access for local development
        # WARNING: This allows unauthenticated write access - only for dev
        'workspace.anonymousAccess.enabled=true',
        'workspace.anonymousAccess.role=owner',
    ]

    # Use NFS storage class for workspace when NFS is enabled
    if ENABLE_NFS:
        demo_helm_set.append('workspace.storage.storageClass=omnia-nfs')

    if ENABLE_AUDIO_DEMO:
        demo_helm_set.extend([
            'audioDemo.enabled=true',
        ])

    if ENABLE_LANGCHAIN:
        demo_helm_set.extend([
            # Enable LangChain demos (mirror of vision-demo and tools-demo)
            'langchainDemo.enabled=true',
            'langchainDemo.image.repository=omnia-langchain-runtime-dev',
            'langchainDemo.image.tag=latest',
            'langchainDemo.image.pullPolicy=Never',
        ])

    if ENABLE_FULL_STACK:
        # Override OPA mode to use Istio ext_authz
        demo_helm_set.extend([
            'opa.mode=extauthz',
            'istio.enabled=true',
        ])

    k8s_yaml(helm(
        './charts/omnia-demos',
        name='omnia-demos',
        namespace='omnia-demo',
        set=demo_helm_set,
    ))

# ============================================================================
# Resource Configuration
# ============================================================================

# Group resources for better UI organization
# Note: facade/runtime images are built by docker_build() but not directly
# referenced in K8s YAML (they're passed as CLI args to the operator).
# The restart-agents local_resource handles restarting agent pods when these change.

# Build resource dependencies - when NFS is enabled, wait for storage to be ready
controller_deps = []
dashboard_deps = []
if ENABLE_NFS:
    controller_deps = ['csi-nfs-controller', 'omnia-nfs-server']
    dashboard_deps = ['csi-nfs-controller', 'omnia-nfs-server']

k8s_resource(
    'omnia-controller-manager',
    labels=['operator'],
    port_forwards=['8082:8082'],  # Operator API
    resource_deps=controller_deps,
)

k8s_resource(
    'omnia-dashboard',
    labels=['dashboard'],
    port_forwards=[
        '3000:3000',  # Dashboard UI
        '3002:3002',  # WebSocket proxy for agent connections
    ],
    resource_deps=dashboard_deps + ['sample-resources'],
)

# Session API server and its dev Postgres
k8s_resource(
    'omnia-postgres',
    labels=['session-api'],
    objects=[
        'omnia-postgres:secret',
    ],
)

# Session-api and memory-api Deployments are created dynamically by the operator
# when it reconciles the dev-agents Workspace CRD. We use local_resource to wait
# for the pods and set up port-forwards. These are NOT tracked as k8s_resource
# because Tilt can't discover operator-managed workloads from Helm output.
# Session-api and memory-api are operator-managed: wait for them, then port-forward.
local_resource(
    'session-dev-agents-default',
    serve_cmd='kubectl wait --for=condition=available deployment/session-dev-agents-default -n dev-agents --timeout=300s && kubectl port-forward -n dev-agents deployment/session-dev-agents-default 8180:8080',
    labels=['session-api'],
    resource_deps=['omnia-postgres', 'sample-resources'],
)

local_resource(
    'memory-dev-agents-default',
    serve_cmd='kubectl wait --for=condition=available deployment/memory-dev-agents-default -n dev-agents --timeout=300s && kubectl port-forward -n dev-agents deployment/memory-dev-agents-default 8083:8080',
    labels=['memory-api'],
    resource_deps=['omnia-postgres', 'sample-resources'],
)

k8s_resource(
    'omnia-doctor',
    labels=['doctor'],
    port_forwards=['8084:8080'],
)

# pgweb — lightweight Postgres web UI for inspecting session data, eval results, etc.
k8s_yaml(blob('''
apiVersion: apps/v1
kind: Deployment
metadata:
  name: omnia-pgweb
  namespace: omnia-system
  labels:
    app.kubernetes.io/name: pgweb
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: pgweb
  template:
    metadata:
      labels:
        app.kubernetes.io/name: pgweb
    spec:
      containers:
        - name: pgweb
          image: sosedoff/pgweb:latest
          args:
            - "--bind=0.0.0.0"
            - "--listen=8081"
            - "--url=postgres://omnia:omnia@omnia-postgres:5432/omnia?sslmode=disable"
          ports:
            - containerPort: 8081
              protocol: TCP
          resources:
            limits:
              cpu: 200m
              memory: 128Mi
            requests:
              cpu: 50m
              memory: 64Mi
'''))

k8s_resource(
    'omnia-pgweb',
    labels=['session-api'],
    port_forwards=['8081:8081'],  # pgweb UI
    resource_deps=['omnia-postgres'],
)

if ENABLE_OBSERVABILITY:
    k8s_resource(
        'omnia-prometheus-server',
        labels=['observability'],
        port_forwards=['9090:9090'],  # Prometheus UI (container port)
    )

    k8s_resource(
        'omnia-grafana',
        labels=['observability'],
        port_forwards=['3001:3000'],  # Grafana UI (container port 3000, local 3001)
    )

    # Loki for log aggregation (queryable in Grafana → Explore)
    k8s_resource(
        'omnia-loki',
        labels=['observability'],
        port_forwards=['3100:3100'],  # Loki HTTP API
    )

    # Alloy for log collection (collects pod logs and ships to Loki)
    k8s_resource(
        'omnia-alloy',
        labels=['observability'],
    )

    # Tempo for distributed tracing
    k8s_resource(
        'omnia-tempo',
        labels=['observability'],
        port_forwards=[
            '3200:3200',   # Tempo HTTP API
            '4317:4317',   # OTLP gRPC
            '4318:4318',   # OTLP HTTP
        ],
    )

# ============================================================================
# Redis for Arena Queue (Enterprise only)
# ============================================================================

if ENABLE_ENTERPRISE:
    # Redis master (Bitnami standalone mode) - required for Arena queue
    k8s_resource(
        'omnia-redis-master',
        labels=['enterprise'],
        port_forwards=['6379:6379'],  # Redis port for local debugging
    )

# ============================================================================
# Enterprise Storage - NFS Server and CSI Driver
# ============================================================================

if ENABLE_NFS:
    # NFS server deployment and backing storage (enterprise shared filesystem)
    k8s_resource(
        'omnia-nfs-server',
        labels=['enterprise'],
        objects=[
            'omnia-nfs-data:persistentvolumeclaim',
        ],
    )

    # NFS CSI driver controller (handles dynamic PV provisioning)
    # Must be ready before workspace-content PVC can be provisioned
    k8s_resource(
        'csi-nfs-controller',
        labels=['enterprise'],
        objects=[
            'omnia-nfs:storageclass',
            'omnia-workspace-content:persistentvolumeclaim',
        ],
        resource_deps=['omnia-nfs-server'],
    )

    # NFS CSI driver node daemonset
    k8s_resource(
        'csi-nfs-node',
        labels=['enterprise'],
    )

    # VS Code Server for browsing/editing workspace content
    k8s_resource(
        'omnia-vscode-server',
        labels=['enterprise'],
        port_forwards=['8888:8080'],  # VS Code Server UI
        resource_deps=['csi-nfs-controller', 'omnia-nfs-server'],
    )

# ============================================================================
# Enterprise Arena Controller
# ============================================================================

if ENABLE_ENTERPRISE:
    # Arena controller dependencies
    arena_deps = []
    if ENABLE_NFS:
        arena_deps.extend(['csi-nfs-controller', 'omnia-nfs-server'])

    k8s_resource(
        'omnia-arena-controller',
        labels=['enterprise'],
        resource_deps=arena_deps + ['omnia-redis-master'],
    )

    # PromptKit LSP server for YAML validation in project editor
    k8s_resource(
        'omnia-promptkit-lsp',
        labels=['enterprise'],
        resource_deps=['omnia-dashboard'],
    )

    # Eval worker for realtime eval execution (consumes Redis stream events)
    k8s_resource(
        'omnia-eval-worker',
        labels=['enterprise'],
        resource_deps=['omnia-redis-master', 'sample-resources'],
    )

# ============================================================================
# Full Stack Mode Resources (Istio, Gateway API)
# ============================================================================

if ENABLE_FULL_STACK:
    # Note: Loki, Alloy, and Tempo resources are defined in ENABLE_OBSERVABILITY above

    # Gateway API and Istio resources that need CRDs installed first
    k8s_resource(
        workload='',  # No workload, just CRs
        new_name='istio-config',
        labels=['istio'],
        objects=[
            'omnia-dashboard-route:httproute',
            'omnia-grafana-route:httproute',
            'omnia-prometheus-route:httproute',
            'omnia-telemetry:telemetry',
        ],
        resource_deps=['istio-crds'],
    )

    # Gateway resources (created by Helm chart)
    k8s_resource(
        workload='',  # No workload, just CRs
        new_name='gateways',
        labels=['istio'],
        objects=[
            'omnia-agents:gateway',
            'omnia-internal:gateway',
        ],
        resource_deps=['istio-crds', 'istio-ingress'],
    )

# ============================================================================
# Demo Mode Resources (Ollama + OPA)
# ============================================================================

if ENABLE_DEMO:
    # Ollama StatefulSet — keep only non-workload objects here so Tilt
    # port-forwards to the ollama-0 pod (not an agent Deployment pod).
    ollama_objects = [
        'ollama-models:persistentvolumeclaim',
        'ollama-credentials:secret',
        'ollama:provider',
        'ollama-tools:provider',
    ]

    k8s_resource(
        'ollama',
        labels=['demo'],
        port_forwards=['11434:11434'],
        extra_pod_selectors={'app.kubernetes.io/name': 'ollama'},
        objects=ollama_objects,
    )

    # Agent CRs are grouped separately so their operator-created Deployments
    # don't get associated with the ollama resource (which breaks port forwarding).
    demo_agent_objects = [
        'demo-vision-prompts:configmap',
        'demo-vision-prompts:promptpack',
        'vision-demo:agentruntime',
        'demo-tools-prompts:configmap',
        'demo-tools-prompts:promptpack',
        'tools-demo:agentruntime',
    ]

    if ENABLE_LANGCHAIN:
        demo_agent_objects.extend([
            'vision-demo-langchain:agentruntime',
            'tools-demo-langchain:agentruntime',
        ])

    k8s_resource(
        workload='',
        new_name='demo-agents',
        labels=['demo'],
        objects=demo_agent_objects,
        resource_deps=['ollama'],
    )

if ENABLE_AUDIO_DEMO:
    # Audio demo resources (Gemini provider + audio-demo agent)
    # Note: User must create gemini-credentials secret manually before deploying
    audio_demo_objects = [
        'gemini:provider',
        'demo-audio-prompts:configmap',
        'demo-audio-prompts:promptpack',
        'audio-demo:agentruntime',
    ]

    k8s_resource(
        workload='',  # No workload, just CRs (Deployment created by operator)
        new_name='audio-demo',
        labels=['demo'],
        objects=audio_demo_objects,
        resource_deps=['omnia-controller-manager'],
    )

# ============================================================================
# Sample Resources for Development
# ============================================================================

# Apply sample resources using local_resource for better control
# Note: When ENABLE_DEMO is true, Ollama resources come from Helm chart, not samples
# After applying, patch the workspace to grant anonymous users owner access for local dev
local_resource(
    'sample-resources',
    cmd='''
        kubectl apply -f config/samples/dev/
        kubectl patch workspace dev-agents --type=merge -p '{"spec":{"anonymousAccess":{"enabled":true,"role":"owner"}}}'
    ''',
    deps=['config/samples/dev'],
    labels=['samples'],
    resource_deps=['omnia-controller-manager'],
)

# Enterprise arena fleet sample: seeds workspace PVC and creates ConfigMap + ArenaSource
# so users can test fleet mode against echo-agent from the dashboard.
if ENABLE_ENTERPRISE:
    local_resource(
        'arena-fleet-sample',
        cmd='''
            kubectl patch workspace dev-agents --type=merge -p '{"spec":{"storage":{"enabled":true,"storageClass":"omnia-nfs","size":"10Gi","accessModes":["ReadWriteMany"]}}}'
            kubectl delete job arena-fleet-seeder -n omnia-system --ignore-not-found
            kubectl apply -f config/samples/dev/enterprise/arena-fleet-seeder.yaml
            kubectl wait --for=condition=complete job/arena-fleet-seeder -n omnia-system --timeout=60s 2>/dev/null || true
            kubectl apply -f config/samples/dev/enterprise/arena-fleet-sample.yaml
            kubectl apply -f config/samples/dev/enterprise/arena-promptkit-examples.yaml
        ''',
        deps=['config/samples/dev/enterprise/arena-fleet-sample.yaml', 'config/samples/dev/enterprise/arena-fleet-seeder.yaml', 'config/samples/dev/enterprise/arena-promptkit-examples.yaml'],
        labels=['enterprise'],
        resource_deps=['omnia-arena-controller', 'sample-resources'],
    )

# Rebuild facade/runtime images and restart agent pods.
# Since these images are passed as CLI args to the operator (not in K8s YAML),
# Tilt can't track them as k8s_image_json_path resources. Instead, we watch
# source deps, rebuild images, and restart pods in a single atomic flow.

_rebuild_facade_cmd = 'docker build --no-cache -f Dockerfile.agent -t omnia-facade-dev:latest .'
_rebuild_runtime_cmd = 'docker build --no-cache -f Dockerfile.runtime'
if USE_LOCAL_PROMPTKIT:
    _rebuild_runtime_cmd += ' --build-arg USE_LOCAL_PROMPTKIT=true'
_rebuild_runtime_cmd += ' -t omnia-runtime-dev:latest .'

_restart_cmd = '''
    kubectl delete po -n dev-agents -l omnia.altairalabs.ai/component=agent 2>/dev/null || true
    kubectl delete po -n omnia-demo -l omnia.altairalabs.ai/component=agent 2>/dev/null || true
'''

# Auto-rebuild agent images when their specific source files change.
# Separated into facade and runtime to avoid unnecessary rebuilds — a change to
# runtime code no longer triggers a facade rebuild (and vice versa).
# Agent pods are restarted after each rebuild since they're operator-managed.

_facade_deps = [
    './cmd/agent',
    './internal/agent',
    './internal/facade',
    './internal/session',
    './internal/httputil',
    './internal/media',
    './internal/tracing',
    './pkg',
    './ee',
    './go.mod',
]

_runtime_deps = [
    './cmd/runtime',
    './internal/runtime',
    './internal/memory',
    './internal/tracing',
    './pkg',
    './go.mod',
]

if USE_LOCAL_PROMPTKIT:
    _runtime_deps.append('./promptkit-local')

local_resource(
    'auto-rebuild-facade',
    cmd=_rebuild_facade_cmd + ' && ' + _restart_cmd,
    deps=_facade_deps,
    labels=['agents'],
)

local_resource(
    'auto-rebuild-runtime',
    cmd=_rebuild_runtime_cmd + ' && ' + _restart_cmd,
    deps=_runtime_deps,
    labels=['agents'],
)

# ============================================================================
# E2E Tests (run against the Tilt dev cluster)
# ============================================================================
# These resources run the e2e test suite against the existing Tilt dev cluster.
# E2E_PREDEPLOYED=true tells the tests to skip operator/infra setup and teardown
# (already handled by Tilt/Helm). E2E_SKIP_SETUP=true skips image building.
# Image refs and session-api URL are set to match the Tilt Helm deployment.
#
# Usage: Click the trigger button in the Tilt UI, or run `tilt trigger e2e-tests`.

_e2e_env = {
    'E2E_SKIP_SETUP': 'true',
    'E2E_PREDEPLOYED': 'true',
    'E2E_SKIP_CLEANUP': 'true',
    'ENABLE_ARENA_E2E': 'true',
    'SESSION_API_URL': 'http://session-dev-agents-default.dev-agents.svc.cluster.local:8080',
    'E2E_FACADE_IMAGE': 'omnia-facade-dev:latest',
    'E2E_RUNTIME_IMAGE': 'omnia-runtime-dev:latest',
    'E2E_SERVICE_ACCOUNT': 'omnia',
    'E2E_METRICS_SERVICE': 'omnia-controller-manager-metrics-service',
}

_e2e_env['KUBECONFIG'] = os.getenv('KUBECONFIG', os.path.join(os.getenv('HOME', ''), '.kube/config'))
_e2e_cmd = 'kubectl config use-context %s && ' % k8s_context() + ' '.join(['%s=%s' % (k, v) for k, v in _e2e_env.items()])

# Full e2e suite — runs all tests against the Tilt dev cluster.
local_resource(
    'e2e-tests',
    cmd=_e2e_cmd + ' go test -tags=e2e -count=1 -v ./test/e2e/ -ginkgo.v -timeout 20m',
    labels=['test'],
    auto_init=False,
    trigger_mode=TRIGGER_MODE_MANUAL,
    resource_deps=['omnia-controller-manager', 'sample-resources'] + (['omnia-arena-controller'] if ENABLE_ENTERPRISE else []),
)

# CRD-only e2e tests — runs only the "Omnia CRDs" context (session-api, agents, tools).
local_resource(
    'e2e-tests-crds',
    cmd=_e2e_cmd + ' go test -tags=e2e -count=1 -v ./test/e2e/ -ginkgo.v -ginkgo.label-filter=crds -timeout 20m',
    labels=['test'],
    auto_init=False,
    trigger_mode=TRIGGER_MODE_MANUAL,
    resource_deps=['omnia-controller-manager', 'sample-resources'],
)

# Policy e2e tests — runs only the "Policy E2E" context (AgentPolicy + ToolPolicy CRDs).
local_resource(
    'e2e-tests-policy',
    cmd=_e2e_cmd + ' go test -tags=e2e -count=1 -v ./test/e2e/ -ginkgo.v -ginkgo.focus="Policy E2E" -timeout 10m',
    labels=['test'],
    auto_init=False,
    trigger_mode=TRIGGER_MODE_MANUAL,
    resource_deps=['omnia-controller-manager'],
)

# Tool calling e2e tests — runs against pre-deployed demo agents with Ollama.
# Requires ENABLE_DEMO=true. Tests real tool execution (calculate, weather) via llama3.2.
if ENABLE_DEMO:
    _tool_e2e_env = dict(_e2e_env)
    _tool_e2e_env['ENABLE_TOOL_CALLING_E2E'] = 'true'
    _tool_e2e_cmd = 'kubectl config use-context %s && ' % k8s_context() + ' '.join(['%s=%s' % (k, v) for k, v in _tool_e2e_env.items()])
    local_resource(
        'e2e-tests-tool-calling',
        cmd=_tool_e2e_cmd + ' go test -tags=e2e -count=1 -v ./test/e2e/ -ginkgo.v -ginkgo.label-filter=tool-calling -timeout 15m',
        labels=['test'],
        auto_init=False,
        trigger_mode=TRIGGER_MODE_MANUAL,
        resource_deps=['omnia-controller-manager'],
    )
