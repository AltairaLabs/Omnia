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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1alpha1.AddToScheme(s)
	_ = omniav1alpha1.AddToScheme(s)
	return s
}

const testNamespace = "default"

// --- helpers ---

func storePolicy(w *PolicyWatcher, namespace, name string, spec omniav1alpha1.SessionPrivacyPolicySpec) {
	p := &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       spec,
	}
	w.policies.Store(namespace+"/"+name, p)
}

//nolint:unparam
func storeAgentRuntime(w *PolicyWatcher, namespace, name, serviceGroup string, ref *corev1.LocalObjectReference) {
	ar := &corev1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
	ar.Spec.ServiceGroup = serviceGroup
	ar.Spec.PrivacyPolicyRef = ref
	w.agents.Store(namespace+"/"+name, ar)
}

func storeWorkspace(w *PolicyWatcher, wsName, targetNamespace string, groups ...corev1alpha1.WorkspaceServiceGroup) {
	ws := &corev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: wsName},
		Spec: corev1alpha1.WorkspaceSpec{
			Namespace: corev1alpha1.NamespaceConfig{Name: targetNamespace},
			Services:  groups,
		},
	}
	w.workspaces.Store(wsName, ws)
}

func basicSpec(enabled bool) omniav1alpha1.SessionPrivacyPolicySpec {
	return omniav1alpha1.SessionPrivacyPolicySpec{
		Recording: omniav1alpha1.RecordingConfig{
			Enabled: enabled,
			PII:     &omniav1alpha1.PIIConfig{Redact: true},
		},
	}
}

// --- unit tests ---

func TestGetEffectivePolicy_NoPolices(t *testing.T) {
	w := &PolicyWatcher{log: logr.Discard()}
	result := w.GetEffectivePolicy("ns", "agent")
	assert.Nil(t, result)
}

func TestGetEffectivePolicy_AgentOverrideWins(t *testing.T) {
	w := &PolicyWatcher{log: logr.Discard()}

	// Global default
	storePolicy(w, "omnia-system", "default", basicSpec(true))

	// Agent-specific policy (recording disabled)
	storePolicy(w, "prod", "strict-policy", omniav1alpha1.SessionPrivacyPolicySpec{
		Recording: omniav1alpha1.RecordingConfig{Enabled: false},
	})

	// AgentRuntime references the strict policy
	storeAgentRuntime(w, "prod", "my-agent", "default", &corev1.LocalObjectReference{Name: "strict-policy"})

	result := w.GetEffectivePolicy("prod", "my-agent")
	require.NotNil(t, result)
	assert.False(t, result.Recording.Enabled, "agent override should win over global default")
}

func TestGetEffectivePolicy_ServiceGroupFallback(t *testing.T) {
	w := &PolicyWatcher{log: logr.Discard()}

	// Service-group policy
	storePolicy(w, "prod", "group-policy", basicSpec(true))

	// Workspace with a "default" service group referencing the policy
	storeWorkspace(w, "prod-ws", "prod",
		corev1alpha1.WorkspaceServiceGroup{
			Name:             "default",
			PrivacyPolicyRef: &corev1.LocalObjectReference{Name: "group-policy"},
		},
	)

	// Agent with no override
	storeAgentRuntime(w, "prod", "my-agent", "default", nil)

	result := w.GetEffectivePolicy("prod", "my-agent")
	require.NotNil(t, result)
	assert.True(t, result.Recording.Enabled, "service group policy should apply")
}

func TestGetEffectivePolicy_NamedServiceGroup(t *testing.T) {
	w := &PolicyWatcher{log: logr.Discard()}

	storePolicy(w, "prod", "analytics-policy", omniav1alpha1.SessionPrivacyPolicySpec{
		Recording: omniav1alpha1.RecordingConfig{Enabled: true, RichData: true},
	})

	storeWorkspace(w, "prod-ws", "prod",
		corev1alpha1.WorkspaceServiceGroup{
			Name:             "analytics",
			PrivacyPolicyRef: &corev1.LocalObjectReference{Name: "analytics-policy"},
		},
	)

	// Agent uses the "analytics" service group
	storeAgentRuntime(w, "prod", "analytics-agent", "analytics", nil)

	result := w.GetEffectivePolicy("prod", "analytics-agent")
	require.NotNil(t, result)
	assert.True(t, result.Recording.RichData, "named service group policy should apply")
}

func TestGetEffectivePolicy_GlobalDefaultFallback(t *testing.T) {
	w := &PolicyWatcher{log: logr.Discard()}

	storePolicy(w, "omnia-system", "default", basicSpec(true))

	// No workspace, no agent override
	result := w.GetEffectivePolicy("any-ns", "any-agent")
	require.NotNil(t, result)
	assert.True(t, result.Recording.Enabled, "global default should apply")
}

func TestGetEffectivePolicy_NoPolicyReturnsNil(t *testing.T) {
	w := &PolicyWatcher{log: logr.Discard()}

	// No policies at all
	result := w.GetEffectivePolicy("any-ns", "any-agent")
	assert.Nil(t, result)
}

