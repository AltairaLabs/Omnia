//go:build envtest

/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// startCELEnv boots an envtest API server with the AgentRuntime CRD installed
// (CEL rules and all), returning a direct client and a teardown func. The test
// is skipped when envtest assets are unavailable so it stays opt-in (also gated
// behind the `envtest` build tag) and off the default `go test` path.
func startCELEnv(t *testing.T) (client.Client, func()) {
	t.Helper()

	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	if dir := firstCELEnvTestBinaryDir(); dir != "" {
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

	c, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		_ = env.Stop()
		t.Fatalf("client: %v", err)
	}

	return c, func() { _ = env.Stop() }
}

func firstCELEnvTestBinaryDir() string {
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

// agentAR builds an agent-mode AgentRuntime with the given facades.
func agentAR(name string, facades ...corev1alpha1.FacadeConfig) *corev1alpha1.AgentRuntime {
	return &corev1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: corev1alpha1.AgentRuntimeSpec{
			Mode:          corev1alpha1.AgentRuntimeModeAgent,
			PromptPackRef: corev1alpha1.PromptPackRef{Name: "pack"},
			Facades:       facades,
		},
	}
}

func ws() corev1alpha1.FacadeConfig {
	return corev1alpha1.FacadeConfig{Type: corev1alpha1.FacadeTypeWebSocket}
}
func a2a() corev1alpha1.FacadeConfig {
	return corev1alpha1.FacadeConfig{Type: corev1alpha1.FacadeTypeA2A}
}
func mcp() corev1alpha1.FacadeConfig {
	return corev1alpha1.FacadeConfig{Type: corev1alpha1.FacadeTypeMCP}
}
func rest() corev1alpha1.FacadeConfig {
	return corev1alpha1.FacadeConfig{Type: corev1alpha1.FacadeTypeREST}
}
func custom(image string) corev1alpha1.FacadeConfig {
	return corev1alpha1.FacadeConfig{Type: corev1alpha1.FacadeTypeCustom, Image: image}
}

func TestFacadesCEL_RejectsEmpty(t *testing.T) {
	c, stop := startCELEnv(t)
	defer stop()

	ar := agentAR("empty-facades")
	err := c.Create(context.Background(), ar)
	if err == nil {
		t.Fatal("expected an AgentRuntime with no facades to be rejected")
	}
}

func TestFacadesCEL_RejectsDuplicateType(t *testing.T) {
	c, stop := startCELEnv(t)
	defer stop()

	ar := agentAR("dup-types", ws(), ws())
	err := c.Create(context.Background(), ar)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate facade types to be rejected, got: %v", err)
	}
}

func TestFacadesCEL_RejectsAgentWithMCP(t *testing.T) {
	c, stop := startCELEnv(t)
	defer stop()

	ar := agentAR("agent-mcp", ws(), mcp())
	err := c.Create(context.Background(), ar)
	if err == nil {
		t.Fatal("expected mode=agent with an mcp facade to be rejected")
	}
}

func TestFacadesCEL_RejectsFunctionWithoutRest(t *testing.T) {
	c, stop := startCELEnv(t)
	defer stop()

	ar := agentAR("fn-no-rest", mcp())
	ar.Spec.Mode = corev1alpha1.AgentRuntimeModeFunction
	// function mode also requires input/output schemas; the facade rule should
	// reject first, but set them so a schema rule isn't what we're asserting.
	err := c.Create(context.Background(), ar)
	if err == nil {
		t.Fatal("expected mode=function without a rest facade to be rejected")
	}
}

func TestFacadesCEL_AcceptsAgentWebSocketA2A(t *testing.T) {
	c, stop := startCELEnv(t)
	defer stop()

	ar := agentAR("agent-ws-a2a", ws(), a2a())
	if err := c.Create(context.Background(), ar); err != nil {
		t.Fatalf("expected a valid agent [websocket, a2a] to be admitted, got: %v", err)
	}
}

func TestFacadesCEL_AcceptsAgentCustomWithImage(t *testing.T) {
	c, stop := startCELEnv(t)
	defer stop()

	ar := agentAR("agent-custom", custom("registry.example.com/byo-facade:v1"))
	if err := c.Create(context.Background(), ar); err != nil {
		t.Fatalf("expected a valid agent [custom+image] to be admitted, got: %v", err)
	}
}

func TestFacadesCEL_RejectsCustomWithoutImage(t *testing.T) {
	c, stop := startCELEnv(t)
	defer stop()

	ar := agentAR("custom-no-image", custom(""))
	err := c.Create(context.Background(), ar)
	if err == nil || !strings.Contains(err.Error(), "image") {
		t.Fatalf("expected mode=agent custom facade without image to be rejected, got: %v", err)
	}
}

func TestFacadesCEL_RejectsFunctionWithCustom(t *testing.T) {
	c, stop := startCELEnv(t)
	defer stop()

	ar := agentAR("fn-custom", rest(), custom("registry.example.com/byo-facade:v1"))
	ar.Spec.Mode = corev1alpha1.AgentRuntimeModeFunction
	err := c.Create(context.Background(), ar)
	if err == nil {
		t.Fatal("expected mode=function with a custom facade to be rejected")
	}
}
