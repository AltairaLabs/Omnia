//go:build e2e
// +build e2e

/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"encoding/base64"
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

		// Workspace-content PVCs. The operator pod has
		// readOnlyRootFilesystem=true, so SkillSource sync writes would fail
		// against /workspace-content without a volume mount. We give each
		// pod a separate dynamically-provisioned PVC backed by kind's
		// local-path provisioner. This sidesteps hostPath permission issues
		// because local-path respects fsGroup. The two PVCs do NOT share
		// data — operator-side writes and runtime-side reads are verified
		// independently in separate specs (see #823).
		skillsOpPVCName    = "skills-e2e-op-workspace-content"
		skillsAgentPVCName = "workspace-test-agents-content"
	)

	BeforeAll(func() {
		if os.Getenv("ENABLE_SKILLS_E2E") != "true" {
			Skip("ENABLE_SKILLS_E2E not set — skipping skills tests")
		}
		if predeployed {
			Skip("Skills e2e patches the operator deployment — incompatible with predeployed mode")
		}

		By("ensuring CRDs are installed and the controller-manager is deployed")
		Expect(ensureManagerDeployed()).To(Succeed())

		By("ensuring the test-agents namespace is Active")
		// The Omnia-CRDs Ordered context force-deletes test-agents in its
		// AfterAll. If Skills runs after that teardown, the namespace is in
		// Terminating state and kubectl create returns but the ns hasn't
		// actually disappeared yet. Wait for it to be Active before any
		// spec tries to create resources inside it.
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "create", "ns", agentsNamespace)
			_, _ = utils.Run(cmd) // ignore AlreadyExists; we care about phase
			cmd = exec.Command("kubectl", "get", "ns", agentsNamespace,
				"-o", "jsonpath={.status.phase}")
			out, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(out).To(Equal("Active"),
				"namespace %s must be Active, got phase %q", agentsNamespace, out)
		}, 2*time.Minute, 2*time.Second).Should(Succeed())

		By("creating dynamically-provisioned PVCs for both pods")
		// kind ships local-path-provisioner under storageClass=standard. The
		// PVs it provisions respect fsGroup, so a nonroot pod with
		// fsGroup=65532 can write to its mount.
		pvcSpec := func(ns, name string) string {
			return fmt.Sprintf(`
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: %s
  namespace: %s
spec:
  accessModes: ["ReadWriteOnce"]
  storageClassName: standard
  resources:
    requests:
      storage: 100Mi
`, name, ns)
		}
		for _, body := range []string{pvcSpec(namespace, skillsOpPVCName), pvcSpec(agentsNamespace, skillsAgentPVCName)} {
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(body)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create PVC")
		}

		By("patching the operator to mount workspace-content + set fsGroup")
		// Strategic merge: add the workspace-content volume + mount, plus
		// fsGroup so the nonroot operator can write to the local-path PV.
		// The existing tmp emptyDir volume is preserved by the merge.
		volPatch := fmt.Sprintf(`{
  "spec": {
    "template": {
      "spec": {
        "securityContext": {"fsGroup": 65532},
        "containers": [{"name": "manager", "volumeMounts": [
          {"name": "tmp", "mountPath": "/tmp"},
          {"name": "workspace-content", "mountPath": "/workspace-content"}
        ]}],
        "volumes": [
          {"name": "tmp", "emptyDir": {}},
          {"name": "workspace-content", "persistentVolumeClaim": {"claimName": "%s"}}
        ]
      }
    }
  }
}`, skillsOpPVCName)
		cmd := exec.Command("kubectl", "patch", "deployment", "omnia-controller-manager",
			"-n", namespace, "--type=strategic", "-p", volPatch)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to patch operator deployment")

		By("waiting for the operator to roll out with the new mount")
		rolloutCmd := exec.Command("kubectl", "rollout", "status",
			"deployment/omnia-controller-manager", "-n", namespace, "--timeout=180s")
		_, err = utils.Run(rolloutCmd)
		Expect(err).NotTo(HaveOccurred(), "operator rollout did not complete")
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
		// Intentionally leave the workspace-content PVCs and the operator's
		// volume mount in place. Deleting the operator's PVC strands the
		// deployment's volume reference; subsequent Describes that trigger
		// any operator restart would fail. The kind cluster teardown at the
		// end of the suite reclaims everything.
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
      "tools": {
        "http_call": {
          "name": "http_call",
          "description": "Make an HTTP request"
        }
      }
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

	It("loads skills into the runtime container end-to-end", func() {
		const (
			arName     = "skills-runtime-test"
			ssName     = "skills-runtime-source"
			cmName     = "skills-runtime-content"
			packName   = "skills-runtime-pack"
			packCMName = "skills-runtime-pack-config"
			provName   = "skills-runtime-provider"
			provSecret = "skills-runtime-provider-secret"
		)

		DeferCleanup(dumpOnFailure)
		DeferCleanup(func() {
			for _, res := range []struct{ kind, name string }{
				{"agentruntime", arName},
				{"provider", provName},
				{"secret", provSecret},
				{"promptpack", packName},
				{"configmap", packCMName},
				{"skillsource", ssName},
				{"configmap", cmName},
			} {
				cmd := exec.Command("kubectl", "delete", res.kind, res.name,
					"-n", agentsNamespace, "--ignore-not-found", "--timeout=30s")
				_, _ = utils.Run(cmd)
			}
		})

		By("creating a SkillSource backed by a ConfigMap")
		skillContent := `---
name: e2e-runtime-skill
description: A skill loaded by the runtime container in this e2e spec
---

# E2E Runtime Skill

When asked, respond cheerfully.
`
		cmYAML := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
data:
  e2e-skill__SKILL.md: |
%s
`, cmName, agentsNamespace, indentLines(skillContent, "    "))
		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(cmYAML)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

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
  targetPath: "skills/e2e"
  createVersionOnSync: false
`, ssName, agentsNamespace, cmName)
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(ssYAML)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for SkillSource Ready (operator wrote into the shared PVC)")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "skillsource", ssName,
				"-n", agentsNamespace, "-o", "jsonpath={.status.phase}")
			out, runErr := utils.Run(cmd)
			g.Expect(runErr).NotTo(HaveOccurred())
			g.Expect(out).To(Equal("Ready"))
		}, 2*time.Minute, 2*time.Second).Should(Succeed())

		By("creating a PromptPack whose spec.skills references the source")
		packCMYAML := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
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
          "system_template": "You are a runtime e2e assistant."
        }
      }
    }
