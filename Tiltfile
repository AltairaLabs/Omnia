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
# Auto-enables if promptkit-local/tools exists (arena worker needs promptarena)
_promptkit_local_exists = os.path.exists('./promptkit-local/tools/arena/cmd/promptarena/main.go')
USE_LOCAL_PROMPTKIT = os.getenv('USE_LOCAL_PROMPTKIT', '').lower() in ('true', '1', 'yes') or _promptkit_local_exists
PROMPTKIT_PATH = os.getenv('PROMPTKIT_PATH', '../PromptKit')

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
    _suppress_images.extend(['omnia-arena-worker-dev', 'omnia-arena-controller-dev'])
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
# Agent Images - Facade and Runtime containers for AgentRuntime pods
# ============================================================================

# Build facade image (WebSocket/HTTP server that handles client connections)
docker_build(
    'omnia-facade-dev',
    context='.',
    dockerfile='./Dockerfile.agent',
    only=[
        './cmd/agent',
        './internal/agent',
        './internal/facade',
        './internal/session',
        './pkg',
        './go.mod',
        './go.sum',
    ],
)

# Build runtime image (LLM interaction and tool execution)
# When USE_LOCAL_PROMPTKIT is enabled, includes local PromptKit source for development
runtime_only = [
    './cmd/runtime',
    './internal/runtime',
    './pkg',
    './api/proto',
    './go.mod',
    './go.sum',
]
runtime_build_args = {}

if USE_LOCAL_PROMPTKIT:
    runtime_only.append('./promptkit-local')
    runtime_build_args['USE_LOCAL_PROMPTKIT'] = 'true'

if USE_LOCAL_PROMPTKIT:
    # Sync local PromptKit source for development builds
    # This copies all subdirectories needed by go.mod replace directives:
    # - pkg: config types used by discovery.go
    # - runtime: agent runtime components
    # - sdk: SDK types
    # - tools/arena: promptarena CLI and engine
    local_resource(
        'sync-promptkit',
        cmd='''
            mkdir -p promptkit-local/tools
            rsync -av --delete "%s/pkg/" promptkit-local/pkg/
            rsync -av --delete "%s/runtime/" promptkit-local/runtime/
            rsync -av --delete "%s/sdk/" promptkit-local/sdk/
            rsync -av --delete "%s/tools/arena/" promptkit-local/tools/arena/
            echo "Synced PromptKit from %s"
        ''' % (PROMPTKIT_PATH, PROMPTKIT_PATH, PROMPTKIT_PATH, PROMPTKIT_PATH, PROMPTKIT_PATH),
        deps=[PROMPTKIT_PATH + '/pkg', PROMPTKIT_PATH + '/runtime', PROMPTKIT_PATH + '/sdk', PROMPTKIT_PATH + '/tools/arena'],
        labels=['dev'],
    )

docker_build(
    'omnia-runtime-dev',
    context='.',
    dockerfile='./Dockerfile.runtime',
    only=runtime_only,
    build_args=runtime_build_args,
)

# ============================================================================
# Enterprise Features - Arena Controller and Worker
# ============================================================================

