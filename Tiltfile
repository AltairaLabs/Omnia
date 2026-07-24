# Tiltfile for Omnia local development
#
# This enables hot-reload development of the dashboard and operator
# on a local Kubernetes cluster (kind, Docker Desktop, etc.)
#
# Usage:
#   tilt up                          # Platform only (dashboard, operator, session/memory
#                                    #   API, postgres, redis, doctor) — lean by default
#   ENABLE_DEMO=true tilt up         # + demo content (omnia-demos: the demo workspace,
#                                    #   agents, ollama, skills, sessions)
#   ENABLE_OBSERVABILITY=true tilt up# + Prometheus/Grafana/Loki/Tempo/Alloy
#   ENABLE_ENTERPRISE=true tilt up   # + Arena Fleet on a dev license, NFS, policy-broker
#   tilt down                        # Stop and clean up
#   tilt up --stream                 # Start with log streaming
#
# Environment variables (default off):
#   ENABLE_DEMO          - Deploy the demo content (default: OFF; set true for the demo workspace + Ollama)
#   ENABLE_ENTERPRISE    - EE: Arena controller + dev license, NFS, policy-broker, Redis
#   ENABLE_OBSERVABILITY - Prometheus/Grafana/Loki/Tempo/Alloy (default: OFF)
#   ENABLE_FULL_STACK    - Istio service mesh + Gateway API
#   (NFS/RWX workspace-content storage is ALWAYS on in Tilt — not a toggle; see
#    the ENABLE_NFS note below. The cluster-default RWO class is never used.)
#   ENABLE_AUDIO_DEMO    - Gemini audio demo agent (needs a gemini-credentials Secret)
#   ENABLE_VOICE_DEMO    - Gemini Live realtime voice agent (needs a gemini-credentials Secret)
#   ENABLE_MEMORY_DEMO   - memory-api dev demo: a seeded galaxy across all tiers
#   ENABLE_LANGCHAIN     - LangChain runtime demo agents
#   DASHBOARD_PROD       - Build the dashboard as a prod image (enables the Monaco LSP editor)
#   ENABLE_ENTRA         - Entra ID (Azure AD) OAuth for the dashboard
#   USE_LOCAL_PROMPTKIT  - Build the runtime against ../PromptKit (auto-on if it exists)
#
# Keep this list, docs/local-development.md, and the os.getenv() reads below in
# sync when you add or remove a flag.
#
# See docs/local-development.md for full setup instructions.

load('ext://helm_resource', 'helm_resource', 'helm_repo')
load('ext://namespace', 'namespace_create')

# ============================================================================
# Configuration
# ============================================================================

# Prometheus/Grafana/Loki/Tempo/Alloy observability stack (default: off).
# Set ENABLE_OBSERVABILITY=true for system-health dashboards, traces, and logs.
ENABLE_OBSERVABILITY = os.getenv('ENABLE_OBSERVABILITY', 'false').lower() in ('true', '1', 'yes')

# Demo content (the `demo` workspace, agents, Ollama, skills, session/memory-api)
# from the omnia-demos chart. Defaults OFF so a bare `tilt up` is platform-only;
# set ENABLE_DEMO=true to deploy it — notably the heavy ollama-vision
# (llava:7b ~7Gi) model.
ENABLE_DEMO = os.getenv('ENABLE_DEMO', 'false').lower() in ('true', '1', 'yes')

# Set to True to enable Audio Demo with Gemini (requires GEMINI_API_KEY)
# Can be set via environment: ENABLE_AUDIO_DEMO=true tilt up
# Note: You must create the gemini-credentials secret manually:
#   kubectl create secret generic gemini-credentials -n omnia-demo --from-literal=api-key=$GEMINI_API_KEY
ENABLE_AUDIO_DEMO = os.getenv('ENABLE_AUDIO_DEMO', '').lower() in ('true', '1', 'yes') or False

# Set to True to enable the Gemini Live realtime VOICE demo (spec.duplex "call"
# console). Requires a Gemini API key with Live/realtime access.
# Can be set via environment: ENABLE_VOICE_DEMO=true tilt up
# Reuses the same secret as the audio demo:
#   kubectl create secret generic gemini-credentials -n omnia-demo --from-literal=api-key=$GEMINI_API_KEY
ENABLE_VOICE_DEMO = os.getenv('ENABLE_VOICE_DEMO', '').lower() in ('true', '1', 'yes')

