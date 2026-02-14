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
	"path/filepath"
	"sync"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/altairalabs/omnia/test/utils"
)

var (
	// Optional Environment Variables:
	// - CERT_MANAGER_INSTALL_SKIP=true: Skips CertManager installation during test setup.
	// - E2E_SKIP_CLEANUP=true: Skips cleanup after tests to allow manual debugging of resources.
	// These variables are useful if CertManager is already installed, avoiding
	// re-installation and conflicts.
	skipCertManagerInstall = os.Getenv("CERT_MANAGER_INSTALL_SKIP") == "true"
	skipCleanup            = os.Getenv("E2E_SKIP_CLEANUP") == "true"
	// isCertManagerAlreadyInstalled will be set true when CertManager CRDs be found on the cluster
	isCertManagerAlreadyInstalled = false

	// projectImage is the name of the image which will be build and loaded
	// with the code source changes to be tested.
	projectImage = "example.com/omnia:v0.0.1"

	// facadeImage is the name of the facade image used by AgentRuntime
	facadeImage = "example.com/omnia-facade:v0.0.1"

	// runtimeImage is the name of the runtime image used by AgentRuntime
	runtimeImage = "example.com/omnia-runtime:v0.0.1"

	// arenaWorkerImage is the name of the arena-worker image used by ArenaJob
	arenaWorkerImage = "example.com/arena-worker:v0.0.1"

	// arenaControllerImage is the name of the arena controller image (Enterprise)
	arenaControllerImage = "example.com/arena-controller:v0.0.1"

	// sessionApiImage is the name of the session-api image
	sessionApiImage = "example.com/omnia-session-api:v0.0.1"
)

// buildResult holds the result of an image build operation
type buildResult struct {
	name string
	err  error
}

