//go:build e2e
// +build e2e

/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
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

// Skills E2E exercises the CRD + reconcile chain for the skills feature:
// SkillSource (configmap variant) → PromptPack.spec.skills → status
// conditions on both CRDs. It does NOT verify the runtime-side skill load
// yet: that requires the operator pod to share a PVC with agent pods at
// /workspace-content, which the Core deploy kustomize does not configure.
// Runtime-log verification is tracked as a follow-up.
var _ = Describe("Skills", Ordered, Label("skills"), func() {
	const (
		skillCMName       = "test-skill-content"
		skillSourceName   = "test-skill-source"
		skillPromptPack   = "test-skills-pack"
		skillConfigMap    = "test-skills-pack-config"
		skillProviderName = "test-skills-provider"
		skillAgentRuntime = "test-skills-agent"
	)

	BeforeAll(func() {
		if os.Getenv("ENABLE_SKILLS_E2E") != "true" {
			Skip("ENABLE_SKILLS_E2E not set — skipping skills tests")
		}

		By("ensuring the test-agents namespace exists")
		cmd := exec.Command("kubectl", "create", "ns", agentsNamespace)
		_, _ = utils.Run(cmd) // ignore if exists
	})

	AfterAll(func() {
		if skipCleanup {
			return
		}
		for _, res := range []struct {
			kind, name string
		}{
			{"agentruntime", skillAgentRuntime},
			{"provider", skillProviderName},
			{"promptpack", skillPromptPack},
			{"configmap", skillConfigMap},
			{"skillsource", skillSourceName},
			{"configmap", skillCMName},
		} {
			cmd := exec.Command("kubectl", "delete", res.kind, res.name,
				"-n", agentsNamespace, "--ignore-not-found", "--timeout=30s")
			_, _ = utils.Run(cmd)
		}
	})

	// dumpOnFailure captures debug state for all skills-related resources when
	// an assertion fails. Called via DeferCleanup so it runs even if the spec
	// panics, and prints directly to GinkgoWriter so CI logs show the diagnosis.
	dumpOnFailure := func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		_, _ = fmt.Fprintf(GinkgoWriter, "\n=== DEBUG: skills e2e failure ===\n")
		for _, q := range []string{
			"skillsource " + skillSourceName,
			"promptpack " + skillPromptPack,
			"configmap " + skillCMName,
		} {
			parts := strings.Fields(q)
			cmd := exec.Command("kubectl", "get", parts[0], parts[1],
				"-n", agentsNamespace, "-o", "yaml")
			if out, err := utils.Run(cmd); err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "%s:\n%s\n", q, out)
			}
		}
		logsCmd := exec.Command("kubectl", "logs",
			"-n", namespace, "-l", "control-plane=controller-manager",
			"--tail=300", "--all-containers=true")
		if out, err := utils.Run(logsCmd); err == nil {
			_, _ = fmt.Fprintf(GinkgoWriter, "controller-manager logs:\n%s\n", out)
		}
	}

	It("reconciles SkillSource and PromptPack.spec.skills end-to-end", func() {
		DeferCleanup(dumpOnFailure)

		By("creating the ConfigMap with a SKILL.md")
		// ConfigMap keys can't contain "/", so the sync layer decodes "__"
		// back to "/" when writing to disk. Here "refunds__SKILL.md"
		// becomes the file "refunds/SKILL.md" under the sync root.
		skillContent := `---
name: refund-processing
description: Process customer refund requests using approved workflows
allowed-tools:
  - http_call
---

# Refund Processing

When a customer requests a refund, verify the order first.
`
		cmYAML := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
data:
  refunds__SKILL.md: |
%s
`, skillCMName, agentsNamespace, indentLines(skillContent, "    "))
		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(cmYAML)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create skill content ConfigMap")

		By("creating a SkillSource of type configmap")
		ssYAML := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: SkillSource
metadata:
  name: %s
  namespace: %s
spec:
  type: configmap
  configMap:
    name: %s
  interval: "1h"
  timeout: "30s"
  targetPath: "skills/test"
  createVersionOnSync: false
`, skillSourceName, agentsNamespace, skillCMName)
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(ssYAML)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create SkillSource")

		By("waiting for SkillSource phase to reach Ready")
		verifyReady := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "skillsource", skillSourceName,
				"-n", agentsNamespace, "-o", "jsonpath={.status.phase}")
			out, runErr := utils.Run(cmd)
			g.Expect(runErr).NotTo(HaveOccurred())
			g.Expect(out).To(Equal("Ready"), "SkillSource should reach Ready")
		}
		Eventually(verifyReady, 2*time.Minute, 2*time.Second).Should(Succeed())

		By("verifying SkillSource.status.skillCount is 1")
		cmd = exec.Command("kubectl", "get", "skillsource", skillSourceName,
			"-n", agentsNamespace, "-o", "jsonpath={.status.skillCount}")
		out, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("1"), "SkillSource should resolve 1 skill")

		By("creating a PromptPack ConfigMap")
		packYAML := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
