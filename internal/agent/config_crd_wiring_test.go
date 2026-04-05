/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package agent

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// TestLoadFromCRD_ClientToolTimeoutPropagates is a controller wiring test for
// #728 Part 2 item 6: CRD field propagation. The field
// AgentRuntime.spec.facade.clientToolTimeout was defined on the API type but
// LoadFromCRD never read it into Config.ClientToolTimeout, so the facade's
// RuntimeHandler always used defaultClientToolTimeout (60s) and the CRD field
// was dead.
//
// Unlike the service-binary wiring tests under cmd/<service>/ which start the
// real server, this test targets the CRD-loading boundary directly. It uses a
// controller-runtime fake client to serve an AgentRuntime with
// ClientToolTimeout set, calls LoadFromCRD, and asserts that
// cfg.ClientToolTimeout is populated. The facade then reads cfg.ClientToolTimeout
// and calls handler.SetClientToolTimeout — that wiring is covered inline in
// cmd/agent/main.go (see the RuntimeHandler construction path).
//
// This follows the codebase pattern: containers read the AgentRuntime CRD
// directly at startup via the k8s client and their Downward-API-injected
// identity; env vars are only for things the operator uniquely knows
// (tracing, handler mode) or runtime plumbing.
func TestLoadFromCRD_ClientToolTimeoutPropagates(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}

	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wiring-test",
			Namespace: "default",
		},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "test-prompts"},
			Facade: v1alpha1.FacadeConfig{
				Type:              v1alpha1.FacadeTypeWebSocket,
				ClientToolTimeout: &metav1.Duration{Duration: 45 * time.Second},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ar).Build()

	cfg, err := LoadFromCRD(context.Background(), c, "wiring-test", "default")
	if err != nil {
		t.Fatalf("LoadFromCRD: %v", err)
	}

	if cfg.ClientToolTimeout != 45*time.Second {
		t.Errorf("cfg.ClientToolTimeout = %v, want 45s — "+
			"spec.facade.clientToolTimeout is not being read from the CRD "+
			"into the agent Config; the field is dead",
			cfg.ClientToolTimeout)
	}
}

// TestLoadFromCRD_ClientToolTimeoutZeroWhenNil verifies the inverse: when
// spec.facade.clientToolTimeout is not set, cfg.ClientToolTimeout stays zero
// and cmd/agent/main.go falls back to the RuntimeHandler default (60s).
// Guards against a future change that always sets a non-zero value.
func TestLoadFromCRD_ClientToolTimeoutZeroWhenNil(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}

	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "wiring-test", Namespace: "default"},
		Spec: v1alpha1.AgentRuntimeSpec{
			PromptPackRef: v1alpha1.PromptPackRef{Name: "test-prompts"},
			Facade: v1alpha1.FacadeConfig{
				Type: v1alpha1.FacadeTypeWebSocket,
				// ClientToolTimeout deliberately not set
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ar).Build()

	cfg, err := LoadFromCRD(context.Background(), c, "wiring-test", "default")
	if err != nil {
		t.Fatalf("LoadFromCRD: %v", err)
	}

	if cfg.ClientToolTimeout != 0 {
		t.Errorf("cfg.ClientToolTimeout = %v, want 0 (unset); future change "+
			"may be populating a default before the nil check",
			cfg.ClientToolTimeout)
	}
}
