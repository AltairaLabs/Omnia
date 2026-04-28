/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// devWorkspaceWithMemoryPolicyRef builds a Workspace named "dev" whose
// "default" service group references the named MemoryPolicy via
// memory.policyRef. The names match the values passed to
// NewK8sPolicyLoader in these tests; widen if a future test needs
// different workspace / group names.
func devWorkspaceWithMemoryPolicyRef(policyName string) *omniav1alpha1.Workspace {
	return &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "dev"},
		Spec: omniav1alpha1.WorkspaceSpec{
			Services: []omniav1alpha1.WorkspaceServiceGroup{{
				Name: "default",
				Memory: &omniav1alpha1.MemoryServiceConfig{
					PolicyRef: &corev1.LocalObjectReference{Name: policyName},
				},
			}},
		},
	}
}

func policySchemeForTest(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(s))
	return s
}

func TestK8sPolicyLoader_LoadsPolicyViaWorkspacePolicyRef(t *testing.T) {
	scheme := policySchemeForTest(t)
	policy := &omniav1alpha1.MemoryPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "strict-compliance"},
		Spec:       omniav1alpha1.MemoryPolicySpec{Schedule: "0 3 * * *"},
	}
	ws := devWorkspaceWithMemoryPolicyRef("strict-compliance")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(policy, ws).Build()
	loader := NewK8sPolicyLoader(c, zap.New(zap.UseDevMode(true)), "dev", "default")

	got, err := loader.Load(context.Background())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "strict-compliance", got.Name)
	assert.Equal(t, "0 3 * * *", got.Spec.Schedule)
}

func TestK8sPolicyLoader_NoWorkspaceContextReturnsNil(t *testing.T) {
	scheme := policySchemeForTest(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	loader := NewK8sPolicyLoader(c, zap.New(zap.UseDevMode(true)), "", "")

	got, err := loader.Load(context.Background())
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestK8sPolicyLoader_WorkspaceNotFoundReturnsNil(t *testing.T) {
	scheme := policySchemeForTest(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	loader := NewK8sPolicyLoader(c, zap.New(zap.UseDevMode(true)), "missing-workspace", "default")

	got, err := loader.Load(context.Background())
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestK8sPolicyLoader_NoPolicyRefReturnsNil(t *testing.T) {
	scheme := policySchemeForTest(t)
	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "dev"},
		Spec: omniav1alpha1.WorkspaceSpec{
			Services: []omniav1alpha1.WorkspaceServiceGroup{{
				Name:   "default",
				Memory: &omniav1alpha1.MemoryServiceConfig{},
			}},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
	loader := NewK8sPolicyLoader(c, zap.New(zap.UseDevMode(true)), "dev", "default")

	got, err := loader.Load(context.Background())
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestK8sPolicyLoader_ServiceGroupMissingReturnsNil(t *testing.T) {
	scheme := policySchemeForTest(t)
	ws := devWorkspaceWithMemoryPolicyRef("any")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
	loader := NewK8sPolicyLoader(c, zap.New(zap.UseDevMode(true)), "dev", "other-group")

	got, err := loader.Load(context.Background())
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestK8sPolicyLoader_NamedPolicyMissingReturnsNil(t *testing.T) {
	scheme := policySchemeForTest(t)
	ws := devWorkspaceWithMemoryPolicyRef("missing-policy")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
	loader := NewK8sPolicyLoader(c, zap.New(zap.UseDevMode(true)), "dev", "default")

	got, err := loader.Load(context.Background())
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestK8sPolicyLoader_ReturnsCachedOnGetError(t *testing.T) {
	scheme := policySchemeForTest(t)
	policy := &omniav1alpha1.MemoryPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "default-policy"},
		Spec:       omniav1alpha1.MemoryPolicySpec{Schedule: "0 3 * * *"},
	}
	ws := devWorkspaceWithMemoryPolicyRef("default-policy")
	var callCount int
	errAfterFirst := errors.New("simulated API outage")
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(policy, ws).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*omniav1alpha1.MemoryPolicy); ok {
					callCount++
					if callCount > 1 {
						return errAfterFirst
					}
				}
				return cl.Get(ctx, key, obj, opts...)
			},
		}).
		Build()

	loader := NewK8sPolicyLoader(c, zap.New(zap.UseDevMode(true)), "dev", "default")
	// Force the second call to bypass the TTL cache so it hits the
	// (failing) K8s client and exercises the cached-fallback branch.
	loader.CacheTTL = time.Nanosecond

	first, err := loader.Load(context.Background())
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, "default-policy", first.Name)

	// Sleep past the nanosecond TTL so the second call refetches and
	// hits the simulated outage.
	time.Sleep(time.Microsecond)

	second, err := loader.Load(context.Background())
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, "default-policy", second.Name)
}

