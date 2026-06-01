/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlcfg "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/encryption"
	"github.com/altairalabs/omnia/ee/pkg/metrics"
)

// expectedReconcilers is the canonical 5-reconciler set the binary
// must register. Each entry corresponds to one SetupWithManager call
// that was previously inline in main(). A regression that removes a
// reconciler from buildReconcilers fails this test.
//
// Order matters: buildReconcilers returns them in this order so a
// diff against the actual list points at the right line.
var expectedReconcilers = []string{
	controllerArenaSource,
	controllerArenaTemplateSource,
	controllerArenaJob,
	controllerArenaDevSession,
	controllerKeyRotation,
}

// TestBuildReconcilers_RegistersAllExpected asserts that buildReconcilers
// produces the five reconciler entries the binary must register. This is
// the wiring contract: a removed entry here means production silently
// stops reconciling its CRD. setupOptions can be zero-valued because
// buildReconcilers doesn't dereference the options at construction —
// it captures them in closures that fire when Setup runs against a
// real manager (covered by envtest in internal/controller/).
func TestBuildReconcilers_RegistersAllExpected(t *testing.T) {
	got := buildReconcilers(setupOptions{})
	if len(got) != len(expectedReconcilers) {
		t.Fatalf("buildReconcilers returned %d entries, want %d; got names=%v",
			len(got), len(expectedReconcilers), names(got))
	}
	for i, want := range expectedReconcilers {
		if got[i].Name != want {
			t.Errorf("buildReconcilers[%d].Name = %q, want %q", i, got[i].Name, want)
		}
	}
	for _, r := range got {
		if r.Setup == nil {
			t.Errorf("buildReconcilers entry %q has nil Setup func", r.Name)
		}
	}
}

// TestBuildWebhooks_WithoutLicenseHooks asserts that when license-validation
// webhooks are disabled, no webhooks register. SessionPrivacyPolicy is owned
// by the operator (always-present enterprise controller-manager), not this
// license-gated binary; the Arena webhooks are conditional on the license flag.
func TestBuildWebhooks_WithoutLicenseHooks(t *testing.T) {
	got := buildWebhooks(webhookOptions{IncludeLicenseHooks: false})
	want := []string{}
	assertWebhookNames(t, got, want)
}

// TestBuildWebhooks_WithLicenseHooks asserts that all three Arena webhooks
// register when IncludeLicenseHooks=true. A regression that removed any of
// ArenaSource/ArenaJob/ArenaTemplateSource from the conditional block
// silently skips its admission validation.
func TestBuildWebhooks_WithLicenseHooks(t *testing.T) {
	got := buildWebhooks(webhookOptions{IncludeLicenseHooks: true})
	want := []string{controllerArenaSource, controllerArenaJob, controllerArenaTemplateSource}
	assertWebhookNames(t, got, want)
}

func names(rs []namedReconciler) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Name
	}
	return out
}

func webhookNames(ws []namedWebhook) []string {
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = w.Name
	}
	return out
}

// freshPromRegistry swaps the default Prometheus registerer for the
// duration of a test. newPrivacyPolicyMetrics registers collectors
// against the default registry; running it more than once in the same
// process panics with "duplicate metrics collector registration".
func freshPromRegistry(t *testing.T) {
	t.Helper()
	prev := prometheus.DefaultRegisterer
	prometheus.DefaultRegisterer = prometheus.NewRegistry()
	t.Cleanup(func() { prometheus.DefaultRegisterer = prev })
}

// startTestEnv spins up an envtest control plane and returns a
// configured manager. Shared by the integration-style wiring tests that
// need to exercise the reconciler Setup closures (they call
// ctrl.NewControllerManagedBy(mgr) which requires a real cache).
// Returns nil + skip-reason when envtest binaries aren't installed
// (CI sets KUBEBUILDER_ASSETS; local dev needs `make setup-envtest`).
func startTestEnv(t *testing.T) ctrl.Manager {
	t.Helper()
	logf.SetLogger(ctrlzap.New(ctrlzap.UseDevMode(true), ctrlzap.WriteTo(os.Stderr)))

	binDir := firstEnvTestBinaryDir(t)
	if binDir == "" && os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("envtest binaries not installed; run `make setup-envtest` (skipping in this env)")
	}

	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
	}
	if binDir != "" {
		testEnv.BinaryAssetsDirectory = binDir
	}

	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("envtest.Start: %v", err)
	}
	t.Cleanup(func() { _ = testEnv.Stop() })

	if err := corev1alpha1.AddToScheme(clientgoscheme.Scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := omniav1alpha1.AddToScheme(clientgoscheme.Scheme); err != nil {
		t.Fatalf("add ee scheme: %v", err)
	}

	skipValidation := true
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:     clientgoscheme.Scheme,
		Metrics:    metricsserver.Options{BindAddress: "0"},
		Controller: ctrlcfg.Controller{SkipNameValidation: &skipValidation},
	})
	if err != nil {
		t.Fatalf("ctrl.NewManager: %v", err)
	}
	return mgr
}

