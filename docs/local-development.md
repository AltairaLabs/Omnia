# Local Development Guide

This guide walks you through setting up a local Kubernetes environment for developing and testing Omnia.

## Prerequisites

Ensure you have the following tools installed:

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.21+ | Building the operator and agent |
| Docker | 20.10+ | Container runtime |
| kubectl | 1.28+ | Kubernetes CLI |
| kind | 0.20+ | Local Kubernetes cluster |
| Helm | 3.12+ | Package manager for Kubernetes |

### Installation Commands

**macOS (Homebrew):**
```bash
brew install go docker kubectl kind helm
```

**Linux:**
```bash
# Go
wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# kubectl
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl

# kind
curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.20.0/kind-linux-amd64
chmod +x ./kind
sudo mv ./kind /usr/local/bin/kind

# Helm
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
```

### Verify Installation

```bash
go version          # go version go1.21.x ...
docker --version    # Docker version 20.10.x ...
kubectl version     # Client Version: v1.28.x ...
kind version        # kind v0.20.x ...
helm version        # version.BuildInfo{Version:"v3.12.x" ...}
```

## Create a Local Kubernetes Cluster

### Option 1: kind (Recommended)

Create a kind cluster with port forwarding for the agent WebSocket facade:

```bash
cat <<EOF | kind create cluster --name omnia-dev --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 30080
    hostPort: 8080
    protocol: TCP
EOF
```

Verify the cluster is running:
```bash
kubectl cluster-info --context kind-omnia-dev
kubectl get nodes
```

### Option 2: minikube

```bash
minikube start --profile omnia-dev --cpus 2 --memory 4096
kubectl config use-context omnia-dev
```

For minikube, you'll use `minikube service` or `minikube tunnel` to access services.

## Deploy Redis (Session Store)

The AgentRuntime supports Redis for distributed session storage. For local development, deploy a simple Redis instance:

```bash
# Create the cache namespace
kubectl create namespace cache

# Deploy Redis
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  namespace: cache
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        ports:
        - containerPort: 6379
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 200m
            memory: 128Mi
---
apiVersion: v1
kind: Service
metadata:
  name: redis
  namespace: cache
spec:
  selector:
    app: redis
  ports:
  - port: 6379
    targetPort: 6379
EOF
```

Verify Redis is running:
```bash
kubectl get pods -n cache
kubectl get svc -n cache
```

## Build and Load Images

Build the operator and agent images locally:

```bash
# Build the operator image
make docker-build IMG=omnia:dev

# Build the agent image
docker build -t omnia-agent:dev -f Dockerfile.agent .

# Load images into kind
kind load docker-image omnia:dev --name omnia-dev
kind load docker-image omnia-agent:dev --name omnia-dev
```

## Deploy the Operator

### Install CRDs

```bash
make install
```

This installs the Custom Resource Definitions (AgentRuntime, PromptPack, ToolRegistry) into your cluster.

### Deploy via Helm

```bash
helm install omnia ./charts/omnia \
  --namespace omnia-system \
  --create-namespace \
  --set image.repository=omnia \
  --set image.tag=dev \
  --set image.pullPolicy=Never \
  --set agent.image.repository=omnia-agent \
  --set agent.image.tag=dev
```

Verify the operator is running:
```bash
kubectl get pods -n omnia-system
kubectl logs -n omnia-system -l app.kubernetes.io/name=omnia -f
```

## Deploy a Sample Agent

Create a namespace for your agent:
```bash
kubectl create namespace agents
```

### 1. Create the PromptPack ConfigMap

Create a ConfigMap with compiled PromptPack JSON (use `packc` to compile from YAML source):

```bash
kubectl apply -n agents -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: demo-prompts
data:
  promptpack.json: |
    {
      "\$schema": "https://promptpack.org/schema/v1/promptpack.schema.json",
      "id": "demo-assistant",
      "name": "Demo Assistant",
      "version": "1.0.0",
      "template_engine": {"version": "v1", "syntax": "{{variable}}"},
      "prompts": {
        "main": {
          "id": "main",
          "name": "Main Assistant",
          "version": "1.0.0",
          "system_template": "You are a helpful AI assistant for local development testing. Respond concisely and helpfully.",
          "parameters": {"temperature": 0.7, "max_tokens": 1024}
        }
      }
    }
EOF
```

### 2. Create the PromptPack

