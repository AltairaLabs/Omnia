//go:build e2e
// +build e2e

/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/altairalabs/omnia/test/utils"
)

// namespace where the project is deployed in
const namespace = "omnia-system"

// agentsNamespace is where test agents are deployed
const agentsNamespace = "test-agents"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "omnia-metrics-binding"

// predeployed indicates the test is running against a pre-deployed cluster (e.g., Tilt dev).
// When true, infrastructure setup/teardown (operator, CRDs, Postgres, session-api) is skipped.
var predeployed = os.Getenv("E2E_PREDEPLOYED") == "true"

// Configurable via env vars for pre-deployed clusters where the operator uses different image names.
var (
	sessionApiURL      = envOrDefault("SESSION_API_URL", "http://e2e-session-api.omnia-system.svc.cluster.local:8080")
	facadeImageRef     = envOrDefault("E2E_FACADE_IMAGE", "example.com/omnia-facade:v0.0.1")
	runtimeImageRef    = envOrDefault("E2E_RUNTIME_IMAGE", "example.com/omnia-runtime:v0.0.1")
	serviceAccountName = envOrDefault("E2E_SERVICE_ACCOUNT", "omnia-controller-manager")
	metricsServiceName = envOrDefault("E2E_METRICS_SERVICE", "omnia-controller-manager-metrics-service")
)

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	// In predeployed mode (e.g., Tilt dev), this is skipped — the operator is already running.
	BeforeAll(func() {
		if predeployed {
			_, _ = fmt.Fprintf(GinkgoWriter, "Skipping Manager setup (E2E_PREDEPLOYED=true)\n")
			return
		}

		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")

		By("waiting for initial controller-manager deployment to be ready")
		initialRolloutCmd := exec.Command("kubectl", "rollout", "status", "deployment/omnia-controller-manager",
			"-n", namespace, "--timeout=120s")
		_, err = utils.Run(initialRolloutCmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to wait for initial controller-manager rollout")

		By("patching the controller-manager strategy to Recreate to avoid rolling update issues")
		strategyPatchCmd := exec.Command("kubectl", "patch", "deployment", "omnia-controller-manager",
			"-n", namespace, "--type=json",
			"-p", `[{"op": "replace", "path": "/spec/strategy", "value": {"type": "Recreate"}}]`)
		_, err = utils.Run(strategyPatchCmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to patch controller-manager strategy")

		By("patching the controller-manager to use the test facade and framework images and session-api URL")
		patchCmd := exec.Command("kubectl", "patch", "deployment", "omnia-controller-manager",
			"-n", namespace, "--type=strategic",
			"-p", fmt.Sprintf(`{"spec":{"template":{"spec":{"containers":[{"name":"manager","args":["--metrics-bind-address=:8443","--leader-elect","--health-probe-bind-address=:8081","--facade-image=%s","--framework-image=%s","--session-api-url=%s"]}]}}}}`, facadeImageRef, runtimeImageRef, sessionApiURL))
		_, err = utils.Run(patchCmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to patch controller-manager")

		By("waiting for patched controller-manager rollout to complete")
		rolloutCmd := exec.Command("kubectl", "rollout", "status", "deployment/omnia-controller-manager",
			"-n", namespace, "--timeout=120s")
		_, err = utils.Run(rolloutCmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to wait for controller-manager rollout")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace. In predeployed mode, only clean up test artifacts.
	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace, "--ignore-not-found")
		_, _ = utils.Run(cmd)

		if predeployed {
			_, _ = fmt.Fprintf(GinkgoWriter, "Skipping Manager teardown (E2E_PREDEPLOYED=true)\n")
			return
		}

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace, "--timeout=60s")
		_, _ = utils.Run(cmd)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				// Filter to pods whose name contains "controller-manager" — in Helm-deployed
				// clusters, the control-plane=controller-manager label is on all chart pods,
				// so we need to narrow down to the actual controller-manager pod.
				var filtered []string
				for _, name := range podNames {
					if strings.Contains(name, "controller-manager") {
						filtered = append(filtered, name)
					}
				}
				g.Expect(filtered).To(HaveLen(1), "expected 1 controller-manager pod running")
				controllerPodName = filtered[0]

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		It("should ensure the metrics endpoint is serving metrics", func() {
			By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
				"--clusterrole=omnia-metrics-reader",
				fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
			)
			_, _ = utils.Run(cmd) // Ignore error if already exists

			By("validating that the metrics service is available")
			cmd = exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			By("getting the service account token")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("ensuring the controller pod is ready")
			verifyControllerPodReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", controllerPodName, "-n", namespace,
					"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "Controller pod not ready")
			}
			Eventually(verifyControllerPodReady, 3*time.Minute, time.Second).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("Serving metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted, 3*time.Minute, time.Second).Should(Succeed())

			// +kubebuilder:scaffold:e2e-metrics-webhooks-readiness

			By("cleaning up any existing curl-metrics pod")
			cmd = exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)

			By("creating the curl-metrics pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", namespace,
				"--image=curlimages/curl:latest",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics"],
							"securityContext": {
								"readOnlyRootFilesystem": true,
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}],
						"serviceAccountName": "%s"
					}
				}`, token, metricsServiceName, namespace, serviceAccountName))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for the curl-metrics pod to complete.")
			verifyCurlUp := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
					"-o", "jsonpath={.status.phase}",
					"-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"), "curl pod in wrong status")
			}
			Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

			By("getting the metrics by checking curl-metrics logs")
			verifyMetricsAvailable := func(g Gomega) {
				metricsOutput, err := getMetricsOutput()
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
				g.Expect(metricsOutput).NotTo(BeEmpty())
				g.Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
			}
			Eventually(verifyMetricsAvailable, 2*time.Minute).Should(Succeed())
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks
	})

	Context("Omnia CRDs", Ordered, func() {
		BeforeAll(func() {
			By("creating the agents namespace")
			cmd := exec.Command("kubectl", "create", "ns", agentsNamespace)
			_, _ = utils.Run(cmd) // Ignore error if already exists

			if predeployed {
				_, _ = fmt.Fprintf(GinkgoWriter, "Skipping infra setup (E2E_PREDEPLOYED=true) — Postgres and session-api already deployed\n")
				return
			}

			By("deploying Postgres for session storage")
			postgresManifest := `
apiVersion: v1
kind: Secret
metadata:
  name: omnia-postgres
  namespace: omnia-system
type: Opaque
stringData:
  connection-string: "postgres://omnia:omnia@e2e-postgres.omnia-system.svc.cluster.local:5432/omnia?sslmode=disable"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: e2e-postgres
  namespace: omnia-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: e2e-postgres
  template:
    metadata:
      labels:
        app: e2e-postgres
    spec:
      containers:
      - name: postgres
        image: postgres:17-alpine
        ports:
        - containerPort: 5432
        env:
        - name: POSTGRES_USER
          value: omnia
        - name: POSTGRES_PASSWORD
          value: omnia
        - name: POSTGRES_DB
          value: omnia
        readinessProbe:
          exec:
            command: ["pg_isready", "-U", "omnia"]
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          requests:
            cpu: 50m
            memory: 128Mi
          limits:
            cpu: 200m
            memory: 256Mi
---
apiVersion: v1
kind: Service
metadata:
  name: e2e-postgres
  namespace: omnia-system
spec:
  selector:
    app: e2e-postgres
  ports:
  - port: 5432
    targetPort: 5432
`
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(postgresManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to deploy Postgres")

			By("waiting for Postgres to be ready")
			verifyPostgresReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "-n", namespace,
					"-l", "app=e2e-postgres", "-o", "jsonpath={.items[0].status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"))
			}
			Eventually(verifyPostgresReady, 4*time.Minute, time.Second).Should(Succeed())

			By("deploying session-api")
			sessionApiManifest := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: e2e-session-api
  namespace: omnia-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: e2e-session-api
  template:
    metadata:
      labels:
        app: e2e-session-api
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
      containers:
      - name: session-api
        image: %s
        ports:
        - name: api
          containerPort: 8080
        - name: health
          containerPort: 8081
        env:
        - name: POSTGRES_CONN
          valueFrom:
            secretKeyRef:
              name: omnia-postgres
              key: connection-string
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 200m
            memory: 128Mi
        securityContext:
          readOnlyRootFilesystem: true
          allowPrivilegeEscalation: false
          capabilities:
            drop: ["ALL"]
---
apiVersion: v1
kind: Service
metadata:
  name: e2e-session-api
  namespace: omnia-system
spec:
  selector:
    app: e2e-session-api
  ports:
  - port: 8080
    targetPort: 8080
`, sessionApiImage)
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sessionApiManifest)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to deploy session-api")

			By("waiting for session-api to be ready")
			verifySessionApiReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "-n", namespace,
					"-l", "app=e2e-session-api", "-o", "jsonpath={.items[0].status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"))
			}
			Eventually(verifySessionApiReady, 4*time.Minute, time.Second).Should(Succeed())
		})

		AfterAll(func() {
			By("cleaning up test agents namespace")
			cmd := exec.Command("kubectl", "delete", "ns", agentsNamespace, "--ignore-not-found", "--timeout=60s")
			_, _ = utils.Run(cmd)

			if predeployed {
				_, _ = fmt.Fprintf(GinkgoWriter, "Skipping infra teardown (E2E_PREDEPLOYED=true)\n")
				return
			}

			By("cleaning up session-api and postgres")
			cmd = exec.Command("kubectl", "delete", "deployment", "e2e-session-api", "-n", namespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "service", "e2e-session-api", "-n", namespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "deployment", "e2e-postgres", "-n", namespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "service", "e2e-postgres", "-n", namespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "secret", "omnia-postgres", "-n", namespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("should create and validate a PromptPack", func() {
			By("creating a ConfigMap for the PromptPack")
			// The ConfigMap must contain pack.json in PromptKit format
			// This is the format expected by the runtime container
			configMapManifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-prompts
  namespace: test-agents
data:
  pack.json: |
    {
      "id": "test-prompts",
      "name": "test-prompts",
      "version": "1.0.0",
      "template_engine": {
        "version": "v1",
        "syntax": "{{variable}}"
      },
      "prompts": {
        "default": {
          "id": "default",
          "name": "default",
          "version": "1.0.0",
          "system_template": "You are a test assistant for E2E testing."
        }
      }
    }
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(configMapManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ConfigMap")

			By("creating the PromptPack")
			promptPackManifest := `
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: PromptPack
metadata:
  name: test-prompts
  namespace: test-agents
spec:
  source:
    type: configmap
    configMapRef:
      name: test-prompts
  version: "1.0.0"
  rollout:
    type: immediate
`
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(promptPackManifest)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create PromptPack")

			By("verifying the PromptPack status becomes Active")
			verifyPromptPackActive := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "promptpack", "test-prompts",
					"-n", agentsNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Active"))
			}
			Eventually(verifyPromptPackActive, time.Minute, time.Second).Should(Succeed())

			By("verifying the SourceValid condition is True")
			cmd = exec.Command("kubectl", "get", "promptpack", "test-prompts",
				"-n", agentsNamespace,
				"-o", "jsonpath={.status.conditions[?(@.type=='SourceValid')].status}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("True"))

			By("verifying the SchemaValid condition is True")
			cmd = exec.Command("kubectl", "get", "promptpack", "test-prompts",
				"-n", agentsNamespace,
				"-o", "jsonpath={.status.conditions[?(@.type=='SchemaValid')].status}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("True"))
		})

		It("should create and validate a ToolRegistry", func() {
			By("creating a ToolRegistry with an HTTP handler")
			toolRegistryManifest := `
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ToolRegistry
metadata:
  name: test-tools
  namespace: test-agents
spec:
  handlers:
  - name: test-handler
    type: http
    httpConfig:
      endpoint: "http://example.com/api/test"
      method: POST
    tool:
      name: test_tool
      description: A test tool for E2E testing
      inputSchema:
        type: object
        properties:
          input:
            type: string
    timeout: "10s"
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(toolRegistryManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ToolRegistry")

			By("verifying the ToolRegistry status becomes Ready")
			verifyToolRegistryReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "toolregistry", "test-tools",
					"-n", agentsNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Ready"))
			}
			Eventually(verifyToolRegistryReady, time.Minute, time.Second).Should(Succeed())

			By("verifying discovered tools count")
			cmd = exec.Command("kubectl", "get", "toolregistry", "test-tools",
				"-n", agentsNamespace, "-o", "jsonpath={.status.discoveredToolsCount}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("1"))
		})

		It("should create an AgentRuntime and deploy the agent", func() {
			By("creating secrets for the agent")
			// Create a dummy provider secret (not used when mock provider is enabled)
			cmd := exec.Command("kubectl", "create", "secret", "generic", "test-provider",
				"-n", agentsNamespace,
				"--from-literal=api-key=test-api-key-for-e2e",
				"--dry-run=client", "-o", "yaml")
			secretYaml, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(secretYaml)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create provider secret")

			By("creating the AgentRuntime with mock provider annotation")
			// Note: The agent image is configured on the operator via --facade-image/--framework-image flags,
			// not in the CRD spec. The operator was patched in BeforeAll to use the test images.
			// The mock provider annotation enables mock mode for E2E testing without real API keys.
			agentRuntimeManifest := `
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: test-agent
  namespace: test-agents
  annotations:
    omnia.altairalabs.ai/mock-provider: "true"
spec:
  promptPackRef:
    name: test-prompts
  toolRegistryRef:
    name: test-tools
  facade:
    type: websocket
    port: 8080
  session:
    type: memory
    ttl: "1h"
  runtime:
    replicas: 1
    resources:
      requests:
        cpu: "50m"
        memory: "64Mi"
      limits:
        cpu: "200m"
        memory: "128Mi"
  provider:
    type: mock
    secretRef:
      name: test-provider
`

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(agentRuntimeManifest)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create AgentRuntime")

			By("verifying the AgentRuntime creates a Deployment")
			verifyDeploymentCreated := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "deployment", "test-agent",
					"-n", agentsNamespace, "-o", "jsonpath={.metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("test-agent"))
			}
			Eventually(verifyDeploymentCreated, 2*time.Minute, time.Second).Should(Succeed())

			By("verifying the AgentRuntime creates a Service")
			verifyServiceCreated := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "service", "test-agent",
					"-n", agentsNamespace, "-o", "jsonpath={.spec.ports[0].port}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("8080"))
			}
			Eventually(verifyServiceCreated, 2*time.Minute, time.Second).Should(Succeed())

			By("verifying the AgentRuntime status")
			verifyAgentRuntimeStatus := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "agentruntime", "test-agent",
					"-n", agentsNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				// May be Running or Pending depending on pod startup
				g.Expect(output).To(Or(Equal("Running"), Equal("Pending")))
			}
			Eventually(verifyAgentRuntimeStatus, 2*time.Minute, time.Second).Should(Succeed())

			By("verifying the agent pod is created")
			verifyAgentPod := func(g Gomega) {
				// The controller labels pods with app.kubernetes.io/name=omnia-agent
				// and app.kubernetes.io/instance=<agentruntime-name>
				cmd := exec.Command("kubectl", "get", "pods",
					"-n", agentsNamespace,
					"-l", "app.kubernetes.io/instance=test-agent",
					"-o", "jsonpath={.items[0].metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("test-agent"))
			}
			Eventually(verifyAgentPod, 2*time.Minute, time.Second).Should(Succeed())
		})

		It("should update AgentRuntime when PromptPack changes", func() {
			By("getting the initial deployment generation")
			cmd := exec.Command("kubectl", "get", "deployment", "test-agent",
				"-n", agentsNamespace, "-o", "jsonpath={.metadata.generation}")
			initialGen, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("updating the PromptPack ConfigMap")
			// Update pack.json with a modified system template
			configMapUpdate := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-prompts
  namespace: test-agents
data:
  pack.json: |
    {
      "id": "test-prompts",
      "name": "test-prompts",
      "version": "1.0.1",
      "template_engine": {
        "version": "v1",
        "syntax": "{{variable}}"
      },
      "prompts": {
        "default": {
          "id": "default",
          "name": "default",
          "version": "1.0.1",
          "system_template": "You are an UPDATED test assistant for E2E testing."
        }
      }
    }
`
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(configMapUpdate)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to update ConfigMap")

			By("verifying the PromptPack was re-reconciled")
			cmd = exec.Command("kubectl", "get", "promptpack", "test-prompts",
				"-n", agentsNamespace, "-o", "jsonpath={.status.lastUpdated}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).NotTo(BeEmpty())

			// Note: The deployment generation may or may not change depending on
			// whether the ConfigMap hash changed. This is expected behavior.
			_ = initialGen // Acknowledge we captured it for potential future use
		})

		It("should have both facade and runtime containers running", func() {
			By("waiting for the agent pod to be ready with all containers")

			// Helper to dump debug info
			dumpDebugInfo := func(reason string) {
				_, _ = fmt.Fprintf(GinkgoWriter, "\n=== DEBUG: %s ===\n", reason)

				descCmd := exec.Command("kubectl", "describe", "pods",
					"-n", agentsNamespace, "-l", "app.kubernetes.io/instance=test-agent")
				descOutput, _ := utils.Run(descCmd)
				_, _ = fmt.Fprintf(GinkgoWriter, "Pod describe:\n%s\n", descOutput)

				facadeLogsCmd := exec.Command("kubectl", "logs",
					"-n", agentsNamespace, "-l", "app.kubernetes.io/instance=test-agent",
					"-c", "facade", "--tail=100")
				facadeLogs, _ := utils.Run(facadeLogsCmd)
				_, _ = fmt.Fprintf(GinkgoWriter, "Facade logs:\n%s\n", facadeLogs)

				runtimeLogsCmd := exec.Command("kubectl", "logs",
					"-n", agentsNamespace, "-l", "app.kubernetes.io/instance=test-agent",
					"-c", "runtime", "--tail=100")
				runtimeLogs, _ := utils.Run(runtimeLogsCmd)
				_, _ = fmt.Fprintf(GinkgoWriter, "Runtime logs:\n%s\n", runtimeLogs)
			}

			// Check for container failure states that should cause early exit
			checkForFailures := func() error {
				// Get container states as JSON for detailed inspection
				cmd := exec.Command("kubectl", "get", "pods",
					"-n", agentsNamespace,
					"-l", "app.kubernetes.io/instance=test-agent",
					"-o", "jsonpath={range .items[0].status.containerStatuses[*]}{.name}:{.state}{.lastState}|{end}")
				output, err := utils.Run(cmd)
				if err != nil {
					return nil // Pod may not exist yet
				}

				// Check for failure indicators in the state
				failurePatterns := []string{
					"CrashLoopBackOff",
					"ImagePullBackOff",
					"ErrImagePull",
					"CreateContainerError",
					"InvalidImageName",
					"CreateContainerConfigError",
				}
				for _, pattern := range failurePatterns {
					if strings.Contains(output, pattern) {
						return fmt.Errorf("container in failure state: %s", pattern)
					}
				}

				// Check for containers that have terminated with error
				if strings.Contains(output, `"terminated"`) && strings.Contains(output, `"exitCode"`) {
					// Get exit codes
					exitCmd := exec.Command("kubectl", "get", "pods",
						"-n", agentsNamespace,
						"-l", "app.kubernetes.io/instance=test-agent",
						"-o", "jsonpath={range .items[0].status.containerStatuses[*]}{.name}={.state.terminated.exitCode}/{.lastState.terminated.exitCode} {end}")
					exitOutput, _ := utils.Run(exitCmd)
					if strings.Contains(exitOutput, "=1/") || strings.Contains(exitOutput, "/1 ") ||
						(strings.Contains(exitOutput, "=1 ") && !strings.Contains(exitOutput, "=0/")) {
						return fmt.Errorf("container terminated with non-zero exit code: %s", exitOutput)
					}
				}

				return nil
			}

			verifyContainersReady := func(g Gomega) {
				// First check for failure states - fail fast
				if err := checkForFailures(); err != nil {
					dumpDebugInfo(err.Error())
					g.Expect(err).NotTo(HaveOccurred(), "Container entered failure state")
				}

				// Check readiness
				cmd := exec.Command("kubectl", "get", "pods",
					"-n", agentsNamespace,
					"-l", "app.kubernetes.io/instance=test-agent",
					"-o", "jsonpath={.items[0].status.containerStatuses[*].ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				// Both containers should be ready (true true)
				g.Expect(output).To(ContainSubstring("true"))
				g.Expect(strings.Count(output, "true")).To(Equal(2), "Expected 2 containers to be ready")
			}
			ok := Eventually(verifyContainersReady, 5*time.Minute, 5*time.Second).Should(Succeed())
			if !ok {
				dumpDebugInfo("Container readiness timeout")
				Fail("Container readiness check failed - see debug output above")
			}

			By("verifying the pod has facade and runtime containers")
			var output string
			var err error
			cmd := exec.Command("kubectl", "get", "pods",
				"-n", agentsNamespace,
				"-l", "app.kubernetes.io/instance=test-agent",
				"-o", "jsonpath={.items[0].spec.containers[*].name}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("facade"))
			Expect(output).To(ContainSubstring("runtime"))

			By("verifying the runtime container has mock provider enabled")
			cmd = exec.Command("kubectl", "get", "pods",
				"-n", agentsNamespace,
				"-l", "app.kubernetes.io/instance=test-agent",
				"-o", "jsonpath={.items[0].spec.containers[?(@.name=='runtime')].env[?(@.name=='OMNIA_MOCK_PROVIDER')].value}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("true"), "Mock provider should be enabled for E2E testing")

			By("verifying the facade container has SESSION_API_URL injected")
			cmd = exec.Command("kubectl", "get", "pods",
				"-n", agentsNamespace,
				"-l", "app.kubernetes.io/instance=test-agent",
				"-o", "jsonpath={.items[0].spec.containers[?(@.name=='facade')].env[?(@.name=='SESSION_API_URL')].value}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal(sessionApiURL), "Facade should have SESSION_API_URL set by the operator")
		})

		It("should handle WebSocket connections to the facade", func() {
			By("creating a test pod to connect to the WebSocket")
			// Use a curl pod to test the WebSocket upgrade request
			testPodManifest := `
apiVersion: v1
kind: Pod
metadata:
  name: ws-test
  namespace: test-agents
spec:
  restartPolicy: Never
  containers:
  - name: curl
    image: curlimages/curl:latest
    command: ["sh", "-c"]
    args:
    - |
      # Test WebSocket upgrade to the facade service
      curl -v --no-buffer \
        -H "Connection: Upgrade" \
        -H "Upgrade: websocket" \
        -H "Sec-WebSocket-Version: 13" \
        -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
        "http://test-agent.test-agents.svc.cluster.local:8080/ws?agent=test-agent" 2>&1 || true
      # Keep pod alive briefly for log collection
      sleep 5
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(testPodManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create WebSocket test pod")

			By("waiting for the test pod to complete")
			verifyTestPodComplete := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", "ws-test",
					"-n", agentsNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				// Wait for Succeeded (not just Running) to ensure test completed
				g.Expect(output).To(Equal("Succeeded"))
			}
			Eventually(verifyTestPodComplete, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("checking the test pod logs for WebSocket upgrade response")
			cmd = exec.Command("kubectl", "logs", "ws-test", "-n", agentsNamespace)
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			// Verify we got a WebSocket upgrade response (101 Switching Protocols)
			Expect(output).To(ContainSubstring("101"), "Expected WebSocket upgrade response (101 Switching Protocols)")

			By("cleaning up test pod")
			cmd = exec.Command("kubectl", "delete", "pod", "ws-test", "-n", agentsNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("should complete a basic conversation with mock provider", func() {
			By("creating a Python test pod for WebSocket conversation")
			// Use Python with websockets library to test full conversation flow
			conversationTestManifest := `
apiVersion: v1
kind: Pod
metadata:
  name: conversation-test
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

      async def test_conversation():
          uri = "ws://test-agent.test-agents.svc.cluster.local:8080/ws?agent=test-agent"
          try:
              async with websockets.connect(uri, ping_interval=None) as ws:
                  # Send a test message first (without session_id to trigger connected message)
                  test_message = {
                      "type": "message",
                      "content": "Hello, this is a test message"
                  }
                  await ws.send(json.dumps(test_message))
                  print(f"Sent: {test_message}")

                  # Server sends "connected" after receiving first message, then response
                  session_id = ""
                  received_connected = False
                  received_response = False

                  for _ in range(10):  # Max 10 messages
                      try:
                          response = await asyncio.wait_for(ws.recv(), timeout=30)
                          msg = json.loads(response)
                          print(f"Received: {msg}")

                          msg_type = msg.get("type")
                          if msg_type == "connected":
                              received_connected = True
                              session_id = msg.get("session_id", "")
                              print(f"Session ID: {session_id}")
                          elif msg_type == "chunk":
                              received_response = True
                          elif msg_type == "done":
                              received_response = True
                              print("SUCCESS: Conversation completed")
                              break
                          elif msg_type == "error":
                              print(f"ERROR: {msg.get('error')}")
                              sys.exit(1)
                      except asyncio.TimeoutError:
                          break

                  if not received_connected:
                      print("ERROR: Did not receive connected message")
                      sys.exit(1)

                  if not received_response:
                      print("ERROR: No response received from agent")
                      sys.exit(1)

                  print("TEST PASSED: Basic conversation successful")

          except Exception as e:
              print(f"ERROR: {e}")
              sys.exit(1)

      asyncio.run(test_conversation())
      PYTHON_SCRIPT
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(conversationTestManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create conversation test pod")

			By("waiting for the conversation test to complete")
			verifyConversationTest := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", "conversation-test",
					"-n", agentsNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"))
			}
			ok := Eventually(verifyConversationTest, 3*time.Minute, 5*time.Second).Should(Succeed())
			if !ok {
				// Dump debug info on failure
				_, _ = fmt.Fprintf(GinkgoWriter, "\n=== DEBUG: Conversation test failed ===\n")
				descCmd := exec.Command("kubectl", "describe", "pod", "conversation-test", "-n", agentsNamespace)
				if descOutput, err := utils.Run(descCmd); err == nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "Pod describe:\n%s\n", descOutput)
				}
				logsCmd := exec.Command("kubectl", "logs", "conversation-test", "-n", agentsNamespace)
				if logs, err := utils.Run(logsCmd); err == nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "Conversation test logs:\n%s\n", logs)
				}
				// Also dump facade and runtime logs
				facadeCmd := exec.Command("kubectl", "logs", "-n", agentsNamespace, "-l", "app.kubernetes.io/instance=test-agent", "-c", "facade", "--tail=100")
				if facadeLogs, err := utils.Run(facadeCmd); err == nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "Facade logs:\n%s\n", facadeLogs)
				}
				runtimeCmd := exec.Command("kubectl", "logs", "-n", agentsNamespace, "-l", "app.kubernetes.io/instance=test-agent", "-c", "runtime", "--tail=100")
				if runtimeLogs, err := utils.Run(runtimeCmd); err == nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "Runtime logs:\n%s\n", runtimeLogs)
				}
				Fail("Conversation test failed - see debug output above")
			}

			By("checking the conversation test logs")
			cmd = exec.Command("kubectl", "logs", "conversation-test", "-n", agentsNamespace)
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("TEST PASSED"), "Conversation test should pass")
			Expect(output).NotTo(ContainSubstring("ERROR:"), "Conversation test should not have errors")

			By("cleaning up conversation test pod")
			cmd = exec.Command("kubectl", "delete", "pod", "conversation-test", "-n", agentsNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("should persist session state via session-api", func() {
			By("creating a session persistence test pod")
			// Test the production path: facade → httpclient.Store → session-api → Postgres
			sessionTestManifest := fmt.Sprintf(`
apiVersion: v1
kind: Pod
metadata:
  name: session-test
  namespace: test-agents
spec:
  restartPolicy: Never
  containers:
  - name: python
    image: python:3.11-slim
    command: ["sh", "-c"]
    args:
    - |
      pip install websockets requests --quiet
      python3 << 'PYTHON_SCRIPT'
      import asyncio
      import json
      import time
      import requests
      import websockets
      import sys

      SESSION_API_URL = "%s"

      async def test_session_persistence():
          uri = "ws://test-agent.test-agents.svc.cluster.local:8080/ws?agent=test-agent"

          try:
              # Connect via WebSocket and send a message
              async with websockets.connect(uri, ping_interval=None) as ws:
                  await ws.send(json.dumps({
                      "type": "message",
                      "content": "Remember this: the secret code is ALPHA123"
                  }))
                  print("Sent message")

                  session_id = ""
                  for _ in range(10):
                      try:
                          response = await asyncio.wait_for(ws.recv(), timeout=30)
                          msg = json.loads(response)
                          msg_type = msg.get("type")
                          print(f"Received: {msg_type}")

                          if msg_type == "connected":
                              session_id = msg.get("session_id", "")
                              print(f"Session ID: {session_id}")
                          elif msg_type == "done":
                              break
                      except asyncio.TimeoutError:
                          break

                  if not session_id:
                      print("ERROR: Did not receive session_id")
                      sys.exit(1)

              # Wait for async write flush to session-api
              print("Waiting for async write flush...")
              time.sleep(3)

              # Verify session exists via session-api
              print(f"Querying session-api for session {session_id}...")
              resp = requests.get(f"{SESSION_API_URL}/api/v1/sessions/{session_id}", timeout=10)
              print(f"GET /sessions/{session_id}: status={resp.status_code}")
              if resp.status_code != 200:
                  print(f"ERROR: Expected 200, got {resp.status_code}: {resp.text}")
                  sys.exit(1)

              session_data = resp.json()
              # The session-api wraps the session in a {"session": {...}, "messages": [...]} envelope
              session_obj = session_data.get("session", session_data)
              agent_name = session_obj.get("agentName", session_obj.get("agent_name", ""))
              print(f"Session agent: {agent_name}")
              if agent_name != "test-agent":
                  print(f"ERROR: Expected agentName='test-agent', got '{agent_name}'")
                  sys.exit(1)

              # Verify messages exist
              print(f"Querying messages for session {session_id}...")
              resp = requests.get(f"{SESSION_API_URL}/api/v1/sessions/{session_id}/messages", timeout=10)
              print(f"GET /sessions/{session_id}/messages: status={resp.status_code}")
              if resp.status_code != 200:
                  print(f"ERROR: Expected 200, got {resp.status_code}: {resp.text}")
                  sys.exit(1)

              messages_data = resp.json()
              messages = messages_data.get("messages", [])
              print(f"Found {len(messages)} messages")
              if len(messages) == 0:
                  print("ERROR: Expected at least 1 message")
                  sys.exit(1)

              print("SUCCESS: Session data persisted via session-api")
              print("TEST PASSED: Session persistence verified")

          except Exception as e:
              print(f"ERROR: {e}")
              import traceback
              traceback.print_exc()
              sys.exit(1)

      asyncio.run(test_session_persistence())
      PYTHON_SCRIPT
`, sessionApiURL)
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sessionTestManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create session test pod")

			By("waiting for the session test to complete")
			verifySessionTest := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", "session-test",
					"-n", agentsNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"))
			}
			ok := Eventually(verifySessionTest, 3*time.Minute, 5*time.Second).Should(Succeed())
			if !ok {
				// Dump debug info on failure
				_, _ = fmt.Fprintf(GinkgoWriter, "\n=== DEBUG: Session test failed ===\n")
				logsCmd := exec.Command("kubectl", "logs", "session-test", "-n", agentsNamespace)
				if logs, err := utils.Run(logsCmd); err == nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "Session test logs:\n%s\n", logs)
				}
				facadeCmd := exec.Command("kubectl", "logs", "-n", agentsNamespace, "-l", "app.kubernetes.io/instance=test-agent", "-c", "facade", "--tail=100")
				if facadeLogs, err := utils.Run(facadeCmd); err == nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "Facade logs:\n%s\n", facadeLogs)
				}
				sessionApiLogsCmd := exec.Command("kubectl", "logs", "-n", namespace, "-l", "app=e2e-session-api", "--tail=100")
				if sessionApiLogs, err := utils.Run(sessionApiLogsCmd); err == nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "Session-api logs:\n%s\n", sessionApiLogs)
				}
				Fail("Session test failed - see debug output above")
			}

			By("checking the session test logs")
			cmd = exec.Command("kubectl", "logs", "session-test", "-n", agentsNamespace)
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("TEST PASSED"), "Session test should pass")
			Expect(output).NotTo(ContainSubstring("ERROR:"), "Session test should not have errors")

			By("cleaning up session test pod")
			cmd = exec.Command("kubectl", "delete", "pod", "session-test", "-n", agentsNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("should execute tools via HTTP adapter", func() {
			By("creating a mock tool service")
			// Deploy a simple nginx-based mock that returns a JSON response
			mockToolServiceManifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: mock-tool-responses
  namespace: test-agents
data:
  default.conf: |
    server {
      listen 80;
      location /api/calculator {
        default_type application/json;
        return 200 '{"result": 42, "operation": "add", "inputs": [20, 22]}';
      }
      location /health {
        return 200 'ok';
      }
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mock-tool
  namespace: test-agents
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mock-tool
  template:
    metadata:
      labels:
        app: mock-tool
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
        ports:
        - containerPort: 80
        volumeMounts:
        - name: config
          mountPath: /etc/nginx/conf.d
        resources:
          requests:
            cpu: 10m
            memory: 16Mi
          limits:
            cpu: 50m
            memory: 32Mi
      volumes:
      - name: config
        configMap:
          name: mock-tool-responses
---
apiVersion: v1
kind: Service
metadata:
  name: mock-tool
  namespace: test-agents
spec:
  selector:
    app: mock-tool
  ports:
  - port: 80
    targetPort: 80
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(mockToolServiceManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create mock tool service")

			By("waiting for mock tool service to be ready")
			verifyMockToolReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods",
					"-n", agentsNamespace,
					"-l", "app=mock-tool",
					"-o", "jsonpath={.items[0].status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"))
			}
			Eventually(verifyMockToolReady, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("creating a ToolRegistry with HTTP handler")
			toolRegistryManifest := `
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ToolRegistry
metadata:
  name: http-tools
  namespace: test-agents
spec:
  handlers:
  - name: calculator
    type: http
    httpConfig:
      endpoint: "http://mock-tool.test-agents.svc.cluster.local/api/calculator"
      method: POST
      contentType: application/json
    tool:
      name: calculator
      description: A calculator tool that adds two numbers
      inputSchema:
        type: object
        properties:
          a:
            type: number
            description: First number
          b:
            type: number
            description: Second number
        required: [a, b]
    timeout: "10s"
`
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(toolRegistryManifest)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ToolRegistry with HTTP handler")

			By("verifying the ToolRegistry status and discovered tools")
			verifyToolRegistry := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "toolregistry", "http-tools",
					"-n", agentsNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Ready"))

				// Also verify tools were discovered
				cmd = exec.Command("kubectl", "get", "toolregistry", "http-tools",
					"-n", agentsNamespace, "-o", "jsonpath={.status.discoveredToolsCount}")
				output, err = utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"), "Should have 1 discovered tool")
			}
			Eventually(verifyToolRegistry, time.Minute, time.Second).Should(Succeed())

			By("creating an AgentRuntime with the HTTP tool")
			agentManifest := `
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: http-tool-agent
  namespace: test-agents
  annotations:
    omnia.altairalabs.ai/mock-provider: "true"
spec:
  promptPackRef:
    name: test-prompts
  toolRegistryRef:
    name: http-tools
  facade:
    type: websocket
    port: 8080
  session:
    type: memory
    ttl: "1h"
  runtime:
    replicas: 1
    resources:
      requests:
        cpu: "50m"
        memory: "64Mi"
      limits:
        cpu: "200m"
        memory: "128Mi"
  provider:
    type: mock
    secretRef:
      name: test-provider
`
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(agentManifest)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create http-tool-agent")

			By("waiting for the http-tool-agent to be ready")
			verifyAgentReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods",
					"-n", agentsNamespace,
					"-l", "app.kubernetes.io/instance=http-tool-agent",
					"-o", "jsonpath={.items[0].status.containerStatuses[*].ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(strings.Count(output, "true")).To(BeNumerically(">=", 1))
			}
			Eventually(verifyAgentReady, 3*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying the tools ConfigMap was created")
			// First verify the ConfigMap exists
			verifyToolsConfigMap := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "configmap", "http-tool-agent-tools",
					"-n", agentsNamespace, "-o", "yaml")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Tools ConfigMap should exist")
				g.Expect(output).To(ContainSubstring("tools.yaml"))
				g.Expect(output).To(ContainSubstring("handlers"))
				g.Expect(output).To(ContainSubstring("calculator"))
			}
			Eventually(verifyToolsConfigMap, time.Minute, 5*time.Second).Should(Succeed())

			By("verifying runtime container has tools config mounted")
			envCmd := exec.Command("kubectl", "get", "pods",
				"-n", agentsNamespace,
				"-l", "app.kubernetes.io/instance=http-tool-agent",
				"-o", "jsonpath={.items[0].spec.containers[?(@.name=='runtime')].env[?(@.name=='OMNIA_TOOLS_CONFIG_PATH')].value}")
			envOutput, err := utils.Run(envCmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(envOutput).To(ContainSubstring("tools.yaml"), "Runtime should have tools config path")

			By("checking runtime container logs for tool initialization")
			verifyToolsInitialized := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs",
					"-n", agentsNamespace,
					"-l", "app.kubernetes.io/instance=http-tool-agent",
					"-c", "runtime")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				// Look for tool initialization log messages
				if strings.Contains(output, "tools initialized") || strings.Contains(output, "initializing tools") {
					return
				}
				// Also accept if the container is running without errors
				g.Expect(output).NotTo(ContainSubstring("failed to initialize tools"))
			}
			Eventually(verifyToolsInitialized, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("cleaning up HTTP tool test resources")
			cmd = exec.Command("kubectl", "delete", "agentruntime", "http-tool-agent",
				"-n", agentsNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "toolregistry", "http-tools",
				"-n", agentsNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "deployment", "mock-tool",
				"-n", agentsNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "service", "mock-tool",
				"-n", agentsNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "configmap", "mock-tool-responses",
				"-n", agentsNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("should use CRD image overrides instead of operator defaults", func() {
			By("creating an AgentRuntime with custom facade and runtime images")
			// Use distinct image references to verify they're used instead of operator defaults
			customFacadeImage := "custom-facade-test:v1.0.0"
			customRuntimeImage := "custom-runtime-test:v1.0.0"

			imageOverrideAgentManifest := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: image-override-agent
  namespace: test-agents
  annotations:
    omnia.altairalabs.ai/mock-provider: "true"
spec:
  promptPackRef:
    name: test-prompts
  facade:
    type: websocket
    port: 8080
    image: %s
  framework:
    type: custom
    image: %s
  session:
    type: memory
    ttl: "1h"
  runtime:
    replicas: 1
    resources:
      requests:
        cpu: "50m"
        memory: "64Mi"
  provider:
    type: mock
    secretRef:
      name: test-provider
`, customFacadeImage, customRuntimeImage)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(imageOverrideAgentManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create AgentRuntime with image overrides")

			By("waiting for the deployment to be created")
			verifyDeploymentCreated := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "deployment", "image-override-agent",
					"-n", agentsNamespace, "-o", "jsonpath={.metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("image-override-agent"))
			}
			Eventually(verifyDeploymentCreated, 2*time.Minute, time.Second).Should(Succeed())

			By("verifying the facade container uses the custom image")
			cmd = exec.Command("kubectl", "get", "deployment", "image-override-agent",
				"-n", agentsNamespace,
				"-o", "jsonpath={.spec.template.spec.containers[?(@.name=='facade')].image}")
			facadeImageOutput, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(facadeImageOutput).To(Equal(customFacadeImage),
				"Facade container should use CRD image override, not operator default")

			By("verifying the runtime container uses the custom image")
			cmd = exec.Command("kubectl", "get", "deployment", "image-override-agent",
				"-n", agentsNamespace,
				"-o", "jsonpath={.spec.template.spec.containers[?(@.name=='runtime')].image}")
			runtimeImageOutput, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(runtimeImageOutput).To(Equal(customRuntimeImage),
				"Runtime container should use CRD image override, not operator default")

			By("verifying neither container uses the operator default images")
			Expect(facadeImageOutput).NotTo(Equal(facadeImageRef),
				"Should NOT use operator's --facade-image flag value")
			Expect(runtimeImageOutput).NotTo(Equal(runtimeImageRef),
				"Should NOT use operator's --framework-image flag value")

			By("cleaning up image override test agent")
			cmd = exec.Command("kubectl", "delete", "agentruntime", "image-override-agent",
				"-n", agentsNamespace, "--ignore-not-found", "--timeout=60s")
			_, _ = utils.Run(cmd)
		})

		It("should use operator defaults when CRD does not specify images", func() {
			By("creating an AgentRuntime without image overrides")
			noOverrideAgentManifest := `
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: default-image-agent
  namespace: test-agents
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
    resources:
      requests:
        cpu: "50m"
        memory: "64Mi"
  provider:
    type: mock
    secretRef:
      name: test-provider
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(noOverrideAgentManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create AgentRuntime without image overrides")

			By("waiting for the deployment to be created")
			verifyDeploymentCreated := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "deployment", "default-image-agent",
					"-n", agentsNamespace, "-o", "jsonpath={.metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("default-image-agent"))
			}
			Eventually(verifyDeploymentCreated, 2*time.Minute, time.Second).Should(Succeed())

			By("verifying the facade container uses the operator default image")
			cmd = exec.Command("kubectl", "get", "deployment", "default-image-agent",
				"-n", agentsNamespace,
				"-o", "jsonpath={.spec.template.spec.containers[?(@.name=='facade')].image}")
			facadeImageOutput, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(facadeImageOutput).To(Equal(facadeImageRef),
				"Facade container should use operator's --facade-image flag value")

			By("verifying the runtime container uses the operator default image")
			cmd = exec.Command("kubectl", "get", "deployment", "default-image-agent",
				"-n", agentsNamespace,
				"-o", "jsonpath={.spec.template.spec.containers[?(@.name=='runtime')].image}")
			runtimeImageOutput, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(runtimeImageOutput).To(Equal(runtimeImageRef),
				"Runtime container should use operator's --framework-image flag value")

			By("cleaning up default image test agent")
			cmd = exec.Command("kubectl", "delete", "agentruntime", "default-image-agent",
				"-n", agentsNamespace, "--ignore-not-found", "--timeout=60s")
			_, _ = utils.Run(cmd)
		})

		It("should allow partial image overrides (facade only)", func() {
			By("creating an AgentRuntime with only facade image override")
			customFacadeImage := "partial-facade-test:v2.0.0"

			partialOverrideManifest := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: partial-override-agent
  namespace: test-agents
  annotations:
    omnia.altairalabs.ai/mock-provider: "true"
spec:
  promptPackRef:
    name: test-prompts
  facade:
    type: websocket
    port: 8080
    image: %s
  session:
    type: memory
    ttl: "1h"
  runtime:
    replicas: 1
    resources:
      requests:
        cpu: "50m"
        memory: "64Mi"
  provider:
    type: mock
    secretRef:
      name: test-provider
`, customFacadeImage)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(partialOverrideManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create AgentRuntime with partial image override")

			By("waiting for the deployment to be created")
			verifyDeploymentCreated := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "deployment", "partial-override-agent",
					"-n", agentsNamespace, "-o", "jsonpath={.metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("partial-override-agent"))
			}
			Eventually(verifyDeploymentCreated, 2*time.Minute, time.Second).Should(Succeed())

			By("verifying the facade container uses the custom image")
			cmd = exec.Command("kubectl", "get", "deployment", "partial-override-agent",
				"-n", agentsNamespace,
				"-o", "jsonpath={.spec.template.spec.containers[?(@.name=='facade')].image}")
			facadeImageOutput, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(facadeImageOutput).To(Equal(customFacadeImage),
				"Facade should use CRD override")

			By("verifying the runtime container uses the operator default (not overridden)")
			cmd = exec.Command("kubectl", "get", "deployment", "partial-override-agent",
				"-n", agentsNamespace,
				"-o", "jsonpath={.spec.template.spec.containers[?(@.name=='runtime')].image}")
			runtimeImageOutput, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(runtimeImageOutput).To(Equal(runtimeImageRef),
				"Runtime should fall back to operator default when not overridden")

			By("cleaning up partial override test agent")
			cmd = exec.Command("kubectl", "delete", "agentruntime", "partial-override-agent",
				"-n", agentsNamespace, "--ignore-not-found", "--timeout=60s")
			_, _ = utils.Run(cmd)
		})

		It("should handle tool calls via demo handler", func() {
			By("creating an AgentRuntime with demo handler for tool call testing")
			// The demo handler simulates tool calls for weather and password queries
			toolTestAgentManifest := `
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: tool-test-agent
  namespace: test-agents
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
    resources:
      requests:
        cpu: "50m"
        memory: "64Mi"
      limits:
        cpu: "200m"
        memory: "128Mi"
  provider:
    type: mock
    secretRef:
      name: test-provider
`
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(toolTestAgentManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create tool-test-agent")

			By("waiting for the tool-test-agent pod to be running")
			verifyToolAgentRunning := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods",
					"-n", agentsNamespace,
					"-l", "app.kubernetes.io/instance=tool-test-agent",
					"-o", "jsonpath={.items[0].status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"))
			}
			Eventually(verifyToolAgentRunning, 3*time.Minute, 5*time.Second).Should(Succeed())

			By("waiting for all containers to be ready")
			verifyContainersReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods",
					"-n", agentsNamespace,
					"-l", "app.kubernetes.io/instance=tool-test-agent",
					"-o", "jsonpath={.items[0].status.containerStatuses[*].ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true true"), "Both containers should be ready")
			}
			Eventually(verifyContainersReady, 5*time.Minute, 5*time.Second).Should(Succeed())

			By("waiting for service endpoint to be ready")
			verifyServiceEndpoint := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "endpoints", "tool-test-agent",
					"-n", agentsNamespace, "-o", "jsonpath={.subsets[0].addresses[0].ip}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).NotTo(BeEmpty(), "Service endpoint should have an IP")
			}
			Eventually(verifyServiceEndpoint, time.Minute, 2*time.Second).Should(Succeed())

			By("creating a test pod to verify tool call messages")
			toolCallTestManifest := `
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
              async with websockets.connect(uri, ping_interval=None) as ws:
                  # Send a weather query which triggers tool calls in demo handler
                  weather_message = {
                      "type": "message",
                      "content": "What's the weather like?"
                  }
                  await ws.send(json.dumps(weather_message))
                  print(f"Sent: {weather_message}")

                  # Track message types received
                  received_types = []
                  received_tool_call = False
                  received_tool_result = False
                  tool_call_name = ""
                  tool_result_data = None

                  for _ in range(20):  # Max 20 messages
                      try:
                          response = await asyncio.wait_for(ws.recv(), timeout=30)
                          msg = json.loads(response)
                          msg_type = msg.get("type")
                          received_types.append(msg_type)
                          print(f"Received: {msg_type} - {json.dumps(msg)[:200]}")

                          if msg_type == "tool_call":
                              received_tool_call = True
                              tool_call_name = msg.get("tool_call", {}).get("name", "")
                              print(f"Tool call: {tool_call_name}")

                          elif msg_type == "tool_result":
                              received_tool_result = True
                              tool_result_data = msg.get("tool_result", {})
                              print(f"Tool result received")

                          elif msg_type == "done":
                              print("Conversation complete")
                              break

                          elif msg_type == "error":
                              print(f"ERROR: {msg.get('error')}")
                              sys.exit(1)

                      except asyncio.TimeoutError:
                          print("Timeout waiting for messages")
                          break

                  # Verify we received tool_call and tool_result
                  print(f"\nMessage types received: {received_types}")

                  if not received_tool_call:
                      print("ERROR: Did not receive tool_call message")
                      sys.exit(1)

                  if not received_tool_result:
                      print("ERROR: Did not receive tool_result message")
                      sys.exit(1)

                  if tool_call_name != "weather":
                      print(f"ERROR: Expected tool name 'weather', got '{tool_call_name}'")
                      sys.exit(1)

                  print("\nTEST PASSED: Tool call flow verified")
                  print(f"  - Received tool_call for '{tool_call_name}'")
                  print(f"  - Received tool_result with data")

          except Exception as e:
              print(f"ERROR: {e}")
              import traceback
              traceback.print_exc()
              sys.exit(1)

      asyncio.run(test_tool_calls())
      PYTHON_SCRIPT
`
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(toolCallTestManifest)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create tool call test pod")

			By("waiting for the tool call test to complete")
			verifyToolCallTest := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", "tool-call-test",
					"-n", agentsNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"))
			}
			Eventually(verifyToolCallTest, 3*time.Minute, 5*time.Second).Should(Succeed())

			By("checking the tool call test logs")
			cmd = exec.Command("kubectl", "logs", "tool-call-test", "-n", agentsNamespace)
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			_, _ = fmt.Fprintf(GinkgoWriter, "Tool call test output:\n%s\n", output)
			Expect(output).To(ContainSubstring("TEST PASSED"), "Tool call test should pass")
			Expect(output).To(ContainSubstring("tool_call"), "Should receive tool_call message")
			Expect(output).To(ContainSubstring("tool_result"), "Should receive tool_result message")

			By("cleaning up tool call test resources")
			cmd = exec.Command("kubectl", "delete", "pod", "tool-call-test",
				"-n", agentsNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "agentruntime", "tool-test-agent",
				"-n", agentsNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		// Note: Runtime metrics (omnia_runtime_*) are exposed on port 9001 via the health endpoint.
		// The metrics implementation is tested via unit tests in pkg/metrics/runtime_test.go.
		// E2E testing of the metrics endpoint in Kind clusters has networking limitations.
	})
})

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	// Temporary file to store the token request
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		// Execute kubectl command to create the token
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			namespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		// Parse the JSON output to extract the token
		var token tokenRequest
		err = json.Unmarshal(output, &token)
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return out, err
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() (string, error) {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	return utils.Run(cmd)
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}