`, packCMName, agentsNamespace, packName, packName)
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(packCMYAML)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

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
`, packName, agentsNamespace, packCMName, ssName)
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(ppYAML)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "promptpack", packName,
				"-n", agentsNamespace,
				"-o", "jsonpath={.status.conditions[?(@.type=='SkillsResolved')].status}")
			out, runErr := utils.Run(cmd)
			g.Expect(runErr).NotTo(HaveOccurred())
			g.Expect(out).To(Equal("True"))
		}, 2*time.Minute, 2*time.Second).Should(Succeed())

		By("seeding the agent's workspace-content PVC with the manifest the operator would have written")
		// Cross-pod data sharing in kind needs NFS (see issue #823). The
		// reconcile-chain spec already proves the operator-side write
		// succeeds; this spec proves the runtime-side load. Use a one-shot
		// Job to write the manifest + skill body into the agent's PVC.
		writerJobName := "skills-runtime-writer"
		// Compact single-line JSON so we can echo it without YAML indent
		// gymnastics. base64 the SKILL.md body for the same reason.
		manifestJSON := fmt.Sprintf(
			`{"version":"1","skills":[{"name":"e2e-runtime-skill","mount_as":"e2e-runtime-skill","content_path":"/workspace-content/skills/e2e/e2e-skill"}]}`)
		skillBody := "---\nname: e2e-runtime-skill\ndescription: Skill seeded by the e2e writer Job\n---\n"
		skillB64 := base64.StdEncoding.EncodeToString([]byte(skillBody))
		writerYAML := fmt.Sprintf(`
apiVersion: batch/v1
kind: Job
metadata:
  name: %s
  namespace: %s
spec:
  ttlSecondsAfterFinished: 60
  template:
    spec:
      restartPolicy: Never
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        fsGroup: 65532
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: writer
        image: busybox:1.36
        command:
          - sh
          - -c
          - 'set -eu; mkdir -p /workspace-content/manifests /workspace-content/skills/e2e/e2e-skill; printf %%s %q > /workspace-content/manifests/%s.json; printf %%s %q | base64 -d > /workspace-content/skills/e2e/e2e-skill/SKILL.md; ls -la /workspace-content/manifests'
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop: ["ALL"]
          readOnlyRootFilesystem: true
        volumeMounts:
        - name: workspace-content
          mountPath: /workspace-content
      volumes:
      - name: workspace-content
        persistentVolumeClaim:
          claimName: %s
`, writerJobName, agentsNamespace, manifestJSON, packName, skillB64, skillsAgentPVCName)
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(writerYAML)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create writer Job")
		DeferCleanup(func() {
			cmd := exec.Command("kubectl", "delete", "job", writerJobName,
				"-n", agentsNamespace, "--ignore-not-found", "--timeout=30s")
			_, _ = utils.Run(cmd)
		})
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "job", writerJobName,
				"-n", agentsNamespace, "-o", "jsonpath={.status.succeeded}")
			out, runErr := utils.Run(cmd)
			g.Expect(runErr).NotTo(HaveOccurred())
			g.Expect(out).To(Equal("1"), "writer Job should complete successfully")
		}, 2*time.Minute, 5*time.Second).Should(Succeed())

		By("creating a Provider for mock mode and an AgentRuntime")
		secretCmd := exec.Command("kubectl", "create", "secret", "generic", provSecret,
			"-n", agentsNamespace, "--from-literal=api-key=e2e-runtime",
			"--dry-run=client", "-o", "yaml")
		secretYAML, err := utils.Run(secretCmd)
		Expect(err).NotTo(HaveOccurred())
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(secretYAML)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		provYAML := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: %s
  namespace: %s
