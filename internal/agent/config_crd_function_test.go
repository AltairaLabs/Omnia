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
}

func TestLoadFromCRD_FunctionMode_EmptyRawSchemaSurfacesAsEmptyBytes(t *testing.T) {
	// Edge case: spec.inputSchema is set but Raw is empty. The CRD CEL
	// gate doesn't actually validate JSON-Schema content (the field is
	// preserve-unknown-fields), so an empty payload IS possible. We
	// capture whatever Raw contains; validateFunctionMode (in cmd/agent)
	// is the gate that rejects empty Raw at startup.
	ar := newFakeAgentRuntime("edge", "prod", v1alpha1.AgentRuntimeSpec{
		Mode:          v1alpha1.AgentRuntimeModeFunction,
		PromptPackRef: v1alpha1.PromptPackRef{Name: "edge-pack"},
		Facade:        v1alpha1.FacadeConfig{Type: v1alpha1.FacadeTypeGRPC},
		InputSchema:   &apiextensionsv1.JSON{Raw: []byte{}},
		OutputSchema:  &apiextensionsv1.JSON{Raw: []byte(functionTestOutputSchema)},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).
		WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	cfg, err := LoadFromCRD(context.Background(), c, "edge", "prod")
	if err != nil {
		t.Fatalf("LoadFromCRD: %v", err)
	}
	if len(cfg.FunctionInputSchemaJSON) != 0 {
		t.Errorf("FunctionInputSchemaJSON should be empty; got %q",
			string(cfg.FunctionInputSchemaJSON))
	}
	if err := cfg.Validate(); err == nil {
		t.Errorf("Validate() must reject function-mode with empty input schema Raw")
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
}

func TestLoadFromCRD_FunctionMode_PassesValidate(t *testing.T) {
	// Wiring invariant: a function-mode AgentRuntime read from the CRD
	// must produce a Config that survives Validate(). Function mode serves
	// HTTP, so its honest facade is rest (#1464).
	ar := newFakeAgentRuntime("summarizer", "prod", v1alpha1.AgentRuntimeSpec{
		Mode:          v1alpha1.AgentRuntimeModeFunction,
		PromptPackRef: v1alpha1.PromptPackRef{Name: "summarizer-pack"},
		Facade: v1alpha1.FacadeConfig{
			Type: v1alpha1.FacadeTypeREST,
		},
		InputSchema:  &apiextensionsv1.JSON{Raw: []byte(functionTestInputSchema)},
		OutputSchema: &apiextensionsv1.JSON{Raw: []byte(functionTestOutputSchema)},
	})

	c := fake.NewClientBuilder().WithScheme(k8s.Scheme()).
		WithRuntimeObjects(ar, testNamespace(ar.Namespace)).Build()

	cfg, err := LoadFromCRD(context.Background(), c, "summarizer", "prod")
	if err != nil {
		t.Fatalf("LoadFromCRD: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("function-mode Config must pass Validate(); got %v", err)
	}
}

func TestLoadFromCRD_FunctionMode_RejectsWebSocketFacade(t *testing.T) {
	// Defensive: even if the CRD CEL gate somehow let a function-mode
	// runtime through with facade.type=websocket, the binary must still
	// refuse the config rather than boot half-working.
	cfg := &Config{
		AgentName:                "x",
		Namespace:                "y",
		PromptPackName:           "p",
		Mode:                     ModeFunction,
		HandlerMode:              HandlerModeRuntime,
		FacadeType:               FacadeTypeWebSocket,
		MediaStorageType:         MediaStorageTypeNone,
		FunctionInputSchemaJSON:  []byte(functionTestInputSchema),
		FunctionOutputSchemaJSON: []byte(functionTestOutputSchema),
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("Validate must reject function-mode + facade.type=websocket")
	}
}

func TestLoadFromCRD_FunctionMode_RejectsMissingSchemas(t *testing.T) {
	cfg := &Config{
		AgentName:        "x",
		Namespace:        "y",
		PromptPackName:   "p",
		Mode:             ModeFunction,
		HandlerMode:      HandlerModeRuntime,
		FacadeType:       FacadeTypeREST,
		MediaStorageType: MediaStorageTypeNone,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("Validate must reject function-mode without schemas")
	}
}

// functionFacadeConfig builds a minimal valid function-mode Config with the
// given facade type and complete schemas, isolating the facade-type check.
func functionFacadeConfig(facade FacadeType) *Config {
	return &Config{
		AgentName:                "x",
		Namespace:                "y",
		PromptPackName:           "p",
		Mode:                     ModeFunction,
		HandlerMode:              HandlerModeRuntime,
		FacadeType:               facade,
		MediaStorageType:         MediaStorageTypeNone,
		FunctionInputSchemaJSON:  []byte(functionTestInputSchema),
		FunctionOutputSchemaJSON: []byte(functionTestOutputSchema),
	}
}

func TestFunctionMode_FacadeTypeValidation(t *testing.T) {
	// Function mode serves HTTP (POST /functions/{name}); only rest and a2a
	// are honest labels. websocket and grpc must be refused even if the CEL
	// gate were bypassed, so the binary never boots half-working (#1464).
	tests := []struct {
		name    string
		facade  FacadeType
		wantErr bool
	}{
		{name: "rest accepted", facade: FacadeTypeREST, wantErr: false},
		{name: "a2a accepted", facade: FacadeTypeA2A, wantErr: false},
		{name: "websocket rejected", facade: FacadeTypeWebSocket, wantErr: true},
		{name: "grpc rejected", facade: FacadeTypeGRPC, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := functionFacadeConfig(tt.facade).Validate()
			if tt.wantErr && err == nil {
				t.Fatalf("Validate() expected error for function + facade.type=%q, got nil", tt.facade)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() unexpected error for function + facade.type=%q: %v", tt.facade, err)
			}
		})
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