```bash
kubectl apply -n agents -f - <<EOF
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: PromptPack
metadata:
  name: demo-assistant
spec:
  source:
    type: configmap
    configMapRef:
      name: demo-prompts
  version: "1.0.0"
  rollout:
    type: immediate
EOF
```

### 3. Create Provider

Create a Secret with your API key and a Provider resource:

```bash
# Replace with your actual API key
kubectl create secret generic openai-credentials \
  -n agents \
  --from-literal=OPENAI_API_KEY="sk-your-openai-api-key"

kubectl apply -n agents -f - <<EOF
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: openai-provider
spec:
  type: openai
  model: gpt-4o
  secretRef:
    name: openai-credentials
EOF
```

### 4. Create Redis Credentials Secret

```bash
kubectl create secret generic redis-credentials \
  -n agents \
  --from-literal=url="redis://redis.cache.svc.cluster.local:6379"
```

### 5. Deploy the AgentRuntime

```bash
kubectl apply -n agents -f - <<EOF
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: demo-agent
spec:
  promptPackRef:
    name: demo-assistant
  providerRef:
    name: openai-provider
  facade:
    type: websocket
    port: 8080
  session:
    type: redis
    storeRef:
      name: redis-credentials
    ttl: "1h"
  runtime:
    replicas: 1
    resources:
      requests:
        cpu: "100m"
        memory: "128Mi"
      limits:
        cpu: "500m"
        memory: "256Mi"
EOF
```

### 6. Verify Deployment

```bash
# Check the AgentRuntime status
kubectl get agentruntime -n agents demo-agent -o yaml

# Check the created resources
kubectl get deployment,service,pods -n agents -l app.kubernetes.io/name=demo-agent

# View operator logs
kubectl logs -n omnia-system -l app.kubernetes.io/name=omnia -f
```

## Test WebSocket Connection

### Expose the Agent Service

**For kind (using NodePort):**

Update the agent service to use NodePort:
```bash
kubectl patch svc demo-agent -n agents -p '{"spec": {"type": "NodePort", "ports": [{"port": 8080, "nodePort": 30080}]}}'
```

The agent is now accessible at `ws://localhost:8080`.

**For minikube:**
```bash
minikube service demo-agent -n agents --url --profile omnia-dev
```

### Connect with websocat

Install websocat:
```bash
# macOS
brew install websocat

# Linux
cargo install websocat
# or download from https://github.com/vi/websocat/releases
```

Test the connection:
```bash
websocat "ws://localhost:8080?agent=demo-agent"
```

Send a test message (JSON format):
```json
{"type":"message","content":"Hello, how are you?"}
```

### Connect with wscat

```bash
npm install -g wscat
wscat -c "ws://localhost:8080?agent=demo-agent"
```

## Running Without a Cluster

For rapid iteration, you can run the operator locally against your cluster:

```bash
# Ensure CRDs are installed
make install

# Run the operator locally (connects to current kubectl context)
make run
```

This runs the operator outside the cluster but manages resources in your kind/minikube cluster.

## E2E Testing and Debugging

### Running E2E Tests

The E2E test suite validates the complete operator workflow:

```bash
make test-e2e
```

This creates a temporary Kind cluster, builds images, deploys the operator, and runs all tests.

### Manual E2E Debugging

When E2E tests fail or you need to debug agent behavior, use the debug helper script for a step-by-step workflow:

```bash
# Initial setup (creates cluster, builds/loads images, deploys operator)
./hack/e2e-debug.sh setup

# Deploy agents for testing
./hack/e2e-debug.sh agent       # Runtime mode agent
./hack/e2e-debug.sh demo-agent  # Demo handler for tool testing

# Test specific functionality
./hack/e2e-debug.sh test-ws     # WebSocket connectivity
./hack/e2e-debug.sh test-tool   # Tool call flow

# Debug
./hack/e2e-debug.sh logs        # View all logs
./hack/e2e-debug.sh shell       # Interactive shell in cluster

# After code changes
./hack/e2e-debug.sh rebuild     # Rebuild and reload images
./hack/e2e-debug.sh cleanup     # Clear test resources

# Full cleanup
./hack/e2e-debug.sh teardown    # Delete cluster
```

### Debugging Workflow Example

When an E2E test fails:

1. **Set up the environment:**
   ```bash
   ./hack/e2e-debug.sh setup
   ```

