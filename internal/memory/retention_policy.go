/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"fmt"
	"sort"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// defaultPolicyName is the well-known name the worker looks for first
// when picking which MemoryRetentionPolicy to apply. Matches the
// sample in config/samples/omnia_v1alpha1_memoryretentionpolicy.yaml.
const defaultPolicyName = "default"

// PolicyLoader fetches the active MemoryRetentionPolicy. Implementations
// may cache; callers must treat the returned policy as read-only.
type PolicyLoader interface {
	Load(ctx context.Context) (*omniav1alpha1.MemoryRetentionPolicy, error)
}

// StaticPolicyLoader wraps a literal policy. Used by unit tests and as
// a fallback when the memory-api runs outside Kubernetes.
type StaticPolicyLoader struct {
	Policy *omniav1alpha1.MemoryRetentionPolicy
}

// Load returns the stored policy unchanged.
func (s *StaticPolicyLoader) Load(_ context.Context) (*omniav1alpha1.MemoryRetentionPolicy, error) {
	return s.Policy, nil
}

// K8sPolicyLoader reads MemoryRetentionPolicy resources from the
// control plane. Caches the last-known policy so a transient API
// outage doesn't stall the retention cron.
type K8sPolicyLoader struct {
	Client client.Client
	Log    logr.Logger

	cached atomic.Pointer[omniav1alpha1.MemoryRetentionPolicy]
}

// NewK8sPolicyLoader wires a client-backed loader.
func NewK8sPolicyLoader(c client.Client, log logr.Logger) *K8sPolicyLoader {
	return &K8sPolicyLoader{Client: c, Log: log}
}

// Load lists MemoryRetentionPolicies cluster-wide and returns the
// policy named "default" if present, otherwise the lexicographically
// first active one. Falls back to the last-known-good policy when the
// API call fails.
func (k *K8sPolicyLoader) Load(ctx context.Context) (*omniav1alpha1.MemoryRetentionPolicy, error) {
	list := &omniav1alpha1.MemoryRetentionPolicyList{}
	if err := k.Client.List(ctx, list); err != nil {
		if cached := k.cached.Load(); cached != nil {
			k.Log.V(1).Info("policy list failed, using cached policy",
				"cachedName", cached.Name, "error", err.Error())
			return cached, nil
		}
		return nil, fmt.Errorf("list MemoryRetentionPolicy: %w", err)
	}

	active := make([]omniav1alpha1.MemoryRetentionPolicy, 0, len(list.Items))
	for i := range list.Items {
		if list.Items[i].DeletionTimestamp.IsZero() {
			active = append(active, list.Items[i])
		}
	}
	if len(active) == 0 {
		k.cached.Store(nil)
		return nil, nil
	}

	sort.Slice(active, func(i, j int) bool {
		if active[i].Name == defaultPolicyName {
			return true
		}
		if active[j].Name == defaultPolicyName {
			return false
		}
		return active[i].Name < active[j].Name
	})
	chosen := active[0].DeepCopy()
	k.cached.Store(chosen)
	return chosen, nil
}

// LegacyIntervalPolicy builds a minimal policy from the legacy
// RETENTION_INTERVAL env var so existing deployments keep working
// until operators install a MemoryRetentionPolicy CRD. The legacy
// worker deleted expired rows cluster-wide; this policy reproduces
// that by applying TTL across all three tiers with no LRU or Decay.
//
// Interval is mapped to "@every Xs" because cron's standard 5-field
// expressions can't express sub-day schedules like "every 10m".
func LegacyIntervalPolicy(interval time.Duration) *omniav1alpha1.MemoryRetentionPolicy {
	schedule := fmt.Sprintf("@every %s", interval.String())
	mode := omniav1alpha1.MemoryRetentionModeTTL
	return &omniav1alpha1.MemoryRetentionPolicy{
		Spec: omniav1alpha1.MemoryRetentionPolicySpec{
			Default: omniav1alpha1.MemoryRetentionDefaults{
				Tiers: omniav1alpha1.MemoryRetentionTierSet{
					Institutional: &omniav1alpha1.MemoryTierConfig{Mode: mode},
					Agent:         &omniav1alpha1.MemoryTierConfig{Mode: mode},
					User:          &omniav1alpha1.MemoryTierConfig{Mode: mode},
				},
				Schedule: schedule,
			},
		},
	}
}
