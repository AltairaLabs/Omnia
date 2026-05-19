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

package agent

import (
	"context"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/k8s"
)

const (
	functionTestInputSchema  = `{"type":"object","required":["q"]}`
	functionTestOutputSchema = `{"type":"object","required":["a"]}`
	modeAgentStr             = "agent"
)

func TestLoadFromCRD_FunctionMode_PopulatesModeAndSchemas(t *testing.T) {
	ar := newFakeAgentRuntime("summarizer", "prod", v1alpha1.AgentRuntimeSpec{
		Mode: v1alpha1.AgentRuntimeModeFunction,
		PromptPackRef: v1alpha1.PromptPackRef{
			Name: "summarizer-pack",
		},
		Facade: v1alpha1.FacadeConfig{
			Type: v1alpha1.FacadeTypeGRPC,
		},
		InputSchema:  &apiextensionsv1.JSON{Raw: []byte(functionTestInputSchema)},
		OutputSchema: &apiextensionsv1.JSON{Raw: []byte(functionTestOutputSchema)},
		InvocationRecording: &v1alpha1.InvocationRecordingConfig{
			State: v1alpha1.InvocationRecordingEnabled,
		},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).
		WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	cfg, err := LoadFromCRD(context.Background(), c, "summarizer", "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Mode != "function" {
		t.Errorf("Mode = %q, want %q", cfg.Mode, "function")
	}
	if string(cfg.FunctionInputSchemaJSON) != functionTestInputSchema {
		t.Errorf("FunctionInputSchemaJSON = %q, want %q",
			string(cfg.FunctionInputSchemaJSON), functionTestInputSchema)
	}
	if string(cfg.FunctionOutputSchemaJSON) != functionTestOutputSchema {
		t.Errorf("FunctionOutputSchemaJSON = %q, want %q",
			string(cfg.FunctionOutputSchemaJSON), functionTestOutputSchema)
	}
	if !cfg.FunctionRecordsInvocations {
		t.Errorf("FunctionRecordsInvocations = false, want true")
	}
}

func TestLoadFromCRD_FunctionMode_RecordingDefaultsDisabled(t *testing.T) {
	ar := newFakeAgentRuntime("classifier", "prod", v1alpha1.AgentRuntimeSpec{
		Mode: v1alpha1.AgentRuntimeModeFunction,
		PromptPackRef: v1alpha1.PromptPackRef{
			Name: "classifier-pack",
		},
		Facade: v1alpha1.FacadeConfig{
			Type: v1alpha1.FacadeTypeGRPC,
		},
		InputSchema:  &apiextensionsv1.JSON{Raw: []byte(functionTestInputSchema)},
		OutputSchema: &apiextensionsv1.JSON{Raw: []byte(functionTestOutputSchema)},
		// invocationRecording block intentionally absent.
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).
		WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	cfg, err := LoadFromCRD(context.Background(), c, "classifier", "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.FunctionRecordsInvocations {
		t.Errorf("FunctionRecordsInvocations = true, want false (default)")
	}
}

func TestLoadFromCRD_AgentMode_DoesNotPopulateFunctionFields(t *testing.T) {
	ar := newFakeAgentRuntime("chat", "prod", v1alpha1.AgentRuntimeSpec{
		Mode: v1alpha1.AgentRuntimeModeAgent,
		PromptPackRef: v1alpha1.PromptPackRef{
			Name: "chat-pack",
		},
		Facade: v1alpha1.FacadeConfig{
			Type: v1alpha1.FacadeTypeWebSocket,
		},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).
		WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	cfg, err := LoadFromCRD(context.Background(), c, "chat", "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Mode != modeAgentStr {
		t.Errorf("Mode = %q, want %q", cfg.Mode, modeAgentStr)
	}
	if len(cfg.FunctionInputSchemaJSON) != 0 {
		t.Errorf("FunctionInputSchemaJSON = %q, want empty", string(cfg.FunctionInputSchemaJSON))
	}
	if len(cfg.FunctionOutputSchemaJSON) != 0 {
		t.Errorf("FunctionOutputSchemaJSON = %q, want empty", string(cfg.FunctionOutputSchemaJSON))
	}
	if cfg.FunctionRecordsInvocations {
		t.Errorf("FunctionRecordsInvocations = true, want false in agent mode")
	}
}

func TestLoadFromCRD_PreModeRuntime_DefaultsToAgent(t *testing.T) {
	// AgentRuntime predates the mode field — spec.mode is zero-value.
	// EffectiveMode() must default to "agent".
	ar := newFakeAgentRuntime("legacy", "prod", v1alpha1.AgentRuntimeSpec{
		PromptPackRef: v1alpha1.PromptPackRef{Name: "legacy-pack"},
		Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeWebSocket},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).
		WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	cfg, err := LoadFromCRD(context.Background(), c, "legacy", "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Mode != modeAgentStr {
		t.Errorf("pre-mode AgentRuntime must load as Mode=%q, got %q", modeAgentStr, cfg.Mode)
	}
}