func TestGetEffectivePolicy_AgentOverrideBeforeServiceGroup(t *testing.T) {
	w := &PolicyWatcher{log: logr.Discard()}

	// Both an agent override and a service group are present; agent should win.
	storePolicy(w, "prod", "agent-policy", omniav1alpha1.SessionPrivacyPolicySpec{
		Recording: omniav1alpha1.RecordingConfig{Enabled: false},
	})
	storePolicy(w, "prod", "group-policy", omniav1alpha1.SessionPrivacyPolicySpec{
		Recording: omniav1alpha1.RecordingConfig{Enabled: true},
	})
	storeWorkspace(w, "prod-ws", "prod",
		corev1alpha1.WorkspaceServiceGroup{
			Name:             "default",
			PrivacyPolicyRef: &corev1.LocalObjectReference{Name: "group-policy"},
		},
	)
	storeAgentRuntime(w, "prod", "my-agent", "default",
		&corev1.LocalObjectReference{Name: "agent-policy"})

	result := w.GetEffectivePolicy("prod", "my-agent")
	require.NotNil(t, result)
	assert.False(t, result.Recording.Enabled, "agent override must win over service group")
}

func TestGetEffectivePolicy_EncryptionPropagated(t *testing.T) {
	w := &PolicyWatcher{log: logr.Discard()}

	storePolicy(w, "omnia-system", "default", omniav1alpha1.SessionPrivacyPolicySpec{
		Recording: omniav1alpha1.RecordingConfig{Enabled: true},
		Encryption: &omniav1alpha1.EncryptionConfig{
			Enabled:     true,
			KMSProvider: omniav1alpha1.KMSProviderAWSKMS,
			KeyID:       "arn:aws:kms:us-east-1:123:key/test",
		},
	})

	result := w.GetEffectivePolicy("any-ns", "any-agent")
	require.NotNil(t, result)
	assert.True(t, result.Encryption.Enabled)
	assert.Equal(t, "arn:aws:kms:us-east-1:123:key/test", result.Encryption.KeyID)
}

func TestPolicyKey(t *testing.T) {
	p := &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
	}
	assert.Equal(t, "ns/test", policyKey(p))
}

// --- loadPolicies tests using fake client ---

func TestLoadPolicies_PopulatesCache(t *testing.T) {
	scheme := testScheme()
	p1 := &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "policy-1", Namespace: testNamespace},
		Spec:       basicSpec(true),
	}
	p2 := &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "policy-2", Namespace: testNamespace},
		Spec:       basicSpec(false),
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(p1, p2).
		Build()

	w := NewPolicyWatcher(fakeClient, logr.Discard())
	require.NoError(t, w.loadPolicies(context.Background()))

	count := 0
	w.policies.Range(func(_, _ any) bool { count++; return true })
	assert.Equal(t, 2, count)
}

func TestLoadPolicies_RemovesDeletedPolicies(t *testing.T) {
	scheme := testScheme()
	p1 := &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "policy-1", Namespace: testNamespace},
		Spec:       basicSpec(true),
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(p1).
		Build()

	w := NewPolicyWatcher(fakeClient, logr.Discard())
	require.NoError(t, w.loadPolicies(context.Background()))

	count := 0
	w.policies.Range(func(_, _ any) bool { count++; return true })
	assert.Equal(t, 1, count)

	require.NoError(t, fakeClient.Delete(context.Background(), p1))
	require.NoError(t, w.loadPolicies(context.Background()))

	count = 0
	w.policies.Range(func(_, _ any) bool { count++; return true })
	assert.Equal(t, 0, count)
}

func TestLoadWorkspaces_PopulatesCache(t *testing.T) {
	scheme := testScheme()
	ws := &corev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "prod-ws"},
		Spec: corev1alpha1.WorkspaceSpec{
			Namespace: corev1alpha1.NamespaceConfig{Name: "prod"},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
	w := NewPolicyWatcher(fakeClient, logr.Discard())
	require.NoError(t, w.loadWorkspaces(context.Background()))

	count := 0
	w.workspaces.Range(func(_, _ any) bool { count++; return true })
	assert.Equal(t, 1, count)
}

func TestLoadAgentRuntimes_PopulatesCache(t *testing.T) {
	scheme := testScheme()
	ar := &corev1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "my-agent", Namespace: "prod"},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ar).Build()
	w := NewPolicyWatcher(fakeClient, logr.Discard())
	require.NoError(t, w.loadAgentRuntimes(context.Background()))

	count := 0
	w.agents.Range(func(_, _ any) bool { count++; return true })
	assert.Equal(t, 1, count)
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
	p := &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "omnia-system"},
		Spec:       basicSpec(true),
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(p).Build()

	w := NewPolicyWatcher(fakeClient, logr.Discard())
	w.SetPollInterval(50 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- w.Start(ctx) }()

	require.Eventually(t, func() bool {
		return w.GetEffectivePolicy("", "") != nil
	}, 2*time.Second, 10*time.Millisecond)

	cancel()

	err := <-errCh
	assert.NoError(t, err)
}

