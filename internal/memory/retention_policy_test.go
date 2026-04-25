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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func policySchemeForTest(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(s))
	return s
}

func TestK8sPolicyLoader_PrefersDefaultName(t *testing.T) {
	scheme := policySchemeForTest(t)
	other := &omniav1alpha1.MemoryPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "alpha"},
		Spec: omniav1alpha1.MemoryPolicySpec{
			Default: omniav1alpha1.MemoryRetentionDefaults{Schedule: "0 1 * * *"},
		},
	}
	def := &omniav1alpha1.MemoryPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
		Spec: omniav1alpha1.MemoryPolicySpec{
			Default: omniav1alpha1.MemoryRetentionDefaults{Schedule: "0 3 * * *"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(other, def).Build()
	loader := NewK8sPolicyLoader(c, zap.New(zap.UseDevMode(true)))

	got, err := loader.Load(context.Background())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "default", got.Name)
	assert.Equal(t, "0 3 * * *", got.Spec.Default.Schedule)
}

func TestK8sPolicyLoader_FallsBackToLexFirst(t *testing.T) {
	scheme := policySchemeForTest(t)
	alpha := &omniav1alpha1.MemoryPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "alpha"},
	}
	beta := &omniav1alpha1.MemoryPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "beta"},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(alpha, beta).Build()
	loader := NewK8sPolicyLoader(c, zap.New(zap.UseDevMode(true)))

	got, err := loader.Load(context.Background())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "alpha", got.Name)
}

func TestK8sPolicyLoader_ReturnsNilWhenNonePresent(t *testing.T) {
	scheme := policySchemeForTest(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	loader := NewK8sPolicyLoader(c, zap.New(zap.UseDevMode(true)))

	got, err := loader.Load(context.Background())
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestK8sPolicyLoader_SkipsDeletedPolicies(t *testing.T) {
	scheme := policySchemeForTest(t)
	deleted := &omniav1alpha1.MemoryPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "default",
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
			Finalizers:        []string{"hold"},
		},
	}
	live := &omniav1alpha1.MemoryPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "live"},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deleted, live).Build()
	loader := NewK8sPolicyLoader(c, zap.New(zap.UseDevMode(true)))

	got, err := loader.Load(context.Background())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "live", got.Name)
}

func TestK8sPolicyLoader_ReturnsCachedOnListError(t *testing.T) {
	scheme := policySchemeForTest(t)
	initial := &omniav1alpha1.MemoryPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
		Spec: omniav1alpha1.MemoryPolicySpec{
			Default: omniav1alpha1.MemoryRetentionDefaults{Schedule: "0 3 * * *"},
		},
	}
	var callCount int
	errListAfterFirst := errors.New("simulated API outage")
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(initial).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, cl client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				callCount++
				if callCount == 1 {
					return cl.List(ctx, list, opts...)
				}
				return errListAfterFirst
			},
		}).
		Build()

	loader := NewK8sPolicyLoader(c, zap.New(zap.UseDevMode(true)))

	first, err := loader.Load(context.Background())
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, "default", first.Name)

	second, err := loader.Load(context.Background())
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, "default", second.Name)
}

func TestK8sPolicyLoader_ReturnsErrorWhenListFailsWithoutCache(t *testing.T) {
	scheme := policySchemeForTest(t)
	simulated := errors.New("simulated API outage")
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
				return simulated
			},
		}).
		Build()

	loader := NewK8sPolicyLoader(c, zap.New(zap.UseDevMode(true)))
	_, err := loader.Load(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated API outage")
}

func TestLegacyIntervalPolicy_ShapesAllTiersTTL(t *testing.T) {
	p := LegacyIntervalPolicy(15 * time.Minute)
	require.NotNil(t, p)
	require.NotNil(t, p.Spec.Default.Tiers.Institutional)
	require.NotNil(t, p.Spec.Default.Tiers.Agent)
	require.NotNil(t, p.Spec.Default.Tiers.User)
	assert.Equal(t, omniav1alpha1.MemoryRetentionModeTTL,
		p.Spec.Default.Tiers.Institutional.Mode)
	assert.Equal(t, "@every 15m0s", p.Spec.Default.Schedule)
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
	policy.Spec.Default.BatchSize = &b
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
