/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package main

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/altairalabs/omnia/api/v1alpha1"
)

func mgmtPortScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

func TestResolveAgentMgmtWSPort_UsesInternalPortFromStatus(t *testing.T) {
	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "rag-hero", Namespace: "demo"},
		Status: v1alpha1.AgentRuntimeStatus{
			ManagementEndpoints: &v1alpha1.ManagementEndpoints{WS: ptr.To(int32(18080))},
		},
	}
	c := fake.NewClientBuilder().WithScheme(mgmtPortScheme(t)).WithObjects(ar).Build()

	got := resolveAgentMgmtWSPort(context.Background(), c, "demo", "rag-hero", logr.Discard())
	if got != 18080 {
		t.Errorf("port = %d, want 18080 (internal mgmt WS port from status)", got)
	}
}

func TestResolveAgentMgmtWSPort_FallsBackWhenNoManagementEndpoints(t *testing.T) {
	ar := &v1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "rag-hero", Namespace: "demo"},
	}
	c := fake.NewClientBuilder().WithScheme(mgmtPortScheme(t)).WithObjects(ar).Build()

	got := resolveAgentMgmtWSPort(context.Background(), c, "demo", "rag-hero", logr.Discard())
	if got != defaultAPIPort {
		t.Errorf("port = %d, want external default %d when no managementEndpoints", got, defaultAPIPort)
	}
}

func TestResolveAgentMgmtWSPort_FallsBackWhenAgentNotFound(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(mgmtPortScheme(t)).Build()

	got := resolveAgentMgmtWSPort(context.Background(), c, "demo", "missing", logr.Discard())
	if got != defaultAPIPort {
		t.Errorf("port = %d, want external default %d when AgentRuntime not found", got, defaultAPIPort)
	}
}