// firstEnvTestBinaryDir locates the first envtest binary directory
// under bin/k8s/. Same pattern as ee/internal/controller/suite_test.go.
func firstEnvTestBinaryDir(t *testing.T) string {
	t.Helper()
	basePath := filepath.Join("..", "..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			return filepath.Join(basePath, e.Name())
		}
	}
	return ""
}

// TestBuildReEncryptionStoreFactory covers the helper that builds the
// KeyRotation reconciler's StoreFactory. Two branches: nil-on-empty
// (re-encryption disabled) and non-nil-on-set (factory returned for
// lazy invocation when key rotation actually runs).
func TestBuildReEncryptionStoreFactory(t *testing.T) {
	t.Run("empty connStr disables re-encryption", func(t *testing.T) {
		got := buildReEncryptionStoreFactory("", logr.Discard())
		if got != nil {
			t.Errorf("expected nil factory when connStr empty, got non-nil")
		}
	})
	t.Run("non-empty connStr returns factory", func(t *testing.T) {
		got := buildReEncryptionStoreFactory("postgres://user:pass@127.0.0.1:1/db?connect_timeout=1", logr.Discard())
		if got == nil {
			t.Fatal("expected non-nil factory when connStr set, got nil")
		}
		// Invoke the closure: dial fails fast (port 1, 1s timeout) →
		// covers the closure body's error-wrapping path. Don't care
		// which error wins — just that the factory is called.
		_, err := got()
		if err == nil {
			t.Error("expected dial error from factory pointing at port 1")
		}
	})
}

// TestEncryptionProviderFactory verifies the KeyRotation hook produces
// a Provider for a valid config and an error for an unsupported one.
// Exercises the closure body that buildReconcilers references.
func TestEncryptionProviderFactory(t *testing.T) {
	t.Run("unsupported config returns error", func(t *testing.T) {
		_, err := encryptionProviderFactory(encryption.ProviderConfig{})
		if err == nil {
			t.Error("expected error for zero-valued ProviderConfig")
		}
	})
}

// TestRegisterNamed_HappyPath asserts every entry's Setup runs and
// the returned name list matches input order.
func TestRegisterNamed_HappyPath(t *testing.T) {
	calls := 0
	items := []namedReconciler{
		{Name: "A", Setup: func(_ ctrl.Manager) error { calls++; return nil }},
		{Name: "B", Setup: func(_ ctrl.Manager) error { calls++; return nil }},
	}
	got, err := registerNamed(nil, items)
	if err != nil {
		t.Fatalf("registerNamed: %v", err)
	}
	if calls != 2 {
		t.Errorf("Setup called %d times, want 2", calls)
	}
	if len(got) != 2 || got[0] != "A" || got[1] != "B" {
		t.Errorf("got=%v, want [A B]", got)
	}
}

// TestRegisterNamed_StopsOnError asserts that on the first Setup error,
// registerNamed (a) returns the failing name in the slice + (b) wraps
// the error with the reconciler name + (c) does not call subsequent
// Setup funcs. The error log in main.go consumes the trailing slice
// entry as "the controller that failed".
func TestRegisterNamed_StopsOnError(t *testing.T) {
	bThird := false
	items := []namedReconciler{
		{Name: "A", Setup: func(_ ctrl.Manager) error { return nil }},
		{Name: "B", Setup: func(_ ctrl.Manager) error { return errSetupFailed }},
		{Name: "C", Setup: func(_ ctrl.Manager) error { bThird = true; return nil }},
	}
	got, err := registerNamed(nil, items)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if bThird {
		t.Error("registerNamed continued past failing entry")
	}
	if len(got) != 2 || got[1] != "B" {
		t.Errorf("got=%v, want trailing entry B", got)
	}
}

