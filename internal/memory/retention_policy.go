/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// PolicyLoader fetches the active MemoryPolicy. Implementations
// may cache; callers must treat the returned policy as read-only.
type PolicyLoader interface {
	Load(ctx context.Context) (*omniav1alpha1.MemoryPolicy, error)
}

// StaticPolicyLoader wraps a literal policy. Used by unit tests and as
// a fallback when the memory-api runs outside Kubernetes.
type StaticPolicyLoader struct {
	Policy *omniav1alpha1.MemoryPolicy
}

// Load returns the stored policy unchanged.
func (s *StaticPolicyLoader) Load(_ context.Context) (*omniav1alpha1.MemoryPolicy, error) {
	return s.Policy, nil
}

// K8sPolicyLoader resolves a MemoryPolicy via the Workspace that owns
// this memory-api process. The workspace's services[<group>].memory
// .policyRef names the policy; the loader Gets it by name.
//
// Caches the last-known policy so a transient API outage doesn't stall
// the retention cron. Returns (nil, nil) for any "not found" condition
// so callers fall back to the baked-in LegacyIntervalPolicy.
type K8sPolicyLoader struct {
	Client       client.Client
	Log          logr.Logger
	Workspace    string
	ServiceGroup string

	cached atomic.Pointer[omniav1alpha1.MemoryPolicy]
}

// NewK8sPolicyLoader constructs a loader bound to a single workspace +
// service group. workspace and serviceGroup must match the values the
// memory-api binary was started with — they're used to resolve the
// Workspace CRD and pick the right service-group entry.
func NewK8sPolicyLoader(c client.Client, log logr.Logger, workspace, serviceGroup string) *K8sPolicyLoader {
	return &K8sPolicyLoader{
		Client:       c,
		Log:          log,
		Workspace:    workspace,
		ServiceGroup: serviceGroup,
	}
}

// Load resolves the active MemoryPolicy. Returns (nil, nil) when the
// workspace, the service group, the policyRef, or the named policy is
// missing — callers treat nil as "use legacy fallback".
func (k *K8sPolicyLoader) Load(ctx context.Context) (*omniav1alpha1.MemoryPolicy, error) {
	if k.Workspace == "" {
		// No workspace context (e.g. local dev outside K8s) — bypass.
		return nil, nil
	}

	ws := &omniav1alpha1.Workspace{}
	if err := k.Client.Get(ctx, client.ObjectKey{Name: k.Workspace}, ws); err != nil {
		if apierrors.IsNotFound(err) {
			k.Log.V(1).Info("workspace not found, using legacy policy", "workspace", k.Workspace)
			return nil, nil
		}
		if cached := k.cached.Load(); cached != nil {
			k.Log.V(1).Info("workspace get failed, using cached policy",
				"workspace", k.Workspace, "cachedName", cached.Name, "error", err.Error())
			return cached, nil
		}
		return nil, fmt.Errorf("get workspace %q: %w", k.Workspace, err)
	}

	ref := findMemoryPolicyRef(ws, k.ServiceGroup)
	if ref == nil {
		k.Log.V(1).Info("workspace has no memory policyRef, using legacy",
			"workspace", k.Workspace, "serviceGroup", k.ServiceGroup)
		k.cached.Store(nil)
		return nil, nil
	}

	policy := &omniav1alpha1.MemoryPolicy{}
	if err := k.Client.Get(ctx, client.ObjectKey{Name: ref.Name}, policy); err != nil {
		if apierrors.IsNotFound(err) {
			k.Log.Info("memorypolicy not found, using legacy",
				"policyRef", ref.Name, "workspace", k.Workspace)
			k.cached.Store(nil)
			return nil, nil
		}
		if cached := k.cached.Load(); cached != nil {
			k.Log.V(1).Info("memorypolicy get failed, using cached policy",
				"policyRef", ref.Name, "cachedName", cached.Name, "error", err.Error())
			return cached, nil
		}
		return nil, fmt.Errorf("get memorypolicy %q: %w", ref.Name, err)
	}

	k.cached.Store(policy)
	return policy, nil
}

// findMemoryPolicyRef walks the workspace's service groups and returns
// the matching group's memory.policyRef (or nil when no match / no ref).
func findMemoryPolicyRef(ws *omniav1alpha1.Workspace, group string) *corev1.LocalObjectReference {
	for i := range ws.Spec.Services {
		svc := &ws.Spec.Services[i]
		if svc.Name != group {
			continue
		}
		if svc.Memory == nil || svc.Memory.PolicyRef == nil {
			return nil
		}
		return svc.Memory.PolicyRef
	}
	return nil
}

// LegacyIntervalPolicy builds a minimal policy from the legacy
// RETENTION_INTERVAL env var so existing deployments keep working
// until operators install a MemoryPolicy CRD. The legacy
// worker deleted expired rows cluster-wide; this policy reproduces
// that by applying TTL across all three tiers with no LRU or Decay.
//
// Interval is mapped to "@every Xs" because cron's standard 5-field
// expressions can't express sub-day schedules like "every 10m".
func LegacyIntervalPolicy(interval time.Duration) *omniav1alpha1.MemoryPolicy {
	schedule := fmt.Sprintf("@every %s", interval.String())
	mode := omniav1alpha1.MemoryRetentionModeTTL
	return &omniav1alpha1.MemoryPolicy{
		Spec: omniav1alpha1.MemoryPolicySpec{
			Tiers: omniav1alpha1.MemoryRetentionTierSet{
				Institutional: &omniav1alpha1.MemoryTierConfig{Mode: mode},
				Agent:         &omniav1alpha1.MemoryTierConfig{Mode: mode},
				User:          &omniav1alpha1.MemoryTierConfig{Mode: mode},
			},
			Schedule: schedule,
		},
	}
}