if ENABLE_ENTERPRISE:
    # Build arena-controller image (enterprise ArenaSource/ArenaJob controllers)
    docker_build(
        'omnia-arena-controller-dev',
        context='.',
        dockerfile='./Dockerfile.arena-controller',
        only=[
            './ee/cmd/omnia-arena-controller',
            './ee/internal',
            './ee/pkg',
            './ee/api',
            './pkg',
            './api',
            './go.mod',
            './go.sum',
        ],
    )

    # Build arena-worker image (evaluation job worker)
    arena_worker_only = [
        './ee/cmd/arena-worker',
        './ee/internal',
        './ee/pkg',
        './ee/api',
        './pkg',
        './api',
        './go.mod',
        './go.sum',
    ]
    if USE_LOCAL_PROMPTKIT:
        arena_worker_only.append('./promptkit-local')

    docker_build(
        'omnia-arena-worker-dev',
        context='.',
        dockerfile='./Dockerfile.arena-worker',
        only=arena_worker_only,
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
    # LangChain runtime image (used when framework.type=langchain)
    'langchainRuntime.image.repository=omnia-langchain-runtime-dev',
    'langchainRuntime.image.tag=latest',
    'langchainRuntime.image.pullPolicy=Never',
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
        # Disable Loki/Alloy/Tempo for simpler setup (use ENABLE_FULL_STACK for full observability)
        'loki.enabled=false',
        'alloy.enabled=false',
        'tempo.enabled=false',
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
        # Enable distributed tracing for agent runtimes
        'tracing.enabled=true',
        'tracing.endpoint=omnia-tempo.omnia-system.svc.cluster.local:4317',
        # Enable Tempo for distributed tracing
        # Uses chart defaults for persistence
        'tempo.enabled=true',
        # Enable Loki for log aggregation
        # Uses chart defaults: SingleBinary mode, persistence enabled, ruler disabled
        'loki.enabled=true',
        # Enable Alloy for telemetry collection
        'alloy.enabled=true',
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
    resource_deps=dashboard_deps,
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

    # Tempo for distributed tracing (runtime instrumentation, no Istio required)
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

# ============================================================================
# Full Stack Mode Resources (Istio, Tempo, Loki, Alloy)
# ============================================================================

if ENABLE_FULL_STACK:
    # Note: Tempo resource already defined in ENABLE_OBSERVABILITY above

    # Loki for log aggregation
    k8s_resource(
        'omnia-loki',
        labels=['observability'],
        port_forwards=['3100:3100'],  # Loki HTTP API
    )

    # Alloy for telemetry collection
    k8s_resource(
        'omnia-alloy',
        labels=['observability'],
    )

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
    # Configure Ollama StatefulSet with port forward and label
    # Group related demo resources together
    # Object list differs based on OPA mode (sidecar vs extauthz)
    ollama_objects = [
        'ollama-models:persistentvolumeclaim',
        'ollama-credentials:secret',
        'ollama:provider',
        # Include vision-demo CRs (the Deployment is created by operator)
        'demo-vision-prompts:configmap',
        'demo-vision-prompts:promptpack',
        'vision-demo:agentruntime',
        # Include tools-demo CRs
        'ollama-tools:provider',
        'demo-tools-prompts:configmap',
        'demo-tools-prompts:promptpack',
        'tools-demo:agentruntime',
    ]

    # Add LangChain demo resources when enabled
    if ENABLE_LANGCHAIN:
        ollama_objects.extend([
            # LangChain vision demo (uses same PromptPack and Provider)
            'vision-demo-langchain:agentruntime',
            # LangChain tools demo
            'tools-demo-langchain:agentruntime',
        ])
    # Build resource_deps
    ollama_deps = []

    k8s_resource(
        'ollama',
        labels=['demo'],
        port_forwards=['11434:11434'],  # Ollama API (via Envoy when OPA enabled)
        extra_pod_selectors={'app.kubernetes.io/name': 'ollama'},
        objects=ollama_objects,
        resource_deps=ollama_deps,
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

# Restart agent pods when facade/framework images are rebuilt
# Since AgentRuntime deployments are created by the operator (not Tilt),
# we need to manually trigger a rollout when the source changes
restart_agents_deps = [
    './cmd/agent',
    './internal/agent',
    './internal/facade',
    './internal/session',
    './cmd/runtime',
    './internal/runtime',
]

# Include promptkit-local when using local PromptKit source
if USE_LOCAL_PROMPTKIT:
    restart_agents_deps.append('./promptkit-local')

local_resource(
    'restart-agents',
    cmd='''
        kubectl rollout restart deployment -n dev-agents -l omnia.altairalabs.ai/component=agent 2>/dev/null || true
        kubectl rollout restart deployment -n omnia-demo -l omnia.altairalabs.ai/component=agent 2>/dev/null || true
    ''',
    deps=restart_agents_deps,
    labels=['agents'],
    resource_deps=['sample-resources'],
    auto_init=False,  # Don't run on initial tilt up
)

# ============================================================================
# Local Resources (optional helpers)
# ============================================================================

# Run tests on file changes (optional - uncomment to enable)
# local_resource(
#     'go-test',
#     cmd='make test',
#     deps=['./internal', './pkg', './api'],
#     labels=['test'],
#     auto_init=False,
# )

# local_resource(
#     'dashboard-lint',
#     cmd='cd dashboard && npm run lint',
#     deps=['./dashboard/src'],
#     labels=['test'],
#     auto_init=False,
# )