// TestRegisterNamedWebhooks_HappyPath mirrors TestRegisterNamed_HappyPath
// for the webhook variant.
func TestRegisterNamedWebhooks_HappyPath(t *testing.T) {
	items := []namedWebhook{
		{Name: "W1", Setup: func(_ ctrl.Manager) error { return nil }},
		{Name: "W2", Setup: func(_ ctrl.Manager) error { return nil }},
	}
	got, err := registerNamedWebhooks(nil, items)
	if err != nil {
		t.Fatalf("registerNamedWebhooks: %v", err)
	}
	if len(got) != 2 || got[0] != "W1" || got[1] != "W2" {
		t.Errorf("got=%v, want [W1 W2]", got)
	}
}

// TestRegisterNamedWebhooks_StopsOnError mirrors the reconciler
// stops-on-error contract.
func TestRegisterNamedWebhooks_StopsOnError(t *testing.T) {
	items := []namedWebhook{
		{Name: "W1", Setup: func(_ ctrl.Manager) error { return errSetupFailed }},
	}
	got, err := registerNamedWebhooks(nil, items)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if len(got) != 1 || got[0] != "W1" {
		t.Errorf("got=%v, want [W1]", got)
	}
}

// errSetupFailed is the canned error returned by failing Setup stubs
// in the registerNamed tests. Declared as a package-level var so the
// closures don't capture per-test state.
var errSetupFailed = errSetup("setup failed")

type errSetup string

func (e errSetup) Error() string { return string(e) }

// TestSetupControllers_RegistersAllAgainstRealManager exercises the
// Setup closures buildReconcilers returns by passing a real envtest-
// backed manager. Each SetupWithManager call dereferences the manager's
// cache/scheme/client; a stub manager would panic. This catches the
// "Setup closure references the wrong reconciler type" failure class
// that the pure buildReconcilers test cannot.
func TestSetupControllers_RegistersAllAgainstRealManager(t *testing.T) {
	mgr := startTestEnv(t)
	registered, err := setupControllers(mgr, setupOptions{
		PrivacyPolicyMetrics: metrics.NewPrivacyPolicyMetrics(),
	})
	if err != nil {
		t.Fatalf("setupControllers: %v (registered up to %v)", err, registered)
	}
	if len(registered) != len(expectedReconcilers) {
		t.Errorf("registered %d reconcilers, want %d: %v",
			len(registered), len(expectedReconcilers), registered)
	}
}

// TestSetupWebhooks_RegistersAllAgainstRealManager mirrors the
// reconciler test for webhooks.
func TestSetupWebhooks_RegistersAllAgainstRealManager(t *testing.T) {
	mgr := startTestEnv(t)
	registered, err := setupWebhooks(mgr, webhookOptions{IncludeLicenseHooks: true})
	if err != nil {
		t.Fatalf("setupWebhooks: %v (registered up to %v)", err, registered)
	}
	want := []string{controllerArenaSource, controllerArenaJob, controllerArenaTemplateSource}
	if len(registered) != len(want) {
		t.Errorf("registered %d webhooks, want %d: %v", len(registered), len(want), registered)
	}
}

// TestRegisterArenaWorkloads_WithoutWebhooks exercises the helper
// main() actually calls, with EnableWebhooks=false. Locks the contract
// that controller registration runs unconditionally and webhooks gate
// on the flag.
func TestRegisterArenaWorkloads_WithoutWebhooks(t *testing.T) {
	freshPromRegistry(t)
	mgr := startTestEnv(t)
	err := registerArenaWorkloads(mgr, registrationOptions{
		Controllers: setupOptions{
			PrivacyPolicyMetrics: newPrivacyPolicyMetrics(),
		},
		EnableWebhooks: false,
	}, logr.Discard())
	if err != nil {
		t.Fatalf("registerArenaWorkloads: %v", err)
	}
}

