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

package integration

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// disableSubcharts returns the Helm --set flags to disable all subcharts.
// This is needed in CI where subchart dependencies aren't available.
func disableSubcharts() []string {
	return []string{
		"--set", "prometheus.enabled=false",
		"--set", "grafana.enabled=false",
		"--set", "loki.enabled=false",
		"--set", "alloy.enabled=false",
		"--set", "tempo.enabled=false",
		"--set", "keda.enabled=false",
		"--set", "redis.enabled=false",
		"--set", "csi-driver-nfs.enabled=false",
	}
}

// TestHelmChartFlagsMatchController verifies that all flags passed to the controller
// by the Helm chart are actually defined in the controller binary.
// This prevents runtime failures due to undefined flags.
func TestHelmChartFlagsMatchController(t *testing.T) {
	// Build the controller binary
	buildCmd := exec.Command("go", "build", "-o", "/tmp/omnia-controller-test", "../../cmd/main.go")
	output, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "Failed to build controller: %s", string(output))

	// Get controller's valid flags by running --help
	helpCmd := exec.Command("/tmp/omnia-controller-test", "--help")
	helpOutput, _ := helpCmd.CombinedOutput() // Ignore error as --help exits with non-zero
	validFlags := extractFlagsFromHelp(string(helpOutput))

	// Render the Helm chart and extract flags passed to the controller
	// Use --skip-crds to avoid CRD rendering and focus on deployment
	// Disable subcharts since we only care about flags in our templates
	helmArgs := []string{
		"template", "omnia", "../../charts/omnia",
		"--set", "arena.enabled=true",
		"--set", "metrics.enabled=true",
		"--set", "devMode=true",
	}
	helmArgs = append(helmArgs, disableSubcharts()...)
	helmCmd := exec.Command("helm", helmArgs...)
	helmOutput, err := helmCmd.CombinedOutput()
	require.NoError(t, err, "Failed to render Helm template: %s", string(helmOutput))

	chartFlags := extractFlagsFromHelmTemplate(string(helmOutput))

	// Verify each flag from the Helm chart is valid
	for _, flag := range chartFlags {
		assert.Contains(t, validFlags, flag,
			"Helm chart passes undefined flag '--%s' to controller. "+
				"Either add the flag to cmd/main.go or remove it from charts/omnia/templates/deployment.yaml",
			flag)
	}
}

// extractFlagsFromHelp parses the --help output to get valid flag names.
func extractFlagsFromHelp(helpText string) []string {
	var flags []string
	// Match flags in format: -flag-name or --flag-name
	re := regexp.MustCompile(`-{1,2}([a-z][a-z0-9-]*)`)
	matches := re.FindAllStringSubmatch(helpText, -1)
	seen := make(map[string]bool)
	for _, match := range matches {
		if len(match) > 1 && !seen[match[1]] {
			flags = append(flags, match[1])
			seen[match[1]] = true
		}
	}
	return flags
}

// extractFlagsFromHelmTemplate parses helm template output to find flags
// passed to the controller container.
func extractFlagsFromHelmTemplate(templateOutput string) []string {
	var flags []string
	// Match flags in format: - --flag-name=value or - --flag-name
	re := regexp.MustCompile(`- --([a-z][a-z0-9-]*)(?:=|$|\s)`)
	matches := re.FindAllStringSubmatch(templateOutput, -1)
	seen := make(map[string]bool)
	for _, match := range matches {
		if len(match) > 1 && !seen[match[1]] {
			// Skip common K8s flags that aren't part of the controller
			if !isKubernetesFlag(match[1]) {
				flags = append(flags, match[1])
				seen[match[1]] = true
			}
		}
	}
	return flags
}

// isKubernetesFlag returns true if the flag is a common Kubernetes/infrastructure flag
// that shouldn't be validated against the controller binary.
func isKubernetesFlag(flag string) bool {
	k8sFlags := map[string]bool{
		"namespace": true,
		"name":      true,
		"selector":  true,
		// wget flags from test pod commands
		"timeout": true,
	}
	return k8sFlags[flag]
}

// TestHelmChartRenders verifies the Helm chart renders without errors
// with various configuration combinations.
func TestHelmChartRenders(t *testing.T) {
	testCases := []struct {
		name   string
		values []string
	}{
		{
			name:   "default values",
			values: []string{},
		},
		{
			name:   "arena enabled",
			values: []string{"--set", "arena.enabled=true"},
		},
		{
			name:   "dev mode",
			values: []string{"--set", "devMode=true"},
		},
		{
			name: "full configuration",
			values: []string{
				"--set", "arena.enabled=true",
				"--set", "metrics.enabled=true",
				"--set", "devMode=true",
				"--set", "dashboard.enabled=true",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"template", "omnia", "../../charts/omnia"}, tc.values...)
			// Disable all subcharts to avoid dependency issues in CI
			args = append(args, disableSubcharts()...)
			cmd := exec.Command("helm", args...)
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "Helm template failed with %s: %s", tc.name, string(output))

			// Verify output contains expected resources
			assert.True(t, strings.Contains(string(output), "kind: Deployment"),
				"Expected Deployment resource in template output")
		})
	}
}
