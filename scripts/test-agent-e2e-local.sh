#!/bin/bash
# Local E2E test script for agent flow with Ollama
# Usage: ./scripts/test-agent-e2e-local.sh

set -euo pipefail

# Configuration
KIND_CLUSTER=${KIND_CLUSTER:-agent-e2e-test}
OLLAMA_MODEL=${OLLAMA_MODEL:-qwen2:0.5b}
IMAGE_TAG=${IMAGE_TAG:-e2e-test}
HELM_TIMEOUT=${HELM_TIMEOUT:-10m}

echo "=== Agent E2E Local Test ==="
echo "KIND_CLUSTER: $KIND_CLUSTER"
echo "OLLAMA_MODEL: $OLLAMA_MODEL"
echo "IMAGE_TAG: $IMAGE_TAG"
echo ""

# Check prerequisites
echo "Checking prerequisites..."
command -v docker >/dev/null 2>&1 || { echo "Error: docker is required"; exit 1; }
command -v kind >/dev/null 2>&1 || { echo "Error: kind is required"; exit 1; }
command -v helm >/dev/null 2>&1 || { echo "Error: helm is required"; exit 1; }
command -v kubectl >/dev/null 2>&1 || { echo "Error: kubectl is required"; exit 1; }

# Clean up any existing cluster
if kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER}$"; then
  echo "Deleting existing kind cluster..."
  kind delete cluster --name $KIND_CLUSTER
fi

# Build Docker images
echo ""
echo "=== Building Docker images ==="

echo "Building omnia (operator) image..."
docker build -t ghcr.io/altairalabs/omnia:$IMAGE_TAG -f Dockerfile .

echo "Building omnia-facade image..."
docker build -t ghcr.io/altairalabs/omnia-facade:$IMAGE_TAG -f Dockerfile.agent .

echo "Building omnia-runtime image..."
docker build -t ghcr.io/altairalabs/omnia-runtime:$IMAGE_TAG -f Dockerfile.runtime .

echo "All images built successfully"
docker images | grep altairalabs | head -5

# Create kind cluster
echo ""
echo "=== Creating kind cluster ==="
cat <<EOF | kind create cluster --name $KIND_CLUSTER --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 30080
    hostPort: 30080
    protocol: TCP
EOF
kubectl cluster-info --context kind-$KIND_CLUSTER

# Load images into kind
echo ""
echo "=== Loading images into kind cluster ==="
kind load docker-image ghcr.io/altairalabs/omnia:$IMAGE_TAG --name $KIND_CLUSTER
kind load docker-image ghcr.io/altairalabs/omnia-facade:$IMAGE_TAG --name $KIND_CLUSTER
kind load docker-image ghcr.io/altairalabs/omnia-runtime:$IMAGE_TAG --name $KIND_CLUSTER
echo "Images loaded into kind"

# Install cert-manager
echo ""
echo "=== Installing cert-manager ==="
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.4/cert-manager.yaml
echo "Waiting for cert-manager to be ready..."
kubectl wait --for=condition=Available deployment/cert-manager -n cert-manager --timeout=120s
kubectl wait --for=condition=Available deployment/cert-manager-webhook -n cert-manager --timeout=120s
kubectl wait --for=condition=Available deployment/cert-manager-cainjector -n cert-manager --timeout=120s

# Install Gateway API CRDs
echo ""
echo "=== Installing Gateway API CRDs ==="
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.0/standard-install.yaml

# Add Helm repositories
echo ""
echo "=== Adding Helm repositories ==="
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts 2>/dev/null || true
helm repo add grafana https://grafana.github.io/helm-charts 2>/dev/null || true
helm repo add kedacore https://kedacore.github.io/charts 2>/dev/null || true
helm repo update

# Build chart dependencies
echo ""
echo "=== Building chart dependencies ==="
helm dependency build ./charts/omnia