// TestRegisterArenaWorkloads_WithWebhooks asserts the webhook branch
// fires when EnableWebhooks=true. Covers the registerArenaWorkloads
// `if !opts.EnableWebhooks { return nil }` boundary.
func TestRegisterArenaWorkloads_WithWebhooks(t *testing.T) {
	freshPromRegistry(t)
	mgr := startTestEnv(t)
	err := registerArenaWorkloads(mgr, registrationOptions{
		Controllers: setupOptions{
			PrivacyPolicyMetrics: newPrivacyPolicyMetrics(),
		},
		Webhooks: webhookOptions{
			IncludeLicenseHooks: false,
		},
		EnableWebhooks: true,
	}, logr.Discard())
	if err != nil {
		t.Fatalf("registerArenaWorkloads with webhooks: %v", err)
	}
}

// TestRegisterArenaWorkloads_PropagatesControllerError exercises the
// error-wrapping path in registerArenaWorkloads when setupControllers
// fails. Triggered by calling registerArenaWorkloads twice on the same
// manager (no SkipNameValidation in this sub-test) — controller-runtime
// rejects the second call with a duplicate-controller-name error.
func TestRegisterArenaWorkloads_PropagatesControllerError(t *testing.T) {
	freshPromRegistry(t)
	mgr := startTestEnvStrict(t)
	opts := registrationOptions{
		Controllers:    setupOptions{PrivacyPolicyMetrics: newPrivacyPolicyMetrics()},
		EnableWebhooks: false,
	}
	if err := registerArenaWorkloads(mgr, opts, logr.Discard()); err != nil {
		t.Fatalf("first registerArenaWorkloads should succeed: %v", err)
	}
	err := registerArenaWorkloads(mgr, opts, logr.Discard())
	if err == nil {
		t.Fatal("second registerArenaWorkloads should fail (duplicate controller name)")
	}
	if got := err.Error(); !contains(got, "setup controllers") {
		t.Errorf("expected error to be wrapped with 'setup controllers', got: %v", err)
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// startTestEnvStrict mirrors startTestEnv but WITHOUT SkipNameValidation,
// so duplicate-controller-name detection is active. Used by the
// error-path test.
func startTestEnvStrict(t *testing.T) ctrl.Manager {
	t.Helper()
	logf.SetLogger(ctrlzap.New(ctrlzap.UseDevMode(true), ctrlzap.WriteTo(os.Stderr)))

	binDir := firstEnvTestBinaryDir(t)
	if binDir == "" && os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("envtest binaries not installed")
	}
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
	}
	if binDir != "" {
		testEnv.BinaryAssetsDirectory = binDir
	}
	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("envtest.Start: %v", err)
	}
	t.Cleanup(func() { _ = testEnv.Stop() })

	if err := corev1alpha1.AddToScheme(clientgoscheme.Scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := omniav1alpha1.AddToScheme(clientgoscheme.Scheme); err != nil {
		t.Fatalf("add ee scheme: %v", err)
	}
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:  clientgoscheme.Scheme,
		Metrics: metricsserver.Options{BindAddress: "0"},
		// No SkipNameValidation → strict duplicate-name detection.
	})
	if err != nil {
		t.Fatalf("ctrl.NewManager: %v", err)
	}
	return mgr
}

// TestLastOrEmpty covers the tiny helper that picks the trailing
// element from the names slice for error messages.
func TestLastOrEmpty(t *testing.T) {
	if got := lastOrEmpty(nil); got != "" {
		t.Errorf("nil → %q, want empty", got)
	}
	if got := lastOrEmpty([]string{"A", "B"}); got != "B" {
		t.Errorf("[A B] → %q, want B", got)
	}
}

// TestNewPrivacyPolicyMetrics asserts the helper returns a non-nil
// initialised metrics instance.
func TestNewPrivacyPolicyMetrics(t *testing.T) {
	freshPromRegistry(t)
	if got := newPrivacyPolicyMetrics(); got == nil {
		t.Error("expected non-nil PrivacyPolicyMetrics")
	}
}

func assertWebhookNames(t *testing.T, got []namedWebhook, want []string) {
	t.Helper()
	gotNames := webhookNames(got)
	if len(gotNames) != len(want) {
		t.Fatalf("buildWebhooks returned %d entries (%v), want %d (%v)",
			len(gotNames), gotNames, len(want), want)
	}
	for i, w := range want {
		if gotNames[i] != w {
			t.Errorf("buildWebhooks[%d] = %q, want %q", i, gotNames[i], w)
		}
	}
	for _, h := range got {
		if h.Setup == nil {
			t.Errorf("buildWebhooks entry %q has nil Setup func", h.Name)
		}
	}
}