# Set to True to enable the memory-API dev demo: fills every memory UI surface
# in the omnia-demo workspace at realistic scale (seeder Job across all tiers,
# ollama nomic-embed-text embeddings, consolidation summarizer). Pulls in the
# tools-demo agent too (the seeder scopes agent-tier memories to it, and it
# provides the live remember/recall chat path).
# Can be set via environment: ENABLE_MEMORY_DEMO=true tilt up
ENABLE_MEMORY_DEMO = os.getenv('ENABLE_MEMORY_DEMO', '').lower() in ('true', '1', 'yes') or False

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

# Set to True to build the dashboard as a PRODUCTION image (next build --webpack,
# NODE_ENV=production) instead of the fast Turbopack dev server.
# This is the only way to run the Monaco LSP editor locally: Turbopack (the Next 16
# dev default) can't load monaco-languageclient, so the editor is gated off in dev.
# A prod build is the deployed bundler/mode, so it both enables the LSP editor and
# faithfully reproduces deployed behaviour. Trade-off: NO hot reload — the dashboard
# image rebuilds on change. Enable with: DASHBOARD_PROD=1 tilt up
DASHBOARD_PROD = os.getenv('DASHBOARD_PROD', '').lower() in ('true', '1', 'yes') or False

# Set to True to enable Entra ID (Azure AD) OAuth for the dashboard.
# Requires a one-time setup: run `./scripts/setup-entra-dev.sh --create-secret`
# to register the app, create the `dashboard-oauth` Secret, and generate
# `charts/omnia/values-dev-entra.yaml` (gitignored).
# Can be set via environment: ENABLE_ENTRA=true tilt up
ENABLE_ENTRA = os.getenv('ENABLE_ENTRA', '').lower() in ('true', '1', 'yes') or False

# Internal NFS server for workspace-content storage — ALWAYS ON in Tilt, every
# config. It provides the ReadWriteMany (RWX) `omnia-nfs` storage class so the
# workspace-content PVC (which the operator mounts unconditionally) is always
# RWX-backed. The cluster-default RWO class (local-path) must NEVER back it:
# switching the PVC's storage class between runs is an immutable change that
# deadlocks Tilt's apply (operator holds the PVC open while Tilt delete-recreates
# it). Pinning RWX/omnia-nfs for every config makes that deadlock impossible.
# Config lives in charts/omnia/values-dev-nfs.yaml (always applied below).
# The chart itself ships local NFS OFF (cloud brings its own RWX CSI).
ENABLE_NFS = True

# Allow deployment to local clusters only (safety check)
allow_k8s_contexts(['kind-omnia-dev', 'docker-desktop', 'minikube', 'kind-kind', 'orbstack'])

# Suppress warnings for images passed as CLI args to operator (not in K8s manifests)
# Also suppress langchain runtime which is referenced via Helm values, not directly in manifests
_suppress_images = ['omnia-facade-dev', 'omnia-runtime-dev', 'omnia-langchain-runtime-dev']
if ENABLE_ENTERPRISE:
    _suppress_images.extend(['omnia-arena-controller-dev', 'omnia-promptkit-lsp-dev', 'omnia-session-api-dev', 'omnia-memory-api-dev', 'omnia-privacy-api-dev'])
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

    # NOTE: no standalone istio/gateway install here. The omnia chart's
    # gateway.enabled creates Gateway-API Gateways (omnia-agents,
    # omnia-internal) under gatewayClassName=istio; Istio auto-provisions a
    # gateway Deployment+Service per Gateway. A manual istio/gateway release
    # was redundant scaffolding (it owned no HTTPRoutes — the routes attach to
    # the chart Gateways) and just added a second, confusing ingress. Local dev
    # reaches services via port-forward regardless, so no in-cluster ingress is
    # required.

    # Enable sidecar injection for agent namespaces
    local_resource(
        'istio-inject-labels',
        cmd='''
            kubectl label namespace omnia-demo istio-injection=enabled --overwrite 2>/dev/null || true
        ''',
        labels=['istio'],
        resource_deps=['istiod'],
    )

# ============================================================================
# Dashboard - Hot reload development
# ============================================================================

# Build dashboard. Default: fast Turbopack dev server with live_update (HMR).
# DASHBOARD_PROD=1: production webpack build (NODE_ENV=production) so the Monaco
# LSP editor loads — no HMR, full image rebuild on change.
if DASHBOARD_PROD:
    print("Dashboard: PRODUCTION build (next build --webpack) — LSP editor enabled, NO hot reload")
    docker_build(
        'omnia-dashboard-dev',
        context='./dashboard',
        dockerfile='./dashboard/Dockerfile',
    )
else:
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
# Privacy API Server - Per-workspace consent and opt-out service (Enterprise)
# ============================================================================

