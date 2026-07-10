//go:build envtest

/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1_test

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// llmProvider builds a minimal llm-role Provider of the given type/model.
func llmProvider(name string, pType corev1alpha1.ProviderType, model string) *corev1alpha1.Provider {
	return &corev1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: corev1alpha1.ProviderSpec{
			Type:  pType,
			Model: model,
		},
	}
}

func TestProviderModelCEL_RejectsEmptyModel(t *testing.T) {
	c, stop := startCELEnv(t)
	defer stop()

	p := llmProvider("claude-no-model", corev1alpha1.ProviderTypeClaude, "")
	err := c.Create(context.Background(), p)
	if err == nil || !strings.Contains(err.Error(), "spec.model is required") {
		t.Fatalf("expected a claude Provider with an empty model to be rejected, got: %v", err)
	}
}

func TestProviderModelCEL_AcceptsModel(t *testing.T) {
	c, stop := startCELEnv(t)
	defer stop()

	p := llmProvider("claude-with-model", corev1alpha1.ProviderTypeClaude, "claude-sonnet-4-20250514")
	if err := c.Create(context.Background(), p); err != nil {
		t.Fatalf("expected a claude Provider with a model to be admitted, got: %v", err)
	}
}

func TestProviderModelCEL_ExemptsMock(t *testing.T) {
	c, stop := startCELEnv(t)
	defer stop()

	p := llmProvider("mock-no-model", corev1alpha1.ProviderTypeMock, "")
	if err := c.Create(context.Background(), p); err != nil {
		t.Fatalf("expected a mock Provider without a model to be admitted, got: %v", err)
	}
}