spec:
  type: mock
  secretRef:
    name: %s
`, provName, agentsNamespace, provSecret)
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(provYAML)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		arYAML := fmt.Sprintf(`
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
        name: %s
`, arName, agentsNamespace, packName, provName)
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(arYAML)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for the agent pod to be Ready (both containers)")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pods",
				"-n", agentsNamespace,
				"-l", fmt.Sprintf("app.kubernetes.io/instance=%s", arName),
				"-o", "jsonpath={.items[0].status.containerStatuses[*].ready}")
			out, runErr := utils.Run(cmd)
			g.Expect(runErr).NotTo(HaveOccurred())
			g.Expect(out).To(Equal("true true"),
				"both facade + runtime containers should report ready, got %q", out)
		}, 5*time.Minute, 5*time.Second).Should(Succeed())

		By("greping runtime container logs for the skill-load line")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "logs",
				"-n", agentsNamespace,
				"-l", fmt.Sprintf("app.kubernetes.io/instance=%s", arName),
				"-c", "runtime", "--tail=500")
			out, runErr := utils.Run(cmd)
			g.Expect(runErr).NotTo(HaveOccurred())
			g.Expect(out).To(ContainSubstring("skill manifest loaded"),
				"runtime container should log skill manifest load on startup")
			g.Expect(out).To(ContainSubstring("e2e-runtime-skill"),
				"the loaded skill name should appear in the log line")
		}, 2*time.Minute, 5*time.Second).Should(Succeed())
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
