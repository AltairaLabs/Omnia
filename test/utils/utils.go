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

package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2" // nolint:revive,staticcheck
)

const (
	certmanagerVersion   = "v1.19.1"
	certmanagerURLTmpl   = "https://github.com/cert-manager/cert-manager/releases/download/%s/cert-manager.yaml"
	certmanagerNamespace = "cert-manager"

	// certmanagerWaitAttempts and certmanagerWaitTimeout bound how long we wait for the
	// cert-manager webhook Deployment to become Available. The wait is retried because, on
	// a busy CI runner that has just loaded many Omnia images into kind, the webhook pod
	// can miss a single timeout window while its upstream image is still being pulled. A
	// healthy webhook satisfies the condition in seconds regardless of the timeout, so the
	// extra attempts only cost wall-clock when the install is genuinely struggling.
	certmanagerWaitAttempts = 3
	certmanagerWaitTimeout  = "3m"

	defaultKindBinary  = "kind"
	defaultKindCluster = "kind"
)

func warnError(err error) {
	_, _ = fmt.Fprintf(GinkgoWriter, "warning: %v\n", err)
}

// Run executes the provided command within this context
func Run(cmd *exec.Cmd) (string, error) {
	dir, _ := GetProjectDir()
	cmd.Dir = dir

	if err := os.Chdir(cmd.Dir); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "chdir dir: %q\n", err)
	}

	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	command := strings.Join(cmd.Args, " ")
	_, _ = fmt.Fprintf(GinkgoWriter, "running: %q\n", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%q failed with error %q: %w", command, string(output), err)
	}

	return string(output), nil
}

// UninstallCertManager uninstalls the cert manager
func UninstallCertManager() {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)
	cmd := exec.Command("kubectl", "delete", "-f", url)
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}

	// Delete leftover leases in kube-system (not cleaned by default)
	kubeSystemLeases := []string{
		"cert-manager-cainjector-leader-election",
		"cert-manager-controller",
	}
	for _, lease := range kubeSystemLeases {
		cmd = exec.Command("kubectl", "delete", "lease", lease,
			"-n", "kube-system", "--ignore-not-found", "--force", "--grace-period=0")
		if _, err := Run(cmd); err != nil {
			warnError(err)
		}
	}
}

// InstallCertManager installs the cert manager bundle.
func InstallCertManager() error {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)
	cmd := exec.Command("kubectl", "apply", "-f", url)
	if _, err := Run(cmd); err != nil {
		return err
	}
	// Wait for cert-manager-webhook to be ready. This can take time if cert-manager
	// was re-installed after uninstalling on a cluster, or if image pulls are slow on a
	// contended CI runner. The wait is retried so a single missed window does not abort
	// the whole suite; on final failure we dump pod state to surface the real cause.
	return waitForCertManagerWebhook()
}

// waitForCertManagerWebhook blocks until the cert-manager webhook Deployment reports
// Available, retrying the wait a bounded number of times. It returns the last error and
// dumps cert-manager pod diagnostics when every attempt is exhausted.
func waitForCertManagerWebhook() error {
	var err error
	for attempt := 1; attempt <= certmanagerWaitAttempts; attempt++ {
		cmd := exec.Command("kubectl", "wait", "deployment.apps/cert-manager-webhook",
			"--for", "condition=Available",
			"--namespace", certmanagerNamespace,
			"--timeout", certmanagerWaitTimeout,
		)
		if _, err = Run(cmd); err == nil {
			return nil
		}
		_, _ = fmt.Fprintf(GinkgoWriter,
			"cert-manager webhook not ready (attempt %d/%d): %v\n",
			attempt, certmanagerWaitAttempts, err)
	}

	dumpCertManagerDiagnostics()
	return err
}

// dumpCertManagerDiagnostics prints cert-manager pod state to the test log so a failed
// install shows the underlying reason (image pull, scheduling, crash) instead of only a
// bare "timed out waiting for the condition".
func dumpCertManagerDiagnostics() {
	for _, args := range [][]string{
		{"get", "pods", "-n", certmanagerNamespace, "-o", "wide"},
		{"describe", "pods", "-n", certmanagerNamespace},
	} {
		out, _ := Run(exec.Command("kubectl", args...))
		_, _ = fmt.Fprintf(GinkgoWriter, "=== kubectl %s ===\n%s\n", strings.Join(args, " "), out)
	}
}