// TestE2E runs the end-to-end (e2e) test suite for the project. These tests execute in an isolated,
// temporary environment to validate project changes with the purpose of being used in CI jobs.
// The default setup requires Kind, builds/loads the Manager Docker image locally, and installs
// CertManager.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting omnia integration test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	// Skip image building and cluster setup if running against a pre-deployed cluster
	// This is useful for Arena E2E tests which use a Helm-deployed cluster
	if os.Getenv("E2E_SKIP_SETUP") == "true" {
		_, _ = fmt.Fprintf(GinkgoWriter, "Skipping BeforeSuite setup (E2E_SKIP_SETUP=true)\n")
		return
	}

	// Build all binaries natively (shared module/build cache) then package into minimal images.
	// This is significantly faster than running 6 independent Docker builds.
	binaries := []struct {
		name  string
		pkg   string
		image string
	}{
		{"manager", "./cmd/main.go", projectImage},
		{"agent", "./cmd/agent", facadeImage},
		{"runtime", "./cmd/runtime", runtimeImage},
		{"session-api", "./cmd/session-api", sessionApiImage},
		{"arena-worker", "./ee/cmd/arena-worker", arenaWorkerImage},
		{"arena-controller", "./ee/cmd/omnia-arena-controller", arenaControllerImage},
	}

	projectDir, err := utils.GetProjectDir()
	Expect(err).NotTo(HaveOccurred())
	distDir := filepath.Join(projectDir, "dist", "e2e")
	Expect(os.MkdirAll(distDir, 0o755)).To(Succeed())

	By("building all binaries natively")
	var wg sync.WaitGroup
	results := make(chan buildResult, len(binaries))
	for _, b := range binaries {
		wg.Add(1)
		go func(name, pkg string) {
			defer wg.Done()
			// Each binary gets its own staging dir with its real name for COPY
			outDir := filepath.Join(distDir, name)
			_ = os.MkdirAll(outDir, 0o755)
			outPath := filepath.Join(outDir, name)
			cmd := exec.Command("go", "build", "-ldflags=-w -s", "-o", outPath, pkg)
			cmd.Dir = projectDir
			cmd.Env = append(os.Environ(),
				"GO111MODULE=on", "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64",
			)
			_, _ = fmt.Fprintf(GinkgoWriter, "running: %q\n", cmd.String())
			out, buildErr := cmd.CombinedOutput()
			if buildErr != nil {
				buildErr = fmt.Errorf("%s failed: %s: %w", name, string(out), buildErr)
			}
			results <- buildResult{name: name, err: buildErr}
		}(b.name, b.pkg)
	}
	go func() {
		wg.Wait()
		close(results)
	}()
	for result := range results {
		ExpectWithOffset(1, result.err).NotTo(HaveOccurred(),
			fmt.Sprintf("Failed to build binary for %s", result.name))
		_, _ = fmt.Fprintf(GinkgoWriter, "Built %s binary successfully\n", result.name)
	}

	By("packaging binaries into container images")
	var pkgWg sync.WaitGroup
	pkgResults := make(chan buildResult, len(binaries))
	for _, b := range binaries {
		pkgWg.Add(1)
		go func(name, image string) {
			defer pkgWg.Done()
			contextDir := filepath.Join(distDir, name)
			// Write a per-binary Dockerfile so the COPY and ENTRYPOINT use the real name.
			// K8s manifests override the container command (e.g. /manager), so the binary
			// must live at the path the manifests expect.
			df := fmt.Sprintf("FROM gcr.io/distroless/static:nonroot\nCOPY %s /%s\nUSER 65532:65532\nENTRYPOINT [\"/%s\"]\n", name, name, name)
			dfErr := os.WriteFile(filepath.Join(contextDir, "Dockerfile"), []byte(df), 0o644)
			if dfErr != nil {
				pkgResults <- buildResult{name: name, err: dfErr}
				return
			}
			cmd := exec.Command("docker", "build", "-t", image, contextDir)
			_, _ = fmt.Fprintf(GinkgoWriter, "running: %q\n", cmd.String())
			out, buildErr := cmd.CombinedOutput()
			if buildErr != nil {
				buildErr = fmt.Errorf("%s docker build failed: %s: %w", name, string(out), buildErr)
			}
			pkgResults <- buildResult{name: name, err: buildErr}
		}(b.name, b.image)
	}
	go func() {
		pkgWg.Wait()
		close(pkgResults)
	}()
	for result := range pkgResults {
		ExpectWithOffset(1, result.err).NotTo(HaveOccurred(),
			fmt.Sprintf("Failed to package the %s image", result.name))
		_, _ = fmt.Fprintf(GinkgoWriter, "Packaged %s image successfully\n", result.name)
	}

	// Load images into Kind in parallel
	By("loading all container images into Kind in parallel")
	var loadWg sync.WaitGroup
	loadResults := make(chan buildResult, 6)

	images := []struct {
		name  string
		image string
	}{
		{"manager(Operator)", projectImage},
		{"facade", facadeImage},
		{"runtime", runtimeImage},
		{"arena-worker", arenaWorkerImage},
		{"arena-controller", arenaControllerImage},
		{"session-api", sessionApiImage},
	}

	for _, img := range images {
		loadWg.Add(1)
		go func(name, image string) {
			defer loadWg.Done()
			err := utils.LoadImageToKindClusterWithName(image)
			loadResults <- buildResult{name: name, err: err}
		}(img.name, img.image)
	}

	go func() {
		loadWg.Wait()
		close(loadResults)
	}()

	for result := range loadResults {
		ExpectWithOffset(1, result.err).NotTo(HaveOccurred(),
			fmt.Sprintf("Failed to load the %s image into Kind", result.name))
		_, _ = fmt.Fprintf(GinkgoWriter, "Loaded %s image into Kind successfully\n", result.name)
	}

	// The tests-e2e are intended to run on a temporary cluster that is created and destroyed for testing.
	// To prevent errors when tests run in environments with CertManager already installed,
	// we check for its presence before execution.
	// Setup CertManager before the suite if not skipped and if not already installed
	if !skipCertManagerInstall {
		By("checking if cert manager is installed already")
		isCertManagerAlreadyInstalled = utils.IsCertManagerCRDsInstalled()
		if !isCertManagerAlreadyInstalled {
			_, _ = fmt.Fprintf(GinkgoWriter, "Installing CertManager...\n")
			Expect(utils.InstallCertManager()).To(Succeed(), "Failed to install CertManager")
		} else {
			_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: CertManager is already installed. Skipping installation...\n")
		}
	}
})

var _ = AfterSuite(func() {
	if skipCleanup {
		_, _ = fmt.Fprintf(GinkgoWriter, "Skipping cleanup (E2E_SKIP_CLEANUP=true) - resources left in cluster for manual debugging\n")
		return
	}
	// Teardown CertManager after the suite if not skipped and if it was not already installed
	if !skipCertManagerInstall && !isCertManagerAlreadyInstalled {
		_, _ = fmt.Fprintf(GinkgoWriter, "Uninstalling CertManager...\n")
		utils.UninstallCertManager()
	}
})