# Deploy Omnia
echo ""
echo "=== Deploying Omnia with demo mode (Ollama) ==="
helm install omnia ./charts/omnia \
  --namespace omnia-system \
  --create-namespace \
  --set image.tag=$IMAGE_TAG \
  --set image.pullPolicy=Never \
  --set facade.image.tag=$IMAGE_TAG \
  --set facade.image.pullPolicy=Never \
  --set framework.image.tag=$IMAGE_TAG \
  --set framework.image.pullPolicy=Never \
  --set dashboard.enabled=false \
  --set demo.enabled=true \
  --set demo.ollama.model=$OLLAMA_MODEL \
  --set demo.ollama.resources.requests.memory="2Gi" \
  --set demo.ollama.resources.limits.memory="4Gi" \
  --set demo.ollama.persistence.enabled=false \
  --set keda.enabled=false \
  --timeout $HELM_TIMEOUT

echo "Helm install complete"

# Wait for operator
echo ""
echo "=== Waiting for operator to be ready ==="
kubectl wait --for=condition=Available deployment/omnia-controller-manager \
  -n omnia-system --timeout=120s || {
  echo "Operator not ready, checking status..."
  kubectl get pods -n omnia-system
  kubectl describe deployment omnia-controller-manager -n omnia-system
  exit 1
}
echo "Operator is ready"

# Wait for Ollama
echo ""
echo "=== Waiting for Ollama pod to be ready ==="
kubectl wait --for=condition=Ready pod/ollama-0 \
  -n omnia-demo --timeout=300s || {
  echo "Ollama pod not ready, checking status..."
  kubectl get pods -n omnia-demo
  kubectl describe pod ollama-0 -n omnia-demo || true
  exit 1
}
echo "Ollama pod ready"

# Wait for model pull
echo ""
echo "=== Waiting for model pull job to complete ==="
kubectl wait --for=condition=Complete job -l app.kubernetes.io/name=ollama-pull-model \
  -n omnia-demo --timeout=600s || {
  echo "Model pull job not complete, checking status..."
  kubectl get jobs -n omnia-demo
  kubectl logs -n omnia-demo -l job-name --tail=50 || true
}

# Verify model is available
echo "Checking available models..."
kubectl exec -n omnia-demo ollama-0 -- ollama list || true

# Wait for AgentRuntime
echo ""
echo "=== Waiting for demo agent to be ready ==="

# Check operator logs
echo "Checking operator logs..."
kubectl logs -n omnia-system -l app.kubernetes.io/name=omnia --tail=20 || true

# Wait for AgentRuntime to be reconciled
echo "Waiting for AgentRuntime to be reconciled..."
for i in {1..60}; do
  STATUS=$(kubectl get agentruntime vision-demo -n omnia-demo -o jsonpath='{.status.phase}' 2>/dev/null || echo "NotFound")
  echo "AgentRuntime status: $STATUS ($i/60)"
  if [ "$STATUS" == "Running" ]; then
    echo "Agent is running!"
    break
  fi
  if [ "$STATUS" == "Failed" ]; then
    echo "AgentRuntime failed!"
    kubectl describe agentruntime vision-demo -n omnia-demo
    exit 1
  fi
  sleep 10
done

# Show all resources
echo ""
echo "=== Resources in omnia-demo ==="
kubectl get all -n omnia-demo

# Check agent pod
echo ""
echo "=== Verifying agent pod ==="
# Use app.kubernetes.io/instance selector - the operator sets name=omnia-agent, instance=<agent-name>
POD_STATUS=$(kubectl get pods -n omnia-demo -l app.kubernetes.io/instance=vision-demo -o jsonpath='{.items[0].status.phase}' 2>/dev/null || echo "NotFound")
echo "Pod status: $POD_STATUS"

if [ "$POD_STATUS" != "Running" ]; then
  echo "Pod not running! Current status: $POD_STATUS"
  kubectl get pods -n omnia-demo -o wide
  kubectl describe pods -n omnia-demo -l app.kubernetes.io/instance=vision-demo || true
  exit 1
fi

echo ""
echo "=== E2E Test Setup Complete ==="
echo ""
echo "To test the agent manually:"
echo "  kubectl port-forward -n omnia-demo svc/vision-demo 8080:8080"
echo ""
echo "Then in another terminal (requires websocat):"
echo "  echo '{\"type\":\"message\",\"content\":\"Hello!\"}' | websocat ws://localhost:8080/ws"
echo ""
echo "To clean up:"
echo "  kind delete cluster --name $KIND_CLUSTER"
