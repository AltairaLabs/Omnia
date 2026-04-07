//go:build e2e
// +build e2e

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
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/altairalabs/omnia/test/utils"
)

// rolloutNamespace is the namespace used for rollout E2E tests.
// It reuses the agents namespace that the CRD Ordered container sets up.
const rolloutNamespace = "test-agents"

// rolloutAgentName is the name of the AgentRuntime used for rollout tests.
const rolloutAgentName = "rollout-agent"

// rolloutCandidateDeployment is the expected name of the candidate Deployment.
const rolloutCandidateDeployment = "rollout-agent-canary"

// isIstioNetworkingCRDInstalled checks if the Istio VirtualService CRD exists.
func isIstioNetworkingCRDInstalled() bool {
	cmd := exec.Command("kubectl", "get", "crd", "virtualservices.networking.istio.io")
	_, err := utils.Run(cmd)
	return err == nil
}

// installIstioNetworkingCRDs installs minimal Istio networking CRDs
// (VirtualService and DestinationRule) so the API server accepts them.
// No Istio control plane is needed.
func installIstioNetworkingCRDs() error {
	crds := `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: virtualservices.networking.istio.io
spec:
  group: networking.istio.io
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          x-kubernetes-preserve-unknown-fields: true
  scope: Namespaced
  names:
    plural: virtualservices
    singular: virtualservice
    kind: VirtualService
    shortNames:
      - vs
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: destinationrules.networking.istio.io
spec:
  group: networking.istio.io
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          x-kubernetes-preserve-unknown-fields: true
  scope: Namespaced
  names:
    plural: destinationrules
    singular: destinationrule
    kind: DestinationRule
    shortNames:
      - dr
`
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(crds)
	_, err := utils.Run(cmd)
	return err
}

// applyRolloutManifest applies a YAML manifest and expects success.
func applyRolloutManifest(yaml string) {
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(yaml)
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to apply rollout manifest")
}

// deleteRolloutResource deletes a resource by type, name, and namespace.
func deleteRolloutResource(resource, name, ns string) {
	cmd := exec.Command("kubectl", "delete", resource, name, "-n", ns,
		"--ignore-not-found", "--timeout=30s")
	_, _ = utils.Run(cmd)
}

// getRolloutJSONPath returns the value of a JSONPath expression for a resource.
func getRolloutJSONPath(resource, name, ns, jsonpath string) (string, error) {
	cmd := exec.Command("kubectl", "get", resource, name, "-n", ns,
		"-o", fmt.Sprintf("jsonpath=%s", jsonpath))
	return utils.Run(cmd)
}

// dumpRolloutDebugInfo logs diagnostic information for debugging rollout test failures.
func dumpRolloutDebugInfo(reason string) {
	_, _ = fmt.Fprintf(GinkgoWriter, "\n=== ROLLOUT DEBUG: %s ===\n", reason)

	arCmd := exec.Command("kubectl", "get", "agentruntime", rolloutAgentName,
		"-n", rolloutNamespace, "-o", "yaml")
	if out, err := utils.Run(arCmd); err == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "AgentRuntime yaml:\n%s\n", out)
	}

	deploys := exec.Command("kubectl", "get", "deployments",
		"-n", rolloutNamespace, "-o", "wide")
	if out, err := utils.Run(deploys); err == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "Deployments:\n%s\n", out)
	}

	eventsCmd := exec.Command("kubectl", "get", "events", "-n", rolloutNamespace,
		"--sort-by=.lastTimestamp")
	if out, err := utils.Run(eventsCmd); err == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "Events:\n%s\n", out)
	}

	logsCmd := exec.Command("kubectl", "logs",
		"-n", "omnia-system",
		"-l", "control-plane=controller-manager",
		"--tail=200", "--all-containers=true")
	if out, err := utils.Run(logsCmd); err == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n%s\n", out)
	}
}