data:
  pack.json: |
    {
      "id": "test-skills-pack",
      "name": "test-skills-pack",
      "version": "1.0.0",
      "template_engine": {"version": "v1", "syntax": "{{variable}}"},
      "prompts": {
        "default": {
          "id": "default",
          "name": "default",
          "version": "1.0.0",
          "system_template": "You are a test assistant with skills."
        }
      },
      "tools": [
        {"name": "http_call", "description": "Make an HTTP request"}
      ]
    }
`, skillConfigMap, agentsNamespace)
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(packYAML)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create PromptPack ConfigMap")

		By("creating a PromptPack that references the SkillSource")
		ppYAML := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: PromptPack
metadata:
  name: %s
  namespace: %s
spec:
  source:
    type: configmap
    configMapRef:
      name: %s
  version: "1.0.0"
  skills:
    - source: %s
  skillsConfig:
    maxActive: 2
`, skillPromptPack, agentsNamespace, skillConfigMap, skillSourceName)
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(ppYAML)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create PromptPack with skills")

		By("waiting for PromptPack SkillsResolved condition to be True")
		verifyCondition := func(condType, expected string) func(Gomega) {
			return func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "promptpack", skillPromptPack,
					"-n", agentsNamespace,
					"-o", fmt.Sprintf("jsonpath={.status.conditions[?(@.type=='%s')].status}", condType))
				out, runErr := utils.Run(cmd)
				g.Expect(runErr).NotTo(HaveOccurred())
				g.Expect(out).To(Equal(expected),
					fmt.Sprintf("condition %s should be %s", condType, expected))
			}
		}
		Eventually(verifyCondition("SkillsResolved", "True"), 2*time.Minute, 2*time.Second).Should(Succeed())

		By("waiting for PromptPack SkillsValid condition to be True")
		Eventually(verifyCondition("SkillsValid", "True"), time.Minute, 2*time.Second).Should(Succeed())

		By("waiting for PromptPack SkillToolsResolved condition to be True")
		// The skill declares allowed-tools: [http_call] and the pack declares
		// tools: [{name: http_call}], so tool scope validation should pass.
		Eventually(verifyCondition("SkillToolsResolved", "True"), time.Minute, 2*time.Second).Should(Succeed())

		By("verifying the PromptPack reaches Active phase")
		verifyActive := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "promptpack", skillPromptPack,
				"-n", agentsNamespace, "-o", "jsonpath={.status.phase}")
			out, runErr := utils.Run(cmd)
			g.Expect(runErr).NotTo(HaveOccurred())
			g.Expect(out).To(Equal("Active"))
		}
		Eventually(verifyActive, 2*time.Minute, 2*time.Second).Should(Succeed())
	})

	It("rejects a PromptPack that references a non-existent SkillSource", func() {
		DeferCleanup(func() {
			cmd := exec.Command("kubectl", "delete", "promptpack", "missing-skill-pack",
				"-n", agentsNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "configmap", "missing-skill-pack-config",
				"-n", agentsNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		By("creating a PromptPack ConfigMap")
		packYAML := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: missing-skill-pack-config
  namespace: %s
data:
  pack.json: |
    {"id":"p","name":"p","version":"1.0.0","template_engine":{"version":"v1","syntax":"{{variable}}"},"prompts":{"default":{"id":"default","name":"default","version":"1.0.0","system_template":"t"}}}
`, agentsNamespace)
		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(packYAML)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("creating a PromptPack pointing at a non-existent SkillSource")
		ppYAML := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: PromptPack
metadata:
  name: missing-skill-pack
  namespace: %s
spec:
  source:
    type: configmap
    configMapRef:
      name: missing-skill-pack-config
  version: "1.0.0"
  skills:
    - source: does-not-exist
`, agentsNamespace)
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(ppYAML)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for SkillsResolved condition to be False")
		verifyResolvedFalse := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "promptpack", "missing-skill-pack",
				"-n", agentsNamespace,
				"-o", "jsonpath={.status.conditions[?(@.type=='SkillsResolved')].status}")
			out, runErr := utils.Run(cmd)
			g.Expect(runErr).NotTo(HaveOccurred())
			g.Expect(out).To(Equal("False"))
		}
		Eventually(verifyResolvedFalse, 2*time.Minute, 2*time.Second).Should(Succeed())
	})
})

// indentLines prefixes every non-empty line with the given indent. Used to
// embed a multiline block into YAML literal scalar context.
func indentLines(s, indent string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = indent + l
		}
	}
	return strings.Join(lines, "\n")
}
