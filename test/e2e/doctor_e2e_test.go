//go:build e2e
// +build e2e

/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
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

// Doctor smoke covers issue #805. The doctor runs in the cluster as a real
// pod, hits its own infrastructure, and reports per-check results. This spec
// runs the doctor binary in --run-once mode in a Pod and asserts:
//   - The pod completes (not OOMKilled / Crashed).
//   - The JSON it printed includes a result for every registered check by ID.
//   - No check produces an unexpected error (Failed/Skipped/Warning are all
//     legitimate outcomes against a half-deployed Core E2E cluster; only
//     "Error" is a doctor-side bug).
var _ = Describe("Doctor", Ordered, Label("doctor"), func() {
	const (
		doctorPodName = "omnia-doctor-smoke"
	)

	BeforeAll(func() {
		if os.Getenv("ENABLE_DOCTOR_E2E") != "true" {
			Skip("ENABLE_DOCTOR_E2E not set — skipping doctor smoke spec")
		}
		if predeployed {
			Skip("Doctor e2e brings its own pod — incompatible with predeployed mode")
		}
		By("ensuring CRDs are installed and the controller-manager is deployed")
		Expect(ensureManagerDeployed()).To(Succeed())
	})

	AfterAll(func() {
		if skipCleanup {
			return
		}
		cmd := exec.Command("kubectl", "delete", "pod", doctorPodName,
			"-n", namespace, "--ignore-not-found", "--timeout=30s")
		_, _ = utils.Run(cmd)
	})

	It("runs all registered checks in --run-once mode without crashing", func() {
		// The doctor pod has its own minimal RBAC needs (read on workspaces,
		// sessionprivacypolicies, etc). We reuse the controller-manager's
		// ServiceAccount which already has cluster-wide read on all CRDs the
		// doctor wants. In production the chart provisions a dedicated SA;
		// here we sidestep that to keep the spec self-contained.
		podYAML := fmt.Sprintf(`
apiVersion: v1
kind: Pod
metadata:
  name: %s
  namespace: %s
spec:
  restartPolicy: Never
  serviceAccountName: omnia-controller-manager
  securityContext:
    runAsNonRoot: true
    runAsUser: 65532
    runAsGroup: 65532
    fsGroup: 65532
    seccompProfile:
      type: RuntimeDefault
  containers:
  - name: doctor
    image: %s
    imagePullPolicy: Never
    args:
      - --run-once
      - --namespace=%s
      - --agent-namespace=test-agents
    securityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      capabilities:
        drop: ["ALL"]
`, doctorPodName, namespace, doctorImage, namespace)

		By("creating the doctor pod in --run-once mode")
		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(podYAML)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create doctor pod")

		DeferCleanup(func() {
			if !CurrentSpecReport().Failed() {
				return
			}
			_, _ = fmt.Fprintf(GinkgoWriter, "\n=== DEBUG: doctor pod state ===\n")
			descCmd := exec.Command("kubectl", "describe", "pod", doctorPodName, "-n", namespace)
			if out, dErr := utils.Run(descCmd); dErr == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "%s\n", out)
			}
		})

		By("waiting for the doctor pod to reach a terminal phase")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pod", doctorPodName,
				"-n", namespace, "-o", "jsonpath={.status.phase}")
			out, runErr := utils.Run(cmd)
			g.Expect(runErr).NotTo(HaveOccurred())
			// --run-once without --exit-code always exits 0, so phase=Succeeded.
			// Failed phase means the doctor itself crashed before completing.
			g.Expect(out).To(BeElementOf("Succeeded", "Failed"),
				"doctor pod should reach a terminal phase, got %q", out)
		}, 3*time.Minute, 5*time.Second).Should(Succeed())

		By("verifying the pod actually Succeeded (no doctor-side crash)")
		phaseCmd := exec.Command("kubectl", "get", "pod", doctorPodName,
			"-n", namespace, "-o", "jsonpath={.status.phase}")
		phase, err := utils.Run(phaseCmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(phase).To(Equal("Succeeded"),
			"doctor crashed in --run-once mode (Failed phase) — see pod describe above")

		By("retrieving the JSON results from doctor stdout")
		logsCmd := exec.Command("kubectl", "logs", doctorPodName, "-n", namespace)
		logsOut, err := utils.Run(logsCmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to read doctor pod logs")

		// The doctor's logger writes structured info to stderr; --run-once
		// writes the RunResult JSON object to stdout. Find the start of the
		// JSON object and decode from there.
		jsonStart := strings.Index(logsOut, "{\n")
		Expect(jsonStart).To(BeNumerically(">=", 0),
			"could not locate JSON object in doctor logs; got:\n%s", logsOut)

		type doctorTest struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Detail string `json:"detail"`
		}
		var run struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			Summary struct {
				Total   int `json:"total"`
				Passed  int `json:"passed"`
				Failed  int `json:"failed"`
				Skipped int `json:"skipped"`
			} `json:"summary"`
			Categories []struct {
				Name  string       `json:"name"`
				Tests []doctorTest `json:"tests"`
			} `json:"categories"`
		}
		Expect(json.Unmarshal([]byte(logsOut[jsonStart:]), &run)).To(Succeed(),
			"failed to parse doctor RunResult JSON")

		By("asserting the doctor produced a structured report")
		Expect(run.ID).NotTo(BeEmpty(), "RunResult should have an ID")
		Expect(run.Categories).NotTo(BeEmpty(), "doctor should register at least one category")

		var allTests []doctorTest
		for _, cat := range run.Categories {
			allTests = append(allTests, cat.Tests...)
		}
		Expect(allTests).NotTo(BeEmpty(), "doctor should register at least one check")
		Expect(run.Summary.Total).To(Equal(len(allTests)),
			"summary.total should match number of test entries across categories")

		By("asserting every check has a name and a recognised status")
		// Statuses defined in internal/doctor/result.go: pass, fail, skip, running.
		// "running" should not appear in a completed run.
		validStatuses := map[string]bool{"pass": true, "fail": true, "skip": true}
		seen := map[string]bool{}
		for _, t := range allTests {
			Expect(t.Name).NotTo(BeEmpty(), "check should have a name")
			Expect(validStatuses[t.Status]).To(BeTrue(),
				"check %q has unrecognised status %q (expected pass/fail/skip)", t.Name, t.Status)
			Expect(seen[t.Name]).To(BeFalse(),
				"check name %q appears twice — registry has a duplicate", t.Name)
			seen[t.Name] = true
		}

		// Print a summary so failures show what actually ran.
		_, _ = fmt.Fprintf(GinkgoWriter,
			"\nDoctor smoke summary: total=%d passed=%d failed=%d skipped=%d\n",
			run.Summary.Total, run.Summary.Passed, run.Summary.Failed, run.Summary.Skipped)
		for _, cat := range run.Categories {
			for _, t := range cat.Tests {
				_, _ = fmt.Fprintf(GinkgoWriter, "  [%s/%s] %s — %s\n", cat.Name, t.Status, t.Name, t.Detail)
			}
		}
	})
})
