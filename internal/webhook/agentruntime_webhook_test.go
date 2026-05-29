/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package webhook

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func functionAR(in, out string) *corev1alpha1.AgentRuntime {
	ar := &corev1alpha1.AgentRuntime{}
	ar.Spec.Mode = corev1alpha1.AgentRuntimeModeFunction
	if in != "" {
		ar.Spec.InputSchema = &apiextensionsv1.JSON{Raw: []byte(in)}
	}
	if out != "" {
		ar.Spec.OutputSchema = &apiextensionsv1.JSON{Raw: []byte(out)}
	}
	return ar
}

func TestAgentRuntimeValidator_Create_ValidSchemas(t *testing.T) {
	v := &AgentRuntimeValidator{}
	ar := functionAR(`{"type":"object","required":["q"]}`, `{"type":"object","required":["a"]}`)
	w, err := v.ValidateCreate(context.Background(), ar)
	require.NoError(t, err)
	assert.Empty(t, w)
}

func TestAgentRuntimeValidator_Create_InvalidInputSchema(t *testing.T) {
	v := &AgentRuntimeValidator{}
	ar := functionAR(`{"type":"not-a-real-type"}`, `{"type":"object"}`)
	_, err := v.ValidateCreate(context.Background(), ar)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.inputSchema")
}

func TestAgentRuntimeValidator_Create_InvalidOutputSchema(t *testing.T) {
	v := &AgentRuntimeValidator{}
	ar := functionAR(`{"type":"object"}`, `{"type":"not-a-real-type"}`)
	_, err := v.ValidateCreate(context.Background(), ar)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.outputSchema")
}

func TestAgentRuntimeValidator_Create_AgentModeSkipsValidation(t *testing.T) {
	// Agent mode: schemas are forbidden by CEL, but even if a bad one is
	// present the validator must not run schema checks.
	v := &AgentRuntimeValidator{}
	ar := &corev1alpha1.AgentRuntime{}
	ar.Spec.Mode = corev1alpha1.AgentRuntimeModeAgent
	ar.Spec.InputSchema = &apiextensionsv1.JSON{Raw: []byte(`{"type":"not-a-real-type"}`)}
	_, err := v.ValidateCreate(context.Background(), ar)
	require.NoError(t, err)
}

func TestAgentRuntimeValidator_Update_AppliesSameRules(t *testing.T) {
	v := &AgentRuntimeValidator{}
	ar := functionAR(`{"type":"not-a-real-type"}`, `{"type":"object"}`)
	_, err := v.ValidateUpdate(context.Background(), ar, ar)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.inputSchema")
}

func TestAgentRuntimeValidator_Delete_AlwaysAllowed(t *testing.T) {
	v := &AgentRuntimeValidator{}
	_, err := v.ValidateDelete(context.Background(), functionAR(`{"bad`, `{"bad`))
	require.NoError(t, err)
}