docker_build(
    'omnia-privacy-api-dev',
    context='.',
    dockerfile='./ee/Dockerfile.privacy-api',
    # Mirror the Dockerfile's COPY set. privacy-api transitively pulls
    # ee/pkg/{audit,redaction}, ee/api, and internal/session (a deep dep
    # tree), so enumerating leaf packages is fragile — include the whole
    # api/ ee/ pkg/ internal/ trees, exactly what the Dockerfile copies.
    only=[
        './api',
        './ee',
        './internal',
        './pkg',
        './go.mod',
        './go.sum',
    ],
)

# ============================================================================
# Doctor - Cluster diagnostic service for Omnia health checks
# ============================================================================
# Doctor is a platform component (chart-owned, default off) but every target it
# probes in dev is demo content (the tools-demo agent + ollama-chat), so it's
# only useful — and only enabled — when ENABLE_DEMO is on.

if ENABLE_DEMO:
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
    # arena-controller deliberately builds against the published PromptKit module
    # (no promptkit-local / go.work) so its binary stays statically linked and the
    # image stays on distroless/static — see ee/Dockerfile.arena-controller.

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

    # Policy-broker sidecar image is built + rolled by the auto-rebuild-policy-broker
    # local_resource (Dynamic Services) so source edits restart the agent pods that
    # carry the sidecar — a plain docker_build would rebuild but never roll them.

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
    # Per-workspace privacy-api image (operator creates Deployments from Workspace.spec.privacy)
    'workspaceServices.privacyApi.image.repository=omnia-privacy-api-dev',
    'workspaceServices.privacyApi.image.tag=latest',
    'workspaceServices.privacyApi.image.pullPolicy=Never',
    # Exercise the consolidation worker locally — 30s tick is short
    # enough that any local seed of stale observations gets processed
    # within a development feedback cycle. Production runs at 6h.
    'workspaceServices.memoryApi.consolidation.interval=30s',
    # Pre-render the Memory Galaxy galaxy layout locally — 30s tick keeps
    # the workspace-wide projection warm so the dashboard galaxy loads
    # instantly during development. Production opts in per environment.
    'workspaceServices.memoryApi.projection.interval=30s',
    # Dev Postgres for workspace services
    'postgres.dev.enabled=true',
    # Doctor is gated on ENABLE_DEMO (see the conditional helm_set.extend below) —
    # all of its probe targets are demo content.
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

# A production-mode dashboard (DASHBOARD_PROD=1) runs with NODE_ENV=production,
# where the auth boot guard refuses to start under the local 'anonymous' auth
# mode unless explicitly acknowledged. Local Tilt is a sandbox, so opt in.
if DASHBOARD_PROD:
    helm_set.append('dashboard.auth.allowAnonymous=true')

# Bitnami Redis subchart — enabled unconditionally for dev so the
# memory-api read-through cache, the dashboard session store, the
# memory event publisher, and (in enterprise mode) the Arena queue
# all get a working backend out of the box. Footprint is tiny:
# standalone, no auth, no persistence, ~50MB. Without this, Doctor's
# RedisReachable check fails on every dev run and any consumer that
# reaches for Redis crashloops on connect.
helm_set.extend([
    'redis.enabled=true',
    'redis.architecture=standalone',
    'redis.auth.enabled=false',
    'redis.master.persistence.enabled=false',
])

# Doctor — only meaningful with demo content, so gate it on ENABLE_DEMO. The
# chart defaults doctor off; these settings enable it and point it at the demo's
# tools-demo agent + ollama-chat.
if ENABLE_DEMO:
    helm_set.extend([
        'doctor.enabled=true',
        'doctor.image.repository=omnia-doctor-dev',
        'doctor.image.tag=latest',
        'doctor.image.pullPolicy=Never',
        'doctor.workspace=demo',
        'doctor.serviceGroup=default',
        'doctor.agentNamespace=omnia-demo',
        'doctor.agentName=tools-demo',
        'doctor.ollamaService=ollama-chat',
    ])

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
        # Arena queue uses Redis. The Bitnami subchart is already
        # enabled unconditionally above; address auto-derives via the
        # chart helper to FQDN form (omnia-redis-master.<ns>.svc.
        # cluster.local:6379). Don't set arena.queue.redis.host
        # explicitly — that override produces a non-FQDN that fails
        # cross-namespace DNS lookup from arena workers in
        # omnia-demo/test-arena namespaces.
        'enterprise.arena.queue.type=redis',
        # PromptKit LSP server for YAML validation
        'enterprise.promptkitLsp.enabled=true',
        'enterprise.promptkitLsp.image.repository=omnia-promptkit-lsp-dev',
        'enterprise.promptkitLsp.image.tag=latest',
        'enterprise.promptkitLsp.image.pullPolicy=Never',
        # Arena Dev Console image for ArenaDevSession controller (creates dynamic pods)
        'enterprise.arena.devConsole.image.repository=omnia-arena-dev-console-dev',
        'enterprise.arena.devConsole.image.tag=latest',
        'enterprise.arena.devConsole.image.pullPolicy=Never',
        # Per-service-group eval worker image. The operator builds one
        # arena-eval-worker-<group> Deployment per workspace service group that
        # has an eval-enabled, non-PromptKit AgentRuntime — there is no longer a
        # cluster-wide singleton or a namespaces list to configure.
        'workspaceServices.evalWorker.image.repository=omnia-eval-worker-dev',
        'workspaceServices.evalWorker.image.tag=latest',
        'workspaceServices.evalWorker.image.pullPolicy=Never',
        # Policy broker sidecar for ToolPolicy enforcement
        'enterprise.policyBroker.image.repository=omnia-policy-broker-dev',
        'enterprise.policyBroker.image.tag=latest',
        'enterprise.policyBroker.image.pullPolicy=Never',
    ])