// IsCertManagerCRDsInstalled checks if any Cert Manager CRDs are installed
// by verifying the existence of key CRDs related to Cert Manager.
func IsCertManagerCRDsInstalled() bool {
	// List of common Cert Manager CRDs
	certManagerCRDs := []string{
		"certificates.cert-manager.io",
		"issuers.cert-manager.io",
		"clusterissuers.cert-manager.io",
		"certificaterequests.cert-manager.io",
		"orders.acme.cert-manager.io",
		"challenges.acme.cert-manager.io",
	}

	// Execute the kubectl command to get all CRDs
	cmd := exec.Command("kubectl", "get", "crds")
	output, err := Run(cmd)
	if err != nil {
		return false
	}

	// Check if any of the Cert Manager CRDs are present
	crdList := GetNonEmptyLines(output)
	for _, crd := range certManagerCRDs {
		for _, line := range crdList {
			if strings.Contains(line, crd) {
				return true
			}
		}
	}

	return false
}

// LoadImageToKindClusterWithName loads a local docker image to the kind cluster
func LoadImageToKindClusterWithName(name string) error {
	cluster := defaultKindCluster
	if v, ok := os.LookupEnv("KIND_CLUSTER"); ok {
		cluster = v
	}
	kindOptions := []string{"load", "docker-image", name, "--name", cluster}
	kindBinary := defaultKindBinary
	if v, ok := os.LookupEnv("KIND"); ok {
		kindBinary = v
	}
	cmd := exec.Command(kindBinary, kindOptions...)
	_, err := Run(cmd)
	return err
}

// GetNonEmptyLines converts given command output string into individual objects
// according to line breakers, and ignores the empty elements in it.
func GetNonEmptyLines(output string) []string {
	var res []string
	elements := strings.Split(output, "\n")
	for _, element := range elements {
		if element != "" {
			res = append(res, element)
		}
	}

	return res
}

// GetProjectDir will return the directory where the project is
func GetProjectDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return wd, fmt.Errorf("failed to get current working directory: %w", err)
	}
	wd = strings.ReplaceAll(wd, "/test/e2e", "")
	return wd, nil
}

// ValidateManifest validates a Kubernetes manifest against the server's CRD schema
// using kubectl apply --dry-run=server. This catches schema validation errors
// like missing required fields before the actual apply.
func ValidateManifest(manifest string) error {
	cmd := exec.Command("kubectl", "apply", "-f", "-", "--dry-run=server")
	cmd.Stdin = strings.NewReader(manifest)
	_, err := Run(cmd)
	return err
}

// ApplyManifestWithValidation first validates the manifest with --dry-run=server,
// then applies it. This ensures schema errors are caught early with clear messages.
func ApplyManifestWithValidation(manifest string) error {
	// First validate
	if err := ValidateManifest(manifest); err != nil {
		return fmt.Errorf("manifest validation failed: %w", err)
	}

	// Then apply
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	_, err := Run(cmd)
	return err
}

// UncommentCode searches for target in the file and remove the comment prefix
// of the target content. The target content may span multiple lines.
func UncommentCode(filename, target, prefix string) error {
	// false positive
	// nolint:gosec
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file %q: %w", filename, err)
	}
	strContent := string(content)

	idx := strings.Index(strContent, target)
	if idx < 0 {
		return fmt.Errorf("unable to find the code %q to be uncomment", target)
	}

	out := new(bytes.Buffer)
	_, err = out.Write(content[:idx])
	if err != nil {
		return fmt.Errorf("failed to write to output: %w", err)
	}

	scanner := bufio.NewScanner(bytes.NewBufferString(target))
	if !scanner.Scan() {
		return nil
	}
	for {
		if _, err = out.WriteString(strings.TrimPrefix(scanner.Text(), prefix)); err != nil {
			return fmt.Errorf("failed to write to output: %w", err)
		}
		// Avoid writing a newline in case the previous line was the last in target.
		if !scanner.Scan() {
			break
		}
		if _, err = out.WriteString("\n"); err != nil {
			return fmt.Errorf("failed to write to output: %w", err)
		}
	}

	if _, err = out.Write(content[idx+len(target):]); err != nil {
		return fmt.Errorf("failed to write to output: %w", err)
	}

	// false positive
	// nolint:gosec
	if err = os.WriteFile(filename, out.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write file %q: %w", filename, err)
	}

	return nil
}
