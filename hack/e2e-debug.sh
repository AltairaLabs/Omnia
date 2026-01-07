#!/bin/bash
# E2E Debug Helper - Step through E2E tests manually with ability to inspect/debug
#
# Usage:
#   ./hack/e2e-debug.sh setup       # Create cluster and deploy operator
#   ./hack/e2e-debug.sh agent       # Deploy a test agent
#   ./hack/e2e-debug.sh demo-agent  # Deploy demo handler agent for tool call testing
#   ./hack/e2e-debug.sh test-ws     # Test WebSocket connection
#   ./hack/e2e-debug.sh test-tool   # Test tool call flow
#   ./hack/e2e-debug.sh logs        # Show all relevant logs
#   ./hack/e2e-debug.sh shell       # Get a shell in the cluster
#   ./hack/e2e-debug.sh cleanup     # Delete test resources (keep cluster)
#   ./hack/e2e-debug.sh teardown    # Delete everything including cluster

set -e

CLUSTER_NAME="${KIND_CLUSTER:-omnia-e2e-debug}"
NAMESPACE="test-agents"
OPERATOR_NS="omnia-system"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log() { echo -e "${BLUE}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }
success() { echo -e "${GREEN}[OK]${NC} $1"; }

wait_for_pod() {
    local label=$1
    local ns=$2
    local timeout=${3:-120}
    log "Waiting for pod with label $label in $ns..."
    kubectl wait --for=condition=Ready pod -l "$label" -n "$ns" --timeout="${timeout}s" || {
        error "Pod not ready. Current status:"
        kubectl get pods -n "$ns" -l "$label" -o wide
        kubectl describe pods -n "$ns" -l "$label"
        return 1
    }
    success "Pod ready"
}

cmd_setup() {
    log "Setting up E2E debug environment..."

    # Check if cluster exists
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        log "Cluster $CLUSTER_NAME already exists"
    else
        log "Creating Kind cluster: $CLUSTER_NAME"
        kind create cluster --name "$CLUSTER_NAME"
    fi

    # Use the cluster
    kubectl cluster-info --context "kind-${CLUSTER_NAME}" || {
        error "Failed to connect to cluster"
        exit 1
    }

    # Build images
    log "Building images..."
    make docker-build IMG=example.com/omnia:dev
    docker build -t example.com/omnia-facade:dev -f Dockerfile.agent .
    docker build -t example.com/omnia-runtime:dev -f Dockerfile.runtime .

    # Load into Kind
    log "Loading images into Kind..."
    kind load docker-image example.com/omnia:dev --name "$CLUSTER_NAME"
    kind load docker-image example.com/omnia-facade:dev --name "$CLUSTER_NAME"
    kind load docker-image example.com/omnia-runtime:dev --name "$CLUSTER_NAME"

    # Install CRDs and deploy operator
    log "Installing CRDs..."
    make install

    log "Deploying operator..."
    make deploy IMG=example.com/omnia:dev

    # Patch with dev images
    kubectl patch deployment omnia-controller-manager -n "$OPERATOR_NS" --type=json -p '[
        {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--facade-image=example.com/omnia-facade:dev"},
        {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--runtime-image=example.com/omnia-runtime:dev"}
    ]'

    kubectl rollout status deployment/omnia-controller-manager -n "$OPERATOR_NS" --timeout=60s

    # Create test namespace
    kubectl create ns "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

    success "Setup complete! Operator running in $OPERATOR_NS"
    echo ""
    log "Next steps:"
    echo "  ./hack/e2e-debug.sh agent       # Deploy test agent"
    echo "  ./hack/e2e-debug.sh demo-agent  # Deploy demo handler agent"
    echo "  ./hack/e2e-debug.sh logs        # View logs"
}

cmd_agent() {
    log "Deploying test agent with runtime mode..."

    # Create prerequisites
    kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-prompts
  namespace: $NAMESPACE
data:
  system: |
    You are a test assistant.
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: PromptPack
metadata:
  name: test-prompts
  namespace: $NAMESPACE
spec:
  version: "1.0.0"
  source:
    type: configmap
    configMapRef:
      name: test-prompts
  rollout:
    type: immediate
---
apiVersion: v1
kind: Secret
metadata:
  name: test-provider
  namespace: $NAMESPACE
type: Opaque
stringData:
  api-key: "test-api-key"
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: test-agent
  namespace: $NAMESPACE
  annotations:
    omnia.altairalabs.ai/mock-provider: "true"
spec:
  promptPackRef:
    name: test-prompts
  facade:
    type: websocket
    port: 8080
  session:
    type: memory
    ttl: "1h"
  runtime:
    replicas: 1
  provider:
    secretRef:
      name: test-provider
EOF

    wait_for_pod "app.kubernetes.io/instance=test-agent" "$NAMESPACE" 180

    success "Test agent deployed!"
    echo ""
    kubectl get pods -n "$NAMESPACE" -l app.kubernetes.io/instance=test-agent
    echo ""
    log "Check environment:"
    echo "  kubectl get pods -n $NAMESPACE -o wide"
    echo "  kubectl logs -n $NAMESPACE -l app.kubernetes.io/instance=test-agent -c facade"
    echo "  kubectl logs -n $NAMESPACE -l app.kubernetes.io/instance=test-agent -c runtime"
}

cmd_demo_agent() {
    log "Deploying demo handler agent for tool call testing..."

    # Create prerequisites if not exist
    kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-prompts
  namespace: $NAMESPACE
data:
  system: |
    You are a test assistant.
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: PromptPack
metadata:
  name: test-prompts
  namespace: $NAMESPACE
spec:
  version: "1.0.0"
  source:
    type: configmap
    configMapRef:
      name: test-prompts
  rollout:
    type: immediate
---
apiVersion: v1
kind: Secret
metadata:
  name: test-provider
  namespace: $NAMESPACE
type: Opaque
stringData:
  api-key: "test-api-key"
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: tool-test-agent
  namespace: $NAMESPACE
spec:
  promptPackRef:
    name: test-prompts
  facade:
    type: websocket
    port: 8080
    handler: demo
  session:
    type: memory
    ttl: "1h"
  runtime:
    replicas: 1
  provider:
    secretRef:
      name: test-provider
EOF

    wait_for_pod "app.kubernetes.io/instance=tool-test-agent" "$NAMESPACE" 180

    # Verify handler mode
    log "Verifying handler mode..."
    HANDLER_MODE=$(kubectl get pods -n "$NAMESPACE" -l app.kubernetes.io/instance=tool-test-agent \
        -o jsonpath='{.items[0].spec.containers[?(@.name=="facade")].env[?(@.name=="OMNIA_HANDLER_MODE")].value}')

    if [ "$HANDLER_MODE" = "demo" ]; then
        success "Handler mode correctly set to 'demo'"
    else
        error "Handler mode is '$HANDLER_MODE', expected 'demo'"
        exit 1
    fi

    success "Demo agent deployed!"
    echo ""
    kubectl get pods -n "$NAMESPACE" -l app.kubernetes.io/instance=tool-test-agent
    echo ""
    log "Test with:"
    echo "  ./hack/e2e-debug.sh test-tool"
}

cmd_test_ws() {
    log "Testing WebSocket connection..."

    local agent=${1:-test-agent}

    # Check service exists
    kubectl get svc "$agent" -n "$NAMESPACE" || {
        error "Service $agent not found. Deploy an agent first."
        exit 1
    }

    log "Creating WebSocket test pod..."
    kubectl run ws-test --rm -i --restart=Never \
        --namespace="$NAMESPACE" \
        --image=curlimages/curl:latest \
        -- sh -c "curl -v -N -H 'Connection: Upgrade' -H 'Upgrade: websocket' -H 'Sec-WebSocket-Key: dGVzdA==' -H 'Sec-WebSocket-Version: 13' http://${agent}.${NAMESPACE}.svc.cluster.local:8080/ws?agent=${agent}"
}

cmd_test_tool() {
    log "Testing tool call flow with demo handler..."

    # Check demo agent exists
    kubectl get pods -n "$NAMESPACE" -l app.kubernetes.io/instance=tool-test-agent -o name | grep -q . || {
        error "Demo agent not found. Run: ./hack/e2e-debug.sh demo-agent"
        exit 1
    }

    log "Creating Python test pod..."
    kubectl apply -f - <<'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: tool-call-test
  namespace: test-agents
spec:
  restartPolicy: Never
  containers:
  - name: python
    image: python:3.11-slim
    command: ["sh", "-c"]
    args:
    - |
      pip install websockets --quiet
      python3 << 'PYTHON_SCRIPT'
      import asyncio
      import json
      import websockets
      import sys

      async def test_tool_calls():
          uri = "ws://tool-test-agent.test-agents.svc.cluster.local:8080/ws?agent=tool-test-agent"
          try:
              print(f"Connecting to {uri}...")
              async with websockets.connect(uri, ping_interval=None) as ws:
                  weather_message = {"type": "message", "content": "What's the weather like?"}
                  await ws.send(json.dumps(weather_message))
                  print(f"Sent: {weather_message}")

                  received_types = []
                  received_tool_call = False
                  received_tool_result = False
                  tool_call_name = ""

                  for _ in range(20):
                      try:
                          response = await asyncio.wait_for(ws.recv(), timeout=30)
                          msg = json.loads(response)
                          msg_type = msg.get("type")
                          received_types.append(msg_type)
                          print(f"Received: {msg_type} - {json.dumps(msg)[:200]}")

                          if msg_type == "tool_call":
                              received_tool_call = True
                              tool_call_name = msg.get("tool_call", {}).get("name", "")
                          elif msg_type == "tool_result":
                              received_tool_result = True
                          elif msg_type == "done":
                              print("Conversation complete")
                              break
                          elif msg_type == "error":
                              print(f"ERROR: {msg.get('error')}")
                              sys.exit(1)
                      except asyncio.TimeoutError:
                          print("Timeout waiting for messages")
                          break

                  print(f"\nMessage types received: {received_types}")

                  if received_tool_call and received_tool_result and tool_call_name == "weather":
                      print("\n✓ TEST PASSED: Tool call flow verified")
                  else:
                      print("\n✗ TEST FAILED")
                      if not received_tool_call: print("  - Missing tool_call")
                      if not received_tool_result: print("  - Missing tool_result")
                      if tool_call_name != "weather": print(f"  - Wrong tool: {tool_call_name}")
                      sys.exit(1)

          except Exception as e:
              print(f"ERROR: {e}")
              import traceback
              traceback.print_exc()
              sys.exit(1)

      asyncio.run(test_tool_calls())
      PYTHON_SCRIPT
EOF

    log "Waiting for test to complete..."
    sleep 5

    for i in {1..30}; do
        STATUS=$(kubectl get pod tool-call-test -n "$NAMESPACE" -o jsonpath='{.status.phase}' 2>/dev/null || echo "Pending")
        if [ "$STATUS" = "Succeeded" ]; then
            success "Test passed!"
            kubectl logs tool-call-test -n "$NAMESPACE"
            kubectl delete pod tool-call-test -n "$NAMESPACE" --ignore-not-found
            return 0
        elif [ "$STATUS" = "Failed" ]; then
            error "Test failed!"
            kubectl logs tool-call-test -n "$NAMESPACE"
            echo ""
            warn "Pod kept for debugging. Delete with: kubectl delete pod tool-call-test -n $NAMESPACE"
            return 1
        fi
        echo -n "."
        sleep 2
    done

    warn "Test timed out. Checking logs..."
    kubectl logs tool-call-test -n "$NAMESPACE"
    echo ""
    warn "Pod kept for debugging. Check with:"
    echo "  kubectl describe pod tool-call-test -n $NAMESPACE"
    echo "  kubectl logs tool-call-test -n $NAMESPACE"
}

cmd_logs() {
    log "Fetching logs..."

    echo ""
    echo "=== Operator Logs ==="
    kubectl logs -n "$OPERATOR_NS" -l control-plane=controller-manager --tail=50 2>/dev/null || warn "No operator logs"

    echo ""
    echo "=== Agent Pods ==="
    kubectl get pods -n "$NAMESPACE" -o wide 2>/dev/null || warn "No pods in $NAMESPACE"

    for pod in $(kubectl get pods -n "$NAMESPACE" -o name 2>/dev/null); do
        echo ""
        echo "=== $pod ==="
        kubectl logs -n "$NAMESPACE" "$pod" --all-containers --tail=30 2>/dev/null || true
    done
}

cmd_shell() {
    log "Starting debug shell in cluster..."
    kubectl run debug-shell --rm -i --tty \
        --namespace="$NAMESPACE" \
        --image=nicolaka/netshoot \
        -- /bin/bash
}

cmd_cleanup() {
    log "Cleaning up test resources (keeping cluster)..."
    kubectl delete agentruntime --all -n "$NAMESPACE" --ignore-not-found
    kubectl delete promptpack --all -n "$NAMESPACE" --ignore-not-found
    kubectl delete toolregistry --all -n "$NAMESPACE" --ignore-not-found
    kubectl delete pod --all -n "$NAMESPACE" --ignore-not-found
    kubectl delete configmap --all -n "$NAMESPACE" --ignore-not-found
    kubectl delete secret --all -n "$NAMESPACE" --ignore-not-found
    success "Test resources cleaned up. Cluster and operator still running."
}

cmd_teardown() {
    log "Tearing down everything..."
    kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
    success "Cluster deleted"
}

cmd_rebuild() {
    log "Rebuilding and reloading images..."

    make docker-build IMG=example.com/omnia:dev
    docker build -t example.com/omnia-facade:dev -f Dockerfile.agent .
    docker build -t example.com/omnia-runtime:dev -f Dockerfile.runtime .

    kind load docker-image example.com/omnia:dev --name "$CLUSTER_NAME"
    kind load docker-image example.com/omnia-facade:dev --name "$CLUSTER_NAME"
    kind load docker-image example.com/omnia-runtime:dev --name "$CLUSTER_NAME"

    log "Restarting operator..."
    kubectl rollout restart deployment/omnia-controller-manager -n "$OPERATOR_NS"
    kubectl rollout status deployment/omnia-controller-manager -n "$OPERATOR_NS" --timeout=60s

    success "Images rebuilt and loaded. Delete and recreate agents to use new images."
}

cmd_help() {
    cat <<EOF
E2E Debug Helper - Step through E2E tests manually

Usage: $0 <command>

Commands:
  setup       Create Kind cluster, build images, deploy operator
  rebuild     Rebuild images and reload into cluster
  agent       Deploy a test agent (runtime mode)
  demo-agent  Deploy demo handler agent (for tool call testing)
  test-ws     Test WebSocket connection to an agent
  test-tool   Run the tool call flow test
  logs        Show logs from operator and agents
  shell       Get an interactive shell in the cluster
  cleanup     Delete test resources (keep cluster/operator)
  teardown    Delete everything including the cluster

Workflow:
  1. ./hack/e2e-debug.sh setup       # One-time setup
  2. ./hack/e2e-debug.sh demo-agent  # Deploy agent to test
  3. ./hack/e2e-debug.sh test-tool   # Run test
  4. ./hack/e2e-debug.sh logs        # Debug if needed
  5. ./hack/e2e-debug.sh cleanup     # Clean up for next test

After code changes:
  ./hack/e2e-debug.sh rebuild        # Rebuild and reload images

EOF
}

# Main
case "${1:-help}" in
    setup)      cmd_setup ;;
    rebuild)    cmd_rebuild ;;
    agent)      cmd_agent ;;
    demo-agent) cmd_demo_agent ;;
    test-ws)    cmd_test_ws "${2:-test-agent}" ;;
    test-tool)  cmd_test_tool ;;
    logs)       cmd_logs ;;
    shell)      cmd_shell ;;
    cleanup)    cmd_cleanup ;;
    teardown)   cmd_teardown ;;
    help|*)     cmd_help ;;
esac
