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

# Allow deployment to local clusters only (safety check)
allow_k8s_contexts(['kind-omnia-dev', 'docker-desktop', 'minikube', 'kind-kind'])

# Create namespace if it doesn't exist
namespace_create('omnia-system')

# ============================================================================
# Helm Repositories (required for subcharts)
# ============================================================================

if ENABLE_OBSERVABILITY:
    helm_repo('prometheus-community', 'https://prometheus-community.github.io/helm-charts')
    helm_repo('grafana', 'https://grafana.github.io/helm-charts')

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
    # Use dev images for agent containers (facade + runtime)
    'facade.image.repository=omnia-facade-dev',
    'facade.image.tag=latest',
    'runtime.image.repository=omnia-runtime-dev',
    'runtime.image.tag=latest',
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
        # Disable Loki/Tempo for simpler setup
        'loki.enabled=false',
        'tempo.enabled=false',
        'alloy.enabled=false',
    ])
else:
    helm_set.extend([
        # Disable observability
        'prometheus.enabled=false',
        'grafana.enabled=false',
        'loki.enabled=false',
    ])

# Deploy the Helm chart with development images
k8s_yaml(helm(
    './charts/omnia',
    name='omnia',
    namespace='omnia-system',
    values=['./charts/omnia/values-dev.yaml'],
    set=helm_set,
))

# ============================================================================
# Resource Configuration
# ============================================================================

# Group resources for better UI organization
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

# ============================================================================
# Sample Resources for Development
# ============================================================================

# Apply sample resources using local_resource for better control
local_resource(
    'sample-resources',
    cmd='kubectl apply -f config/samples/dev/',
    deps=['config/samples/dev'],
    labels=['samples'],
    resource_deps=['omnia-controller-manager'],
)

# Restart agent pods when facade/runtime images are rebuilt
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