var _ = Describe("Rollout E2E", func() {
	const promptPackName = "rollout-prompts"

	BeforeEach(func() {
		By("ensuring rollout test namespace exists")
		cmd := exec.Command("kubectl", "create", "ns", rolloutNamespace,
			"--dry-run=client", "-o", "yaml")
		yaml, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(yaml)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("Rollout lifecycle", Ordered, func() {
		BeforeAll(func() {
			By("checking if operator is deployed")
			cmd := exec.Command("kubectl", "get", "deployment", "omnia-controller-manager", "-n", "omnia-system")
			if _, err := utils.Run(cmd); err != nil {
				Skip("Operator not deployed — rollout E2E tests require a running operator")
			}

			By("checking that the AgentRuntime CRD is installed")
			cmd = exec.Command("kubectl", "get", "crd", "agentruntimes.omnia.altairalabs.ai")
			if _, err := utils.Run(cmd); err != nil {
				Skip("AgentRuntime CRD not installed")
			}

			By("creating PromptPack resources for rollout tests")
			applyRolloutManifest(fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s-v1
  namespace: %s
data:
  pack.json: |
    {
      "id": "%s",
      "name": "%s",
      "version": "1.0.0",
      "template_engine": {"version": "v1", "syntax": "{{variable}}"},
      "prompts": {
        "default": {
          "id": "default",
          "name": "default",
          "version": "1.0.0",
          "system_template": "You are the stable version."
        }
      }
    }
`, promptPackName, rolloutNamespace, promptPackName, promptPackName))

			applyRolloutManifest(fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: PromptPack
metadata:
  name: %s
  namespace: %s
spec:
  source:
    type: configmap
    configMapRef:
      name: %s-v1
  version: "1.0.0"
`, promptPackName, rolloutNamespace, promptPackName))

			By("waiting for PromptPack to become Active")
			Eventually(func(g Gomega) {
				output, err := getRolloutJSONPath("promptpack", promptPackName, rolloutNamespace, "{.status.phase}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Active"))
			}, 60*time.Second, 2*time.Second).Should(Succeed())

			By("creating provider secret and Provider for rollout tests")
			cmd = exec.Command("kubectl", "create", "secret", "generic", "rollout-provider",
				"-n", rolloutNamespace,
				"--from-literal=api-key=rollout-test-key",
				"--dry-run=client", "-o", "yaml")
			secretYaml, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(secretYaml)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			applyRolloutManifest(fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: rollout-provider
  namespace: %s
spec:
  type: mock
  secretRef:
    name: rollout-provider
`, rolloutNamespace))
		})

		AfterAll(func() {
			if CurrentSpecReport().Failed() {
				dumpRolloutDebugInfo("rollout lifecycle test failure")
				return
			}

			By("cleaning up rollout test resources")
			deleteRolloutResource("agentruntime", rolloutAgentName, rolloutNamespace)
			deleteRolloutResource("promptpack", promptPackName, rolloutNamespace)
			deleteRolloutResource("provider", "rollout-provider", rolloutNamespace)
			deleteRolloutResource("configmap", promptPackName+"-v1", rolloutNamespace)
			deleteRolloutResource("secret", "rollout-provider", rolloutNamespace)
		})

		It("should create candidate Deployment when rollout candidate is set", func() {
			By("creating an AgentRuntime with a rollout candidate")
			candidateVersion := "2.0.0"
			applyRolloutManifest(fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: %s
  namespace: %s
  annotations:
    omnia.altairalabs.ai/mock-provider: "true"
spec:
  promptPackRef:
    name: %s
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
  providers:
    - name: default
      providerRef:
        name: rollout-provider
  rollout:
    candidate:
      promptPackVersion: "%s"
    steps:
      - setWeight: 20
      - pause:
          duration: "10m"
      - setWeight: 50
      - pause:
          duration: "10m"
      - setWeight: 100
`, rolloutAgentName, rolloutNamespace, promptPackName, candidateVersion))

			DeferCleanup(func() {
				if CurrentSpecReport().Failed() {
					dumpRolloutDebugInfo("candidate Deployment creation failure")
				}
			})

			By("verifying the stable Deployment is created")
			Eventually(func(g Gomega) {
				output, err := getRolloutJSONPath("deployment", rolloutAgentName, rolloutNamespace, "{.metadata.name}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal(rolloutAgentName))
			}, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying the candidate (canary) Deployment is created")
			Eventually(func(g Gomega) {
				output, err := getRolloutJSONPath("deployment", rolloutCandidateDeployment, rolloutNamespace, "{.metadata.name}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal(rolloutCandidateDeployment))
			}, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying the rollout status is active")
			Eventually(func(g Gomega) {
				output, err := getRolloutJSONPath("agentruntime", rolloutAgentName, rolloutNamespace, "{.status.rollout.active}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying the rollout status tracks candidate version")
			Eventually(func(g Gomega) {
				output, err := getRolloutJSONPath("agentruntime", rolloutAgentName, rolloutNamespace, "{.status.rollout.candidateVersion}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal(candidateVersion))
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying the rollout current weight matches the first step")
			Eventually(func(g Gomega) {
				output, err := getRolloutJSONPath("agentruntime", rolloutAgentName, rolloutNamespace, "{.status.rollout.currentWeight}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("20"))
			}, 2*time.Minute, 2*time.Second).Should(Succeed())
		})

		It("should clean up candidate Deployment when rollout is removed", func() {
			By("removing the rollout from the AgentRuntime")
			cmd := exec.Command("kubectl", "patch", "agentruntime", rolloutAgentName,
				"-n", rolloutNamespace, "--type=merge",
				"-p", `{"spec":{"rollout":null}}`)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the candidate Deployment is deleted")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "deployment", rolloutCandidateDeployment,
					"-n", rolloutNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred(), "candidate Deployment should be deleted")
			}, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying the stable Deployment still exists")
			output, err := getRolloutJSONPath("deployment", rolloutAgentName, rolloutNamespace, "{.metadata.name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal(rolloutAgentName))

			By("verifying the rollout status is cleared")
			Eventually(func(g Gomega) {
				output, err := getRolloutJSONPath("agentruntime", rolloutAgentName, rolloutNamespace, "{.status.rollout.active}")
				g.Expect(err).NotTo(HaveOccurred())
				// When rollout is removed, active should be false or empty
				g.Expect(output).To(SatisfyAny(Equal("false"), BeEmpty()))
			}, 60*time.Second, 2*time.Second).Should(Succeed())
		})
	})

	Context("Istio traffic routing", Ordered, func() {
		const vsName = "rollout-vs"
		const drName = "rollout-dr"
		const istioAgentName = "rollout-istio-agent"
		const istioCandidateDeployment = "rollout-istio-agent-canary"
		const istioPromptPack = "rollout-istio-prompts"

		BeforeAll(func() {
			By("checking if operator is deployed")
			cmd := exec.Command("kubectl", "get", "deployment", "omnia-controller-manager", "-n", "omnia-system")
			if _, err := utils.Run(cmd); err != nil {
				Skip("Operator not deployed — rollout E2E tests require a running operator")
			}

			if !isIstioNetworkingCRDInstalled() {
				By("installing Istio networking CRDs")
				err := installIstioNetworkingCRDs()
				if err != nil {
					Skip("Failed to install Istio networking CRDs: " + err.Error())
				}

				// Verify they are available
				Eventually(func() bool {
					return isIstioNetworkingCRDInstalled()
				}, 30*time.Second, time.Second).Should(BeTrue(), "Istio CRDs not available after install")
			}

			By("creating PromptPack for Istio rollout tests")
			applyRolloutManifest(fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s-v1
  namespace: %s
data:
  pack.json: |
    {
      "id": "%s",
      "name": "%s",
      "version": "1.0.0",
      "template_engine": {"version": "v1", "syntax": "{{variable}}"},
      "prompts": {
        "default": {
          "id": "default",
          "name": "default",
          "version": "1.0.0",
          "system_template": "Istio rollout stable version."
        }
      }
    }
`, istioPromptPack, rolloutNamespace, istioPromptPack, istioPromptPack))

			applyRolloutManifest(fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: PromptPack
metadata:
  name: %s
  namespace: %s
spec:
  source:
    type: configmap
    configMapRef:
      name: %s-v1
  version: "1.0.0"
`, istioPromptPack, rolloutNamespace, istioPromptPack))

			Eventually(func(g Gomega) {
				output, err := getRolloutJSONPath("promptpack", istioPromptPack, rolloutNamespace, "{.status.phase}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Active"))
			}, 60*time.Second, 2*time.Second).Should(Succeed())

			By("creating provider for Istio rollout tests")
			cmd = exec.Command("kubectl", "create", "secret", "generic", "rollout-istio-provider",
				"-n", rolloutNamespace,
				"--from-literal=api-key=istio-rollout-test-key",
				"--dry-run=client", "-o", "yaml")
			secretYaml, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(secretYaml)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			applyRolloutManifest(fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: rollout-istio-provider
  namespace: %s
spec:
  type: mock
  secretRef:
    name: rollout-istio-provider
`, rolloutNamespace))

			By("creating a VirtualService with 100/0 stable/canary weights")
			applyRolloutManifest(fmt.Sprintf(`
apiVersion: networking.istio.io/v1
kind: VirtualService
metadata:
  name: %s
  namespace: %s
spec:
  hosts:
    - %s.%s.svc.cluster.local
  http:
    - name: primary
      route:
        - destination:
            host: %s.%s.svc.cluster.local
            subset: stable
          weight: 100
        - destination:
            host: %s.%s.svc.cluster.local
            subset: canary
          weight: 0
`, vsName, rolloutNamespace,
				istioAgentName, rolloutNamespace,
				istioAgentName, rolloutNamespace,
				istioAgentName, rolloutNamespace))

			By("creating a DestinationRule with stable and canary subsets")
			applyRolloutManifest(fmt.Sprintf(`
apiVersion: networking.istio.io/v1
kind: DestinationRule
metadata:
  name: %s
  namespace: %s
spec:
  host: %s.%s.svc.cluster.local
  subsets:
    - name: stable
      labels:
        app.kubernetes.io/instance: %s
    - name: canary
      labels:
        app.kubernetes.io/instance: %s-canary
`, drName, rolloutNamespace,
				istioAgentName, rolloutNamespace,
				istioAgentName, istioAgentName))
		})

		AfterAll(func() {
			if CurrentSpecReport().Failed() {
				dumpRolloutDebugInfo("Istio rollout test failure")

				vsCmd := exec.Command("kubectl", "get", "virtualservice", vsName,
					"-n", rolloutNamespace, "-o", "yaml")
				if out, err := utils.Run(vsCmd); err == nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "VirtualService yaml:\n%s\n", out)
				}
				return
			}

			deleteRolloutResource("agentruntime", istioAgentName, rolloutNamespace)
			deleteRolloutResource("virtualservice", vsName, rolloutNamespace)
			deleteRolloutResource("destinationrule", drName, rolloutNamespace)
			deleteRolloutResource("promptpack", istioPromptPack, rolloutNamespace)
			deleteRolloutResource("configmap", istioPromptPack+"-v1", rolloutNamespace)
			deleteRolloutResource("provider", "rollout-istio-provider", rolloutNamespace)
			deleteRolloutResource("secret", "rollout-istio-provider", rolloutNamespace)
		})

		It("should patch VirtualService weights during rollout", func() {
			By("creating an AgentRuntime with Istio traffic routing")
			candidateVersion := "2.0.0"
			applyRolloutManifest(fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: %s
  namespace: %s
  annotations:
    omnia.altairalabs.ai/mock-provider: "true"
spec:
  promptPackRef:
    name: %s
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
  providers:
    - name: default
      providerRef:
        name: rollout-istio-provider
  rollout:
    candidate:
      promptPackVersion: "%s"
    steps:
      - setWeight: 30
      - pause:
          duration: "30m"
      - setWeight: 100
    trafficRouting:
      istio:
        virtualService:
          name: %s
          routes:
            - primary
        destinationRule:
          name: %s
`, istioAgentName, rolloutNamespace, istioPromptPack, candidateVersion, vsName, drName))

			DeferCleanup(func() {
				if CurrentSpecReport().Failed() {
					dumpRolloutDebugInfo("Istio VirtualService weight patching failure")
				}
			})

			By("verifying the candidate Deployment is created")
			Eventually(func(g Gomega) {
				output, err := getRolloutJSONPath("deployment", istioCandidateDeployment, rolloutNamespace, "{.metadata.name}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal(istioCandidateDeployment))
			}, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying the VirtualService weights were updated")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "virtualservice", vsName,
					"-n", rolloutNamespace,
					"-o", "jsonpath={.spec.http[0].route[1].weight}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				// The canary route weight should be > 0 after the first step
				g.Expect(output).To(Equal("30"),
					"VirtualService canary weight should be 30 after first rollout step")
			}, 2*time.Minute, 2*time.Second).Should(Succeed())

			By("verifying the stable route weight is complementary")
			cmd := exec.Command("kubectl", "get", "virtualservice", vsName,
				"-n", rolloutNamespace,
				"-o", "jsonpath={.spec.http[0].route[0].weight}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("70"),
				"VirtualService stable weight should be 70 (100 - 30)")
		})
	})
})