2. **Deploy the failing test's agent:**
   ```bash
   ./hack/e2e-debug.sh demo-agent
   ```

3. **Check the deployed resources:**
   ```bash
   kubectl get pods -n test-agents -o wide
   kubectl describe agentruntime -n test-agents tool-test-agent
   ```

4. **View logs for errors:**
   ```bash
   ./hack/e2e-debug.sh logs
   # Or watch specific container logs:
   kubectl logs -n test-agents -l app.kubernetes.io/instance=tool-test-agent -c facade -f
   kubectl logs -n test-agents -l app.kubernetes.io/instance=tool-test-agent -c runtime -f
   ```

5. **Run the specific test manually:**
   ```bash
   ./hack/e2e-debug.sh test-tool
   ```

6. **Get a shell for deeper debugging:**
   ```bash
   ./hack/e2e-debug.sh shell
   # Inside the shell, you can curl services, check DNS, etc.
   curl -v http://tool-test-agent.test-agents.svc.cluster.local:8080/health
   ```

7. **After fixing code, rebuild and retry:**
   ```bash
   ./hack/e2e-debug.sh rebuild
   ./hack/e2e-debug.sh cleanup
   ./hack/e2e-debug.sh demo-agent
   ./hack/e2e-debug.sh test-tool
   ```

## Cleanup

### Delete Sample Resources

```bash
kubectl delete namespace agents
kubectl delete namespace cache
```

### Uninstall Operator

```bash
helm uninstall omnia -n omnia-system
kubectl delete namespace omnia-system
make uninstall  # Remove CRDs
```

### Delete Cluster

**kind:**
```bash
kind delete cluster --name omnia-dev
```

**minikube:**
```bash
minikube delete --profile omnia-dev
```

## Troubleshooting

### Common Issues

#### Operator pod is not starting

Check the pod status and logs:
```bash
kubectl describe pod -n omnia-system -l app.kubernetes.io/name=omnia
kubectl logs -n omnia-system -l app.kubernetes.io/name=omnia
```

Common causes:
- Image not loaded into kind: Run `kind load docker-image omnia:dev --name omnia-dev`
- Missing RBAC permissions: Check if CRDs are installed with `kubectl get crds | grep omnia`

#### AgentRuntime stuck in Pending

Check the operator logs for errors:
```bash
kubectl logs -n omnia-system -l app.kubernetes.io/name=omnia | grep -i error
```

Common causes:
- PromptPack not found: Ensure the PromptPack exists in the same namespace
- ConfigMap missing: Verify the referenced ConfigMap exists
- Secret missing: Check provider and Redis secrets exist

#### Cannot connect to WebSocket

```bash
# Verify the service exists
kubectl get svc -n agents demo-agent

# Check the pod is running
kubectl get pods -n agents -l app.kubernetes.io/name=demo-agent

# Check pod logs
kubectl logs -n agents -l app.kubernetes.io/name=demo-agent

# Verify port forwarding (alternative to NodePort)
kubectl port-forward svc/demo-agent -n agents 8080:8080
```

#### Redis connection failures

Verify Redis is accessible:
```bash
# Check Redis pod
kubectl get pods -n cache

# Test connectivity from agent namespace
kubectl run redis-test --rm -it --image=redis:7-alpine -n agents -- redis-cli -h redis.cache.svc.cluster.local ping
```

#### PromptPack status shows Failed

Check the condition details:
```bash
kubectl get promptpack -n agents demo-assistant -o jsonpath='{.status.conditions}'
```

Common causes:
- ConfigMap not found or empty
- Invalid source configuration

### Debug Commands

```bash
# View all Omnia resources
kubectl get agentruntime,promptpack,toolregistry -A

# Describe a specific resource
kubectl describe agentruntime demo-agent -n agents

# Watch operator logs in real-time
kubectl logs -n omnia-system -l app.kubernetes.io/name=omnia -f

# Check events for issues
kubectl get events -n agents --sort-by='.lastTimestamp'

# Exec into the agent pod for debugging
kubectl exec -it -n agents $(kubectl get pods -n agents -l app.kubernetes.io/name=demo-agent -o jsonpath='{.items[0].metadata.name}') -- /bin/sh
```

### Getting Help

If you encounter issues not covered here:
1. Check the [GitHub Issues](https://github.com/altairalabs/omnia/issues)
2. Review operator logs for detailed error messages
3. Open a new issue with reproduction steps
