//go:build envtest

/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package webhook_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniawebhook "github.com/altairalabs/omnia/internal/webhook"
)

// startWebhookEnv boots an envtest API server with the AgentRuntime CRD and the
// ValidatingWebhookConfiguration from config/webhook installed, then starts a
// manager serving the AgentRuntime validating webhook against envtest's
// generated serving cert. It returns a direct (uncached) client and a teardown
// func. If envtest assets are unavailable, the test is skipped — this keeps the
// heavy test opt-in and off the default `go test` path (also gated by the
// `envtest` build tag).
func startWebhookEnv(t *testing.T) (client.Client, func()) {
	t.Helper()
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{filepath.Join("..", "..", "config", "webhook")},
		},
	}
	if dir := firstEnvTestBinaryDir(); dir != "" {
		env.BinaryAssetsDirectory = dir
	}

	cfg, err := env.Start()
	if err != nil {
		t.Skipf("envtest unavailable (run 'make setup-envtest' or set KUBEBUILDER_ASSETS): %v", err)
	}

	if err := corev1alpha1.AddToScheme(scheme.Scheme); err != nil {
		_ = env.Stop()
		t.Fatalf("add scheme: %v", err)
	}

	wo := env.WebhookInstallOptions
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:  scheme.Scheme,
		Metrics: metricsserver.Options{BindAddress: "0"},
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    wo.LocalServingHost,
			Port:    wo.LocalServingPort,
			CertDir: wo.LocalServingCertDir,
		}),
	})
	if err != nil {
		_ = env.Stop()
		t.Fatalf("manager: %v", err)
	}
	if err := omniawebhook.SetupAgentRuntimeWebhookWithManager(mgr); err != nil {
		_ = env.Stop()
		t.Fatalf("setup webhook: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = mgr.Start(ctx) }()

	waitForWebhookServer(t, wo.LocalServingHost, wo.LocalServingPort)

	c, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		cancel()
		_ = env.Stop()
		t.Fatalf("client: %v", err)
	}

	return c, func() { cancel(); _ = env.Stop() }
}

// waitForWebhookServer blocks until the manager's TLS webhook port accepts a
// connection, so Create calls aren't racing the server's startup.
func waitForWebhookServer(t *testing.T, host string, port int) {
	t.Helper()
	addr := fmt.Sprintf("%s:%d", host, port)
	dialer := &net.Dialer{Timeout: time.Second}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{InsecureSkipVerify: true}) //nolint:gosec // envtest local server
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("webhook server did not come up on %s", addr)
}

// firstEnvTestBinaryDir mirrors the controller suite helper: find the first
// envtest binary dir under bin/k8s so the test runs from an IDE without
// KUBEBUILDER_ASSETS set. Returns "" if none is found (env.Start then relies on
// KUBEBUILDER_ASSETS).
func firstEnvTestBinaryDir() string {
	base := filepath.Join("..", "..", "bin", "k8s")
	entries, err := os.ReadDir(base)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			return filepath.Join(base, e.Name())
		}
	}
	return ""
}

// functionAR builds a function-mode AgentRuntime that satisfies the CRD's CEL
// rules (function mode requires both schemas and a rest/a2a facade) so the
// object reaches the validating webhook rather than being rejected by CEL first.
func functionAR(name string, inputSchema, outputSchema string) *corev1alpha1.AgentRuntime {
	ar := &corev1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
	}
	ar.Spec.Mode = corev1alpha1.AgentRuntimeModeFunction
	ar.Spec.PromptPackRef = corev1alpha1.PromptPackRef{Name: "pack"}
	ar.Spec.Facades = []corev1alpha1.FacadeConfig{{Type: corev1alpha1.FacadeTypeREST}}
	ar.Spec.InputSchema = &apiextensionsv1.JSON{Raw: []byte(inputSchema)}
	ar.Spec.OutputSchema = &apiextensionsv1.JSON{Raw: []byte(outputSchema)}
	return ar
}

func TestWebhook_RejectsInvalidFunctionSchema(t *testing.T) {
	c, stop := startWebhookEnv(t)
	defer stop()

	ar := functionAR("bad-fn", `{"type":"not-a-real-type"}`, `{"type":"object"}`)
	err := c.Create(context.Background(), ar)
	if err == nil {
		t.Fatal("expected the API server to reject an invalid function-mode schema")
	}
}

func TestWebhook_AcceptsValidFunctionSchema(t *testing.T) {
	c, stop := startWebhookEnv(t)
	defer stop()

	ar := functionAR("good-fn", `{"type":"object","required":["q"]}`, `{"type":"object","required":["a"]}`)
	if err := c.Create(context.Background(), ar); err != nil {
		t.Fatalf("expected a valid function-mode AgentRuntime to be admitted, got: %v", err)
	}
}