// TestK8sPolicyLoader_TTLCacheShortCircuitsK8s proves the TTL cache:
// two back-to-back Loads within the freshness window only call the
// K8s API once. This is what protects the hot Save path from
// hammering the API server.
func TestK8sPolicyLoader_TTLCacheShortCircuitsK8s(t *testing.T) {
	scheme := policySchemeForTest(t)
	policy := &omniav1alpha1.MemoryPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "default-policy"},
		Spec:       omniav1alpha1.MemoryPolicySpec{Schedule: "0 3 * * *"},
	}
	ws := devWorkspaceWithMemoryPolicyRef("default-policy")
	var policyGets int
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(policy, ws).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*omniav1alpha1.MemoryPolicy); ok {
					policyGets++
				}
				return cl.Get(ctx, key, obj, opts...)
			},
		}).
		Build()

	loader := NewK8sPolicyLoader(c, zap.New(zap.UseDevMode(true)), "dev", "default")

	for i := 0; i < 5; i++ {
		_, err := loader.Load(context.Background())
		require.NoError(t, err)
	}
	assert.Equal(t, 1, policyGets,
		"5 Loads inside the TTL window must result in 1 K8s API GET")
}

// TestK8sPolicyLoader_NoopFetchAlsoCached proves the "no policy
// bound" answer is cached too — without this an unbound workspace
// would still hit the API on every Load.
func TestK8sPolicyLoader_NoopFetchAlsoCached(t *testing.T) {
	scheme := policySchemeForTest(t)
	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "dev"},
		Spec: omniav1alpha1.WorkspaceSpec{
			Services: []omniav1alpha1.WorkspaceServiceGroup{{Name: "default"}},
		},
	}
	var workspaceGets int
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ws).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*omniav1alpha1.Workspace); ok {
					workspaceGets++
				}
				return cl.Get(ctx, key, obj, opts...)
			},
		}).
		Build()

	loader := NewK8sPolicyLoader(c, zap.New(zap.UseDevMode(true)), "dev", "default")

	for i := 0; i < 3; i++ {
		got, err := loader.Load(context.Background())
		require.NoError(t, err)
		assert.Nil(t, got)
	}
	assert.Equal(t, 1, workspaceGets,
		"the no-policy answer must be cached so subsequent Loads short-circuit")
}

func TestK8sPolicyLoader_ReturnsErrorWhenWorkspaceGetFailsWithoutCache(t *testing.T) {
	scheme := policySchemeForTest(t)
	simulated := errors.New("simulated API outage")
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return simulated
			},
		}).
		Build()

	loader := NewK8sPolicyLoader(c, zap.New(zap.UseDevMode(true)), "dev", "default")
	_, err := loader.Load(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated API outage")
}

func TestLegacyIntervalPolicy_ShapesAllTiersTTL(t *testing.T) {
	p := LegacyIntervalPolicy(15 * time.Minute)
	require.NotNil(t, p)
	require.NotNil(t, p.Spec.Tiers.Institutional)
	require.NotNil(t, p.Spec.Tiers.Agent)
	require.NotNil(t, p.Spec.Tiers.User)
	assert.Equal(t, omniav1alpha1.MemoryRetentionModeTTL,
		p.Spec.Tiers.Institutional.Mode)
	assert.Equal(t, "@every 15m0s", p.Spec.Schedule)
}

