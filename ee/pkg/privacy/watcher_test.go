/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

func newTestPolicy(name string, level omniav1alpha1.PolicyLevel) *omniav1alpha1.SessionPrivacyPolicy {
	p := &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: omniav1alpha1.SessionPrivacyPolicySpec{
			Level: level,
			Recording: omniav1alpha1.RecordingConfig{
				Enabled: true,
				PII: &omniav1alpha1.PIIConfig{
					Redact:   true,
					Patterns: []string{"ssn"},
				},
			},
			UserOptOut: &omniav1alpha1.UserOptOutConfig{Enabled: true},
		},
	}
	return p
}

func TestPolicyWatcher_GetEffectivePolicy_NoPolices(t *testing.T) {
	w := &PolicyWatcher{log: logr.Discard()}
	result := w.GetEffectivePolicy("ns", "agent")
	assert.Nil(t, result)
}

func TestPolicyWatcher_GetEffectivePolicy_GlobalOnly(t *testing.T) {
	w := &PolicyWatcher{log: logr.Discard()}
	global := newTestPolicy("global", omniav1alpha1.PolicyLevelGlobal)
	w.policies.Store("global", global)

	result := w.GetEffectivePolicy("ns", "agent")
	require.NotNil(t, result)
	assert.True(t, result.Recording.Enabled)
	assert.True(t, result.Recording.PII.Redact)
}

func TestPolicyWatcher_GetEffectivePolicy_GlobalAndWorkspace(t *testing.T) {
	w := &PolicyWatcher{log: logr.Discard()}

	global := newTestPolicy("global", omniav1alpha1.PolicyLevelGlobal)
	w.policies.Store("global", global)

	ws := newTestPolicy("ws", omniav1alpha1.PolicyLevelWorkspace)
	ws.Spec.WorkspaceRef = &corev1alpha1.LocalObjectReference{Name: "my-ns"}
	ws.Spec.Recording.PII = &omniav1alpha1.PIIConfig{
		Redact:   true,
		Encrypt:  true,
		Patterns: []string{"email"},
	}
	w.policies.Store("ws", ws)

	result := w.GetEffectivePolicy("my-ns", "agent")
	require.NotNil(t, result)
	assert.True(t, result.Recording.PII.Redact)
	assert.True(t, result.Recording.PII.Encrypt)
	assert.Contains(t, result.Recording.PII.Patterns, "ssn")
	assert.Contains(t, result.Recording.PII.Patterns, "email")
}

func TestPolicyWatcher_GetEffectivePolicy_FullChain(t *testing.T) {
	w := &PolicyWatcher{log: logr.Discard()}

	global := newTestPolicy("global", omniav1alpha1.PolicyLevelGlobal)
	w.policies.Store("global", global)

	ws := newTestPolicy("ws", omniav1alpha1.PolicyLevelWorkspace)
	ws.Spec.WorkspaceRef = &corev1alpha1.LocalObjectReference{Name: "my-ns"}
	w.policies.Store("ws", ws)

	agent := newTestPolicy("agent", omniav1alpha1.PolicyLevelAgent)
	agent.Spec.AgentRef = &corev1alpha1.NamespacedObjectReference{
		Name: "my-agent", Namespace: "my-ns",
	}
	w.policies.Store("agent", agent)

	result := w.GetEffectivePolicy("my-ns", "my-agent")
	require.NotNil(t, result)
	assert.True(t, result.Recording.Enabled)
	assert.NotNil(t, result.UserOptOut)
}

func TestPolicyWatcher_GetEffectivePolicy_WorkspaceMismatch(t *testing.T) {
	w := &PolicyWatcher{log: logr.Discard()}

	global := newTestPolicy("global", omniav1alpha1.PolicyLevelGlobal)
	w.policies.Store("global", global)

	ws := newTestPolicy("ws", omniav1alpha1.PolicyLevelWorkspace)
	ws.Spec.WorkspaceRef = &corev1alpha1.LocalObjectReference{Name: "other-ns"}
	w.policies.Store("ws", ws)

	result := w.GetEffectivePolicy("my-ns", "agent")
	require.NotNil(t, result)
	// Only global should match — workspace doesn't match namespace
}

func TestPolicyWatcher_OnAddAndDelete(t *testing.T) {
	w := &PolicyWatcher{log: logr.Discard()}

	p := newTestPolicy("test-policy", omniav1alpha1.PolicyLevelGlobal)
	w.onAdd(p)

	// Should be in cache
	result := w.GetEffectivePolicy("ns", "agent")
	require.NotNil(t, result)

	// Delete it
	w.onDelete(p)
	result = w.GetEffectivePolicy("ns", "agent")
	assert.Nil(t, result)
}

func TestPolicyWatcher_OnDelete_WrongType(t *testing.T) {
	w := &PolicyWatcher{log: logr.Discard()}
	// Should not panic
	w.onDelete("not-a-policy")
}

func TestPolicyWatcher_OnAdd_WrongType(t *testing.T) {
	w := &PolicyWatcher{log: logr.Discard()}
	// Should not panic
	w.onAdd("not-a-policy")
}

func TestPolicyKey(t *testing.T) {
	p := &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
	}
	assert.Equal(t, "ns/test", policyKey(p))
}

func TestCollectPolicies(t *testing.T) {
	w := &PolicyWatcher{log: logr.Discard()}
	w.policies.Store("a", newTestPolicy("a", omniav1alpha1.PolicyLevelGlobal))
	w.policies.Store("b", newTestPolicy("b", omniav1alpha1.PolicyLevelWorkspace))

	result := w.collectPolicies()
	assert.Len(t, result, 2)
}

func TestBuildPolicyChain_EmptyNamespace(t *testing.T) {
	global := newTestPolicy("global", omniav1alpha1.PolicyLevelGlobal)
	chain := buildPolicyChain([]*omniav1alpha1.SessionPrivacyPolicy{global}, "", "")
	assert.Len(t, chain, 1)
}

func TestFindByLevel_GlobalDefault(t *testing.T) {
	global := newTestPolicy("global", omniav1alpha1.PolicyLevelGlobal)
	result := findByLevel([]*omniav1alpha1.SessionPrivacyPolicy{global}, omniav1alpha1.PolicyLevelGlobal, "", "")
	require.NotNil(t, result)
	assert.Equal(t, "global", result.Name)
}

func TestFindByLevel_NoMatch(t *testing.T) {
	global := newTestPolicy("global", omniav1alpha1.PolicyLevelGlobal)
	result := findByLevel([]*omniav1alpha1.SessionPrivacyPolicy{global}, omniav1alpha1.PolicyLevelWorkspace, "ns", "")
	assert.Nil(t, result)
}
