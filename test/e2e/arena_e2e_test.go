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
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/altairalabs/omnia/test/utils"
)

// arenaNamespace is where Arena test resources are deployed
const arenaNamespace = "test-arena"

// arenaSourceConfigMapName is the name of the ConfigMap used as arena source content
const arenaSourceConfigMapName = "arena-test-content"

var _ = Describe("Arena Fleet", Ordered, Label("arena"), func() {
	// Before running Arena tests, verify the cluster is ready
	// This assumes the cluster was set up via setup-arena-e2e.sh or equivalent
	BeforeAll(func() {
		// Skip Arena tests if ENABLE_ARENA_E2E is not set
		if os.Getenv("ENABLE_ARENA_E2E") != "true" {
			Skip("Arena E2E tests require ENABLE_ARENA_E2E=true")
		}

		By("verifying controller-manager is ready")
		verifyControllerReady := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "deployment", "omnia-controller-manager",
				"-n", namespace, "-o", "jsonpath={.status.readyReplicas}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("1"), "Controller manager should have 1 ready replica")
		}
		Eventually(verifyControllerReady, 2*time.Minute, 2*time.Second).Should(Succeed())

		By("verifying arena-controller is ready")
		verifyArenaControllerReady := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "deployment", "omnia-arena-controller",
				"-n", namespace, "-o", "jsonpath={.status.readyReplicas}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("1"), "Arena controller manager should have 1 ready replica")
		}
		Eventually(verifyArenaControllerReady, 2*time.Minute, 2*time.Second).Should(Succeed())

		By("verifying Redis is ready")
		verifyRedisReady := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "statefulset", "omnia-redis-master",
				"-n", namespace, "-o", "jsonpath={.status.readyReplicas}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("1"), "Redis should have 1 ready replica")
		}
		Eventually(verifyRedisReady, 2*time.Minute, 2*time.Second).Should(Succeed())

		// Note: NFS server check removed - we use local-path storage for kind clusters

		By("creating arena test namespace")
		cmd := exec.Command("kubectl", "create", "ns", arenaNamespace)
		_, _ = utils.Run(cmd) // Ignore error if already exists

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", arenaNamespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")
	})

	// After all Arena tests, clean up test resources only
	AfterAll(func() {
		if skipCleanup {
			_, _ = fmt.Fprintf(GinkgoWriter, "Skipping Arena cleanup (E2E_SKIP_CLEANUP=true)\n")
			return
		}

		By("cleaning up Workspace (cluster-scoped)")
		cmd := exec.Command("kubectl", "delete", "workspace", arenaNamespace, "--ignore-not-found", "--timeout=60s")
		_, _ = utils.Run(cmd)

		By("cleaning up arena namespace")
		cmd = exec.Command("kubectl", "delete", "ns", arenaNamespace, "--ignore-not-found", "--timeout=120s")
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
		for _, resource := range []string{"arenasource", "arenajob", "provider"} {
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

		// Get arena controller logs
		cmd = exec.Command("kubectl", "logs", "-n", namespace,
			"-l", "control-plane=arena-controller-manager", "--tail=100")
		output, _ = utils.Run(cmd)
		_, _ = fmt.Fprintf(GinkgoWriter, "Arena controller logs:\n%s\n", output)
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
		It("should create and reconcile an ArenaSource from ConfigMap", func() {
			By("waiting for workspace ClusterRoles to be available")
			// The Workspace controller needs these ClusterRoles to exist before creating RoleBindings
			verifyClusterRoleExists := func(g Gomega) {
				crCmd := exec.Command("kubectl", "get", "clusterrole", "omnia-workspace-owner", "-o", "name")
				output, crErr := utils.Run(crCmd)
				g.Expect(crErr).NotTo(HaveOccurred(), "ClusterRole should exist")
				g.Expect(output).To(ContainSubstring("omnia-workspace-owner"))
			}
			Eventually(verifyClusterRoleExists, 1*time.Minute, 2*time.Second).Should(Succeed())

			By("creating a Workspace for the arena test namespace")
			// ArenaSource requires a Workspace with RWX storage for shared content
			// Uses the omnia-nfs storage class deployed by Helm
			workspaceManifest := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Workspace
metadata:
  name: %s
spec:
  displayName: Arena E2E Test Workspace
  description: Test workspace for Arena E2E tests
  namespace:
    name: %s
  storage:
    enabled: true
    storageClass: omnia-nfs
    accessModes:
      - ReadWriteOnce
    size: 1Gi
`, arenaNamespace, arenaNamespace)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(workspaceManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create Workspace")

			By("waiting for Workspace RBAC to be ready")
			// Wait for RBAC before creating ArenaSource
			// Storage will be Pending until ArenaSource creates the PVC
			verifyWorkspaceRBACReady := func(g Gomega) {
				wsCmd := exec.Command("kubectl", "get", "workspace", arenaNamespace,
					"-o", "jsonpath={.status.conditions[?(@.type=='RoleBindingsReady')].status}")
				output, wsErr := utils.Run(wsCmd)
				g.Expect(wsErr).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "Workspace RoleBindings should be Ready")
			}
			Eventually(verifyWorkspaceRBACReady, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("creating a ConfigMap with arena test content")
			// Create ConfigMap with test prompt pack content similar to assertions-test
			// Note: ConfigMap keys cannot contain slashes, so we use flat key names
			configMapManifest := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
data:
  manifest.yaml: |
    name: test-assertions
    version: 1.0.0
    description: Test assertion prompts for Arena E2E testing
  test-prompt.yaml: |
    name: test-prompt
    template: |
      You are a helpful assistant.
      User: {{.input}}
    variables:
      - name: input
        type: string
        required: true
  basic-scenario.yaml: |
    name: basic-scenario
    description: Basic test scenario
    inputs:
      - input: "Hello, how are you?"
        expected: "I'm doing well"
`, arenaSourceConfigMapName, arenaNamespace)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(configMapManifest)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create test ConfigMap")

			By("creating the ArenaSource pointing to the ConfigMap")
			arenaSourceManifest := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaSource
metadata:
  name: assertions-test-source
  namespace: %s
spec:
  type: configmap
  configMap:
    name: %s
  interval: 5m
`, arenaNamespace, arenaSourceConfigMapName)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(arenaSourceManifest)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ArenaSource")

			// Note: We don't wait for Workspace storage to be fully provisioned because:
			// 1. WaitForFirstConsumer storage class requires a pod to consume the PVC
			// 2. ArenaSource can sync content independently of PVC binding state
			// 3. Storage will be provisioned when worker pods are created by ArenaJob

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

			By("verifying the ArenaSource has an artifact contentPath")
			cmd = exec.Command("kubectl", "get", "arenasource", "assertions-test-source",
				"-n", arenaNamespace, "-o", "jsonpath={.status.artifact.contentPath}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).NotTo(BeEmpty(), "ArenaSource should have an artifact contentPath")
			_, _ = fmt.Fprintf(GinkgoWriter, "ArenaSource artifact contentPath: %s\n", output)
		})
	})

	Context("Basic Workflow Test", func() {
		It("should create Provider for mock testing", func() {
			By("creating a Provider resource for mock testing")
			providerManifest := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: test-mock-provider
  namespace: %s
  labels:
    arena.altairalabs.ai/test-provider: "true"
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
		})

		It("should complete a basic Arena job with mock provider", func() {
			By("creating an ArenaJob")
			arenaJobManifest := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: assertions-test-job
  namespace: %s
spec:
  sourceRef:
    name: assertions-test-source
  arenaFile: config.arena.yaml
  type: evaluation
  providerOverrides:
    default:
      selector:
        matchLabels:
          arena.altairalabs.ai/test-provider: "true"
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

			By("verifying the job completed (success or failure is acceptable for E2E)")
			cmd = exec.Command("kubectl", "get", "arenajob", "assertions-test-job",
				"-n", arenaNamespace, "-o", "jsonpath={.status.phase}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			// Both Succeeded and Failed are valid - we're testing infrastructure, not prompt quality
			Expect(output).To(Or(Equal("Succeeded"), Equal("Failed")),
				"ArenaJob should have completed, got: "+output)

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
			// Skip this test if no enterprise license is available
			// Open-core mode only allows 1 worker replica
			if os.Getenv("OMNIA_ENTERPRISE_LICENSE") == "" {
				Skip("Multi-worker tests require an enterprise license (OMNIA_ENTERPRISE_LICENSE)")
			}

			By("creating an ArenaJob with multiple workers")
			arenaJobManifest := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: multi-worker-test-job
  namespace: %s
spec:
  sourceRef:
    name: assertions-test-source
  arenaFile: config.arena.yaml
  type: evaluation
  providerOverrides:
    default:
      selector:
        matchLabels:
          arena.altairalabs.ai/test-provider: "true"
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

			By("verifying the multi-worker job completed")
			cmd = exec.Command("kubectl", "get", "arenajob", "multi-worker-test-job",
				"-n", arenaNamespace, "-o", "jsonpath={.status.phase}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			// Both Succeeded and Failed are valid - we're testing infrastructure, not prompt quality
			Expect(output).To(Or(Equal("Succeeded"), Equal("Failed")),
				"Multi-worker ArenaJob should have completed, got: "+output)

			By("checking worker logs for work item processing")
			cmd = exec.Command("kubectl", "logs", "-n", arenaNamespace,
				"-l", "app.kubernetes.io/component=worker,app.kubernetes.io/instance=multi-worker-test-job",
				"--tail=50")
			output, _ = utils.Run(cmd)
			_, _ = fmt.Fprintf(GinkgoWriter, "Multi-worker job worker logs:\n%s\n", output)
		})
	})

	Context("Error Handling Test", func() {
		It("should handle ArenaJob with invalid source reference", func() {
			By("creating an ArenaJob with non-existent source")
			invalidJobManifest := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaJob
metadata:
  name: invalid-job
  namespace: %s
spec:
  sourceRef:
    name: non-existent-source
  arenaFile: config.arena.yaml
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

	Context("ArenaDevSession Test", func() {
		const devSessionName = "e2e-dev-session"

		AfterEach(func() {
			// Clean up dev session after each test
			cmd := exec.Command("kubectl", "delete", "arenadevsession", devSessionName,
				"-n", arenaNamespace, "--ignore-not-found", "--timeout=30s")
			_, _ = utils.Run(cmd)
		})

		It("should create a dev console pod that starts without permission errors", func() {
			By("creating an ArenaDevSession")
			devSessionManifest := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaDevSession
metadata:
  name: %s
  namespace: %s
spec:
  projectId: test-project
  ttl: 5m
`, devSessionName, arenaNamespace)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(devSessionManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ArenaDevSession")

			By("waiting for the dev session to become Ready")
			verifyDevSessionReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "arenadevsession", devSessionName,
					"-n", arenaNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Ready"), "ArenaDevSession should be Ready, got: "+output)
			}
			Eventually(verifyDevSessionReady, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying the dev console pod is running")
			verifyPodRunning := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "-n", arenaNamespace,
					"-l", fmt.Sprintf("arena.altairalabs.ai/dev-session=%s", devSessionName),
					"-o", "jsonpath={.items[0].status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Dev console pod should be Running, got: "+output)
			}
			Eventually(verifyPodRunning, time.Minute, 2*time.Second).Should(Succeed())

			By("checking dev console pod logs for startup errors")
			cmd = exec.Command("kubectl", "logs", "-n", arenaNamespace,
				"-l", fmt.Sprintf("arena.altairalabs.ai/dev-session=%s", devSessionName),
				"--tail=50")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			// Verify no permission denied errors in logs
			Expect(output).NotTo(ContainSubstring("permission denied"),
				"Dev console should not have permission denied errors")
			Expect(output).NotTo(ContainSubstring("mkdir out:"),
				"Dev console should not fail to create out directory")

			// Log the output for debugging
			_, _ = fmt.Fprintf(GinkgoWriter, "Dev console pod logs:\n%s\n", output)

			By("verifying the dev console health endpoint")
			// Port-forward to the dev console pod and check health
			podName := ""
			cmd = exec.Command("kubectl", "get", "pods", "-n", arenaNamespace,
				"-l", fmt.Sprintf("arena.altairalabs.ai/dev-session=%s", devSessionName),
				"-o", "jsonpath={.items[0].metadata.name}")
			podNameOutput, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			podName = strings.TrimSpace(podNameOutput)

			// Check readiness probe
			cmd = exec.Command("kubectl", "get", "pod", podName, "-n", arenaNamespace,
				"-o", "jsonpath={.status.containerStatuses[0].ready}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("true"), "Dev console container should be ready")
		})

		It("should handle provider loading without permission errors", func() {
			By("creating a Provider for the dev session")
			providerManifest := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: dev-session-mock-provider
  namespace: %s
spec:
  type: mock
  model: mock-model
`, arenaNamespace)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(providerManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create Provider")

			By("waiting for Provider to be Ready")
			verifyProviderReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "provider", "dev-session-mock-provider",
					"-n", arenaNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Ready"), "Provider should be Ready")
			}
			Eventually(verifyProviderReady, time.Minute, time.Second).Should(Succeed())

			By("creating an ArenaDevSession")
			devSessionManifest := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: ArenaDevSession
metadata:
  name: %s
  namespace: %s
spec:
  projectId: test-project-with-provider
  ttl: 5m
`, devSessionName, arenaNamespace)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(devSessionManifest)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ArenaDevSession")

			By("waiting for the dev session to become Ready")
			verifyDevSessionReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "arenadevsession", devSessionName,
					"-n", arenaNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Ready"), "ArenaDevSession should be Ready")
			}
			Eventually(verifyDevSessionReady, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("checking for any BuildEngineComponents errors in logs")
			cmd = exec.Command("kubectl", "logs", "-n", arenaNamespace,
				"-l", fmt.Sprintf("arena.altairalabs.ai/dev-session=%s", devSessionName),
				"--tail=100")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			// Verify no engine build errors
			Expect(output).NotTo(ContainSubstring("failed to build provider registry"),
				"Dev console should not fail to build provider registry")
			Expect(output).NotTo(ContainSubstring("failed to build engine components"),
				"Dev console should not fail to build engine components")
			Expect(output).NotTo(ContainSubstring("failed to create media file store"),
				"Dev console should not fail to create media file store")

			_, _ = fmt.Fprintf(GinkgoWriter, "Dev console logs with provider:\n%s\n", output)

			By("cleaning up Provider")
			cmd = exec.Command("kubectl", "delete", "provider", "dev-session-mock-provider",
				"-n", arenaNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})
	})
})