func TestParseRetentionDuration(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"5m", 5 * time.Minute},
		{"2h", 2 * time.Hour},
		{"30d", 30 * 24 * time.Hour},
		{"1d12h", 36 * time.Hour},
	}
	for _, c := range cases {
		got, err := parseRetentionDuration(c.in)
		require.NoError(t, err, "input %q", c.in)
		assert.Equal(t, c.want, got, "input %q", c.in)
	}
}

func TestParseRetentionDuration_Invalid(t *testing.T) {
	for _, in := range []string{"", "nope", "d", "abcd"} {
		_, err := parseRetentionDuration(in)
		assert.Error(t, err, "input %q should fail", in)
	}
}

func TestBranchesForMode(t *testing.T) {
	cases := []struct {
		mode omniav1alpha1.MemoryRetentionMode
		want []RetentionBranch
	}{
		{omniav1alpha1.MemoryRetentionModeManual, nil},
		{"", nil},
		{omniav1alpha1.MemoryRetentionModeTTL, []RetentionBranch{BranchTTL}},
		{omniav1alpha1.MemoryRetentionModeLRU, []RetentionBranch{BranchLRU}},
		{omniav1alpha1.MemoryRetentionModeDecay, []RetentionBranch{BranchDecay}},
		{omniav1alpha1.MemoryRetentionModeComposite, []RetentionBranch{BranchTTL, BranchLRU, BranchDecay}},
	}
	for _, c := range cases {
		got := branchesForMode(c.mode)
		assert.Equal(t, c.want, got, "mode %q", c.mode)
	}
}

func TestTierConfigNilPolicy(t *testing.T) {
	assert.Nil(t, tierConfig(nil, TierInstitutional))
}

func TestTierPredicate(t *testing.T) {
	assert.Contains(t, TierInstitutional.sqlPredicate(), "agent_id IS NULL")
	assert.Contains(t, TierAgent.sqlPredicate(), "agent_id IS NOT NULL")
	assert.Contains(t, TierUser.sqlPredicate(), "virtual_user_id IS NOT NULL")
	var unknown Tier = "???"
	assert.Equal(t, "FALSE", unknown.sqlPredicate())
}

func TestResolveBatchSize(t *testing.T) {
	policy := &omniav1alpha1.MemoryPolicy{}
	assert.Equal(t, defaultRetentionBatchSize, resolveBatchSize(policy))

	b := int32(42)
	policy.Spec.BatchSize = &b
	assert.Equal(t, int32(42), resolveBatchSize(policy))
}

func TestResolveStaleAfter(t *testing.T) {
	// nil config returns (0, nil) so the branch is skipped.
	d, err := resolveStaleAfter(nil)
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), d)

	// Disabled flag returns (0, nil).
	disabled := false
	d, err = resolveStaleAfter(&omniav1alpha1.MemoryLRUConfig{
		Enabled:    &disabled,
		StaleAfter: "30d",
	})
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), d)

	d, err = resolveStaleAfter(&omniav1alpha1.MemoryLRUConfig{StaleAfter: "2h"})
	require.NoError(t, err)
	assert.Equal(t, 2*time.Hour, d)

	_, err = resolveStaleAfter(&omniav1alpha1.MemoryLRUConfig{StaleAfter: "nope"})
	require.Error(t, err)
}

func TestResolveGraceDays(t *testing.T) {
	assert.Equal(t, int32(7), resolveGraceDays(nil))

	g := int32(3)
	cfg := &omniav1alpha1.MemoryConsentRevocationConfig{GraceDays: &g}
	assert.Equal(t, int32(3), resolveGraceDays(cfg))
}
