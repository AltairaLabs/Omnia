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

// MCP-on-functions end-to-end suite. Validates the wire contract for
// the Streamable HTTP MCP server on function-mode pods (#1123–#1133):
//
//   1. The CRD's spec.facade.mcp.enabled is honored — the operator
//      emits the mcp container/service port, the agent binary starts
//      the listener.
//   2. POST /mcp speaks JSON-RPC 2.0 per the MCP 2025-03-26 spec:
//      initialize / tools/list / tools/call.
//   3. tools/list advertises the function's CRD-declared input schema
//      as the Tool's inputSchema — no generic {query:string} fallback.
//   4. Unauthenticated requests reach the server (allow-unauthenticated
//      fallback) instead of being dropped at the network layer.
//
// What this suite intentionally does NOT cover:
//   - Auth chain failure paths (the WWW-Authenticate header shape).
//     The cmd/agent wiring test exercises that without a cluster.
//   - End-to-end interop with PromptKit's StreamableClient. That
//     validation belongs in a separate test that runs from outside
//     the cluster via port-forward; this suite uses a Python pod
//     because it matches the existing function-mode test pattern
//     and keeps the assertion focused on the wire protocol.

const mcpFunctionsNamespace = "test-mcp-functions"

var _ = Describe("Functions MCP", Ordered, Label("functions", "mcp"), func() {
	const (
		functionName       = "test-fn-mcp"
		functionPackName   = "test-fn-mcp-prompts"
		functionProviderID = "test-fn-mcp-provider"
	)

	BeforeAll(func() {
		By("ensuring CRDs are installed and the controller-manager is deployed")
		Expect(ensureManagerDeployed()).To(Succeed())

		By("ensuring session-api + postgres are deployed in omnia-system")
		Expect(ensureSessionApiDeployed()).To(Succeed())

		By("creating the test-mcp-functions namespace if absent")
		cmd := exec.Command("kubectl", "create", "ns", mcpFunctionsNamespace)
		_, _ = utils.Run(cmd) // tolerate AlreadyExists

		By("applying a workspace pointing at the shared session-api")
		sessionApiURL := "http://omnia-session-api.omnia-system.svc.cluster.local:8080"
		workspaceManifest := fmt.Sprintf(`
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Workspace
metadata:
  name: e2e-mcp-functions-workspace
spec:
  displayName: E2E MCP Functions Workspace
  namespace:
    name: %[1]s
  services:
    - name: default
      mode: external
      external:
        sessionURL: "%[2]s"
        memoryURL: "%[2]s"
`, mcpFunctionsNamespace, sessionApiURL)
		wsCmd := exec.Command("kubectl", "apply", "-f", "-")
		wsCmd.Stdin = strings.NewReader(workspaceManifest)
		_, err := utils.Run(wsCmd)
		Expect(err).NotTo(HaveOccurred(), "failed to apply Workspace")

		By("waiting for the Workspace to report Ready")
		verifyWs := func(g Gomega) {
			c := exec.Command("kubectl", "get", "workspace", "e2e-mcp-functions-workspace",
				"-o", "jsonpath={.status.services[0].ready}")
			out, e := utils.Run(c)
			g.Expect(e).NotTo(HaveOccurred())
			g.Expect(out).To(Equal("true"))
		}
		Eventually(verifyWs, 2*time.Minute, 2*time.Second).Should(Succeed())

		By("creating a minimal PromptPack ConfigMap + CR")
		packManifest := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: %[1]s
  namespace: %[2]s
data:
  pack.json: |
    {
      "id": "%[1]s",
      "name": "%[1]s",
      "version": "1.0.0",
      "template_engine": { "version": "v1", "syntax": "{{variable}}" },
      "prompts": {
        "default": {
          "id": "default",
          "name": "default",
          "version": "1.0.0",
          "system_template": "You are an echo service."
        }
      }
    }
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: PromptPack
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  packName: %[1]s
  source:
    type: configmap
    configMapRef:
      name: %[1]s
  version: "1.0.0"
`, functionPackName, mcpFunctionsNamespace)
		packCmd := exec.Command("kubectl", "apply", "-f", "-")
		packCmd.Stdin = strings.NewReader(packManifest)
		_, err = utils.Run(packCmd)
		Expect(err).NotTo(HaveOccurred(), "failed to apply PromptPack")

		By("creating a mock Provider")
		providerManifest := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %[1]s
  namespace: %[2]s
type: Opaque
stringData:
  api-key: mock-not-a-real-key
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  type: mock
  credential:
    secretRef:
      name: %[1]s
`, functionProviderID, mcpFunctionsNamespace)
		provCmd := exec.Command("kubectl", "apply", "-f", "-")
		provCmd.Stdin = strings.NewReader(providerManifest)
		_, err = utils.Run(provCmd)
		Expect(err).NotTo(HaveOccurred(), "failed to apply Provider")

		By("creating an MCP-enabled function-mode AgentRuntime")
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
    - type: mcp
      mcp:
        port: 9998
  inputSchema:
    type: object
    required: ["message"]
    properties:
      message:
        type: string
        description: "text to echo"
  outputSchema:
    type: object
    properties:
      echo:
        type: string
  runtime:
    replicas: 1
    resources:
      requests:
        cpu: "50m"
        memory: "64Mi"
      limits:
        cpu: "200m"
        memory: "256Mi"
`, functionName, mcpFunctionsNamespace, functionPackName)
		arCmd := exec.Command("kubectl", "apply", "-f", "-")
		arCmd.Stdin = strings.NewReader(arManifest)
		_, err = utils.Run(arCmd)
		Expect(err).NotTo(HaveOccurred(), "failed to apply AgentRuntime")

		DeferCleanup(func() {
			if !CurrentSpecReport().Failed() {
				return
			}
			_, _ = fmt.Fprintf(GinkgoWriter, "\n=== DEBUG: mcp-functions setup failed ===\n")
			arGet := exec.Command("kubectl", "get", "agentruntime", functionName, "-n", mcpFunctionsNamespace, "-o", "yaml")
			if out, e := utils.Run(arGet); e == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "AgentRuntime:\n%s\n", out)
			}
			svcGet := exec.Command("kubectl", "get", "svc", functionName, "-n", mcpFunctionsNamespace, "-o", "yaml")
			if out, e := utils.Run(svcGet); e == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Service:\n%s\n", out)
			}
			podLogs := exec.Command("kubectl", "logs",
				"-n", mcpFunctionsNamespace,
				"-l", "app.kubernetes.io/name="+functionName,
				"--tail=200", "--all-containers=true")
			if out, e := utils.Run(podLogs); e == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "agent pod logs:\n%s\n", out)
			}
		})

		By("waiting for the function-mode Deployment to be Ready")
		verifyReady := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "deployment", functionName,
				"-n", mcpFunctionsNamespace,
				"-o", "jsonpath={.status.readyReplicas}")
			out, e := utils.Run(cmd)
			g.Expect(e).NotTo(HaveOccurred())
			g.Expect(out).To(Equal("1"), "function-mode pod must have 1 ready replica")
		}
		Eventually(verifyReady, 3*time.Minute, 2*time.Second).Should(Succeed())

		By("waiting for the Service to expose the mcp port")
		verifyMCPPort := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "svc", functionName,
				"-n", mcpFunctionsNamespace,
				"-o", "jsonpath={.spec.ports[?(@.name=='mcp')].port}")
			out, e := utils.Run(cmd)
			g.Expect(e).NotTo(HaveOccurred())
			g.Expect(out).To(Equal("9998"), "Service must expose mcp port 9998")
		}
		Eventually(verifyMCPPort, 1*time.Minute, 2*time.Second).Should(Succeed())
	})

	AfterAll(func() {
		_, _ = utils.Run(exec.Command("kubectl", "delete", "workspace",
			"e2e-mcp-functions-workspace", "--ignore-not-found", "--timeout=30s"))
		_, _ = utils.Run(exec.Command("kubectl", "delete", "ns",
			mcpFunctionsNamespace, "--ignore-not-found", "--timeout=60s"))
	})

	It("serves initialize / tools/list / tools/call per the MCP 2025-03-26 spec", func() {
		// The test pod posts JSON-RPC envelopes to /mcp and asserts the
		// wire-level contract. We intentionally don't use a Python MCP
		// client library — the protocol is so thin that requests +
		// json prove the assertions more directly. PromptKit's
		// StreamableClient interop is exercised separately via the
		// Go test process (out of scope here).
		testManifest := fmt.Sprintf(`
apiVersion: v1
kind: Pod
metadata:
  name: fn-mcp-test
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
      import requests

      MCP_URL = "http://%[2]s.%[1]s.svc.cluster.local:9998/mcp"
      HEADERS = {"Content-Type": "application/json"}

      def rpc(method, params=None, req_id=1):
          body = {"jsonrpc": "2.0", "id": req_id, "method": method,
                  "params": params or {}}
          r = requests.post(MCP_URL, headers=HEADERS, json=body, timeout=15)
          print(f"  {method} -> status={r.status_code}")
          if r.status_code != 200:
              print(f"ERROR: {method} returned non-200: {r.status_code} body={r.text[:200]}")
              sys.exit(1)
          return r.json()

      # 1. initialize must return protocolVersion=2025-03-26 and advertise tools.
      print("=== initialize ===")
      init_resp = rpc("initialize")
      if init_resp.get("error"):
          print(f"ERROR: initialize returned JSON-RPC error: {init_resp['error']!r}")
          sys.exit(1)
      result = init_resp["result"]
      if result["protocolVersion"] != "2025-03-26":
          print(f"ERROR: protocolVersion={result['protocolVersion']!r} want 2025-03-26")
          sys.exit(1)
      if "tools" not in (result.get("capabilities") or {}):
          print(f"ERROR: capabilities missing tools: {result.get('capabilities')!r}")
          sys.exit(1)
      print(f"  protocolVersion={result['protocolVersion']} serverInfo={result['serverInfo']!r}")

      # 2. tools/list must return exactly one tool with our CRD-declared schema.
      print("=== tools/list ===")
      list_resp = rpc("tools/list", req_id=2)
      if list_resp.get("error"):
          print(f"ERROR: tools/list returned JSON-RPC error: {list_resp['error']!r}")
          sys.exit(1)
      tools = list_resp["result"]["tools"]
      if len(tools) != 1:
          print(f"ERROR: expected 1 tool, got {len(tools)}: {tools!r}")
          sys.exit(1)
      tool = tools[0]
      print(f"  tool: name={tool['name']!r} inputSchema_keys={list((tool.get('inputSchema') or {}).get('properties', {}).keys())!r}")
      if tool["name"].lower() != "%[2]s":
          print(f"ERROR: expected tool name=%[2]s; got {tool['name']!r}")
          sys.exit(1)
      schema = tool.get("inputSchema") or {}
      props = (schema.get("properties") or {})
      if "message" not in props:
          print(f"ERROR: inputSchema.properties missing 'message'; got {props!r}")
          sys.exit(1)
      required = schema.get("required") or []
      if "message" not in required:
          print(f"ERROR: 'message' should be in inputSchema.required; got {required!r}")
          sys.exit(1)

      # 3. tools/call routes to the FunctionInvoker. The mock runtime
      # may or may not emit schema-conforming JSON; we tolerate either
      # outcome but the response shape must be a valid CallToolResult.
      print("=== tools/call ===")
      call_resp = rpc("tools/call", {"name": "%[2]s",
                                       "arguments": {"message": "hi"}}, req_id=3)
      if call_resp.get("error"):
          print(f"ERROR: tools/call returned JSON-RPC protocol error: {call_resp['error']!r}")
          sys.exit(1)
      ct_result = call_resp["result"]
      content = ct_result.get("content")
      if not isinstance(content, list) or not content:
          print(f"ERROR: CallToolResult.content must be a non-empty list; got {content!r}")
          sys.exit(1)
      print(f"  isError={ct_result.get('isError', False)} content[0].type={content[0].get('type')!r}")
      # IsError true or false is acceptable here — the runtime is mocked.

      # 4. Unknown tool name → CallToolResult{isError:true}, NOT a
      # JSON-RPC protocol error.
      print("=== tools/call with unknown tool ===")
      unk_resp = rpc("tools/call", {"name": "nonexistent",
                                      "arguments": {}}, req_id=4)
      if unk_resp.get("error"):
          print(f"ERROR: unknown tool should return CallToolResult{{isError:true}}, not protocol error: {unk_resp['error']!r}")
          sys.exit(1)
      if not unk_resp["result"].get("isError"):
          print(f"ERROR: unknown tool name must set isError=true; got {unk_resp['result']!r}")
          sys.exit(1)

      # 5. Unknown JSON-RPC method → -32601.
      print("=== unknown method ===")
      bad_method = rpc("does/not/exist", req_id=5)
      err = bad_method.get("error")
      if not err or err.get("code") != -32601:
          print(f"ERROR: unknown method should return code -32601; got {err!r}")
          sys.exit(1)

      print("=== ALL MCP ASSERTIONS PASSED ===")
      PYTHON_SCRIPT
`, mcpFunctionsNamespace, functionName)
		applyCmd := exec.Command("kubectl", "apply", "-f", "-")
		applyCmd.Stdin = strings.NewReader(testManifest)
		_, err := utils.Run(applyCmd)
		Expect(err).NotTo(HaveOccurred(), "failed to apply test pod")

		DeferCleanup(func() {
			_, _ = utils.Run(exec.Command("kubectl", "delete", "pod",
				"fn-mcp-test", "-n", mcpFunctionsNamespace,
				"--ignore-not-found", "--timeout=30s"))
		})

		By("waiting for the MCP test pod to complete")
		verifyComplete := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pod", "fn-mcp-test",
				"-n", mcpFunctionsNamespace,
				"-o", "jsonpath={.status.phase}")
			out, e := utils.Run(cmd)
			g.Expect(e).NotTo(HaveOccurred())
			g.Expect(out).To(Equal("Succeeded"), "test pod must reach Succeeded phase")
		}
		Eventually(verifyComplete, 3*time.Minute, 3*time.Second).Should(Succeed(),
			"test pod failed — see kubectl logs fn-mcp-test -n "+mcpFunctionsNamespace)

		By("emitting test pod logs for visibility")
		logsCmd := exec.Command("kubectl", "logs", "fn-mcp-test", "-n", mcpFunctionsNamespace)
		if out, e := utils.Run(logsCmd); e == nil {
			_, _ = fmt.Fprintf(GinkgoWriter, "fn-mcp-test logs:\n%s\n", out)
		}
	})
})
