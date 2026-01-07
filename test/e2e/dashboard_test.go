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

var _ = Describe("Dashboard WebSocket Proxy", Ordered, func() {
	const (
		dashboardNamespace = "dashboard-test"
		dashboardImage     = "example.com/omnia-dashboard:v0.0.1"
	)

	BeforeAll(func() {
		By("building the dashboard image")
		cmd := exec.Command("docker", "build", "-t", dashboardImage, "-f", "dashboard/Dockerfile", "./dashboard")
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to build dashboard image")

		By("loading dashboard image into Kind")
		err = utils.LoadImageToKindClusterWithName(dashboardImage)
		Expect(err).NotTo(HaveOccurred(), "Failed to load dashboard image into Kind")

		By("creating dashboard test namespace")
		cmd = exec.Command("kubectl", "create", "ns", dashboardNamespace)
		_, _ = utils.Run(cmd) // Ignore if exists

		By("deploying test agent for dashboard to connect to")
		// Create necessary prerequisites
		prereqManifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: dashboard-test-prompts
  namespace: dashboard-test
data:
  system.txt: |
    You are a test assistant for dashboard E2E testing.
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: PromptPack
metadata:
  name: dashboard-test-prompts
  namespace: dashboard-test
spec:
  source:
    type: configmap
    configMapRef:
      name: dashboard-test-prompts
  version: "1.0.0"
  rollout:
    type: immediate
---
apiVersion: v1
kind: Secret
metadata:
  name: dashboard-test-provider
  namespace: dashboard-test
type: Opaque
stringData:
  api-key: "test-api-key-dashboard"
---
apiVersion: omnia.altairalabs.ai/v1alpha1
kind: AgentRuntime
metadata:
  name: dashboard-test-agent
  namespace: dashboard-test
  annotations:
    omnia.altairalabs.ai/mock-provider: "true"
spec:
  promptPackRef:
    name: dashboard-test-prompts
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
  provider:
    secretRef:
      name: dashboard-test-provider
`
		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(prereqManifest)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test agent prerequisites")

		By("waiting for test agent to be ready")
		verifyAgentReady := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pods",
				"-n", dashboardNamespace,
				"-l", "app.kubernetes.io/instance=dashboard-test-agent",
				"-o", "jsonpath={.items[0].status.containerStatuses[*].ready}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.Count(output, "true")).To(Equal(2), "Both containers should be ready")
		}
		Eventually(verifyAgentReady, 3*time.Minute, 5*time.Second).Should(Succeed())

		By("deploying the dashboard")
		dashboardManifest := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: omnia-dashboard
  namespace: dashboard-test
spec:
  replicas: 1
  selector:
    matchLabels:
      app: omnia-dashboard
  template:
    metadata:
      labels:
        app: omnia-dashboard
    spec:
      containers:
      - name: dashboard
        image: %s
        imagePullPolicy: Never
        ports:
        - containerPort: 3000
        env:
        - name: NODE_ENV
          value: "production"
        - name: NEXT_PUBLIC_DEMO_MODE
          value: "false"
        - name: OPERATOR_API_URL
          value: "http://omnia-controller-manager.omnia-system.svc.cluster.local:8081"
        - name: SERVICE_DOMAIN
          value: "svc.cluster.local"
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
---
apiVersion: v1
kind: Service
metadata:
  name: omnia-dashboard
  namespace: dashboard-test
spec:
  selector:
    app: omnia-dashboard
  ports:
  - port: 3000
    targetPort: 3000
`, dashboardImage)

		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(dashboardManifest)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy dashboard")

		By("waiting for dashboard to be ready")
		verifyDashboardReady := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pods",
				"-n", dashboardNamespace,
				"-l", "app=omnia-dashboard",
				"-o", "jsonpath={.items[0].status.phase}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("Running"))
		}
		Eventually(verifyDashboardReady, 3*time.Minute, 5*time.Second).Should(Succeed())

		// Wait a bit for the server to fully start
		time.Sleep(5 * time.Second)
	})

	AfterAll(func() {
		By("cleaning up dashboard test namespace")
		cmd := exec.Command("kubectl", "delete", "ns", dashboardNamespace, "--ignore-not-found")
		_, _ = utils.Run(cmd)
	})

	It("should serve the dashboard health endpoint", func() {
		By("checking dashboard health via curl pod")
		healthTestManifest := `
apiVersion: v1
kind: Pod
metadata:
  name: dashboard-health-test
  namespace: dashboard-test
spec:
  restartPolicy: Never
  containers:
  - name: curl
    image: curlimages/curl:latest
    command: ["sh", "-c"]
    args:
    - |
      echo "Testing dashboard health endpoint..."
      curl -v http://omnia-dashboard.dashboard-test.svc.cluster.local:3000/api/health 2>&1
      echo ""
      echo "Test complete"
`
		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(healthTestManifest)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for health test to complete")
		verifyHealthTest := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pod", "dashboard-health-test",
				"-n", dashboardNamespace, "-o", "jsonpath={.status.phase}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("Succeeded"))
		}
		Eventually(verifyHealthTest, 2*time.Minute, 5*time.Second).Should(Succeed())

		By("checking health test logs")
		cmd = exec.Command("kubectl", "logs", "dashboard-health-test", "-n", dashboardNamespace)
		output, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(ContainSubstring("200"), "Health endpoint should return 200")

		By("cleaning up health test pod")
		cmd = exec.Command("kubectl", "delete", "pod", "dashboard-health-test", "-n", dashboardNamespace, "--ignore-not-found")
		_, _ = utils.Run(cmd)
	})

	It("should proxy WebSocket connections to agents", func() {
		By("creating a WebSocket test pod")
		wsTestManifest := `
apiVersion: v1
kind: Pod
metadata:
  name: dashboard-ws-test
  namespace: dashboard-test
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

      async def test_websocket_proxy():
          # Connect through the dashboard's WebSocket proxy
          uri = "ws://omnia-dashboard.dashboard-test.svc.cluster.local:3000/api/agents/dashboard-test/dashboard-test-agent/ws"
          print(f"Connecting to: {uri}")

          try:
              async with websockets.connect(uri, ping_interval=None, close_timeout=30) as ws:
                  print("WebSocket connected to dashboard proxy")

                  # Send a test message
                  test_message = {
                      "type": "message",
                      "content": "Hello from dashboard WebSocket test"
                  }
                  await ws.send(json.dumps(test_message))
                  print(f"Sent: {test_message}")

                  # Wait for responses
                  received_connected = False
                  received_response = False

                  for _ in range(15):  # Max 15 messages
                      try:
                          response = await asyncio.wait_for(ws.recv(), timeout=30)
                          msg = json.loads(response)
                          msg_type = msg.get("type")
                          print(f"Received: {msg_type}")

                          if msg_type == "connected":
                              received_connected = True
                              session_id = msg.get("session_id", "")
                              print(f"Session ID: {session_id}")

                          elif msg_type == "chunk":
                              received_response = True
                              print(f"Chunk: {msg.get('content', '')[:50]}...")

                          elif msg_type == "done":
                              received_response = True
                              print("Response complete")
                              break

                          elif msg_type == "error":
                              print(f"ERROR from agent: {msg.get('error')}")
                              sys.exit(1)

                      except asyncio.TimeoutError:
                          print("Timeout waiting for message")
                          break

                  if not received_connected:
                      print("ERROR: Did not receive connected message")
                      sys.exit(1)

                  if not received_response:
                      print("ERROR: Did not receive response")
                      sys.exit(1)

                  print("\nTEST PASSED: Dashboard WebSocket proxy works correctly")

          except websockets.exceptions.ConnectionClosed as e:
              print(f"ERROR: WebSocket connection closed: {e}")
              sys.exit(1)
          except Exception as e:
              print(f"ERROR: {e}")
              import traceback
              traceback.print_exc()
              sys.exit(1)

      asyncio.run(test_websocket_proxy())
      PYTHON_SCRIPT
`
		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(wsTestManifest)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create WebSocket test pod")

		By("waiting for WebSocket test to complete")
		verifyWsTest := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pod", "dashboard-ws-test",
				"-n", dashboardNamespace, "-o", "jsonpath={.status.phase}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("Succeeded"))
		}
		Eventually(verifyWsTest, 3*time.Minute, 5*time.Second).Should(Succeed())

		By("checking WebSocket test logs")
		cmd = exec.Command("kubectl", "logs", "dashboard-ws-test", "-n", dashboardNamespace)
		output, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		_, _ = fmt.Fprintf(GinkgoWriter, "WebSocket test output:\n%s\n", output)
		Expect(output).To(ContainSubstring("TEST PASSED"), "Dashboard WebSocket proxy test should pass")
		Expect(output).NotTo(ContainSubstring("ERROR:"), "Should not have errors")

		By("cleaning up WebSocket test pod")
		cmd = exec.Command("kubectl", "delete", "pod", "dashboard-ws-test", "-n", dashboardNamespace, "--ignore-not-found")
		_, _ = utils.Run(cmd)
	})

	It("should return proper error when agent doesn't exist", func() {
		By("creating a test pod to connect to non-existent agent")
		errorTestManifest := `
apiVersion: v1
kind: Pod
metadata:
  name: dashboard-error-test
  namespace: dashboard-test
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

      async def test_nonexistent_agent():
          # Try to connect to a non-existent agent
          uri = "ws://omnia-dashboard.dashboard-test.svc.cluster.local:3000/api/agents/dashboard-test/nonexistent-agent/ws"
          print(f"Connecting to non-existent agent: {uri}")

          try:
              async with websockets.connect(uri, ping_interval=None, close_timeout=10) as ws:
                  # Send a message
                  await ws.send(json.dumps({"type": "message", "content": "test"}))

                  # We expect the connection to close or return an error
                  try:
                      response = await asyncio.wait_for(ws.recv(), timeout=10)
                      msg = json.loads(response)
                      print(f"Received: {msg}")

                      # Getting an error message is acceptable
                      if msg.get("type") == "error":
                          print("Got error response as expected")
                          print("TEST PASSED: Proper error handling for non-existent agent")
                          sys.exit(0)

                  except asyncio.TimeoutError:
                      print("Timeout - connection may have been rejected")

          except websockets.exceptions.ConnectionClosed as e:
              # Connection being closed is expected for non-existent agent
              print(f"Connection closed (expected): {e.code} {e.reason}")
              print("TEST PASSED: Connection properly closed for non-existent agent")
              sys.exit(0)

          except Exception as e:
              # Connection refused is also acceptable
              if "refused" in str(e).lower() or "failed" in str(e).lower():
                  print(f"Connection rejected (expected): {e}")
                  print("TEST PASSED: Connection properly rejected for non-existent agent")
                  sys.exit(0)
              print(f"Unexpected error: {e}")
              sys.exit(1)

          # If we got here without an error, the test failed
          print("ERROR: Expected connection to fail for non-existent agent")
          sys.exit(1)

      asyncio.run(test_nonexistent_agent())
      PYTHON_SCRIPT
`
		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(errorTestManifest)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for error test to complete")
		verifyErrorTest := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pod", "dashboard-error-test",
				"-n", dashboardNamespace, "-o", "jsonpath={.status.phase}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("Succeeded"))
		}
		Eventually(verifyErrorTest, 2*time.Minute, 5*time.Second).Should(Succeed())

		By("checking error test logs")
		cmd = exec.Command("kubectl", "logs", "dashboard-error-test", "-n", dashboardNamespace)
		output, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		_, _ = fmt.Fprintf(GinkgoWriter, "Error test output:\n%s\n", output)
		Expect(output).To(ContainSubstring("TEST PASSED"), "Error handling test should pass")

		By("cleaning up error test pod")
		cmd = exec.Command("kubectl", "delete", "pod", "dashboard-error-test", "-n", dashboardNamespace, "--ignore-not-found")
		_, _ = utils.Run(cmd)
	})
})
