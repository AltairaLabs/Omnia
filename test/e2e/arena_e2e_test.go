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
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/altairalabs/omnia/test/utils"
)

// arenaNamespace is where Arena test resources are deployed
const arenaNamespace = "test-arena"

// promptKitGitURL is the GitHub URL for the promptkit repository
const promptKitGitURL = "https://github.com/altairalabs/promptkit"

// promptKitExamplePath is the path within the promptkit repo to the assertions-test example
const promptKitExamplePath = "examples/assertions-test"

var _ = Describe("Arena Fleet", Ordered, func() {
	// Before running Arena tests, set up the namespace, CRDs, controller, and Redis
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, _ = utils.Run(cmd) // Ignore error if already exists

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")

		By("patching the controller-manager to enable dev mode for testing")
		patchCmd := exec.Command("kubectl", "patch", "deployment", "omnia-controller-manager",
			"-n", namespace, "--type=json",
			"-p", `[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--dev-mode"}]`)
		_, err = utils.Run(patchCmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to patch controller-manager with dev mode")

		By("waiting for controller-manager to be ready")
		verifyControllerReady := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "deployment", "omnia-controller-manager",
				"-n", namespace, "-o", "jsonpath={.status.readyReplicas}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("1"), "Controller manager should have 1 ready replica")
		}
		Eventually(verifyControllerReady, 2*time.Minute, 2*time.Second).Should(Succeed())

		By("creating arena test namespace")
		cmd = exec.Command("kubectl", "create", "ns", arenaNamespace)
		_, _ = utils.Run(cmd) // Ignore error if already exists

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", arenaNamespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("deploying Redis for Arena queue storage")
		redisManifest := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  namespace: test-arena
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
      securityContext:
        runAsNonRoot: true
        runAsUser: 999
        fsGroup: 999
        seccompProfile:
          type: RuntimeDefault
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
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities:
            drop:
              - ALL
        volumeMounts:
        - name: data
          mountPath: /data
      volumes:
      - name: data
        emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: redis
  namespace: test-arena
spec:
  selector:
    app: redis
  ports:
  - port: 6379
    targetPort: 6379
`
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(redisManifest)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy Redis for Arena")

		By("waiting for Redis to be ready")
		verifyRedisReady := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pods", "-n", arenaNamespace,
				"-l", "app=redis", "-o", "jsonpath={.items[0].status.phase}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("Running"))
		}
		Eventually(verifyRedisReady, 2*time.Minute, time.Second).Should(Succeed())
	})

	// After all Arena tests, clean up resources
	AfterAll(func() {
		if skipCleanup {
			_, _ = fmt.Fprintf(GinkgoWriter, "Skipping Arena cleanup (E2E_SKIP_CLEANUP=true)\n")
			return
		}

		By("cleaning up arena namespace")
		cmd := exec.Command("kubectl", "delete", "ns", arenaNamespace, "--ignore-not-found", "--timeout=120s")
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace, "--ignore-not-found", "--timeout=60s")
		_, _ = utils.Run(cmd)
	})

	// Helper to dump debug info on failure
	dumpArenaDebugInfo := func(reason string) {
		_, _ = fmt.Fprintf(GinkgoWriter, "\n=== DEBUG: %s ===\n", reason)

		// Get all pods in arena namespace
		cmd := exec.Command("kubectl", "get", "pods", "-n", arenaNamespace, "-o", "wide")
		output, _ := utils.Run(cmd)
		_, _ = fmt.Fprintf(GinkgoWriter, "Pods:\n%s\n", output)

		// Get all Arena resources
		for _, resource := range []string{"arenasource", "arenaconfig", "arenajob", "provider"} {
			cmd = exec.Command("kubectl", "get", resource, "-n", arenaNamespace, "-o", "yaml")
			output, _ = utils.Run(cmd)
			_, _ = fmt.Fprintf(GinkgoWriter, "%s:\n%s\n", resource, output)
		}

		// Get events
		cmd = exec.Command("kubectl", "get", "events", "-n", arenaNamespace, "--sort-by=.lastTimestamp")
		output, _ = utils.Run(cmd)
		_, _ = fmt.Fprintf(GinkgoWriter, "Events:\n%s\n", output)

		// Get controller logs
		cmd = exec.Command("kubectl", "logs", "-n", namespace,
			"-l", "control-plane=controller-manager", "--tail=100")
		output, _ = utils.Run(cmd)
		_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n%s\n", output)
	}

	// After each test, check for failures and dump debug info
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			dumpArenaDebugInfo(specReport.FullText())
		}
	})

	SetDefaultEventuallyTimeout(3 * time.Minute)
	SetDefaultEventuallyPollingInterval(2 * time.Second)

	Context("ArenaSource", func() {
		It("should create and reconcile an ArenaSource from Git", func() {
			By("creating the ArenaSource pointing to promptkit GitHub repo")
			arenaSourceManifest := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaSource
metadata:
  name: assertions-test-source
  namespace: %s
spec:
  type: git
  git:
    url: %s
    ref:
      branch: main
    path: %s
  interval: 5m
  timeout: 5m
`, arenaNamespace, promptKitGitURL, promptKitExamplePath)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(arenaSourceManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ArenaSource")

			By("verifying the ArenaSource status becomes Ready")
			verifyArenaSourceReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "arenasource", "assertions-test-source",
					"-n", arenaNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Ready"), "ArenaSource should be Ready, got: "+output)
			}
			Eventually(verifyArenaSourceReady, 6*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying the ArenaSource has an artifact")
			cmd = exec.Command("kubectl", "get", "arenasource", "assertions-test-source",
				"-n", arenaNamespace, "-o", "jsonpath={.status.artifact.revision}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).NotTo(BeEmpty(), "ArenaSource should have an artifact revision")
			_, _ = fmt.Fprintf(GinkgoWriter, "ArenaSource artifact revision: %s\n", output)

			By("verifying the ArenaSource has an artifact URL")
			cmd = exec.Command("kubectl", "get", "arenasource", "assertions-test-source",
				"-n", arenaNamespace, "-o", "jsonpath={.status.artifact.url}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).NotTo(BeEmpty(), "ArenaSource should have an artifact URL")
			_, _ = fmt.Fprintf(GinkgoWriter, "ArenaSource artifact URL: %s\n", output)
		})
	})

	Context("ArenaConfig", func() {
		It("should create and validate an ArenaConfig with mock provider", func() {
			By("creating a Provider resource for mock testing")
			providerManifest := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: test-mock-provider
  namespace: %s
spec:
  type: mock
  model: mock-model
  defaults:
    temperature: "0.7"
    maxTokens: 1000
`, arenaNamespace)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(providerManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create Provider")

			By("waiting for Provider to be Ready")
			verifyProviderReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "provider", "test-mock-provider",
					"-n", arenaNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Ready"), "Provider should be Ready, got: "+output)
			}
			Eventually(verifyProviderReady, time.Minute, time.Second).Should(Succeed())

			By("creating the ArenaConfig")
			arenaConfigManifest := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaConfig
metadata:
  name: assertions-test-config
  namespace: %s
spec:
  sourceRef:
    name: assertions-test-source
  scenarios:
    include:
      - "scenarios/*.yaml"
  providers:
    - name: test-mock-provider
  evaluation:
    concurrency: 2
    timeout: "60s"
`, arenaNamespace)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(arenaConfigManifest)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ArenaConfig")

			By("verifying the ArenaConfig status becomes Ready")
			verifyArenaConfigReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "arenaconfig", "assertions-test-config",
					"-n", arenaNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Ready"), "ArenaConfig should be Ready, got: "+output)
			}
			Eventually(verifyArenaConfigReady, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying the ArenaConfig resolved the source")
			cmd = exec.Command("kubectl", "get", "arenaconfig", "assertions-test-config",
				"-n", arenaNamespace, "-o", "jsonpath={.status.resolvedSource.revision}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).NotTo(BeEmpty(), "ArenaConfig should have resolved source revision")
			_, _ = fmt.Fprintf(GinkgoWriter, "ArenaConfig resolved source revision: %s\n", output)
		})
	})

	Context("Basic Workflow Test", func() {
		It("should complete a basic Arena job with mock provider", func() {
			By("creating an ArenaJob")
			arenaJobManifest := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: assertions-test-job
  namespace: %s
spec:
  configRef:
    name: assertions-test-config
  type: evaluation
  workers:
    replicas: 1
`, arenaNamespace)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(arenaJobManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ArenaJob")

			By("verifying the ArenaJob status becomes Running")
			verifyJobRunning := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "arenajob", "assertions-test-job",
					"-n", arenaNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				// Job should progress from Pending to Running
				g.Expect(output).To(Or(Equal("Running"), Equal("Pending")),
					"Job should be Running or Pending, got: "+output)
			}
			Eventually(verifyJobRunning, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying worker pods are created")
			verifyWorkerPods := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "-n", arenaNamespace,
					"-l", "app.kubernetes.io/component=worker",
					"-o", "jsonpath={.items[*].metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).NotTo(BeEmpty(), "Should have worker pods")
			}
			Eventually(verifyWorkerPods, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("waiting for the ArenaJob to complete")
			verifyJobCompleted := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "arenajob", "assertions-test-job",
					"-n", arenaNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				// Job should complete (Succeeded or Failed)
				g.Expect(output).To(Or(Equal("Succeeded"), Equal("Failed")),
					"Job should be Succeeded or Failed, got: "+output)
			}
			Eventually(verifyJobCompleted, 5*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying the job completed successfully")
			cmd = exec.Command("kubectl", "get", "arenajob", "assertions-test-job",
				"-n", arenaNamespace, "-o", "jsonpath={.status.phase}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("Succeeded"), "ArenaJob should have succeeded")

			By("verifying job progress was tracked")
			cmd = exec.Command("kubectl", "get", "arenajob", "assertions-test-job",
				"-n", arenaNamespace, "-o", "jsonpath={.status.progress.completed}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).NotTo(BeEmpty(), "Job should have completed items")
			_, _ = fmt.Fprintf(GinkgoWriter, "ArenaJob completed items: %s\n", output)

			By("verifying job has result summary")
			cmd = exec.Command("kubectl", "get", "arenajob", "assertions-test-job",
				"-n", arenaNamespace, "-o", "jsonpath={.status.result}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			_, _ = fmt.Fprintf(GinkgoWriter, "ArenaJob result: %s\n", output)
		})
	})

	Context("Multi-Worker Test", func() {
		It("should process work items across multiple workers", func() {
			By("creating an ArenaJob with multiple workers")
			arenaJobManifest := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: multi-worker-test-job
  namespace: %s
spec:
  configRef:
    name: assertions-test-config
  type: evaluation
  workers:
    replicas: 3
`, arenaNamespace)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(arenaJobManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create multi-worker ArenaJob")

			By("verifying multiple worker pods are created")
			verifyMultipleWorkers := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "-n", arenaNamespace,
					"-l", "app.kubernetes.io/component=worker,app.kubernetes.io/instance=multi-worker-test-job",
					"-o", "jsonpath={.items[*].metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				// Should have multiple worker pods
				workers := strings.Fields(output)
				g.Expect(len(workers)).To(BeNumerically(">=", 1),
					"Should have at least 1 worker pod, got: "+output)
			}
			Eventually(verifyMultipleWorkers, 3*time.Minute, 5*time.Second).Should(Succeed())

			By("waiting for the multi-worker job to complete")
			verifyJobCompleted := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "arenajob", "multi-worker-test-job",
					"-n", arenaNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Or(Equal("Succeeded"), Equal("Failed")),
					"Job should be Succeeded or Failed, got: "+output)
			}
			Eventually(verifyJobCompleted, 5*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying the multi-worker job succeeded")
			cmd = exec.Command("kubectl", "get", "arenajob", "multi-worker-test-job",
				"-n", arenaNamespace, "-o", "jsonpath={.status.phase}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("Succeeded"), "Multi-worker ArenaJob should have succeeded")

			By("checking worker logs for work item processing")
			cmd = exec.Command("kubectl", "logs", "-n", arenaNamespace,
				"-l", "app.kubernetes.io/component=worker,app.kubernetes.io/instance=multi-worker-test-job",
				"--tail=50")
			output, _ = utils.Run(cmd)
			_, _ = fmt.Fprintf(GinkgoWriter, "Multi-worker job worker logs:\n%s\n", output)
		})
	})

	Context("Error Handling Test", func() {
		It("should handle invalid ArenaConfig gracefully", func() {
			By("creating an ArenaConfig with non-existent source")
			invalidConfigManifest := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaConfig
metadata:
  name: invalid-config
  namespace: %s
spec:
  sourceRef:
    name: non-existent-source
  providers:
    - name: test-mock-provider
`, arenaNamespace)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(invalidConfigManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create invalid ArenaConfig")

			By("verifying the ArenaConfig reports an error or invalid state")
			verifyConfigError := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "arenaconfig", "invalid-config",
					"-n", arenaNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				// Should be in Error, Invalid, or Pending state (not Ready)
				g.Expect(output).NotTo(Equal("Ready"),
					"Invalid config should not be Ready, got: "+output)
			}
			Eventually(verifyConfigError, time.Minute, 2*time.Second).Should(Succeed())

			By("cleaning up invalid config")
			cmd = exec.Command("kubectl", "delete", "arenaconfig", "invalid-config",
				"-n", arenaNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("should handle ArenaJob with invalid config reference", func() {
			By("creating an ArenaJob with non-existent config")
			invalidJobManifest := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: invalid-job
  namespace: %s
spec:
  configRef:
    name: non-existent-config
  type: evaluation
`, arenaNamespace)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(invalidJobManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create invalid ArenaJob")

			By("verifying the ArenaJob reports an error state")
			verifyJobError := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "arenajob", "invalid-job",
					"-n", arenaNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				// Should be in Failed or Pending state (not Succeeded or Running)
				g.Expect(output).NotTo(Equal("Succeeded"),
					"Invalid job should not succeed, got: "+output)
				g.Expect(output).NotTo(Equal("Running"),
					"Invalid job should not be running, got: "+output)
			}
			Eventually(verifyJobError, time.Minute, 2*time.Second).Should(Succeed())

			By("cleaning up invalid job")
			cmd = exec.Command("kubectl", "delete", "arenajob", "invalid-job",
				"-n", arenaNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})
	})
})