else:
    # Disable enterprise features
    helm_set.extend([
        'enterprise.enabled=false',
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

# The omnia-demos chart deploys into omnia-demo (when any demo variant is on),
# so the namespace must exist before the chart renders.
if ENABLE_DEMO or ENABLE_AUDIO_DEMO or ENABLE_MEMORY_DEMO or ENABLE_VOICE_DEMO:
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

# Build values files list. values-dev-nfs.yaml is ALWAYS applied — it pins
# workspace-content to RWX/omnia-nfs so the storage-class-change deadlock can
# never happen (see the ENABLE_NFS note above). It must come before the
# enterprise overlay so enterprise can still layer on top.
helm_values = ['./charts/omnia/values-dev.yaml', './charts/omnia/values-dev-nfs.yaml']
if ENABLE_ENTERPRISE:
    helm_values.append('./charts/omnia/values-dev-enterprise.yaml')
if ENABLE_FULL_STACK:
    helm_values.append('./charts/omnia/values-istio-prometheus.yaml')
if ENABLE_ENTRA:
    _entra_values = './charts/omnia/values-dev-entra.yaml'
    if not os.path.exists(_entra_values):
        fail('ENABLE_ENTRA=true but %s is missing. Run ./scripts/setup-entra-dev.sh --create-secret first.' % _entra_values)
    helm_values.append(_entra_values)

# Install CRDs via server-side apply. Client-side apply (what k8s_yaml uses)
# can't handle the 262144-byte last-applied-configuration annotation limit
# for large CRDs that embed PodOverrides schemas (agentruntimes ~763K,
# workspaces ~567K). See charts/omnia/CLAUDE.md rule #6. `make install`
# does the equivalent `kubectl apply --server-side --force-conflicts`.
local_resource(
    'omnia-crds',
    cmd='make install',
    # Watch only the Go source — `make install` regenerates config/crd/bases/
    # as a side-effect, so watching the output dir would cause a loop.
    deps=['api/v1alpha1'],
    labels=['setup'],
)

# Render the chart via Tilt's helm() (which injects the release namespace into
# resources that omit it) and then strip CRD documents from the blob before
# feeding to k8s_yaml — CRDs are already applied above via make install.
# NOTE: when enterprise.enabled=true, EE CRDs under templates/enterprise/ are
# also filtered out here, but make install only ships core CRDs; revisit the
# setup resource if you need Tilt-based enterprise deploys.
_rendered = str(helm(
    './charts/omnia',
    name='omnia',
    namespace='omnia-system',
    values=helm_values,
    set=helm_set,
))
_docs = _rendered.split('\n---\n')
_non_crd_docs = [d for d in _docs if 'kind: CustomResourceDefinition' not in d]
k8s_yaml(blob('\n---\n'.join(_non_crd_docs)))

# The chart renders a `default` SessionRetentionPolicy CR (controlled by
# `sessionRetention.defaultPolicy.create`). Without an explicit dep it races
# `omnia-crds` and Tilt fails with "no matches for kind SessionRetentionPolicy"
# when the CRD isn't established yet. Group it with omnia-crds so the apply
# waits.
k8s_resource(
    new_name='default-retention-policy',
    objects=['default:sessionretentionpolicy'],
    labels=['setup'],
    resource_deps=['omnia-crds'],
)

# ============================================================================
# Demo Charts (separate from main Omnia chart)
# ============================================================================

# The omnia-demos chart is the single source of local dev content: it provides
# the `demo` workspace, agents, ollama (chat/vision), skills, and an
# operator-managed session/memory-api. It deploys by default so a bare
# `tilt up` has a working demo workspace; set ENABLE_DEMO=false to skip it. The
# other ENABLE_* flags add optional demo surfaces (audio, langchain, memory
# seeder, enterprise arena) and also pull the chart in.
#
# Build demo helm set values
if ENABLE_DEMO or ENABLE_AUDIO_DEMO or ENABLE_MEMORY_DEMO or ENABLE_VOICE_DEMO:
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

    if ENABLE_VOICE_DEMO:
        demo_helm_set.extend([
            'voiceDemo.enabled=true',
        ])
        # Voice-only (no ENABLE_DEMO): skip the heavy Ollama-backed vision/tools
        # demos so a `ENABLE_VOICE_DEMO=true tilt up` brings just the demo
        # workspace + the Gemini Live voice agent, not a 7Gi model pull.
        if not ENABLE_DEMO:
            demo_helm_set.extend([
                'visionDemo.enabled=false',
                'toolsDemo.enabled=false',
                'compositionDemo.enabled=false',
                'ollama.instances=null',
            ])

    if ENABLE_LANGCHAIN:
        demo_helm_set.extend([
            # Enable LangChain demos (mirror of vision-demo and tools-demo)
            'langchainDemo.enabled=true',
            'langchainDemo.image.repository=omnia-langchain-runtime-dev',
            'langchainDemo.image.tag=latest',
            'langchainDemo.image.pullPolicy=Never',
        ])

    if ENABLE_MEMORY_DEMO:
        # Dev-only seeder image consumed by the memory-demo post-install Job.
        # only=[...] keeps the build context to what `go build ./demos/memory-seeder`
        # needs (it imports just pkg/identity beyond stdlib).
        docker_build(
            'omnia-memory-seeder-dev',
            '.',
            dockerfile='demos/memory-seeder/Dockerfile',
            only=[
                './demos/memory-seeder',
                './pkg/identity',
                './go.mod',
                './go.sum',
            ],
        )
        demo_helm_set.extend([
            'memoryDemo.enabled=true',
            # The seeder scopes agent-tier memories to the tools-demo agent, and
            # tools-demo provides the live remember/recall chat path.
            'toolsDemo.enabled=true',
        ])

    if ENABLE_ENTERPRISE:
        # Keep the demos chart's enterprise flag in sync with the omnia chart
        # (which gets enterprise.enabled=true + devMode=true above), so the EE
        # demos (Arena Fleet) light up against the operator's dev license.
        demo_helm_set.append('enterprise.enabled=true')

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

# Build resource dependencies - when NFS is enabled, wait for storage to be ready.
# `omnia-crds` must land before the operator/dashboard come up, or the operator's
# first reconcile will fail watching CRDs that don't yet exist.
controller_deps = ['omnia-crds']
# The dashboard is a SPA that reaches the operator / session-api over the network
# at request time (with its own loading/error states) — it needs nothing deployed
# first. Don't gate it on CRDs or, especially, the slow NFS controller/server
# (which it never uses): get the UI up as fast as possible.
dashboard_deps = []
if ENABLE_NFS:
    controller_deps = ['omnia-crds', 'csi-nfs-controller', 'omnia-nfs-server']

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
    resource_deps=dashboard_deps,
)

# Session API server and its dev Postgres
k8s_resource(
    'omnia-postgres',
    labels=['database'],
    objects=[
        'omnia-postgres:secret',
    ],
)

# Session-api and memory-api Deployments are created dynamically by the operator
# when it reconciles the `demo` Workspace CRD (from the omnia-demos chart). They
# run as `session-demo-default` / `memory-demo-default` in the omnia-demo
# namespace; the dashboard reaches them in-cluster, so no port-forward is needed.

# Doctor is only deployed when ENABLE_DEMO is on (all its probe targets are demo
# content), so only register its Tilt resource then.
if ENABLE_DEMO:
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
            - "--url=postgres://omnia:omnia@omnia-postgres:5432/omnia_sessions?sslmode=disable"
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
    labels=['database'],
    port_forwards=['8081:8081'],  # pgweb UI
    resource_deps=['omnia-postgres'],
)

