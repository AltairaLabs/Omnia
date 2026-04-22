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

package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestProjectLegacyA2AAuth_NilAgent(t *testing.T) {
	t.Parallel()
	// Should not panic on a nil pointer.
	projectLegacyA2AAuth(nil)
}

func TestProjectLegacyA2AAuth_NoA2AConfig(t *testing.T) {
	t.Parallel()
	ar := &omniav1alpha1.AgentRuntime{}
	projectLegacyA2AAuth(ar)
	if ar.Spec.ExternalAuth != nil {
		t.Errorf("ExternalAuth = %+v, want nil when no A2A config to project", ar.Spec.ExternalAuth)
	}
}

func TestProjectLegacyA2AAuth_NoLegacyAuth(t *testing.T) {
	t.Parallel()
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			A2A: &omniav1alpha1.A2AConfig{},
		},
	}
	projectLegacyA2AAuth(ar)
	if ar.Spec.ExternalAuth != nil {
		t.Errorf("ExternalAuth = %+v, want nil when legacy auth absent", ar.Spec.ExternalAuth)
	}
}

func TestProjectLegacyA2AAuth_ProjectsToNewShape(t *testing.T) {
	t.Parallel()
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			A2A: &omniav1alpha1.A2AConfig{
				Authentication: &omniav1alpha1.A2AAuthConfig{
					SecretRef: &corev1.LocalObjectReference{Name: "legacy-token"},
				},
			},
		},
	}
	projectLegacyA2AAuth(ar)
	if ar.Spec.ExternalAuth == nil {
		t.Fatal("ExternalAuth not created")
	}
	if ar.Spec.ExternalAuth.SharedToken == nil {
		t.Fatal("ExternalAuth.SharedToken not populated")
	}
	if got := ar.Spec.ExternalAuth.SharedToken.SecretRef.Name; got != "legacy-token" {
		t.Errorf("SecretRef.Name = %q, want %q", got, "legacy-token")
	}
}

func TestProjectLegacyA2AAuth_PreservesExistingExternalAuth(t *testing.T) {
	// When externalAuth.sharedToken is already set, the projection must
	// not overwrite — operators who deliberately moved to the new shape
	// shouldn't have legacy state silently take precedence.
	t.Parallel()
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			A2A: &omniav1alpha1.A2AConfig{
				Authentication: &omniav1alpha1.A2AAuthConfig{
					SecretRef: &corev1.LocalObjectReference{Name: "legacy-token"},
				},
			},
			ExternalAuth: &omniav1alpha1.AgentExternalAuth{
				SharedToken: &omniav1alpha1.SharedTokenAuth{
					SecretRef: corev1.LocalObjectReference{Name: "operator-chosen-token"},
				},
			},
		},
	}
	projectLegacyA2AAuth(ar)
	if got := ar.Spec.ExternalAuth.SharedToken.SecretRef.Name; got != "operator-chosen-token" {
		t.Errorf("SecretRef.Name = %q, want %q (legacy must NOT overwrite)", got, "operator-chosen-token")
	}
}

func TestProjectLegacyA2AAuth_AppendsAlongsideOtherValidators(t *testing.T) {
	// When externalAuth is set with non-sharedToken validators, projection
	// should populate sharedToken alongside them.
	t.Parallel()
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			A2A: &omniav1alpha1.A2AConfig{
				Authentication: &omniav1alpha1.A2AAuthConfig{
					SecretRef: &corev1.LocalObjectReference{Name: "legacy-token"},
				},
			},
			ExternalAuth: &omniav1alpha1.AgentExternalAuth{
				OIDC: &omniav1alpha1.OIDCAuth{
					Issuer:   "https://idp.example.com",
					Audience: "omnia",
				},
			},
		},
	}
	projectLegacyA2AAuth(ar)
	if ar.Spec.ExternalAuth.OIDC == nil {
		t.Error("existing OIDC config dropped by projection")
	}
	if ar.Spec.ExternalAuth.SharedToken == nil {
		t.Error("legacy A2A auth was not projected alongside OIDC")
	}
}

func TestProjectLegacyA2AAuth_Idempotent(t *testing.T) {
	t.Parallel()
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			A2A: &omniav1alpha1.A2AConfig{
				Authentication: &omniav1alpha1.A2AAuthConfig{
					SecretRef: &corev1.LocalObjectReference{Name: "legacy"},
				},
			},
		},
	}
	projectLegacyA2AAuth(ar)
	first := ar.Spec.ExternalAuth.SharedToken.SecretRef.Name
	projectLegacyA2AAuth(ar)
	second := ar.Spec.ExternalAuth.SharedToken.SecretRef.Name
	if first != second {
		t.Errorf("non-idempotent: %q vs %q", first, second)
	}
}
