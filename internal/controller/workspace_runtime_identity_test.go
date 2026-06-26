/*
Copyright 2026 Altaira Labs.

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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const (
	wsIdentityLabel = "azure.workload.identity/use"
	tkTeam          = "team"
	wsSAName        = "ws-runtime-wi"
	nsName          = "ns1"
)

func arWithSA(sa string) *omniav1alpha1.AgentRuntime {
	ar := &omniav1alpha1.AgentRuntime{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: nsName}}
	if sa != "" {
		ar.Spec.PodOverrides = &omniav1alpha1.PodOverrides{ServiceAccountName: sa}
	}
	return ar
}

// --- effectiveServiceAccountName precedence: agent SA > workspace SA > <name>-facade ---

func TestEffectiveServiceAccountName_Precedence(t *testing.T) {
	wsSA := &omniav1alpha1.RuntimeDefaults{ServiceAccountName: wsSAName}
	cases := []struct {
		name string
		ar   *omniav1alpha1.AgentRuntime
		ws   *omniav1alpha1.RuntimeDefaults
		want string
	}{
		{"agent SA wins over workspace", arWithSA("own-sa"), wsSA, "own-sa"},
		{"workspace SA when agent has none", arWithSA(""), wsSA, wsSAName},
		{"facade default when neither", arWithSA(""), nil, "a-facade"},
		{"facade default when workspace has no SA", arWithSA(""), &omniav1alpha1.RuntimeDefaults{PodLabels: map[string]string{wsIdentityLabel: labelValueTrue}}, "a-facade"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, effectiveServiceAccountName(tc.ar, tc.ws))
		})
	}
}

// --- effectivePodOverrides: workspace layered under agent, unit opt-out ---

func TestEffectivePodOverrides_NoWorkspace_ReturnsAgent(t *testing.T) {
	ar := arWithSA("")
	assert.Nil(t, effectivePodOverrides(ar, nil), "no workspace + no agent overrides → nil")

	ar.Spec.PodOverrides = &omniav1alpha1.PodOverrides{PriorityClassName: "high"}
	assert.Same(t, ar.Spec.PodOverrides, effectivePodOverrides(ar, nil),
		"no workspace → agent overrides returned unchanged")
}

func TestEffectivePodOverrides_EmptyWorkspace_ReturnsAgent(t *testing.T) {
	ar := arWithSA("")
	empty := &omniav1alpha1.RuntimeDefaults{}
	assert.Nil(t, effectivePodOverrides(ar, empty), "empty workspace defaults add nothing")
}

func TestEffectivePodOverrides_AgentOwnsIdentity_OptsOutAsUnit(t *testing.T) {
	ar := arWithSA("own-sa")
	ws := &omniav1alpha1.RuntimeDefaults{
		ServiceAccountName: wsSAName,
		PodLabels:          map[string]string{wsIdentityLabel: labelValueTrue},
	}
	got := effectivePodOverrides(ar, ws)
	require.NotNil(t, got)
	assert.Equal(t, "own-sa", got.ServiceAccountName, "agent SA preserved")
	assert.NotContains(t, got.Labels, wsIdentityLabel,
		"agent that brings its own SA must NOT inherit the workspace identity label")
}

func TestEffectivePodOverrides_InheritsWorkspaceIdentity(t *testing.T) {
	ar := arWithSA("") // no SA → inherits
	ar.Spec.PodOverrides = &omniav1alpha1.PodOverrides{
		Labels:            map[string]string{tkTeam: "blue"},
		PriorityClassName: "high",
	}
	ws := &omniav1alpha1.RuntimeDefaults{
		ServiceAccountName: wsSAName,
		PodLabels:          map[string]string{wsIdentityLabel: labelValueTrue, tkTeam: "red"},
		PodAnnotations:     map[string]string{"a": "1"},
	}
	got := effectivePodOverrides(ar, ws)
	require.NotNil(t, got)
	assert.Equal(t, wsSAName, got.ServiceAccountName)
	assert.Equal(t, labelValueTrue, got.Labels[wsIdentityLabel], "workspace identity label applied")
	assert.Equal(t, "blue", got.Labels[tkTeam], "agent label wins on key collision")
	assert.Equal(t, "1", got.Annotations["a"], "workspace annotation applied")
	assert.Equal(t, "high", got.PriorityClassName, "agent's other pod fields preserved")

	// Must not mutate the agent's own overrides.
	assert.NotContains(t, ar.Spec.PodOverrides.Labels, wsIdentityLabel,
		"merge must not mutate the agent's PodOverrides")
}

// --- reconciler resolution against a fake client ---

func TestEffectiveFacadeServiceAccountName_ResolvesWorkspace(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec: omniav1alpha1.WorkspaceSpec{
			Namespace: omniav1alpha1.NamespaceConfig{Name: nsName},
			Runtime:   &omniav1alpha1.RuntimeDefaults{ServiceAccountName: wsSAName},
		},
	}
	r := &AgentRuntimeReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build(),
		Scheme: scheme,
	}

	// Agent in ns1, no SA → inherits the workspace SA.
	assert.Equal(t, wsSAName, r.effectiveFacadeServiceAccountName(arWithSA("")))
	// Agent in ns1 with its own SA → wins.
	assert.Equal(t, "own-sa", r.effectiveFacadeServiceAccountName(arWithSA("own-sa")))
	// Agent in a namespace with no workspace → facade default.
	other := &omniav1alpha1.AgentRuntime{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns2"}}
	assert.Equal(t, "b-facade", r.effectiveFacadeServiceAccountName(other))

	// And the merged pod overrides carry the workspace SA for the inheriting agent.
	eff := r.effectivePodOverridesForAgent(arWithSA(""))
	require.NotNil(t, eff)
	assert.Equal(t, wsSAName, eff.ServiceAccountName)
}

func TestResolveWorkspaceRuntimeDefaults_NilClient(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	assert.Nil(t, r.resolveWorkspaceRuntimeDefaults(nsName))
}

func TestResolveWorkspaceRuntimeDefaults_NoMatch(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	r := &AgentRuntimeReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), Scheme: scheme}
	assert.Nil(t, r.resolveWorkspaceRuntimeDefaults("nope"))
}
