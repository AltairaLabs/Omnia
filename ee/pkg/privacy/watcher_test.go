/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(s)
	return s
}

const testNamespace = "default"

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

func TestCollectPolicies_SkipsNonPolicyValues(t *testing.T) {
	w := &PolicyWatcher{log: logr.Discard()}
	w.policies.Store("valid", newTestPolicy("valid", omniav1alpha1.PolicyLevelGlobal))
	w.policies.Store("invalid", "not-a-policy")

	result := w.collectPolicies()
	assert.Len(t, result, 1)
	assert.Equal(t, "valid", result[0].Name)
}

func TestBuildPolicyChain_EmptyNamespace(t *testing.T) {
	global := newTestPolicy("global", omniav1alpha1.PolicyLevelGlobal)
	chain := buildPolicyChain([]*omniav1alpha1.SessionPrivacyPolicy{global}, "", "")
	assert.Len(t, chain, 1)
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

// --- loadPolicies tests using fake client ---

func TestLoadPolicies_PopulatesCache(t *testing.T) {
	scheme := testScheme()
	p1 := newTestPolicy("policy-1", omniav1alpha1.PolicyLevelGlobal)
	p1.Namespace = testNamespace
	p2 := newTestPolicy("policy-2", omniav1alpha1.PolicyLevelWorkspace)
	p2.Namespace = testNamespace
	p2.Spec.WorkspaceRef = &corev1alpha1.LocalObjectReference{Name: "my-ns"}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(p1, p2).
		Build()

	w := NewPolicyWatcher(fakeClient, logr.Discard())
	err := w.loadPolicies(context.Background())
	require.NoError(t, err)

	policies := w.collectPolicies()
	assert.Len(t, policies, 2)
}

func TestLoadPolicies_RemovesDeletedPolicies(t *testing.T) {
	scheme := testScheme()
	p1 := newTestPolicy("policy-1", omniav1alpha1.PolicyLevelGlobal)
	p1.Namespace = testNamespace

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(p1).
		Build()

	w := NewPolicyWatcher(fakeClient, logr.Discard())

	// First load — policy-1 should be cached
	err := w.loadPolicies(context.Background())
	require.NoError(t, err)
	assert.Len(t, w.collectPolicies(), 1)

	// Delete the policy from the fake client
	require.NoError(t, fakeClient.Delete(context.Background(), p1))

	// Second load — cache should be empty
	err = w.loadPolicies(context.Background())
	require.NoError(t, err)
	assert.Len(t, w.collectPolicies(), 0)
}

func TestLoadPolicies_GetEffectivePolicy_AfterLoad(t *testing.T) {
	scheme := testScheme()
	global := newTestPolicy("global-policy", omniav1alpha1.PolicyLevelGlobal)
	global.Namespace = testNamespace

	ws := newTestPolicy("ws-policy", omniav1alpha1.PolicyLevelWorkspace)
	ws.Namespace = testNamespace
	ws.Spec.WorkspaceRef = &corev1alpha1.LocalObjectReference{Name: "prod"}
	ws.Spec.Recording.PII = &omniav1alpha1.PIIConfig{
		Redact:   true,
		Encrypt:  true,
		Patterns: []string{"email"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(global, ws).
		Build()

	w := NewPolicyWatcher(fakeClient, logr.Discard())
	require.NoError(t, w.loadPolicies(context.Background()))

	// Should merge global + workspace for "prod" namespace
	ep := w.GetEffectivePolicy("prod", "my-agent")
	require.NotNil(t, ep)
	assert.True(t, ep.Recording.Enabled)
	assert.True(t, ep.Recording.PII.Redact)
	assert.True(t, ep.Recording.PII.Encrypt)
	assert.Contains(t, ep.Recording.PII.Patterns, "ssn")
	assert.Contains(t, ep.Recording.PII.Patterns, "email")

	// Different namespace should only get global
	ep2 := w.GetEffectivePolicy("staging", "my-agent")
	require.NotNil(t, ep2)
	assert.True(t, ep2.Recording.PII.Redact)
	assert.False(t, ep2.Recording.PII.Encrypt)
}

func TestNewPolicyWatcher(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	w := NewPolicyWatcher(fakeClient, logr.Discard())
	require.NotNil(t, w)
	assert.NotNil(t, w.client)
}

func TestStart_CancellationStopsPolling(t *testing.T) {
	scheme := testScheme()
	p := newTestPolicy("start-test", omniav1alpha1.PolicyLevelGlobal)
	p.Namespace = testNamespace
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(p).Build()

	w := NewPolicyWatcher(fakeClient, logr.Discard())
	w.SetPollInterval(50 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- w.Start(ctx) }()

	// Wait for initial load
	require.Eventually(t, func() bool {
		return w.GetEffectivePolicy("", "") != nil
	}, 2*time.Second, 10*time.Millisecond)

	cancel()

	err := <-errCh
	assert.NoError(t, err)
}

func TestStart_InitialLoadError(t *testing.T) {
	// A client with no scheme can't list SessionPrivacyPolicy — triggers error.
	badClient := fake.NewClientBuilder().Build()

	w := NewPolicyWatcher(badClient, logr.Discard())
	err := w.Start(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "initial policy load failed")
}

func TestEffectivePolicy_IncludesEncryption(t *testing.T) {
	w := &PolicyWatcher{log: logr.Discard()}
	policy := &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "global", Namespace: "omnia-system"},
		Spec: omniav1alpha1.SessionPrivacyPolicySpec{
			Level:     omniav1alpha1.PolicyLevelGlobal,
			Recording: omniav1alpha1.RecordingConfig{Enabled: true},
			Encryption: &omniav1alpha1.EncryptionConfig{
				Enabled:     true,
				KMSProvider: omniav1alpha1.KMSProviderAWSKMS,
				KeyID:       "arn:aws:kms:us-east-1:123:key/test",
			},
		},
	}
	w.policies.Store("omnia-system/global", policy)

	eff := w.GetEffectivePolicy("default", "my-agent")
	require.NotNil(t, eff)
	assert.True(t, eff.Encryption.Enabled)
	assert.Equal(t, "arn:aws:kms:us-east-1:123:key/test", eff.Encryption.KeyID)
	assert.Equal(t, omniav1alpha1.KMSProviderAWSKMS, eff.Encryption.KMSProvider)
}

func TestStart_PollPicksUpNewPolicies(t *testing.T) {
	scheme := testScheme()

	// Start with an empty cluster.
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	w := NewPolicyWatcher(fakeClient, logr.Discard())
	w.SetPollInterval(50 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = w.Start(ctx) }()

	// Initially no policies.
	time.Sleep(30 * time.Millisecond)
	assert.Nil(t, w.GetEffectivePolicy("", ""))

	// Create a policy in the fake client after startup.
	p := newTestPolicy("late-arrival", omniav1alpha1.PolicyLevelGlobal)
	p.Namespace = testNamespace
	require.NoError(t, fakeClient.Create(context.Background(), p))

	// Poll loop should pick it up.
	require.Eventually(t, func() bool {
		return w.GetEffectivePolicy("", "") != nil
	}, 2*time.Second, 10*time.Millisecond)
}
