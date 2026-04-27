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

// DefaultPolicyCacheTTL is the freshness window after which the
// K8sPolicyLoader re-fetches from the API server. 30 seconds is
// short enough that operator changes propagate quickly and long
// enough that high-throughput Save calls don't burn the K8s API.
const DefaultPolicyCacheTTL = 30 * time.Second

// K8sPolicyLoader resolves a MemoryPolicy via the Workspace that owns
// this memory-api process. The workspace's services[<group>].memory
// .policyRef names the policy; the loader Gets it by name.
//
// Caches the last-known policy with a TTL (DefaultPolicyCacheTTL by
// default). Within the freshness window Load returns the cached value
// without hitting the K8s API — important because Load runs on the
// hot Save path (enforceAboutForKind) and was previously doing two
// API GETs per write.
//
// Outside the TTL Load re-fetches; on transient error it falls back
// to the cached value (so an API outage doesn't stall writes), and
// on "not found" it clears the cache so callers see the legacy
// fallback.
type K8sPolicyLoader struct {
	Client       client.Client
	Log          logr.Logger
	Workspace    string
	ServiceGroup string
	// CacheTTL is the freshness window. Zero uses
	// DefaultPolicyCacheTTL.
	CacheTTL time.Duration

	// cache packs (policy, fetchedAt, noopFlag) into one atomic
	// pointer so concurrent Loads always see a consistent triple.
	// Earlier separate atomic.Pointer + atomic.Int64 + atomic.Bool
	// fields could tear: a successful fetch racing with a markNoop
	// call could leave the cache reading "no policy at fresh
	// timestamp" when one was actually bound, defeating the cache
	// for up to one TTL window.
	cache atomic.Pointer[policyCacheEntry]
}

// policyCacheEntry is the atomic unit. Policy is nil when the most
// recent successful fetch resolved to "no policy bound" (workspace
// missing, no policyRef, named policy not found) — distinguished
// from "cache empty" by FetchedAt being non-zero.
type policyCacheEntry struct {
	Policy    *omniav1alpha1.MemoryPolicy
	FetchedAt time.Time
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

	ttl := k.CacheTTL
	if ttl <= 0 {
		ttl = DefaultPolicyCacheTTL
	}
	if entry := k.cache.Load(); entry != nil && time.Since(entry.FetchedAt) < ttl {
		return entry.Policy, nil
	}

	ws := &omniav1alpha1.Workspace{}
	if err := k.Client.Get(ctx, client.ObjectKey{Name: k.Workspace}, ws); err != nil {
		if apierrors.IsNotFound(err) {
			k.Log.V(1).Info("workspace not found, using legacy policy", "workspace", k.Workspace)
			k.markNoop()
			return nil, nil
		}
		if cached := k.cachedPolicy(); cached != nil {
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
		k.markNoop()
		return nil, nil
	}

	policy := &omniav1alpha1.MemoryPolicy{}
	if err := k.Client.Get(ctx, client.ObjectKey{Name: ref.Name}, policy); err != nil {
		if apierrors.IsNotFound(err) {
			k.Log.Info("memorypolicy not found, using legacy",
				"policyRef", ref.Name, "workspace", k.Workspace)
			k.markNoop()
			return nil, nil
		}
		if cached := k.cachedPolicy(); cached != nil {
			k.Log.V(1).Info("memorypolicy get failed, using cached policy",
				"policyRef", ref.Name, "cachedName", cached.Name, "error", err.Error())
			return cached, nil
		}
		return nil, fmt.Errorf("get memorypolicy %q: %w", ref.Name, err)
	}

	k.cache.Store(&policyCacheEntry{Policy: policy, FetchedAt: time.Now()})
	return policy, nil
}

// cachedPolicy returns the last cached policy (or nil if cache is
// empty / the last fetch resolved to "no policy"). Used by the
// transient-error fallback paths to keep serving stale-but-known
// content during an API outage.
func (k *K8sPolicyLoader) cachedPolicy() *omniav1alpha1.MemoryPolicy {
	if entry := k.cache.Load(); entry != nil {
		return entry.Policy
	}
	return nil
}

// markNoop records that the most recent successful fetch resolved
// to "no policy" so subsequent Loads within the TTL skip the K8s
// round trip. Stored as one atomic write to avoid the read-tear
// window the previous separate-fields implementation had.
func (k *K8sPolicyLoader) markNoop() {
	k.cache.Store(&policyCacheEntry{Policy: nil, FetchedAt: time.Now()})
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
