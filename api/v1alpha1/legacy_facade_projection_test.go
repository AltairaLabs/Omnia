/*
Copyright 2026.

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

package v1alpha1

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestProjectLegacyFacadeA2A_copiesWhenNewFieldUnset(t *testing.T) {
	t.Parallel()
	enabled := true
	port := int32(9999)
	ar := &AgentRuntime{
		Spec: AgentRuntimeSpec{
			A2A: &A2AConfig{Enabled: enabled, Port: &port},
		},
	}

	ProjectLegacyFacadeA2A(ar)

	if ar.Spec.Facade.A2A == nil {
		t.Fatal("expected Facade.A2A to be populated, got nil")
	}
	if diff := cmp.Diff(ar.Spec.A2A, ar.Spec.Facade.A2A); diff != "" {
		t.Errorf("Facade.A2A != legacy A2A: %s", diff)
	}
}

func TestProjectLegacyFacadeA2A_preservesExplicitNewField(t *testing.T) {
	t.Parallel()
	legacyEnabled := true
	newEnabled := false
	ar := &AgentRuntime{
		Spec: AgentRuntimeSpec{
			A2A:    &A2AConfig{Enabled: legacyEnabled},
			Facade: FacadeConfig{A2A: &A2AConfig{Enabled: newEnabled}},
		},
	}

	ProjectLegacyFacadeA2A(ar)

	if ar.Spec.Facade.A2A.Enabled != newEnabled {
		t.Errorf("Facade.A2A.Enabled was overwritten: got %v want %v",
			ar.Spec.Facade.A2A.Enabled, newEnabled)
	}
}

func TestProjectLegacyFacadeA2A_noLegacyNoNew(t *testing.T) {
	t.Parallel()
	ar := &AgentRuntime{}
	ProjectLegacyFacadeA2A(ar)
	if ar.Spec.Facade.A2A != nil {
		t.Errorf("expected nil, got %+v", ar.Spec.Facade.A2A)
	}
}

func TestProjectLegacyFacadeA2A_nilAgent(t *testing.T) {
	t.Parallel()
	// Must not panic.
	ProjectLegacyFacadeA2A(nil)
}

func TestProjectLegacyFacadeA2A_idempotent(t *testing.T) {
	t.Parallel()
	port := int32(9999)
	ar := &AgentRuntime{
		Spec: AgentRuntimeSpec{
			A2A: &A2AConfig{Enabled: true, Port: &port},
		},
	}

	ProjectLegacyFacadeA2A(ar)
	first := ar.Spec.Facade.A2A
	ProjectLegacyFacadeA2A(ar)
	if ar.Spec.Facade.A2A != first {
		t.Errorf("second call mutated Facade.A2A pointer")
	}
}

func TestProjectLegacyFacadeA2A_deepCopyIsolatesInnerPointers(t *testing.T) {
	t.Parallel()
	port := int32(9999)
	ar := &AgentRuntime{
		Spec: AgentRuntimeSpec{
			A2A: &A2AConfig{Enabled: true, Port: &port},
		},
	}

	ProjectLegacyFacadeA2A(ar)

	// Mutating the legacy field's inner pointer must not affect the
	// projected Facade.A2A — proves DeepCopy was used, not shallow copy.
	*ar.Spec.A2A.Port = 1234
	if ar.Spec.Facade.A2A.Port == nil || *ar.Spec.Facade.A2A.Port != 9999 {
		t.Errorf("Facade.A2A.Port aliased the legacy pointer; got %v want 9999",
			ar.Spec.Facade.A2A.Port)
	}
}
