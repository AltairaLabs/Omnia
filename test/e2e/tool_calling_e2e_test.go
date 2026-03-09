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

// demoNamespace is where Tilt demo agents are deployed.
const demoNamespace = "omnia-demo"

// toolsDemoAgent is the name of the tools-demo agent deployed by the omnia-demos chart.
const toolsDemoAgent = "tools-demo"

var _ = Describe("Tool Calling E2E", Ordered, Label("tool-calling"), func() {
	BeforeAll(func() {
		if os.Getenv("ENABLE_TOOL_CALLING_E2E") != "true" {
			Skip("ENABLE_TOOL_CALLING_E2E not set — skipping tool calling tests")
		}

		By("verifying the tools-demo agent pod is running")
		verifyAgentRunning := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pods",
				"-n", demoNamespace,
				"-l", fmt.Sprintf("app.kubernetes.io/instance=%s", toolsDemoAgent),
				"-o", "jsonpath={.items[0].status.phase}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("Running"))
		}
		Eventually(verifyAgentRunning, 3*time.Minute, 5*time.Second).Should(Succeed())

		By("verifying all containers are ready")
		verifyReady := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pods",
				"-n", demoNamespace,
				"-l", fmt.Sprintf("app.kubernetes.io/instance=%s", toolsDemoAgent),
				"-o", "jsonpath={.items[0].status.containerStatuses[*].ready}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("true true"), "Both facade and runtime containers should be ready")
		}
		Eventually(verifyReady, 5*time.Minute, 5*time.Second).Should(Succeed())

		By("verifying Ollama is healthy")
		verifyOllama := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pods",
				"-n", demoNamespace,
				"-l", "app.kubernetes.io/name=ollama",
				"-o", "jsonpath={.items[0].status.phase}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("Running"))
		}
		Eventually(verifyOllama, 2*time.Minute, 5*time.Second).Should(Succeed())

		By("verifying the service endpoint is ready")
		verifyEndpoint := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "endpoints", toolsDemoAgent,
				"-n", demoNamespace, "-o", "jsonpath={.subsets[0].addresses[0].ip}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).NotTo(BeEmpty(), "Service endpoint should have an IP")
		}
		Eventually(verifyEndpoint, time.Minute, 2*time.Second).Should(Succeed())
	})

	AfterAll(func() {
		for _, pod := range []string{"tool-calc-test", "tool-weather-test"} {
			cmd := exec.Command("kubectl", "delete", "pod", pod,
				"-n", demoNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		}
	})

	It("should execute the calculate tool via Ollama", func() {
		podName := "tool-calc-test"
		wsURI := fmt.Sprintf("ws://%s.%s.svc.cluster.local:8080/ws?agent=%s",
			toolsDemoAgent, demoNamespace, toolsDemoAgent)

		By("creating a test pod that asks the agent to calculate something")
		manifest := fmt.Sprintf(`
apiVersion: v1
kind: Pod
metadata:
  name: %s
  namespace: %s
spec:
  restartPolicy: Never
  containers:
  - name: python
    image: python:3.11-slim
    command: ["sh", "-c"]
    args:
    - |
      pip install websockets --quiet
      python3 << 'PYTHON_SCRIPT'
      import asyncio
      import json
      import websockets
      import sys

      async def test_calculate():
          uri = "%s"
          try:
              async with websockets.connect(uri, ping_interval=None, open_timeout=30) as ws:
                  msg = {"type": "message", "content": "What is 42 * 17? Use the calculate tool."}
                  await ws.send(json.dumps(msg))
                  print(f"Sent: {msg['content']}")

                  received_types = []
                  tool_call_names = []
                  tool_result_count = 0
                  received_done = False

                  for _ in range(30):
                      try:
                          response = await asyncio.wait_for(ws.recv(), timeout=120)
                          data = json.loads(response)
                          msg_type = data.get("type")
                          received_types.append(msg_type)
                          print(f"  [{msg_type}] {json.dumps(data)[:300]}")

                          if msg_type == "tool_call":
                              tc = data.get("tool_call", {})
                              tool_call_names.append(tc.get("name", "unknown"))

                          elif msg_type == "tool_result":
                              tool_result_count += 1
                              tr = data.get("tool_result", {})
                              if tr.get("error"):
                                  print(f"  TOOL ERROR: {tr['error']}")

                          elif msg_type == "done":
                              received_done = True
                              break

                          elif msg_type == "error":
                              print(f"ERROR: {data.get('error')}")
                              sys.exit(1)

                      except asyncio.TimeoutError:
                          print("Timeout waiting for messages")
                          break

                  print(f"\nMessage types: {received_types}")
                  print(f"Tool calls: {tool_call_names}")
                  print(f"Tool results: {tool_result_count}")

                  errors = []
                  if "calculate" not in tool_call_names:
                      errors.append(f"Expected 'calculate' tool call, got: {tool_call_names}")
                  if tool_result_count == 0:
                      errors.append("No tool_result received")
                  if not received_done:
                      errors.append("Conversation did not complete (no 'done' message)")

                  if errors:
                      for e in errors:
                          print(f"ERROR: {e}")
                      sys.exit(1)

                  print("\nTEST PASSED: Calculate tool call verified")

          except Exception as e:
              print(f"ERROR: {e}")
              import traceback
              traceback.print_exc()
              sys.exit(1)

      asyncio.run(test_calculate())
      PYTHON_SCRIPT
`, podName, demoNamespace, wsURI)

		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(manifest)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create calc test pod")

		By("waiting for the test to complete")
		verifyComplete := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pod", podName,
				"-n", demoNamespace, "-o", "jsonpath={.status.phase}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(BeElementOf("Succeeded", "Failed"))
		}
		Eventually(verifyComplete, 5*time.Minute, 5*time.Second).Should(Succeed())

		By("checking the test pod logs")
		cmd = exec.Command("kubectl", "logs", podName, "-n", demoNamespace)
		output, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		_, _ = fmt.Fprintf(GinkgoWriter, "Calculate test output:\n%s\n", output)

		Expect(output).To(ContainSubstring("TEST PASSED"), "Calculate tool call test should pass")
		Expect(output).To(ContainSubstring("tool_call"), "Should receive tool_call message")
		Expect(output).To(ContainSubstring("tool_result"), "Should receive tool_result message")
	})

	It("should execute multi-step tool calls for weather lookup via Ollama", func() {
		podName := "tool-weather-test"
		wsURI := fmt.Sprintf("ws://%s.%s.svc.cluster.local:8080/ws?agent=%s",
			toolsDemoAgent, demoNamespace, toolsDemoAgent)

		By("creating a test pod that asks for weather (requires search_places → get_weather)")
		manifest := fmt.Sprintf(`
apiVersion: v1
kind: Pod
metadata:
  name: %s
  namespace: %s
spec:
  restartPolicy: Never
  containers:
  - name: python
    image: python:3.11-slim
    command: ["sh", "-c"]
    args:
    - |
      pip install websockets --quiet
      python3 << 'PYTHON_SCRIPT'
      import asyncio
      import json
      import websockets
      import sys

      async def test_weather():
          uri = "%s"
          try:
              async with websockets.connect(uri, ping_interval=None, open_timeout=30) as ws:
                  msg = {
                      "type": "message",
                      "content": "What is the current weather in London? First use search_places to find the coordinates, then use get_weather with those coordinates."
                  }
                  await ws.send(json.dumps(msg))
                  print(f"Sent: {msg['content']}")

                  received_types = []
                  tool_call_names = []
                  tool_result_count = 0
                  received_done = False

                  for _ in range(50):
                      try:
                          response = await asyncio.wait_for(ws.recv(), timeout=120)
                          data = json.loads(response)
                          msg_type = data.get("type")
                          received_types.append(msg_type)
                          print(f"  [{msg_type}] {json.dumps(data)[:300]}")

                          if msg_type == "tool_call":
                              tc = data.get("tool_call", {})
                              tool_call_names.append(tc.get("name", "unknown"))

                          elif msg_type == "tool_result":
                              tool_result_count += 1
                              tr = data.get("tool_result", {})
                              if tr.get("error"):
                                  print(f"  TOOL ERROR: {tr['error']}")

                          elif msg_type == "done":
                              received_done = True
                              break

                          elif msg_type == "error":
                              print(f"ERROR: {data.get('error')}")
                              sys.exit(1)

                      except asyncio.TimeoutError:
                          print("Timeout waiting for messages")
                          break

                  print(f"\nMessage types: {received_types}")
                  print(f"Tool calls: {tool_call_names}")
                  print(f"Tool results: {tool_result_count}")

                  errors = []
                  if len(tool_call_names) == 0:
                      errors.append("No tool calls made by the model")
                  if tool_result_count == 0:
                      errors.append("No tool_result received")
                  if not received_done:
                      errors.append("Conversation did not complete (no 'done' message)")

                  # Verify only known tools were called
                  known_tools = {"search_places", "get_weather", "calculate"}
                  unknown = [t for t in tool_call_names if t not in known_tools]
                  if unknown:
                      errors.append(f"Unknown tool calls: {unknown}")

                  if errors:
                      for e in errors:
                          print(f"ERROR: {e}")
                      sys.exit(1)

                  print("\nTEST PASSED: Weather multi-step tool call verified")
                  print(f"  Tool chain: {' -> '.join(tool_call_names)}")

          except Exception as e:
              print(f"ERROR: {e}")
              import traceback
              traceback.print_exc()
              sys.exit(1)

      asyncio.run(test_weather())
      PYTHON_SCRIPT
`, podName, demoNamespace, wsURI)

		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(manifest)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create weather test pod")

		By("waiting for the test to complete")
		verifyComplete := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pod", podName,
				"-n", demoNamespace, "-o", "jsonpath={.status.phase}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(BeElementOf("Succeeded", "Failed"))
		}
		Eventually(verifyComplete, 8*time.Minute, 5*time.Second).Should(Succeed())

		By("checking the test pod logs")
		cmd = exec.Command("kubectl", "logs", podName, "-n", demoNamespace)
		output, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		_, _ = fmt.Fprintf(GinkgoWriter, "Weather test output:\n%s\n", output)

		Expect(output).To(ContainSubstring("TEST PASSED"), "Weather tool call test should pass")
		Expect(output).To(ContainSubstring("tool_call"), "Should receive tool_call message")
		Expect(output).To(ContainSubstring("tool_result"), "Should receive tool_result message")
	})
})
