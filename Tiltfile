# Tiltfile for Omnia local development
#
# This enables hot-reload development of the dashboard and operator
# on a local Kubernetes cluster (kind, Docker Desktop, etc.)
#
# Usage:
#   tilt up              # Start development
#   tilt down            # Stop and clean up
#   tilt up --stream     # Start with log streaming
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

# Set to True to enable full production-like stack
# Includes: Istio service mesh, Tempo (tracing), Loki (logging), Alloy (collector),
#           Gateway API, and dashboard with builtin auth
# Requires: 16GB+ RAM recommended
# Can be set via environment: ENABLE_FULL_STACK=true tilt up
ENABLE_FULL_STACK = os.getenv('ENABLE_FULL_STACK', '').lower() in ('true', '1', 'yes') or False

# Allow deployment to local clusters only (safety check)
allow_k8s_contexts(['kind-omnia-dev', 'docker-desktop', 'minikube', 'kind-kind'])

# Suppress warnings for images passed as CLI args to operator (not in K8s manifests)
update_settings(suppress_unused_image_warnings=['omnia-facade-dev', 'omnia-runtime-dev'])

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
docker_build(
    'omnia-operator-dev',
    context='.',
    dockerfile='./Dockerfile',
    # Only rebuild when Go files change
    only=[
        './cmd',
        './api',
        './internal',
        './pkg',
        './go.mod',
        './go.sum',
    ],
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
docker_build(
    'omnia-runtime-dev',
    context='.',
    dockerfile='./Dockerfile.runtime',
    only=[
        './cmd/runtime',
        './internal/runtime',
        './pkg',
        './api/proto',
        './go.mod',
        './go.sum',
    ],
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
]

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
        # Enable Tempo for tracing (works without Istio via runtime instrumentation)
        'tempo.enabled=true',
        'tempo.persistence.enabled=false',
        # Enable tracing for agent runtime containers
        'tracing.enabled=true',
        'tracing.endpoint=omnia-tempo.omnia-system.svc.cluster.local:4317',
        # Disable Loki/Alloy for simpler setup
        'loki.enabled=false',
        'alloy.enabled=false',
    ])
else:
    helm_set.extend([
        # Disable observability
        'prometheus.enabled=false',
        'grafana.enabled=false',
        'loki.enabled=false',
    ])

if ENABLE_DEMO:
    # Create demo namespace
    namespace_create('omnia-demo')
    helm_set.extend([
        # Enable demo mode with Ollama
        'demo.enabled=true',
        'demo.namespace=omnia-demo',
        # Enable OPA model validation (sidecar mode, no Istio required)
        'demo.opa.enabled=true',
        'demo.opa.mode=sidecar',
        # Use persistence for model cache
        'demo.ollama.persistence.enabled=true',
    ])

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

    # When full stack is enabled, also enable OPA in extauthz mode (uses Istio)
    if ENABLE_DEMO:
        # Override OPA mode to use Istio ext_authz
        helm_set.extend([
            'demo.opa.mode=extauthz',
        ])

# Build values files list
helm_values = ['./charts/omnia/values-dev.yaml']
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
# Resource Configuration
# ============================================================================

# Group resources for better UI organization
# Note: facade/runtime images are built by docker_build() but not directly
# referenced in K8s YAML (they're passed as CLI args to the operator).
# The restart-agents local_resource handles restarting agent pods when these change.
k8s_resource(
    'omnia-controller-manager',
    labels=['operator'],
    port_forwards=['8082:8082'],  # Operator API
)

k8s_resource(
    'omnia-dashboard',
    labels=['dashboard'],
    port_forwards=[
        '3000:3000',  # Dashboard UI
        '3002:3002',  # WebSocket proxy for agent connections
    ],
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
        'ollama-opa-config:configmap',
        'ollama-credentials:secret',
        'ollama:provider',
        # Include vision-demo CRs (the Deployment is created by operator)
        'demo-vision-prompts:configmap',
        'demo-vision-prompts:promptpack',
        'vision-demo:agentruntime',
    ]
    # Sidecar mode uses Envoy config, extauthz mode uses EnvoyFilter
    if ENABLE_FULL_STACK:
        ollama_objects.append('ollama-opa-ext-authz:envoyfilter')
    else:
        ollama_objects.append('ollama-envoy-config:configmap')

    # Build resource_deps - need istio-crds when using EnvoyFilter
    ollama_deps = []
    if ENABLE_FULL_STACK:
        ollama_deps.append('istio-crds')

    k8s_resource(
        'ollama',
        labels=['demo'],
        port_forwards=['11434:11434'],  # Ollama API (via Envoy when OPA enabled)
        extra_pod_selectors={'app.kubernetes.io/name': 'ollama'},
        objects=ollama_objects,
        resource_deps=ollama_deps,
    )

    # Label the model pull job
    # Note: The job has its own init container to wait for Ollama, so no resource_deps needed
    k8s_resource(
        'ollama-pull-model',
        labels=['demo'],
    )

# ============================================================================
# Sample Resources for Development
# ============================================================================

# Apply sample resources using local_resource for better control
# Note: When ENABLE_DEMO is true, Ollama resources come from Helm chart, not samples
local_resource(
    'sample-resources',
    cmd='kubectl apply -f config/samples/dev/',
    deps=['config/samples/dev'],
    labels=['samples'],
    resource_deps=['omnia-controller-manager'],
)

# Restart agent pods when facade/framework images are rebuilt
# Since AgentRuntime deployments are created by the operator (not Tilt),
# we need to manually trigger a rollout when the source changes
local_resource(
    'restart-agents',
    cmd='kubectl rollout restart deployment -n dev-agents -l omnia.altairalabs.ai/component=agent 2>/dev/null || true',
    deps=[
        './cmd/agent',
        './internal/agent',
        './internal/facade',
        './internal/session',
        './cmd/runtime',
        './internal/runtime',
    ],
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