# VS Code server — local-dev-only browser for the NFS workspace-content volume
# (the only way to see what's on it). Defined inline here, NOT in the chart:
# like pgweb it's a dev convenience, not a platform feature. Mounts the always-on
# omnia-nfs workspace-content PVC. Lives in the storage group.
k8s_yaml(blob('''
apiVersion: apps/v1
kind: Deployment
metadata:
  name: omnia-vscode-server
  namespace: omnia-system
  labels:
    app.kubernetes.io/name: vscode-server
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: vscode-server
  template:
    metadata:
      labels:
        app.kubernetes.io/name: vscode-server
    spec:
      containers:
        - name: code-server
          image: codercom/code-server:4.96.4
          args:
            - --auth
            - none
            - --bind-addr
            - "0.0.0.0:8080"
            - /workspace
          ports:
            - containerPort: 8080
              protocol: TCP
          volumeMounts:
            - name: workspace-content
              mountPath: /workspace
          resources:
            limits:
              cpu: "1"
              memory: 1Gi
            requests:
              cpu: 100m
              memory: 256Mi
      volumes:
        - name: workspace-content
          persistentVolumeClaim:
            claimName: omnia-workspace-content
'''))

k8s_resource(
    'omnia-vscode-server',
    labels=['storage'],
    port_forwards=['8888:8080'],  # VS Code Server UI
    resource_deps=['csi-nfs-controller', 'omnia-nfs-server'],
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
# Redis (always-on for dev)
# ============================================================================
# Bitnami Redis subchart is enabled unconditionally so memory cache,
# session store, event publisher, and (in enterprise mode) Arena queue
# all have a working backend in dev. Resource label "infra" rather
# than "enterprise" because it serves OSS consumers too.

k8s_resource(
    'omnia-redis-master',
    labels=['infra'],
    port_forwards=['6379:6379'],  # Redis port for local debugging
)

# ============================================================================
# Storage - NFS Server and CSI Driver (ALWAYS ON in dev; ENABLE_NFS is forced
# True above so workspace-content is always RWX/omnia-nfs)
# ============================================================================

if ENABLE_NFS:
    # NFS server deployment and backing storage (shared RWX filesystem)
    k8s_resource(
        'omnia-nfs-server',
        labels=['storage'],
        objects=[
            'omnia-nfs-data:persistentvolumeclaim',
        ],
    )

    # NFS CSI driver controller (handles dynamic PV provisioning)
    # Must be ready before workspace-content PVC can be provisioned
    k8s_resource(
        'csi-nfs-controller',
        labels=['storage'],
        objects=[
            'omnia-nfs:storageclass',
            'omnia-workspace-content:persistentvolumeclaim',
        ],
        resource_deps=['omnia-nfs-server'],
    )

    # NFS CSI driver node daemonset
    k8s_resource(
        'csi-nfs-node',
        labels=['storage'],
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

    # PromptKit LSP server for YAML validation in project editor. Independent
    # of the dashboard — they talk over the network, not via startup order.
    k8s_resource(
        'omnia-promptkit-lsp',
        labels=['enterprise'],
    )

    # The cluster-wide omnia-eval-worker singleton was removed. The operator now
    # builds one arena-eval-worker-<group> Deployment per workspace service group
    # with an eval-enabled, non-PromptKit AgentRuntime. Those are created
    # dynamically (not part of the Helm release) so there is no static
    # k8s_resource to register here.

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
        # istiod is the Gateway-API controller that programs these Gateways and
        # auto-provisions their gateway pods (was depending on the now-removed
        # standalone istio-ingress release).
        resource_deps=['istio-crds', 'istiod'],
    )

# ============================================================================
# Demo Mode Resources (Ollama + OPA)
# ============================================================================

# Tilt-UI grouping + port-forwards for the demo content (from the omnia-demos
# chart, gated on ENABLE_DEMO). Ollama runs one Deployment per model instance
# (.Values.ollama.instances): ollama-chat (text/tools) and ollama-vision
# (multimodal). Port-forward each to a distinct localhost port (11434 = chat,
# 11435 = vision); group the shared credentials Secret + Provider CRs under chat.
if ENABLE_DEMO:
    k8s_resource(
        'ollama-chat',
        labels=['demo'],
        port_forwards=['11434:11434'],
        objects=[
            'ollama-chat-models:persistentvolumeclaim',
            'ollama-credentials:secret',
            'ollama:provider',
            'ollama-tools:provider',
        ],
    )
    k8s_resource(
        'ollama-vision',
        labels=['demo'],
        port_forwards=['11435:11434'],
        objects=[
            'ollama-vision-models:persistentvolumeclaim',
        ],
    )

    # The demo Workspace requests RWX (NFS) content storage, so hold its apply
    # until the NFS server is Ready — otherwise the operator provisions the
    # content PVC against a server that's still coming up and it stalls. NFS
    # reliability itself is handled by the nfs-server module-loader initContainer;
    # this is the ordering guard on top. With NFS disabled the Workspace has no
    # storage dependency and applies immediately. The demo *agents* deliberately
    # do NOT depend on this — only the Workspace's content storage needs NFS.
    k8s_resource(
        new_name='demo-workspace',
        objects=['demo:workspace'],
        labels=['demo'],
        resource_deps=(['omnia-nfs-server'] if ENABLE_NFS else []),
    )

    # Agent CRs are grouped separately so their operator-created Deployments
    # don't get associated with the ollama resources (which breaks port forwarding).
    demo_agent_objects = [
        'demo-vision-prompts:configmap',
        'demo-vision-prompts:promptpack',
        'vision-demo:agentruntime',
        'demo-tools-prompts:configmap',
        'demo-tools-prompts:promptpack',
        'tools-demo:agentruntime',
        'demo-composition-prompts:configmap',
        'demo-composition:promptpack',
        'composition-demo:agentruntime',
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
        resource_deps=['ollama-chat', 'ollama-vision'],
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

if ENABLE_VOICE_DEMO:
    # Voice demo resources (Gemini Live realtime duplex agent).
    # Note: User must create the gemini-credentials secret before deploying.
    voice_demo_objects = [
        'voice-provider:provider',
        'demo-voice-prompts:configmap',
        'demo-voice-prompts:promptpack',
        'voice-demo:agentruntime',
    ]

    k8s_resource(
        workload='',  # No workload, just CRs (Deployment created by operator)
        new_name='voice-demo',
        labels=['demo'],
        objects=voice_demo_objects,
        resource_deps=['omnia-controller-manager'],
    )

# ============================================================================
# Dev Content
# ============================================================================
#
# All local dev content (the `demo` workspace, agents, providers, ollama, skills,
# and operator-managed session/memory-api) comes from the omnia-demos Helm chart
# deployed above. The legacy config/samples/dev/ mechanism (the `dev-agents`
# workspace) and its enterprise arena-fleet fixtures have been removed — the
# demos chart's arenaDemo (gated on enterprise.enabled) provides arena content.

# Rebuild facade/runtime images and restart agent pods.
# Since these images are passed as CLI args to the operator (not in K8s YAML),
# Tilt can't track them as k8s_image_json_path resources. Instead, we watch
# source deps, rebuild images, and restart pods in a single atomic flow.

_rebuild_facade_cmd = 'docker build -f Dockerfile.agent -t omnia-facade-dev:latest .'
_rebuild_runtime_cmd = 'docker build -f Dockerfile.runtime'
if USE_LOCAL_PROMPTKIT:
    _rebuild_runtime_cmd += ' --build-arg USE_LOCAL_PROMPTKIT=true'
_rebuild_runtime_cmd += ' -t omnia-runtime-dev:latest .'

_restart_cmd = '''
    kubectl delete po -n omnia-demo -l omnia.altairalabs.ai/component=agent 2>/dev/null || true
'''

# Memory-api and session-api Deployments are operator-managed; Tilt builds the
# images but doesn't own the workloads, so source edits never reach the
# running pod without an explicit roll. Without this, fixes to the memory-api
# binary silently sit in the image while the cluster runs the old code —
# exactly the wiring trap that bit hybrid recall on the multi-tier path.
_rebuild_memory_api_cmd = 'docker build -f Dockerfile.memory-api -t omnia-memory-api-dev:latest .'
_rebuild_session_api_cmd = 'docker build -f Dockerfile.session-api -t omnia-session-api-dev:latest .'
# Policy-broker is an operator-injected sidecar in agent pods (EE/ToolPolicy
# enforcement). A source edit rebuilds the image, but only restarting the agent
# pods picks up the new sidecar — same wiring trap as the api binaries above.
_rebuild_policy_broker_cmd = 'docker build -f ./ee/Dockerfile.policy-broker -t omnia-policy-broker-dev:latest .'
# Delete the pod (not the deployment): the operator owns the Deployment spec
# and reconciles `kubectl rollout restart` annotations away. Pod deletion lets
# the existing ReplicaSet recreate with the same `:latest` image we just
# rebuilt (imagePullPolicy: Never picks up the new image without an image-tag
# bump). Same pattern as agent auto-rebuild.
_restart_memory_api_cmd = 'kubectl delete po -n omnia-demo -l app.kubernetes.io/component=memory-api 2>/dev/null || true'
_restart_session_api_cmd = 'kubectl delete po -n omnia-demo -l app.kubernetes.io/component=session-api 2>/dev/null || true'
# privacy-api is the same operator-managed-Deployment trap: rebuild the image,
# then delete the pod so the ReplicaSet recreates with the new :latest.
_rebuild_privacy_api_cmd = 'docker build -f ./ee/Dockerfile.privacy-api -t omnia-privacy-api-dev:latest .'
_restart_privacy_api_cmd = 'kubectl delete po -n omnia-demo -l app.kubernetes.io/component=privacy-api 2>/dev/null || true'

_privacy_api_deps = [
    './ee/cmd/privacy-api',
    './ee/pkg/privacy',
    './ee/pkg/audit',
    './ee/pkg/redaction',
    './ee/api',
    './internal/serviceauth',
    './internal/session',
    './internal/tracing',
    './pkg',
    './api',
    './go.mod',
]

_memory_api_deps = [
    './cmd/memory-api',
    './api',
    './internal/memory',
    './internal/session',
    './internal/pgutil',
    './internal/httputil',
    './internal/tracing',
    './pkg',
    './go.mod',
]

_session_api_deps = [
    './cmd/session-api',
    './api',
    './internal/session',
    './internal/pgutil',
    './internal/httputil',
    './internal/tracing',
    './pkg',
    './go.mod',
]

_policy_broker_deps = [
    './ee/cmd/policy-broker',
    './ee/api',
    './ee/pkg',
    './api',
    './internal',
    './pkg',
    './go.mod',
    './go.sum',
]

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
    labels=['dynamic-services'],
)

local_resource(
    'auto-rebuild-runtime',
    cmd=_rebuild_runtime_cmd + ' && ' + _restart_cmd,
    deps=_runtime_deps,
    labels=['dynamic-services'],
)

local_resource(
    'auto-rebuild-memory-api',
    cmd=_rebuild_memory_api_cmd + ' && ' + _restart_memory_api_cmd,
    deps=_memory_api_deps,
    labels=['dynamic-services'],
)

local_resource(
    'auto-rebuild-session-api',
    cmd=_rebuild_session_api_cmd + ' && ' + _restart_session_api_cmd,
    deps=_session_api_deps,
    labels=['dynamic-services'],
)

local_resource(
    'auto-rebuild-privacy-api',
    cmd=_rebuild_privacy_api_cmd + ' && ' + _restart_privacy_api_cmd,
    deps=_privacy_api_deps,
    labels=['dynamic-services'],
)

# ============================================================================
# Port-forwards — API documentation endpoints
# Expose /docs at localhost:<port>/docs for each workspace API service.
# session-api:  http://localhost:8085/docs
# memory-api:   http://localhost:8086/docs
# privacy-api:  http://localhost:8087/docs
# ============================================================================

local_resource(
    'pf-session-api',
    serve_cmd='kubectl port-forward -n omnia-demo svc/session-demo-default 8085:8080',
    links=[link('http://localhost:8085/docs', 'session-api docs')],
    labels=['port-forwards'],
)

local_resource(
    'pf-memory-api',
    serve_cmd='kubectl port-forward -n omnia-demo svc/memory-demo-default 8086:8080',
    links=[link('http://localhost:8086/docs', 'memory-api docs')],
    labels=['port-forwards'],
)

local_resource(
    'pf-privacy-api',
    serve_cmd='kubectl port-forward -n omnia-demo svc/privacy-demo 8087:8080',
    links=[link('http://localhost:8087/docs', 'privacy-api docs')],
    labels=['port-forwards'],
)

# Policy-broker sidecar (EE/ToolPolicy). Rebuild the image and restart the
# agent pods that carry it so source edits actually reach the running sidecar.
if ENABLE_ENTERPRISE:
    local_resource(
        'auto-rebuild-policy-broker',
        cmd=_rebuild_policy_broker_cmd + ' && ' + _restart_cmd,
        deps=_policy_broker_deps,
        labels=['dynamic-services'],
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
    'SESSION_API_URL': 'http://session-demo-default.omnia-demo.svc.cluster.local:8080',
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
    resource_deps=['omnia-controller-manager'] + (['omnia-arena-controller'] if ENABLE_ENTERPRISE else []),
)

# CRD-only e2e tests — runs only the "Omnia CRDs" context (session-api, agents, tools).
local_resource(
    'e2e-tests-crds',
    cmd=_e2e_cmd + ' go test -tags=e2e -count=1 -v ./test/e2e/ -ginkgo.v -ginkgo.label-filter=crds -timeout 20m',
    labels=['test'],
    auto_init=False,
    trigger_mode=TRIGGER_MODE_MANUAL,
    resource_deps=['omnia-controller-manager'],
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
# Tests real tool execution (calculate, weather) via llama3.2 against the
# tools-demo agent. Manual-trigger. Gated on ENABLE_DEMO — it needs the demo's
# tools-demo agent.
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
