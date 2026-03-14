//go:build e2e

/*
Copyright 2026 Altaira Labs.

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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/altairalabs/omnia/test/utils"
)

// evalWorkerNamespaceCI is the isolated namespace used in CI mode.
// In predeployed mode we use the main namespace instead, since the
// eval worker is already watching it.
const evalWorkerNamespaceCI = "test-eval-worker"

// effectiveEvalNamespace returns the namespace for PromptPack ConfigMaps
// and Redis Stream events. In predeployed mode the eval worker already
// watches the main namespace, so we use that directly.
func effectiveEvalNamespace() string {
	if predeployed {
		return namespace
	}
	return evalWorkerNamespaceCI
}

// evalCurlPod is the name of the helper pod used for HTTP requests.
const evalCurlPod = "eval-e2e-curl"

// sessionAPIEndpoint returns the in-cluster session-api URL.
func sessionAPIEndpoint() string {
	if predeployed {
		return fmt.Sprintf(
			"http://omnia-session-api.%s.svc.cluster.local:8080",
			namespace)
	}
	return fmt.Sprintf(
		"http://e2e-eval-session-api.%s.svc.cluster.local:8080",
		namespace)
}

// evalWorkerLabel returns the pod label selector for the eval worker.
func evalWorkerLabel() string {
	if predeployed {
		return "app.kubernetes.io/name=omnia-eval-worker"
	}
	return "app=e2e-eval-worker"
}

// curlFromCluster runs curl from a helper pod inside the cluster.
func curlFromCluster(url string) (string, error) {
	cmd := exec.Command("kubectl", "exec", evalCurlPod,
		"-n", namespace, "--",
		"curl", "-s", url)
	return utils.Run(cmd)
}

var _ = Describe("Eval Worker Pipeline", Ordered, Label("arena"), func() {
	BeforeAll(func() {
		if os.Getenv("ENABLE_ARENA_E2E") != "true" {
			Skip("Eval worker E2E tests require ENABLE_ARENA_E2E=true")
		}

		By("verifying controller-manager is ready")
		verifyReady := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "deployment",
				"omnia-controller-manager",
				"-n", namespace, "-o", "jsonpath={.status.readyReplicas}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).NotTo(BeEmpty())
			g.Expect(output).NotTo(Equal("0"))
		}
		Eventually(verifyReady, 2*time.Minute, 2*time.Second).Should(Succeed())

		By("verifying Redis is ready")
		verifyRedis := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "statefulset",
				"omnia-redis-master",
				"-n", namespace, "-o", "jsonpath={.status.readyReplicas}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).NotTo(BeEmpty())
			g.Expect(output).NotTo(Equal("0"))
		}
		Eventually(verifyRedis, 2*time.Minute, 2*time.Second).Should(Succeed())

		if predeployed {
			By("verifying existing eval worker is ready (predeployed)")
			verifyWorker := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "deployment",
					"omnia-eval-worker",
					"-n", namespace,
					"-o", "jsonpath={.status.readyReplicas}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).NotTo(BeEmpty())
				g.Expect(output).NotTo(Equal("0"))
			}
			Eventually(verifyWorker, 2*time.Minute, 2*time.Second).
				Should(Succeed())

			By("verifying existing session-api is ready (predeployed)")
			verifySAPI := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "deployment",
					"omnia-session-api",
					"-n", namespace,
					"-o", "jsonpath={.status.readyReplicas}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).NotTo(Equal("0"))
			}
			Eventually(verifySAPI, 2*time.Minute, 2*time.Second).
				Should(Succeed())
		}

		if !predeployed {
			By("creating eval worker test namespace")
			cmd := exec.Command("kubectl", "create", "ns",
				evalWorkerNamespaceCI)
			_, _ = utils.Run(cmd) // Ignore error if already exists

			By("labeling namespace with restricted security policy")
			cmd = exec.Command("kubectl", "label", "--overwrite", "ns",
				evalWorkerNamespaceCI,
				"pod-security.kubernetes.io/enforce=restricted")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		}

		By("creating a curl helper pod for HTTP requests")
		cmd := exec.Command("kubectl", "run", evalCurlPod,
			"-n", namespace,
			"--image=curlimages/curl:8.5.0",
			"--restart=Never",
			"--command", "--", "sleep", "3600")
		_, _ = utils.Run(cmd) // Ignore if already exists
		verifyCurl := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pod", evalCurlPod,
				"-n", namespace,
				"-o", "jsonpath={.status.phase}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("Running"))
		}
		Eventually(verifyCurl, time.Minute, 2*time.Second).Should(Succeed())
	})

	AfterAll(func() {
		if skipCleanup {
			_, _ = fmt.Fprintf(GinkgoWriter,
				"Skipping eval worker cleanup (E2E_SKIP_CLEANUP=true)\n")
			return
		}

		// Always clean up curl pod
		cmd := exec.Command("kubectl", "delete", "pod", evalCurlPod,
			"-n", namespace, "--ignore-not-found")
		_, _ = utils.Run(cmd)

		if !predeployed {
			By("cleaning up eval worker resources")
			for _, resource := range []string{
				"deployment/e2e-eval-worker",
				"deployment/e2e-eval-session-api",
				"service/e2e-eval-session-api",
				"deployment/e2e-eval-postgres",
				"service/e2e-eval-postgres",
				"secret/eval-postgres-conn",
				"serviceaccount/e2e-eval-worker",
			} {
				cmd := exec.Command("kubectl", "delete", resource,
					"-n", namespace,
					"--ignore-not-found", "--timeout=30s")
				_, _ = utils.Run(cmd)
			}
		}

		if !predeployed {
			By("cleaning up eval worker namespace")
			nsCmd := exec.Command("kubectl", "delete", "ns",
				evalWorkerNamespaceCI,
				"--ignore-not-found", "--timeout=120s")
			_, _ = utils.Run(nsCmd)
		}
	})

	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			dumpEvalWorkerDebugInfo(specReport.FullText())
		}
	})

	SetDefaultEventuallyTimeout(3 * time.Minute)
	SetDefaultEventuallyPollingInterval(2 * time.Second)

	Context("End-to-End Eval Pipeline", func() {
		const (
			testSessionID = "00000000-e2e0-4000-a000-000000000001"
			testMessageID = "00000000-e2e0-4000-a000-000000000011"
			testAgentName = "eval-e2e-agent"
			testPackName  = "eval-e2e-pack"
		)

		It("should set up eval infrastructure", func() {
			if predeployed {
				By("using existing Tilt infrastructure (predeployed)")
				return
			}

			By("deploying a Postgres instance for session-api")
			postgresManifest := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: e2e-eval-postgres
  namespace: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: e2e-eval-postgres
  template:
    metadata:
      labels:
        app: e2e-eval-postgres
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 70
        fsGroup: 70
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: postgres
        image: postgres:16-alpine
        ports:
        - containerPort: 5432
        env:
        - name: POSTGRES_DB
          value: sessions
        - name: POSTGRES_USER
          value: omnia
        - name: POSTGRES_PASSWORD
          value: testpass
        - name: PGDATA
          value: /tmp/pgdata
        securityContext:
          readOnlyRootFilesystem: false
          allowPrivilegeEscalation: false
          capabilities:
            drop: ["ALL"]
---
apiVersion: v1
kind: Service
metadata:
  name: e2e-eval-postgres
  namespace: %s
spec:
  selector:
    app: e2e-eval-postgres
  ports:
  - port: 5432
    targetPort: 5432
---
apiVersion: v1
kind: Secret
metadata:
  name: eval-postgres-conn
  namespace: %s
stringData:
  connection-string: >-
    postgres://omnia:testpass@e2e-eval-postgres:5432/sessions?sslmode=disable
`, namespace, namespace, namespace)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(postgresManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to deploy Postgres")

			By("waiting for Postgres to be ready")
			verifyPostgres := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods",
					"-n", namespace,
					"-l", "app=e2e-eval-postgres",
					"-o", "jsonpath={.items[0].status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"))
			}
			Eventually(verifyPostgres, 4*time.Minute, 2*time.Second).
				Should(Succeed())

			By("deploying session-api for eval results")
			sessionApiManifest := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: e2e-eval-session-api
  namespace: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: e2e-eval-session-api
  template:
    metadata:
      labels:
        app: e2e-eval-session-api
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        seccompProfile:
          type: RuntimeDefault
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
              name: eval-postgres-conn
              key: connection-string
        - name: REDIS_ADDRS
          value: "omnia-redis-master.%s.svc.cluster.local:6379"
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 5
        securityContext:
          readOnlyRootFilesystem: true
          allowPrivilegeEscalation: false
          capabilities:
            drop: ["ALL"]
---
apiVersion: v1
kind: Service
metadata:
  name: e2e-eval-session-api
  namespace: %s
spec:
  selector:
    app: e2e-eval-session-api
  ports:
  - port: 8080
    targetPort: 8080
`, namespace, sessionApiImage, namespace, namespace)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sessionApiManifest)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to deploy session-api")

			By("waiting for session-api to be ready")
			verifySAPI := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods",
					"-n", namespace,
					"-l", "app=e2e-eval-session-api",
					"-o", "jsonpath={.items[0].status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"))
			}
			Eventually(verifySAPI, 4*time.Minute, 2*time.Second).
				Should(Succeed())

			By("deploying the eval worker")
			evalWorkerManifest := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: e2e-eval-worker
  namespace: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: e2e-eval-worker
  template:
    metadata:
      labels:
        app: e2e-eval-worker
    spec:
      serviceAccountName: e2e-eval-worker
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: eval-worker
        image: %s
        ports:
        - name: metrics
          containerPort: 9090
        env:
        - name: REDIS_ADDR
          value: "omnia-redis-master.%s.svc.cluster.local:6379"
        - name: NAMESPACES
          value: "%s"
        - name: SESSION_API_URL
          value: "http://e2e-eval-session-api.%s.svc.cluster.local:8080"
        - name: LOG_LEVEL
          value: "debug"
        readinessProbe:
          httpGet:
            path: /readyz
            port: 9090
          initialDelaySeconds: 3
          periodSeconds: 5
        securityContext:
          readOnlyRootFilesystem: true
          allowPrivilegeEscalation: false
          capabilities:
            drop: ["ALL"]
`, namespace, evalWorkerImage, namespace, evalWorkerNamespaceCI, namespace)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(evalWorkerManifest)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to deploy eval worker")

			By("creating ServiceAccount and RBAC for eval worker")
			rbacManifest := fmt.Sprintf(`
apiVersion: v1
kind: ServiceAccount
metadata:
  name: e2e-eval-worker
  namespace: %s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: e2e-eval-worker
  namespace: %s
rules:
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: e2e-eval-worker
  namespace: %s
subjects:
- kind: ServiceAccount
  name: e2e-eval-worker
  namespace: %s
roleRef:
  kind: Role
  name: e2e-eval-worker
  apiGroup: rbac.authorization.k8s.io
`, namespace, evalWorkerNamespaceCI, evalWorkerNamespaceCI, namespace)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(rbacManifest)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create RBAC")

			By("waiting for the eval worker to be ready")
			verifyWorker := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods",
					"-n", namespace,
					"-l", "app=e2e-eval-worker",
					"-o", "jsonpath={.items[0].status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"))
			}
			Eventually(verifyWorker, 3*time.Minute, 2*time.Second).
				Should(Succeed())
		})

		It("should create the PromptPack ConfigMap", func() {
			By("creating a PromptPack ConfigMap with rule-based evals")
			packJSON := `{
  "id": "eval-e2e-pack",
  "version": "1.0.0",
  "prompts": {
    "default": {
      "id": "default",
      "name": "Default",
      "version": "1.0.0",
      "system_template": "You are a helpful assistant."
    }
  },
  "evals": [
    {
      "id": "greeting-check",
      "type": "contains",
      "trigger": "every_turn",
      "description": "Check if response contains a greeting",
      "params": {
        "patterns": ["hello"]
      }
    }
  ]
}`
			packConfigMap := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
data:
  pack.json: |
    %s
`, testPackName, effectiveEvalNamespace(),
				strings.ReplaceAll(packJSON, "\n", "\n    "))

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(packConfigMap)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(),
				"Failed to create PromptPack ConfigMap")
		})

		It("should process an eval event and write results", func() {
			sapiURL := sessionAPIEndpoint()

			By("ensuring Redis Stream consumer group exists")
			streamKey := fmt.Sprintf(
				"omnia:eval-events:%s", effectiveEvalNamespace())
			consumerGroup := "omnia-eval-workers-cluster"
			cmd := exec.Command("kubectl", "exec", "omnia-redis-master-0",
				"-n", namespace, "--",
				"redis-cli", "XGROUP", "CREATE", streamKey,
				consumerGroup, "0", "MKSTREAM")
			_, _ = utils.Run(cmd) // Ignore if already exists

			By("seeding Redis with test session data")
			sessionJSON := fmt.Sprintf(`{
  "id": "%s",
  "agentName": "%s",
  "namespace": "%s",
  "createdAt": "2026-03-09T00:00:00Z",
  "updatedAt": "2026-03-09T00:00:01Z",
  "messages": [],
  "status": "active"
}`, testSessionID, testAgentName, effectiveEvalNamespace())

			sessionKey := fmt.Sprintf("hot:session:{%s}", testSessionID)
			cmd = exec.Command("kubectl", "exec", "omnia-redis-master-0",
				"-n", namespace, "--",
				"redis-cli", "SET", sessionKey, sessionJSON)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(),
				"Failed to seed session in Redis")

			By("seeding Redis with test messages")
			msgJSON := fmt.Sprintf(`{
  "id": "%s",
  "role": "assistant",
  "content": "hello world! How can I help you today?",
  "timestamp": "2026-03-09T00:00:01Z"
}`, testMessageID)

			msgsKey := fmt.Sprintf("hot:session:{%s}:msgs", testSessionID)
			cmd = exec.Command("kubectl", "exec", "omnia-redis-master-0",
				"-n", namespace, "--",
				"redis-cli", "RPUSH", msgsKey, msgJSON)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(),
				"Failed to seed messages in Redis")

			By("publishing a session event to the Redis Stream")
			event := map[string]interface{}{
				"eventType":         "message.assistant",
				"sessionId":         testSessionID,
				"agentName":         testAgentName,
				"namespace":         effectiveEvalNamespace(),
				"messageId":         testMessageID,
				"messageRole":       "assistant",
				"promptPackName":    testPackName,
				"promptPackVersion": "1.0.0",
				"timestamp":         time.Now().UTC().Format(time.RFC3339),
				"evalTiers":         []string{"lightweight"},
			}
			eventJSON, err := json.Marshal(event)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("kubectl", "exec", "omnia-redis-master-0",
				"-n", namespace, "--",
				"redis-cli", "XADD", streamKey, "*",
				"payload", string(eventJSON))
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(),
				"Failed to publish event to Redis Stream")
			_, _ = fmt.Fprintf(GinkgoWriter,
				"Published event to stream, ID: %s\n", output)

			By("waiting for eval results to appear in session-api")
			resultsURL := fmt.Sprintf(
				"%s/api/v1/sessions/%s/eval-results",
				sapiURL, testSessionID)
			verifyResults := func(g Gomega) {
				out, curlErr := curlFromCluster(resultsURL)
				g.Expect(curlErr).NotTo(HaveOccurred(),
					"Failed to query eval results")

				var resp struct {
					Results []struct {
						EvalID string `json:"evalId"`
					} `json:"results"`
				}
				g.Expect(json.Unmarshal([]byte(out), &resp)).
					To(Succeed(), "Failed to parse eval results")
				g.Expect(resp.Results).NotTo(BeEmpty(),
					"Should have at least one eval result")
			}
			Eventually(verifyResults, 2*time.Minute, 5*time.Second).
				Should(Succeed())

			By("verifying the eval result details")
			output, err = curlFromCluster(resultsURL)
			Expect(err).NotTo(HaveOccurred())

			var resp struct {
				Results []struct {
					SessionID string `json:"sessionId"`
					EvalID    string `json:"evalId"`
					EvalType  string `json:"evalType"`
					Passed    bool   `json:"passed"`
					Source    string `json:"source"`
					AgentName string `json:"agentName"`
					Namespace string `json:"namespace"`
				} `json:"results"`
			}
			Expect(json.Unmarshal([]byte(output), &resp)).To(Succeed())
			_, _ = fmt.Fprintf(GinkgoWriter,
				"Eval results: %s\n", output)

			Expect(resp.Results).To(HaveLen(1),
				"Should have exactly 1 eval result")
			result := resp.Results[0]
			Expect(result.SessionID).To(Equal(testSessionID))
			Expect(result.EvalID).To(Equal("greeting-check"))
			Expect(result.EvalType).To(Equal("contains"))
			Expect(result.Passed).To(BeTrue(),
				"Message contains 'hello' so eval should pass")
			Expect(result.Source).To(Equal("worker"))
			Expect(result.AgentName).To(Equal(testAgentName))
			Expect(result.Namespace).To(Equal(effectiveEvalNamespace()))

			By("checking eval worker metrics (best-effort)")
			podIPCmd := exec.Command("kubectl", "get", "pod",
				"-n", namespace,
				"-l", evalWorkerLabel(),
				"-o", "jsonpath={.items[0].status.podIP}")
			podIP, podIPErr := utils.Run(podIPCmd)
			if podIPErr == nil && podIP != "" {
				metricsURL := fmt.Sprintf("http://%s:9090/metrics",
					strings.TrimSpace(podIP))
				mOut, mErr := curlFromCluster(metricsURL)
				if mErr == nil {
					_, _ = fmt.Fprintf(GinkgoWriter,
						"Eval worker metrics (excerpt):\n")
					for _, line := range strings.Split(mOut, "\n") {
						if strings.HasPrefix(line, "omnia_eval_worker_") {
							_, _ = fmt.Fprintf(GinkgoWriter,
								"  %s\n", line)
						}
					}
				}
			}
		})

		It("should handle a failing eval correctly", func() {
			failSessionID := "00000000-e2e0-4000-a000-000000000002"
			failMessageID := "00000000-e2e0-4000-a000-000000000022"
			sapiURL := sessionAPIEndpoint()

			By("seeding Redis with a session that does NOT contain 'hello'")
			sessionJSON := fmt.Sprintf(`{
  "id": "%s",
  "agentName": "%s",
  "namespace": "%s",
  "createdAt": "2026-03-09T00:01:00Z",
  "updatedAt": "2026-03-09T00:01:01Z",
  "messages": [],
  "status": "active"
}`, failSessionID, testAgentName, effectiveEvalNamespace())

			sessionKey := fmt.Sprintf("hot:session:{%s}", failSessionID)
			cmd := exec.Command("kubectl", "exec", "omnia-redis-master-0",
				"-n", namespace, "--",
				"redis-cli", "SET", sessionKey, sessionJSON)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			msgJSON := fmt.Sprintf(`{
  "id": "%s",
  "role": "assistant",
  "content": "I can help you with that task.",
  "timestamp": "2026-03-09T00:01:01Z"
}`, failMessageID)

			msgsKey := fmt.Sprintf("hot:session:{%s}:msgs", failSessionID)
			cmd = exec.Command("kubectl", "exec", "omnia-redis-master-0",
				"-n", namespace, "--",
				"redis-cli", "RPUSH", msgsKey, msgJSON)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("publishing a session event for the failing case")
			event := map[string]interface{}{
				"eventType":         "message.assistant",
				"sessionId":         failSessionID,
				"agentName":         testAgentName,
				"namespace":         effectiveEvalNamespace(),
				"messageId":         failMessageID,
				"messageRole":       "assistant",
				"promptPackName":    testPackName,
				"promptPackVersion": "1.0.0",
				"timestamp":         time.Now().UTC().Format(time.RFC3339),
				"evalTiers":         []string{"lightweight"},
			}
			eventJSON, err := json.Marshal(event)
			Expect(err).NotTo(HaveOccurred())

			streamKey := fmt.Sprintf(
				"omnia:eval-events:%s", effectiveEvalNamespace())
			cmd = exec.Command("kubectl", "exec", "omnia-redis-master-0",
				"-n", namespace, "--",
				"redis-cli", "XADD", streamKey, "*",
				"payload", string(eventJSON))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for the failing eval result")
			failURL := fmt.Sprintf(
				"%s/api/v1/sessions/%s/eval-results",
				sapiURL, failSessionID)
			verifyFail := func(g Gomega) {
				out, curlErr := curlFromCluster(failURL)
				g.Expect(curlErr).NotTo(HaveOccurred())

				var resp struct {
					Results []struct {
						Passed bool `json:"passed"`
					} `json:"results"`
				}
				g.Expect(json.Unmarshal([]byte(out), &resp)).
					To(Succeed())
				g.Expect(resp.Results).NotTo(BeEmpty())
			}
			Eventually(verifyFail, 2*time.Minute, 5*time.Second).
				Should(Succeed())

			By("verifying the eval failed as expected")
			output, err := curlFromCluster(failURL)
			Expect(err).NotTo(HaveOccurred())

			var resp struct {
				Results []struct {
					EvalID   string `json:"evalId"`
					EvalType string `json:"evalType"`
					Passed   bool   `json:"passed"`
				} `json:"results"`
			}
			Expect(json.Unmarshal([]byte(output), &resp)).To(Succeed())
			Expect(resp.Results).To(HaveLen(1))
			Expect(resp.Results[0].EvalID).To(Equal("greeting-check"))
			Expect(resp.Results[0].Passed).To(BeFalse(),
				"Message does not contain 'hello'")
		})
	})
})

// dumpEvalWorkerDebugInfo logs diagnostic information when a test fails.
func dumpEvalWorkerDebugInfo(reason string) {
	_, _ = fmt.Fprintf(GinkgoWriter,
		"\n=== EVAL WORKER DEBUG: %s ===\n", reason)

	label := evalWorkerLabel()

	cmd := exec.Command("kubectl", "get", "pods",
		"-n", namespace, "-l", label, "-o", "wide")
	output, _ := utils.Run(cmd)
	_, _ = fmt.Fprintf(GinkgoWriter, "Eval worker pods:\n%s\n", output)

	cmd = exec.Command("kubectl", "logs",
		"-n", namespace, "-l", label, "--tail=100")
	output, _ = utils.Run(cmd)
	_, _ = fmt.Fprintf(GinkgoWriter, "Eval worker logs:\n%s\n", output)

	cmd = exec.Command("kubectl", "exec", "omnia-redis-master-0",
		"-n", namespace, "--",
		"redis-cli", "XINFO", "STREAM",
		fmt.Sprintf("omnia:eval-events:%s", effectiveEvalNamespace()))
	output, _ = utils.Run(cmd)
	_, _ = fmt.Fprintf(GinkgoWriter, "Redis stream info:\n%s\n", output)

	cmd = exec.Command("kubectl", "get", "events",
		"-n", namespace, "--sort-by=.lastTimestamp")
	output, _ = utils.Run(cmd)
	_, _ = fmt.Fprintf(GinkgoWriter, "Events:\n%s\n", output)
}
