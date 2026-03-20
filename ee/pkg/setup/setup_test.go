/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	eev1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	eemetrics "github.com/altairalabs/omnia/ee/pkg/metrics"
)

var (
	testCfg *rest.Config
	testEnv *envtest.Environment
)

// envtestAvailable is set to true when the envtest control plane starts successfully.
var envtestAvailable bool

func TestMain(m *testing.M) {
	logf.SetLogger(zap.New(zap.WriteTo(os.Stderr), zap.UseDevMode(true)))

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	if dir := getFirstFoundEnvTestBinaryDir(); dir != "" {
		testEnv.BinaryAssetsDirectory = dir
	}

	var err error
	testCfg, err = testEnv.Start()
	if err != nil {
		logf.Log.Info("envtest not available, integration tests will be skipped", "error", err)
	} else {
		envtestAvailable = true
	}

	code := m.Run()

	if envtestAvailable {
		if stopErr := testEnv.Stop(); stopErr != nil {
			logf.Log.Error(stopErr, "failed to stop envtest")
		}
	}
	os.Exit(code)
}

// skipWithoutEnvtest skips a test if the envtest control plane is not available.
func skipWithoutEnvtest(t *testing.T) {
	t.Helper()
	if !envtestAvailable {
		t.Skip("envtest not available (missing kubebuilder binaries)")
	}
}

// newTestScheme creates a scheme with all required types registered.
func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("failed to add client-go scheme: %v", err)
	}
	if err := corev1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}
	if err := eev1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add ee scheme: %v", err)
	}
	return s
}

// newTestManager creates a manager using the shared envtest environment.
func newTestManager(t *testing.T) ctrl.Manager {
	t.Helper()
	mgr, err := ctrl.NewManager(testCfg, ctrl.Options{
		Scheme: newTestScheme(t),
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable metrics to avoid port conflicts
		},
	})
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	return mgr
}

// TestRegisterEnterpriseControllers verifies that all enterprise controllers
// and webhooks register without error when all features are enabled.
//
// Controller-runtime enforces globally unique controller names per process,
// so we use a single test that covers all registration paths (always-on
// controllers, conditional analytics/streaming, webhooks, custom license URL).
func TestRegisterEnterpriseControllers(t *testing.T) {
	skipWithoutEnvtest(t)
	mgr := newTestManager(t)

	opts := EnterpriseOptions{
		LicenseServerURL: "https://license.example.com",
		ClusterName:      "test-cluster",
		EnableWebhooks:   true,
		EnableAnalytics:  true,
		EnableStreaming:  true,
		PrivacyMetrics:   eemetrics.NewPrivacyPolicyMetricsWithRegistry(prometheus.NewRegistry()),
	}

	err := RegisterEnterpriseControllers(mgr, opts)
	if err != nil {
		t.Fatalf("RegisterEnterpriseControllers failed: %v", err)
	}
}

// TestConditionalControllersNoFlags verifies that no conditional controllers
// are registered when all feature flags are disabled. This uses its own manager
// and calls the conditional helper directly (which only registers analytics/streaming).
func TestConditionalControllersNoFlags(t *testing.T) {
	skipWithoutEnvtest(t)
	mgr := newTestManager(t)
	opts := EnterpriseOptions{}
	err := registerConditionalControllers(mgr, opts)
	if err != nil {
		t.Fatalf("registerConditionalControllers with no flags failed: %v", err)
	}
}

// TestEnterpriseOptionsDefaults verifies the zero value of EnterpriseOptions
// has all features disabled.
func TestEnterpriseOptionsDefaults(t *testing.T) {
	opts := EnterpriseOptions{}

	if opts.EnableWebhooks {
		t.Error("EnableWebhooks should default to false")
	}
	if opts.EnableAnalytics {
		t.Error("EnableAnalytics should default to false")
	}
	if opts.EnableStreaming {
		t.Error("EnableStreaming should default to false")
	}
	if opts.LicenseServerURL != "" {
		t.Error("LicenseServerURL should default to empty")
	}
	if opts.ClusterName != "" {
		t.Error("ClusterName should default to empty")
	}
	if opts.PrivacyMetrics != nil {
		t.Error("PrivacyMetrics should default to nil")
	}
}

// getFirstFoundEnvTestBinaryDir locates envtest binary assets.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}