func TestStart_InitialLoadError(t *testing.T) {
	// A client with no scheme can't list any resource — triggers error.
	badClient := fake.NewClientBuilder().Build()
	w := NewPolicyWatcher(badClient, logr.Discard())
	err := w.Start(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "initial policy load failed")
}

func TestStart_PollPicksUpNewPolicies(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	w := NewPolicyWatcher(fakeClient, logr.Discard())
	w.SetPollInterval(50 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = w.Start(ctx) }()

	time.Sleep(30 * time.Millisecond)
	assert.Nil(t, w.GetEffectivePolicy("", ""))

	p := &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "omnia-system"},
		Spec:       basicSpec(true),
	}
	require.NoError(t, fakeClient.Create(context.Background(), p))

	require.Eventually(t, func() bool {
		return w.GetEffectivePolicy("", "") != nil
	}, 2*time.Second, 10*time.Millisecond)
}

// TestOnPolicyChange_CallbackFiredOnAdd verifies that the OnPolicyChange
// callback is invoked when a new policy is loaded (nil→policy transition).
func TestOnPolicyChange_CallbackFiredOnAdd(t *testing.T) {
	scheme := testScheme()
	p1 := &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: testNamespace},
		Spec:       basicSpec(true),
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(p1).Build()
	w := NewPolicyWatcher(fakeClient, logr.Discard())

	type transition struct {
		old, new *omniav1alpha1.SessionPrivacyPolicy
	}
	var transitions []transition
	w.OnPolicyChange(func(old, new *omniav1alpha1.SessionPrivacyPolicy) {
		transitions = append(transitions, transition{old: old, new: new})
	})

	require.NoError(t, w.loadPolicies(context.Background()))

	require.Len(t, transitions, 1, "callback must fire once on first load")
	assert.Nil(t, transitions[0].old, "old must be nil on first observation")
	assert.Equal(t, "p1", transitions[0].new.Name)
}

// TestOnPolicyChange_CallbackFiredOnUpdate verifies that the callback receives
// the old value on a subsequent load when the policy is already cached.
func TestOnPolicyChange_CallbackFiredOnUpdate(t *testing.T) {
	scheme := testScheme()
	p1 := &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: testNamespace},
		Spec:       basicSpec(true),
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(p1).Build()
	w := NewPolicyWatcher(fakeClient, logr.Discard())

	type transition struct {
		old, new *omniav1alpha1.SessionPrivacyPolicy
	}
	var transitions []transition
	w.OnPolicyChange(func(old, new *omniav1alpha1.SessionPrivacyPolicy) {
		transitions = append(transitions, transition{old: old, new: new})
	})

	// First load: add the policy.
	require.NoError(t, w.loadPolicies(context.Background()))
	require.Len(t, transitions, 1)

	// Second load with same policy: callback fires again (old→new, same name).
	require.NoError(t, w.loadPolicies(context.Background()))
	require.Len(t, transitions, 2, "callback must fire again on reload")
	assert.NotNil(t, transitions[1].old, "old must be the previous version on reload")
	assert.Equal(t, "p1", transitions[1].new.Name)
}

// TestOnPolicyChange_CallbackFiredOnDelete verifies that the callback receives
// (old, nil) when a policy is evicted from the cache.
func TestOnPolicyChange_CallbackFiredOnDelete(t *testing.T) {
	scheme := testScheme()
	p1 := &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: testNamespace},
		Spec:       basicSpec(true),
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(p1).Build()
	w := NewPolicyWatcher(fakeClient, logr.Discard())

	type transition struct {
		old, new *omniav1alpha1.SessionPrivacyPolicy
	}
	var transitions []transition
	w.OnPolicyChange(func(old, new *omniav1alpha1.SessionPrivacyPolicy) {
		transitions = append(transitions, transition{old: old, new: new})
	})

	// First load: policy added.
	require.NoError(t, w.loadPolicies(context.Background()))
	require.Len(t, transitions, 1)

	// Delete the policy from the fake client and reload.
	require.NoError(t, fakeClient.Delete(context.Background(), p1))
	require.NoError(t, w.loadPolicies(context.Background()))

	require.Len(t, transitions, 2, "callback must fire on deletion")
	assert.Equal(t, "p1", transitions[1].old.Name, "old must carry the deleted policy")
	assert.Nil(t, transitions[1].new, "new must be nil on deletion")
}

// TestOnPolicyChange_ReplacedCallback verifies that a second OnPolicyChange
// call replaces the first callback rather than chaining them.
func TestOnPolicyChange_ReplacedCallback(t *testing.T) {
	scheme := testScheme()
	p1 := &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: testNamespace},
		Spec:       basicSpec(true),
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(p1).Build()
	w := NewPolicyWatcher(fakeClient, logr.Discard())

	first := 0
	second := 0
	w.OnPolicyChange(func(_, _ *omniav1alpha1.SessionPrivacyPolicy) { first++ })
	w.OnPolicyChange(func(_, _ *omniav1alpha1.SessionPrivacyPolicy) { second++ })

	require.NoError(t, w.loadPolicies(context.Background()))

	assert.Zero(t, first, "first callback must be replaced, not chained")
	assert.Equal(t, 1, second, "second callback must be called once")
}
