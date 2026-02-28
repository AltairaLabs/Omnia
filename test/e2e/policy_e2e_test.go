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
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/altairalabs/omnia/test/utils"
)

// isIstioCRDInstalled checks if the Istio AuthorizationPolicy CRD exists.
func isIstioCRDInstalled() bool {
	cmd := exec.Command("kubectl", "get", "crd", "authorizationpolicies.security.istio.io")
	_, err := utils.Run(cmd)
	return err == nil
}

// applyPolicy applies a YAML manifest and expects success.
func applyPolicy(yaml string) {
	ExpectWithOffset(1, utils.ApplyManifestWithValidation(yaml)).To(Succeed(), "Failed to apply policy manifest")
}

// deletePolicy deletes a resource by type, name, and namespace.
func deletePolicy(resource, name, ns string) {
	cmd := exec.Command("kubectl", "delete", resource, name, "-n", ns, "--ignore-not-found", "--timeout=30s")
	_, _ = utils.Run(cmd)
}

// getJSONPath returns the value of a JSONPath expression for a resource.
func getJSONPath(resource, name, ns, jsonpath string) (string, error) {
	cmd := exec.Command("kubectl", "get", resource, name, "-n", ns,
		"-o", fmt.Sprintf("jsonpath=%s", jsonpath))
	return utils.Run(cmd)
}

// waitForPhase polls until the resource reaches the expected phase.
func waitForPhase(resource, name, ns, expectedPhase string) {
	EventuallyWithOffset(1, func(g Gomega) {
		output, err := getJSONPath(resource, name, ns, "{.status.phase}")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(output).To(Equal(expectedPhase))
	}, 60*time.Second, 2*time.Second).Should(Succeed(),
		fmt.Sprintf("Timed out waiting for %s/%s to reach phase %s", resource, name, expectedPhase))
}

// conditionMessage extracts a condition message by type from status.conditions.
func conditionMessage(resource, name, ns, condType string) string {
	jsonpath := fmt.Sprintf(`{.status.conditions[?(@.type=="%s")].message}`, condType)
	output, err := getJSONPath(resource, name, ns, jsonpath)
	if err != nil {
		return ""
	}
	return output
}

