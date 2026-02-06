//go:build smoke

/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.

Smoke tests for arena-dev-console that run against a live cluster.
Run with: go test -tags=smoke -v ./ee/cmd/arena-dev-console/server/... -namespace=dev-agents

These tests require:
- A running Kubernetes cluster with arena-dev-console pods
- kubectl configured to access the cluster
*/

package server

import (
	"bytes"
	"flag"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testNamespace = flag.String("namespace", "dev-agents", "Namespace containing dev console pods")

// runKubectl runs a kubectl command and returns the output.
func runKubectl(t *testing.T, args ...string) string {
	cmd := exec.Command("kubectl", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		t.Logf("kubectl %s failed: %v\nstderr: %s", strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimSpace(stdout.String())
}

// TestSmokeDevConsolePodRunning verifies that dev console pods are running
// and don't have permission denied errors.
func TestSmokeDevConsolePodRunning(t *testing.T) {
	// Find dev console pods
	pods := runKubectl(t, "get", "pods", "-n", *testNamespace,
		"-l", "app.kubernetes.io/component=arena-dev-console",
		"-o", "jsonpath={.items[*].metadata.name}")

	if pods == "" {
		// Try alternative label selector for adc- pods
		pods = runKubectl(t, "get", "pods", "-n", *testNamespace,
			"-o", "jsonpath={range .items[?(@.metadata.name)]}{.metadata.name}{\"\\n\"}{end}")
		// Filter for adc- pods
		var adcPods []string
		for _, p := range strings.Split(pods, "\n") {
			if strings.HasPrefix(p, "adc-") {
				adcPods = append(adcPods, p)
			}
		}
		if len(adcPods) == 0 {
			t.Skip("No arena-dev-console pods found in namespace " + *testNamespace)
		}
		pods = strings.Join(adcPods, " ")
	}

	t.Logf("Found dev console pods: %s", pods)

	for _, pod := range strings.Fields(pods) {
		t.Run(pod, func(t *testing.T) {
			// Check pod status
			status := runKubectl(t, "get", "pod", pod, "-n", *testNamespace,
				"-o", "jsonpath={.status.phase}")
			assert.Equal(t, "Running", status, "Pod should be Running")

			// Check container ready status
			ready := runKubectl(t, "get", "pod", pod, "-n", *testNamespace,
				"-o", "jsonpath={.status.containerStatuses[0].ready}")
			assert.Equal(t, "true", ready, "Container should be ready")
		})
	}
}

// TestSmokeDevConsoleNoPermissionErrors checks pod logs for permission errors.
func TestSmokeDevConsoleNoPermissionErrors(t *testing.T) {
	// Find dev console pods
	pods := runKubectl(t, "get", "pods", "-n", *testNamespace,
		"-o", "jsonpath={range .items[*]}{.metadata.name}{\"\\n\"}{end}")

	var adcPods []string
	for _, p := range strings.Split(pods, "\n") {
		if strings.HasPrefix(p, "adc-") {
			adcPods = append(adcPods, p)
		}
	}

	if len(adcPods) == 0 {
		t.Skip("No arena-dev-console pods found")
	}

	for _, pod := range adcPods {
		t.Run(pod, func(t *testing.T) {
			logs := runKubectl(t, "logs", pod, "-n", *testNamespace, "--tail=200")

			// Check for known error patterns
			assert.NotContains(t, logs, "permission denied",
				"Logs should not contain permission denied errors")
			assert.NotContains(t, logs, "mkdir out:",
				"Logs should not show mkdir out: errors")
			assert.NotContains(t, logs, "failed to create media file store",
				"Logs should not show media file store errors")
			assert.NotContains(t, logs, "failed to build provider registry",
				"Logs should not show provider registry build errors")

			// Verify startup was successful
			assert.Contains(t, logs, "starting arena-dev-console",
				"Logs should show successful startup")
		})
	}
}

// TestSmokeDevConsoleHealthEndpoint checks the health endpoint via port-forward.
func TestSmokeDevConsoleHealthEndpoint(t *testing.T) {
	// Find a dev console pod
	pods := runKubectl(t, "get", "pods", "-n", *testNamespace,
		"-o", "jsonpath={range .items[*]}{.metadata.name}{\"\\n\"}{end}")

	var podName string
	for _, p := range strings.Split(pods, "\n") {
		if strings.HasPrefix(p, "adc-") {
			podName = p
			break
		}
	}

	if podName == "" {
		t.Skip("No arena-dev-console pods found")
	}

	// Check readiness via kubectl
	ready := runKubectl(t, "get", "pod", podName, "-n", *testNamespace,
		"-o", "jsonpath={.status.containerStatuses[0].ready}")
	require.Equal(t, "true", ready, "Pod should be ready (health check passed)")
}

// TestSmokeDevConsoleProviderLoading tests that the dev console can load providers.
func TestSmokeDevConsoleProviderLoading(t *testing.T) {
	// Check if there are providers in the namespace
	providers := runKubectl(t, "get", "provider", "-n", *testNamespace,
		"-o", "jsonpath={.items[*].metadata.name}")

	if providers == "" {
		t.Log("No providers found in namespace, checking pod can still start")
	} else {
		t.Logf("Found providers: %s", providers)
	}

	// Find dev console pods
	pods := runKubectl(t, "get", "pods", "-n", *testNamespace,
		"-o", "jsonpath={range .items[*]}{.metadata.name}{\"\\n\"}{end}")

	var adcPods []string
	for _, p := range strings.Split(pods, "\n") {
		if strings.HasPrefix(p, "adc-") {
			adcPods = append(adcPods, p)
		}
	}

	if len(adcPods) == 0 {
		t.Skip("No arena-dev-console pods found")
	}

	for _, pod := range adcPods {
		t.Run(pod, func(t *testing.T) {
			logs := runKubectl(t, "logs", pod, "-n", *testNamespace, "--tail=200")

			// Check for K8s provider loader initialization
			assert.Contains(t, logs, "K8s provider loader initialized",
				"Should show K8s provider loader was initialized")

			// If providers exist, check they were loaded without errors
			if providers != "" {
				assert.NotContains(t, logs, "failed to load providers",
					"Should not fail to load providers")
			}
		})
	}
}

// TestSmokeDevConsoleRestartRecovery tests that a pod can restart without issues.
func TestSmokeDevConsoleRestartRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping restart test in short mode")
	}

	// Find a dev console pod
	pods := runKubectl(t, "get", "pods", "-n", *testNamespace,
		"-o", "jsonpath={range .items[*]}{.metadata.name}{\"\\n\"}{end}")

	var podName string
	for _, p := range strings.Split(pods, "\n") {
		if strings.HasPrefix(p, "adc-") {
			podName = p
			break
		}
	}

	if podName == "" {
		t.Skip("No arena-dev-console pods found")
	}

	// Get the deployment/replicaset that owns this pod
	ownerKind := runKubectl(t, "get", "pod", podName, "-n", *testNamespace,
		"-o", "jsonpath={.metadata.ownerReferences[0].kind}")
	ownerName := runKubectl(t, "get", "pod", podName, "-n", *testNamespace,
		"-o", "jsonpath={.metadata.ownerReferences[0].name}")

	t.Logf("Pod %s owned by %s/%s", podName, ownerKind, ownerName)

	// Delete the pod
	t.Log("Deleting pod to test restart recovery...")
	runKubectl(t, "delete", "pod", podName, "-n", *testNamespace, "--wait=false")

	// Wait for new pod to come up
	t.Log("Waiting for new pod to start...")
	time.Sleep(10 * time.Second)

	// Check for new running pod
	var newPodName string
	for i := 0; i < 30; i++ {
		pods := runKubectl(t, "get", "pods", "-n", *testNamespace,
			"-o", "jsonpath={range .items[*]}{.metadata.name} {.status.phase}{\"\\n\"}{end}")

		for _, line := range strings.Split(pods, "\n") {
			parts := strings.Fields(line)
			if len(parts) >= 2 && strings.HasPrefix(parts[0], "adc-") && parts[1] == "Running" {
				newPodName = parts[0]
				break
			}
		}
		if newPodName != "" {
			break
		}
		time.Sleep(2 * time.Second)
	}

	require.NotEmpty(t, newPodName, "New pod should have started")
	t.Logf("New pod started: %s", newPodName)

	// Check logs of new pod for permission errors
	time.Sleep(5 * time.Second) // Give it time to initialize
	logs := runKubectl(t, "logs", newPodName, "-n", *testNamespace, "--tail=100")

	assert.NotContains(t, logs, "permission denied",
		"Restarted pod should not have permission denied errors")
	assert.Contains(t, logs, "starting arena-dev-console",
		"Restarted pod should show successful startup")
}
