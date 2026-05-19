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

package main

import (
	"testing"

	"github.com/altairalabs/omnia/internal/agent"
)

const (
	goodInputSchema  = `{"type":"object","required":["q"]}`
	goodOutputSchema = `{"type":"object","required":["a"]}`
)

func validFunctionConfig() *agent.Config {
	return &agent.Config{
		AgentName:                "summarizer",
		Namespace:                "prod",
		Mode:                     "function",
		FunctionInputSchemaJSON:  []byte(goodInputSchema),
		FunctionOutputSchemaJSON: []byte(goodOutputSchema),
	}
}

func TestValidateFunctionMode_AcceptsValidConfig(t *testing.T) {
	if err := validateFunctionMode(validFunctionConfig()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateFunctionMode_RejectsMissingAgentName(t *testing.T) {
	cfg := validFunctionConfig()
	cfg.AgentName = ""
	err := validateFunctionMode(cfg)
	if err == nil {
		t.Fatalf("expected error for missing agent name")
	}
}

func TestValidateFunctionMode_RejectsMissingInputSchema(t *testing.T) {
	cfg := validFunctionConfig()
	cfg.FunctionInputSchemaJSON = nil
	err := validateFunctionMode(cfg)
	if err == nil {
		t.Fatalf("expected error for missing input schema")
	}
}

func TestValidateFunctionMode_RejectsMissingOutputSchema(t *testing.T) {
	cfg := validFunctionConfig()
	cfg.FunctionOutputSchemaJSON = nil
	err := validateFunctionMode(cfg)
	if err == nil {
		t.Fatalf("expected error for missing output schema")
	}
}

func TestBuildFunctionRegistry_RegistersSingleFunctionByLowercaseName(t *testing.T) {
	cfg := validFunctionConfig()
	cfg.AgentName = "Summarizer-Service" // Mixed case to confirm canonicalisation.
	cfg.FunctionRecordsInvocations = true

	reg, err := buildFunctionRegistry(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Lookup must use the lowercase form per the k8s naming convention.
	spec, ok := reg.GetFunction("summarizer-service")
	if !ok {
		t.Fatalf("expected to find function by lowercase name")
	}
	if spec.Name != "summarizer-service" {
		t.Errorf("Name = %q, want lowercase canonical %q", spec.Name, "summarizer-service")
	}
	if !spec.RecordsInvocations {
		t.Errorf("RecordsInvocations = false, want true")
	}

	// The original mixed-case name should NOT match.
	if _, ok := reg.GetFunction("Summarizer-Service"); ok {
		t.Errorf("registry must not match the mixed-case original name")
	}
}

func TestBuildFunctionRegistry_RejectsInvalidInputSchema(t *testing.T) {
	cfg := validFunctionConfig()
	cfg.FunctionInputSchemaJSON = []byte(`{not json`)
	_, err := buildFunctionRegistry(cfg)
	if err == nil {
		t.Fatalf("expected error for malformed input schema")
	}
}

func TestBuildFunctionRegistry_RejectsInvalidOutputSchema(t *testing.T) {
	cfg := validFunctionConfig()
	cfg.FunctionOutputSchemaJSON = []byte(`{"type":42}`) // type must be string
	_, err := buildFunctionRegistry(cfg)
	if err == nil {
		t.Fatalf("expected error for invalid output schema")
	}
}
