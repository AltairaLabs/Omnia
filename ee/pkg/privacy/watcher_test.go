/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

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

func TestPolicyWatcher_OnDelete_Tombstone(t *testing.T) {
	w := &PolicyWatcher{log: logr.Discard()}

	p := newTestPolicy("tomb-policy", omniav1alpha1.PolicyLevelGlobal)
	w.onAdd(p)

	// Verify policy is cached
	result := w.GetEffectivePolicy("ns", "agent")
	require.NotNil(t, result)

	// Delete via tombstone (DeletedFinalStateUnknown wraps the real object)
	tombstone := cache.DeletedFinalStateUnknown{
		Key: "/tomb-policy",
		Obj: p,
	}
	w.onDelete(tombstone)

	result = w.GetEffectivePolicy("ns", "agent")
	assert.Nil(t, result)
}

func TestPolicyWatcher_OnDelete_TombstoneWrongInnerType(t *testing.T) {
	w := &PolicyWatcher{log: logr.Discard()}

	// Tombstone wrapping a non-policy object — should not panic
	tombstone := cache.DeletedFinalStateUnknown{
		Key: "bad-key",
		Obj: "not-a-policy",
	}
	w.onDelete(tombstone)
}

func TestNewPolicyWatcher(t *testing.T) {
	// Start a test HTTP server that responds to API discovery.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := `{"kind":"SessionPrivacyPolicyList",` +
			`"apiVersion":"omnia.altairalabs.com/v1alpha1",` +
			`"metadata":{},"items":[]}`
		_, _ = w.Write([]byte(resp))
	}))
	defer srv.Close()

	cfg := &rest.Config{Host: srv.URL}
	pw, err := NewPolicyWatcher(cfg, logr.Discard())
	require.NoError(t, err)
	require.NotNil(t, pw)
	assert.NotNil(t, pw.informer)
}

func TestPolicyWatcher_Start_SyncFailure(t *testing.T) {
	// Cancel the context immediately so cache sync fails.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Use a test server that will never respond (context already cancelled).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg := &rest.Config{Host: srv.URL}
	pw, err := NewPolicyWatcher(cfg, logr.Discard())
	require.NoError(t, err)

	err = pw.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cache sync failed")
}

func TestCollectPolicies_SkipsNonPolicyValues(t *testing.T) {
	w := &PolicyWatcher{log: logr.Discard()}
	w.policies.Store("valid", newTestPolicy("valid", omniav1alpha1.PolicyLevelGlobal))
	w.policies.Store("invalid", "not-a-policy")

	result := w.collectPolicies()
	assert.Len(t, result, 1)
	assert.Equal(t, "valid", result[0].Name)
}

func TestBuildPolicyChain_AgentWithoutNamespace(t *testing.T) {
	agent := newTestPolicy("agent", omniav1alpha1.PolicyLevelAgent)
	agent.Spec.AgentRef = &corev1alpha1.NamespacedObjectReference{
		Name: "my-agent", Namespace: "ns",
	}
	// agentName set but namespace empty — agent policy should not be included
	chain := buildPolicyChain([]*omniav1alpha1.SessionPrivacyPolicy{agent}, "", "my-agent")
	assert.Len(t, chain, 0)
}

func TestFindByLevel_WorkspaceNilRef(t *testing.T) {
	// Workspace-level policy with nil WorkspaceRef should not match.
	ws := newTestPolicy("ws", omniav1alpha1.PolicyLevelWorkspace)
	ws.Spec.WorkspaceRef = nil

	result := findByLevel([]*omniav1alpha1.SessionPrivacyPolicy{ws}, omniav1alpha1.PolicyLevelWorkspace, "ns", "")
	assert.Nil(t, result)
}

func TestFindByLevel_AgentNilRef(t *testing.T) {
	// Agent-level policy with nil AgentRef should not match.
	agent := newTestPolicy("agent", omniav1alpha1.PolicyLevelAgent)
	agent.Spec.AgentRef = nil

	result := findByLevel([]*omniav1alpha1.SessionPrivacyPolicy{agent}, omniav1alpha1.PolicyLevelAgent, "ns", "my-agent")
	assert.Nil(t, result)
}

func TestFindByLevel_AgentNameMismatch(t *testing.T) {
	agent := newTestPolicy("agent", omniav1alpha1.PolicyLevelAgent)
	agent.Spec.AgentRef = &corev1alpha1.NamespacedObjectReference{
		Name: "other-agent", Namespace: "ns",
	}

	result := findByLevel([]*omniav1alpha1.SessionPrivacyPolicy{agent}, omniav1alpha1.PolicyLevelAgent, "ns", "my-agent")
	assert.Nil(t, result)
}

func TestFindByLevel_AgentNamespaceMismatch(t *testing.T) {
	agent := newTestPolicy("agent", omniav1alpha1.PolicyLevelAgent)
	agent.Spec.AgentRef = &corev1alpha1.NamespacedObjectReference{
		Name: "my-agent", Namespace: "other-ns",
	}

	result := findByLevel([]*omniav1alpha1.SessionPrivacyPolicy{agent}, omniav1alpha1.PolicyLevelAgent, "ns", "my-agent")
	assert.Nil(t, result)
}