var _ = Describe("Policy E2E", Ordered, func() {
	const policyNamespace = "test-agents"

	BeforeAll(func() {
		By("ensuring test-agents namespace exists")
		cmd := exec.Command("kubectl", "create", "ns", policyNamespace, "--dry-run=client", "-o", "yaml")
		yaml, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(yaml)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("ToolPolicy", func() {
		const toolPolicyName = "e2e-tool-policy"

		AfterEach(func() {
			deletePolicy("toolpolicy", toolPolicyName, policyNamespace)
		})

		It("should compile CEL rules and set Active phase", func() {
			By("applying a valid ToolPolicy")
			applyPolicy(fmt.Sprintf(`
apiVersion: ee.omnia.altairalabs.ai/v1alpha1
kind: ToolPolicy
metadata:
  name: %s
  namespace: %s
spec:
  selector:
    registry: e2e-registry
  rules:
    - name: amount-limit
      deny:
        cel: "has(body.amount) && double(body.amount) > 500.0"
        message: "Amount exceeds limit"
  mode: enforce
  onFailure: deny
`, toolPolicyName, policyNamespace))

			By("waiting for Active phase")
			waitForPhase("toolpolicy", toolPolicyName, policyNamespace, "Active")

			By("verifying ruleCount")
			output, err := getJSONPath("toolpolicy", toolPolicyName, policyNamespace, "{.status.ruleCount}")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("1"))
		})

		It("should transition to Error on invalid CEL", func() {
			By("applying a ToolPolicy with valid CEL first")
			applyPolicy(fmt.Sprintf(`
apiVersion: ee.omnia.altairalabs.ai/v1alpha1
kind: ToolPolicy
metadata:
  name: %s
  namespace: %s
spec:
  selector:
    registry: e2e-registry
  rules:
    - name: valid-rule
      deny:
        cel: "true"
        message: "always deny"
  mode: enforce
  onFailure: deny
`, toolPolicyName, policyNamespace))

			waitForPhase("toolpolicy", toolPolicyName, policyNamespace, "Active")

			By("updating with invalid CEL expression")
			cmd := exec.Command("kubectl", "patch", "toolpolicy", toolPolicyName,
				"-n", policyNamespace, "--type=merge",
				"-p", `{"spec":{"rules":[{"name":"bad-rule","deny":{"cel":"this is not valid CEL !!!","message":"bad"}}]}}`)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Error phase")
			waitForPhase("toolpolicy", toolPolicyName, policyNamespace, "Error")

			By("verifying ruleCount is 0")
			output, err := getJSONPath("toolpolicy", toolPolicyName, policyNamespace, "{.status.ruleCount}")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("0"))

			By("fixing the CEL expression")
			cmd = exec.Command("kubectl", "patch", "toolpolicy", toolPolicyName,
				"-n", policyNamespace, "--type=merge",
				"-p", `{"spec":{"rules":[{"name":"fixed-rule","deny":{"cel":"false","message":"never deny"}}]}}`)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Active phase again")
			waitForPhase("toolpolicy", toolPolicyName, policyNamespace, "Active")
		})

		It("should support audit mode", func() {
			By("applying a ToolPolicy in audit mode")
			applyPolicy(fmt.Sprintf(`
apiVersion: ee.omnia.altairalabs.ai/v1alpha1
kind: ToolPolicy
metadata:
  name: %s
  namespace: %s
spec:
  selector:
    registry: e2e-registry
  rules:
    - name: audit-rule
      deny:
        cel: "true"
        message: "would deny"
  mode: audit
  onFailure: deny
`, toolPolicyName, policyNamespace))

			waitForPhase("toolpolicy", toolPolicyName, policyNamespace, "Active")

			By("switching to enforce mode")
			cmd := exec.Command("kubectl", "patch", "toolpolicy", toolPolicyName,
				"-n", policyNamespace, "--type=merge",
				"-p", `{"spec":{"mode":"enforce"}}`)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			waitForPhase("toolpolicy", toolPolicyName, policyNamespace, "Active")
		})

		It("should clean up on delete", func() {
			By("applying a ToolPolicy")
			applyPolicy(fmt.Sprintf(`
apiVersion: ee.omnia.altairalabs.ai/v1alpha1
kind: ToolPolicy
metadata:
  name: %s
  namespace: %s
spec:
  selector:
    registry: e2e-registry
  rules:
    - name: temp-rule
      deny:
        cel: "false"
        message: "never"
  mode: enforce
  onFailure: deny
`, toolPolicyName, policyNamespace))

			waitForPhase("toolpolicy", toolPolicyName, policyNamespace, "Active")

			By("deleting the ToolPolicy")
			cmd := exec.Command("kubectl", "delete", "toolpolicy", toolPolicyName,
				"-n", policyNamespace, "--timeout=30s")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the resource is gone")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "toolpolicy", toolPolicyName,
					"-n", policyNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred(), "ToolPolicy should be deleted")
			}, 30*time.Second, 2*time.Second).Should(Succeed())
		})
	})

	Context("AgentPolicy", func() {
		const agentPolicyName = "e2e-agent-policy"

		AfterEach(func() {
			deletePolicy("agentpolicy", agentPolicyName, policyNamespace)
		})

		It("should create Istio AuthorizationPolicy for tool allowlist", func() {
			if !isIstioCRDInstalled() {
				Skip("Istio CRDs not installed — skipping AuthorizationPolicy creation test")
			}

			By("applying an AgentPolicy with tool allowlist")
			applyPolicy(fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentPolicy
metadata:
  name: %s
  namespace: %s
spec:
  toolAccess:
    mode: allowlist
    rules:
      - registry: e2e-registry
        tools: ["tool-a", "tool-b"]
  mode: enforce
`, agentPolicyName, policyNamespace))

			By("waiting for Active phase")
			waitForPhase("agentpolicy", agentPolicyName, policyNamespace, "Active")

			By("verifying Istio AuthorizationPolicy was created")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "authorizationpolicy",
					"-n", policyNamespace, "-l",
					fmt.Sprintf("omnia.altairalabs.ai/agentpolicy=%s", agentPolicyName),
					"-o", "json")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				// Verify at least one AuthorizationPolicy exists
				var result struct {
					Items []json.RawMessage `json:"items"`
				}
				g.Expect(json.Unmarshal([]byte(output), &result)).To(Succeed())
				g.Expect(len(result.Items)).To(BeNumerically(">=", 1),
					"Expected at least 1 Istio AuthorizationPolicy")
			}, 60*time.Second, 2*time.Second).Should(Succeed())

			By("deleting the AgentPolicy")
			cmd := exec.Command("kubectl", "delete", "agentpolicy", agentPolicyName,
				"-n", policyNamespace, "--timeout=30s")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying Istio AuthorizationPolicy is cleaned up")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "authorizationpolicy",
					"-n", policyNamespace, "-l",
					fmt.Sprintf("omnia.altairalabs.ai/agentpolicy=%s", agentPolicyName),
					"-o", "json")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				var result struct {
					Items []json.RawMessage `json:"items"`
				}
				g.Expect(json.Unmarshal([]byte(output), &result)).To(Succeed())
				g.Expect(result.Items).To(BeEmpty(),
					"Istio AuthorizationPolicies should be cleaned up after AgentPolicy deletion")
			}, 60*time.Second, 2*time.Second).Should(Succeed())
		})

		It("should use AUDIT action in permissive mode", func() {
			if !isIstioCRDInstalled() {
				Skip("Istio CRDs not installed — skipping permissive mode test")
			}

			By("applying an AgentPolicy in permissive mode")
			applyPolicy(fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentPolicy
metadata:
  name: %s
  namespace: %s
spec:
  toolAccess:
    mode: allowlist
    rules:
      - registry: e2e-registry
        tools: ["tool-a"]
  mode: permissive
`, agentPolicyName, policyNamespace))

			By("waiting for Active phase")
			waitForPhase("agentpolicy", agentPolicyName, policyNamespace, "Active")

			By("verifying AuthorizationPolicy uses AUDIT action")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "authorizationpolicy",
					"-n", policyNamespace, "-l",
					fmt.Sprintf("omnia.altairalabs.ai/agentpolicy=%s", agentPolicyName),
					"-o", "jsonpath={.items[*].spec.action}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				// In permissive mode, all generated policies should use AUDIT
				g.Expect(output).To(ContainSubstring("AUDIT"),
					"Expected AUDIT action in permissive mode, got: %s", output)
				g.Expect(output).NotTo(ContainSubstring("DENY"),
					"DENY action should not be present in permissive mode")
			}, 60*time.Second, 2*time.Second).Should(Succeed())
		})

		It("should reach Active phase without Istio", func() {
			By("applying an AgentPolicy with claim mapping only")
			applyPolicy(fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentPolicy
metadata:
  name: %s
  namespace: %s
spec:
  claimMapping:
    forwardClaims:
      - claim: team
        header: X-Omnia-Claim-Team
  mode: enforce
`, agentPolicyName, policyNamespace))

			By("waiting for Active phase")
			waitForPhase("agentpolicy", agentPolicyName, policyNamespace, "Active")
		})

		It("should report validation errors in status", func() {
			By("applying an AgentPolicy with invalid claim header (missing X-Omnia-Claim- prefix)")
			yaml := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentPolicy
metadata:
  name: %s
  namespace: %s
spec:
  claimMapping:
    forwardClaims:
      - claim: team
        header: X-Bad-Header
  mode: enforce
`, agentPolicyName, policyNamespace)

			// The CRD has a pattern validation on the header field, so this may be
			// rejected at admission time. Try applying and check either admission
			// error or status error.
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(yaml)
			_, applyErr := utils.Run(cmd)

			if applyErr != nil {
				By("verifying admission rejected the invalid header pattern")
				Expect(applyErr.Error()).To(ContainSubstring("X-Omnia-Claim-"),
					"Expected validation error about X-Omnia-Claim- prefix")
			} else {
				By("waiting for Error phase due to invalid header")
				waitForPhase("agentpolicy", agentPolicyName, policyNamespace, "Error")

				msg := conditionMessage("agentpolicy", agentPolicyName, policyNamespace, "Valid")
				Expect(msg).To(ContainSubstring("X-Omnia-Claim-"),
					"Expected error message about X-Omnia-Claim- prefix")
			}
		})
	})
})
