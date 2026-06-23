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

package controller

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	eev1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	ctx       context.Context
	cancel    context.CancelFunc
	testEnv   *envtest.Environment
	cfg       *rest.Config
	k8sClient client.Client
	// gatewayCRDsInstalled records whether the Gateway API standard CRDs were
	// located in the module cache and loaded into envtest. Specs that need
	// HTTPRoute/Gateway gate on it.
	gatewayCRDsInstalled bool
)

// gatewayAPICRDDir resolves the sigs.k8s.io/gateway-api standard CRD directory
// inside the local module cache. The module version is read from the repo's
// go.mod (build info is empty for `go test` binaries) and the cache root from
// `go env GOMODCACHE`. Returns "" when it can't be located so the dependent
// specs can skip rather than fail the whole suite.
func gatewayAPICRDDir() string {
	const modPath = "sigs.k8s.io/gateway-api"
	version := gatewayAPIModuleVersion()
	if version == "" {
		return ""
	}
	modCache := goModCache()
	if modCache == "" {
		return ""
	}
	dir := filepath.Join(modCache, modPath+"@"+version, "config", "crd", "standard")
	if _, err := os.Stat(dir); err != nil {
		return ""
	}
	return dir
}

// gatewayAPIModuleVersion reads the pinned sigs.k8s.io/gateway-api version from
// the repo's go.mod (../../go.mod relative to the controller package).
func gatewayAPIModuleVersion() string {
	data, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		return ""
	}
	f, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return ""
	}
	for _, req := range f.Require {
		if req.Mod.Path == "sigs.k8s.io/gateway-api" {
			return req.Mod.Version
		}
	}
	return ""
}

// goModCache returns the module cache root via `go env GOMODCACHE` (the env var
// is not set in the test process), falling back to $GOPATH/pkg/mod or
// ~/go/pkg/mod.
func goModCache() string {
	if out, err := exec.Command("go", "env", "GOMODCACHE").Output(); err == nil {
		if dir := strings.TrimSpace(string(out)); dir != "" {
			return dir
		}
	}
	if gp := os.Getenv("GOPATH"); gp != "" {
		return filepath.Join(gp, "pkg", "mod")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "go", "pkg", "mod")
	}
	return ""
}

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	var err error
	err = omniav1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = eev1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = gatewayv1.Install(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	By("bootstrapping test environment")
	crdPaths := []string{filepath.Join("..", "..", "config", "crd", "bases")}
	if dir := gatewayAPICRDDir(); dir != "" {
		crdPaths = append(crdPaths, dir)
		gatewayCRDsInstalled = true
	}
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     crdPaths,
		ErrorIfCRDPathMissing: true,
		CRDs:                  minimalIstioCRDs(),
	}

	// Retrieve the first found binary directory to allow running tests from IDEs
	if getFirstFoundEnvTestBinaryDir() != "" {
		testEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
	}

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// minimalIstioCRDs returns just-enough CRD definitions for networking.istio.io
// VirtualService and DestinationRule so the rollout controller's unstructured
// patches can round-trip against the envtest API server. Upstream Istio CRDs
// aren't checked into this repo and the rollout code only reads/writes a small
// set of nested fields, so we declare the CRDs with
// x-kubernetes-preserve-unknown-fields: true and skip schema validation.
func minimalIstioCRDs() []*apiextensionsv1.CustomResourceDefinition {
	preserveUnknown := true
	openAPI := &apiextensionsv1.JSONSchemaProps{
		Type:                   "object",
		XPreserveUnknownFields: &preserveUnknown,
	}
	versions := []apiextensionsv1.CustomResourceDefinitionVersion{{
		Name:    "v1",
		Served:  true,
		Storage: true,
		Schema:  &apiextensionsv1.CustomResourceValidation{OpenAPIV3Schema: openAPI},
	}}
	return []*apiextensionsv1.CustomResourceDefinition{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "virtualservices.networking.istio.io"},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Group: "networking.istio.io",
				Names: apiextensionsv1.CustomResourceDefinitionNames{
					Plural:   "virtualservices",
					Singular: "virtualservice",
					Kind:     "VirtualService",
					ListKind: "VirtualServiceList",
				},
				Scope:    apiextensionsv1.NamespaceScoped,
				Versions: versions,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "destinationrules.networking.istio.io"},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Group: "networking.istio.io",
				Names: apiextensionsv1.CustomResourceDefinitionNames{
					Plural:   "destinationrules",
					Singular: "destinationrule",
					Kind:     "DestinationRule",
					ListKind: "DestinationRuleList",
				},
				Scope:    apiextensionsv1.NamespaceScoped,
				Versions: versions,
			},
		},
	}
}

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
// ENVTEST-based tests depend on specific binaries, usually located in paths set by
// controller-runtime. When running tests directly (e.g., via an IDE) without using
// Makefile targets, the 'BinaryAssetsDirectory' must be explicitly configured.
//
// This function streamlines the process by finding the required binaries, similar to
// setting the 'KUBEBUILDER_ASSETS' environment variable. To ensure the binaries are
// properly set up, run 'make setup-envtest' beforehand.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}
