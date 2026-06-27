//go:build e2e
// +build e2e

/*
Copyright 2026.

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

// Function-mode end-to-end suite. The load-bearing claim under test:
// a function invocation creates a `sessions` row that operators can
// pivot from the facade's Loki log line back to a dashboard row —
// even when the call fails before the runtime is invoked (e.g.
// input_invalid). This was the entire point of the Functions-as-
// sessions rework; if it doesn't hold, the rework is a regression.
//
// The happy path (success → status=completed with model output) is
// intentionally NOT exercised here — the mock provider doesn't emit
// schema-conforming JSON without per-test scenario config, which is
// out of scope for PR 2. The failure path is what the user explicitly
// called out: "We'll need click-through from the sessionId into the
// Loki logs to troubleshoot."
// functionsNamespace is a dedicated namespace for this suite. Other
// suites (Skills, Doctor, Policy, the session-test in e2e_test.go)
// share `test-agents` and stomp on each other's create / delete; this
// suite uses its own namespace so a sibling AfterAll mid-deleting
// test-agents can't race our BeforeAll's `kubectl apply`.
const functionsNamespace = "test-functions"

var _ = Describe("Functions mode", Ordered, Label("functions"), func() {
	const (
		functionName       = "test-fn"
		functionPackName   = "test-fn-prompts"
		functionProviderID = "test-fn-provider"
	)

	BeforeAll(func() {
		// Same idempotent setup the other Describes call: installs the
		// operator CRDs (PromptPack, Provider, AgentRuntime) and the
		// controller-manager. Without this, kubectl apply against
		// PromptPack errors with "no matches for kind PromptPack"
		// because we run before Skills' / Manager's BeforeAll.
		By("ensuring CRDs are installed and the controller-manager is deployed")
		Expect(ensureManagerDeployed()).To(Succeed())

		// session-api + postgres land in omnia-system. The existing
		// "Omnia CRDs" Describe deploys them in its BeforeAll, but
		// ginkgo orders our Describe ahead of it — without this
		// idempotent helper the test pod's `GET /api/v1/sessions` blows
		// up with a DNS NXDOMAIN because the Service doesn't exist yet.
		By("ensuring session-api + postgres are deployed in omnia-system")
		Expect(ensureSessionApiDeployed()).To(Succeed())

		By("creating the test-functions namespace if absent")
		cmd := exec.Command("kubectl", "create", "ns", functionsNamespace)
		_, _ = utils.Run(cmd) // tolerate AlreadyExists

		// The facade's initSessionStore() resolves the session-api URL by
		// looking up a Workspace CR. Without one for our namespace, it
		// falls back to MemoryStore — at which point function invocations
		// "succeed" but no row ever lands in Postgres, so the e2e's
		// assertion (`GET /sessions?namespace=...&agent=...`) returns
		// zero rows. Create a Workspace pointing at the e2e session-api.
		workspaceManifest := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Workspace
metadata:
  name: e2e-functions-workspace
spec:
  displayName: E2E Functions Workspace
  namespace:
    name: %[1]s
  services:
    - name: default
      mode: external
      external:
        sessionURL: "%[2]s"
        memoryURL: "%[2]s"
`, functionsNamespace, sessionApiURL)
		wsCmd := exec.Command("kubectl", "apply", "-f", "-")
		wsCmd.Stdin = strings.NewReader(workspaceManifest)
		_, err := utils.Run(wsCmd)
		Expect(err).NotTo(HaveOccurred(), "failed to apply Workspace")
		By("waiting for the Workspace to report Ready")
		verifyWs := func(g Gomega) {
			c := exec.Command("kubectl", "get", "workspace", "e2e-functions-workspace",
				"-o", "jsonpath={.status.services[0].ready}")
			out, e := utils.Run(c)
			g.Expect(e).NotTo(HaveOccurred())
			g.Expect(out).To(Equal("true"))
		}
		Eventually(verifyWs, 2*time.Minute, 2*time.Second).Should(Succeed())

		By("creating a minimal PromptPack ConfigMap + CR")
		promptPackManifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-fn-prompts
  namespace: test-functions
data:
  pack.json: |
    {
      "id": "test-fn-prompts",
      "name": "test-fn-prompts",
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
          "system_template": "You are a test echoer."
        }
      }
    }
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: PromptPack
metadata:
  name: test-fn-prompts
  namespace: test-functions
spec:
  source:
    type: configmap
    configMapRef:
      name: test-fn-prompts
  version: "1.0.0"
`
		applyCmd := exec.Command("kubectl", "apply", "-f", "-")
		applyCmd.Stdin = strings.NewReader(promptPackManifest)
		_, err = utils.Run(applyCmd)
		Expect(err).NotTo(HaveOccurred(), "failed to apply PromptPack")

		By("creating a mock Provider for the function pod")
		// The dummy secret keeps the Provider CR valid against the CRD
		// shape; the mock-provider annotation on the AgentRuntime tells
		// the runtime sidecar to ignore real credentials.
		providerSecret := `
apiVersion: v1
kind: Secret
metadata:
  name: test-fn-provider
  namespace: test-functions
type: Opaque
stringData:
  api-key: mock-not-a-real-key
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: test-fn-provider
  namespace: test-functions
spec:
  type: mock
  credential:
    secretRef:
      name: test-fn-provider
`
		secretCmd := exec.Command("kubectl", "apply", "-f", "-")
		secretCmd.Stdin = strings.NewReader(providerSecret)
		_, err = utils.Run(secretCmd)
		Expect(err).NotTo(HaveOccurred(), "failed to apply Provider")

		By("creating a function-mode AgentRuntime with strict input schema")
		// inputSchema demands "q" so we can drive the input_invalid path
		// with an empty body. outputSchema is loose — we don't exercise
		// the happy path in this PR (mock provider doesn't emit
		// schema-conforming JSON without scenario config).
		arManifest := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: %[1]s
  namespace: %[2]s
  annotations:
    omnia.altairalabs.ai/mock-provider: "true"
spec:
  mode: function
  promptPackRef:
    name: %[3]s
  facades:
    - type: rest
      port: 8080
      extraEnv:
        - name: OMNIA_FACADE_ALLOW_UNAUTHENTICATED
          value: "true"
  inputSchema:
    type: object
    required: ["q"]
    properties:
      q:
        type: string
  outputSchema:
    type: object
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
        name: %[4]s
`, functionName, functionsNamespace, functionPackName, functionProviderID)
		arCmd := exec.Command("kubectl", "apply", "-f", "-")
		arCmd.Stdin = strings.NewReader(arManifest)
		_, err = utils.Run(arCmd)
		Expect(err).NotTo(HaveOccurred(), "failed to apply function-mode AgentRuntime")

		// On failure, dump the operator + agent pod state so the CI run
		// produces a usable diagnostic instead of a bare "Deployment
		// never appeared".
		DeferCleanup(func() {
			if !CurrentSpecReport().Failed() {
				return
			}
			_, _ = fmt.Fprintf(GinkgoWriter, "\n=== DEBUG: function-mode setup failed ===\n")
			arGet := exec.Command("kubectl", "get", "agentruntime", functionName, "-n", functionsNamespace, "-o", "yaml")
			if out, e := utils.Run(arGet); e == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "AgentRuntime:\n%s\n", out)
			}
			events := exec.Command("kubectl", "get", "events", "-n", functionsNamespace, "--sort-by=.lastTimestamp")
			if out, e := utils.Run(events); e == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Events:\n%s\n", out)
			}
			ctlLogs := exec.Command("kubectl", "logs",
				"-n", namespace,
				"-l", "control-plane=controller-manager",
				"--tail=200", "--all-containers=true")
			if out, e := utils.Run(ctlLogs); e == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "controller-manager logs:\n%s\n", out)
			}
		})

		By("waiting for the function-mode Deployment to be Ready")
		verifyReady := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "deployment", functionName,
				"-n", functionsNamespace,
				"-o", "jsonpath={.status.readyReplicas}")
			out, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(out).To(Equal("1"), "function-mode pod must have 1 ready replica")
		}
		Eventually(verifyReady, 3*time.Minute, 2*time.Second).Should(Succeed())
	})

	AfterAll(func() {
		// Single-shot cleanup: deleting the namespace removes everything
		// we created inside it (CR + ConfigMap + Secret + Pod + Service +
		// Deployment owned by the operator). The Workspace is
		// cluster-scoped so it needs a separate delete.
		_, _ = utils.Run(exec.Command("kubectl", "delete", "workspace",
			"e2e-functions-workspace", "--ignore-not-found", "--timeout=30s"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "ns",
			functionsNamespace, "--ignore-not-found", "--timeout=60s"))
	})

	It("creates a session row with status=error on input_invalid", func() {
		// The test pod does three things in sequence:
		//  1. POST a body without the required `q` field → expect 400.
		//  2. List sessions for this agent + namespace → expect exactly
		//     one row with status=error and the "function" tag.
		//  3. GET that session's runtime events → expect at least one
		//     row with eventType=function.input_invalid carrying the
		//     validator error.
		//
		// Step 2 is the load-bearing assertion: a real `sessions` row
		// landed in postgres for a call that never reached the runtime.
		// That's what makes Loki / dashboard pivoting work for the
		// failure cases operators most need to troubleshoot.
		testManifest := fmt.Sprintf(`
apiVersion: v1
kind: Pod
metadata:
  name: fn-mode-test
  namespace: %[1]s
spec:
  restartPolicy: Never
  containers:
  - name: python
    image: python:3.11-slim
    command: ["sh", "-c"]
    args:
    - |
      pip install requests --quiet
      python3 << 'PYTHON_SCRIPT'
      import json
      import sys
      import time
      import requests

      FN_URL = "http://%[2]s.%[1]s.svc.cluster.local:8080/functions/%[2]s"
      SESSION_API = "%[3]s"
      NS = "%[1]s"

      # Step 1: drive input_invalid.
      print(f"POST {FN_URL} with body missing required 'q'")
      resp = requests.post(FN_URL, json={}, timeout=15)
      print(f"  status={resp.status_code} body={resp.text[:200]}")
      if resp.status_code != 400:
          print(f"ERROR: expected 400 input_invalid; got {resp.status_code}")
          sys.exit(1)
      body = resp.json()
      if body.get("error") != "input_invalid":
          print(f"ERROR: expected error=input_invalid; got {body!r}")
          sys.exit(1)

      # Give the deferred closeSession call a moment to land (the 400
      # is returned synchronously but the session UpdateSessionStatus
      # runs in the handler's defer).
      time.sleep(3)

      # Step 2: a sessions row must exist for this function, with
      # status=error.
      list_url = f"{SESSION_API}/api/v1/sessions?namespace={NS}&agent=%[2]s"
      print(f"GET {list_url}")
      resp = requests.get(list_url, timeout=10)
      print(f"  status={resp.status_code}")
      if resp.status_code != 200:
          print(f"ERROR: listing sessions failed: {resp.text[:200]}")
          sys.exit(1)
      data = resp.json()
      sessions = data.get("sessions") or []
      print(f"  found {len(sessions)} sessions")
      if not sessions:
          print("ERROR: no session row found for the function call")
          sys.exit(1)

      # Pick the most recent one (DESC by created_at server-side).
      sess = sessions[0]
      sess_id = sess.get("id") or sess.get("ID")
      status = sess.get("status")
      tags = sess.get("tags") or []
      print(f"  session_id={sess_id} status={status} tags={tags}")
      if status != "error":
          print(f"ERROR: expected status=error; got {status!r}")
          sys.exit(1)
      if "function" not in tags:
          print(f"ERROR: session missing 'function' tag; got tags={tags!r}")
          sys.exit(1)

      # Step 3: the runtime_events table must have a function.input_invalid
      # entry for this session — that's how the dashboard surfaces the
      # validator error on the session detail page.
      events_url = f"{SESSION_API}/api/v1/sessions/{sess_id}/events"
      print(f"GET {events_url}")
      resp = requests.get(events_url, timeout=10)
      if resp.status_code != 200:
          print(f"ERROR: listing events failed: {resp.text[:200]}")
          sys.exit(1)
      events_body = resp.json()
      # The events endpoint returns either a bare array or an envelope
      # like {"events": [...]} / {"data": [...]} depending on version.
      # Handle both without assuming a particular shape.
      if isinstance(events_body, list):
          events = events_body
      elif isinstance(events_body, dict):
          events = events_body.get("events") or events_body.get("data") or []
      else:
          events = []
      print(f"  found {len(events)} events")
      matched = [e for e in events if e.get("eventType") == "function.input_invalid"]
      if not matched:
          types = [e.get("eventType") for e in events]
          print(f"ERROR: no function.input_invalid runtime event; got types={types!r}")
          sys.exit(1)
      err_msg = matched[0].get("errorMessage", "")
      print(f"  function.input_invalid errorMessage={err_msg[:120]!r}")
      if not err_msg:
          print("ERROR: function.input_invalid event has no errorMessage")
          sys.exit(1)

      print("PASS: function invocation recorded as a session with status=error + failure event")
      PYTHON_SCRIPT
`, functionsNamespace, functionName, sessionApiURL)
		applyCmd := exec.Command("kubectl", "apply", "-f", "-")
		applyCmd.Stdin = strings.NewReader(testManifest)
		_, err := utils.Run(applyCmd)
		Expect(err).NotTo(HaveOccurred(), "failed to create test pod")

		DeferCleanup(func() {
			if !CurrentSpecReport().Failed() {
				return
			}
			_, _ = fmt.Fprintf(GinkgoWriter, "\n=== DEBUG: function-mode session-row test failed ===\n")
			logCmd := exec.Command("kubectl", "logs", "fn-mode-test", "-n", functionsNamespace)
			if out, e := utils.Run(logCmd); e == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Test pod logs:\n%s\n", out)
			}
			facadeCmd := exec.Command("kubectl", "logs",
				"-n", functionsNamespace,
				"-l", "app.kubernetes.io/instance="+functionName,
				"-c", "facade", "--tail=200")
			if out, e := utils.Run(facadeCmd); e == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Facade logs:\n%s\n", out)
			}
			runtimeCmd := exec.Command("kubectl", "logs",
				"-n", functionsNamespace,
				"-l", "app.kubernetes.io/instance="+functionName,
				"-c", "runtime", "--tail=200")
			if out, e := utils.Run(runtimeCmd); e == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Runtime logs:\n%s\n", out)
			}
			sessionApiCmd := exec.Command("kubectl", "logs",
				"-n", namespace,
				"-l", "app=e2e-session-api", "--tail=200")
			if out, e := utils.Run(sessionApiCmd); e == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Session-api logs:\n%s\n", out)
			}
		})

		By("waiting for the test pod to Succeed")
		verifyComplete := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pod", "fn-mode-test",
				"-n", functionsNamespace, "-o", "jsonpath={.status.phase}")
			out, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			// Failed is terminal — if it landed there, drop out of the
			// retry loop instead of waiting for Succeeded.
			g.Expect(out).To(Or(Equal("Succeeded"), Equal("Failed")))
		}
		Eventually(verifyComplete, 3*time.Minute, 3*time.Second).Should(Succeed())

		By("confirming the test pod reported PASS")
		cmd := exec.Command("kubectl", "logs", "fn-mode-test", "-n", functionsNamespace)
		out, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "failed to read test pod logs")
		Expect(out).To(ContainSubstring("PASS:"),
			"test pod must report PASS; full log:\n%s", out)
	})
})
