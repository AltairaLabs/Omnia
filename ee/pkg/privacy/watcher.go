/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// globalPolicyNamespace and globalPolicyName identify the cluster-wide
// default SessionPrivacyPolicy that GetEffectivePolicy falls back to when
// neither an agent override nor a workspace service-group policy applies.
// It lives outside any single workspace's namespace, so loadPolicies fetches
// it explicitly (via Get) in addition to listing the watcher's own namespace
// (#1899).
const (
	globalPolicyNamespace = "omnia-system"
	globalPolicyName      = "default"
)

// EffectivePolicy contains the computed privacy policy fields relevant to
// the session-api's redaction and opt-out logic.
type EffectivePolicy struct {
	// Recording holds the effective recording config (PII, facadeData, runtimeData).
	Recording omniav1alpha1.RecordingConfig
	// UserOptOut holds the effective user opt-out config.
	UserOptOut *omniav1alpha1.UserOptOutConfig
	// Encryption holds the effective encryption config for KMS provider selection.
	Encryption omniav1alpha1.EncryptionConfig
}

// PolicyChangeCallback is invoked when the watcher observes a change to a
// cached SessionPrivacyPolicy. old is nil on first observation; new is nil
// when the policy has been deleted.
type PolicyChangeCallback func(old, new *omniav1alpha1.SessionPrivacyPolicy)

// PolicyWatcher polls SessionPrivacyPolicy, Workspace, and AgentRuntime CRDs
// and maintains an in-memory cache for fast deterministic policy lookup.
//
// Resolution order for GetEffectivePolicy(namespace, agentName):
//  1. AgentRuntime.spec.privacyPolicyRef (agent override)
//  2. Workspace service group's privacyPolicyRef (group default)
//  3. Global default at omnia-system/default
//  4. nil (no policy applies)
//
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=sessionprivacypolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=workspaces,verbs=get
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentruntimes,verbs=get;list;watch
type PolicyWatcher struct {
	client       client.Client
	ownWorkspace string   // name of the Workspace this service instance belongs to
	ownNamespace string   // namespace this service instance's pod runs in (#1899)
	policies     sync.Map // key: "namespace/name" -> *omniav1alpha1.SessionPrivacyPolicy (own ns + global default)
	workspaces   sync.Map // key: name -> *corev1alpha1.Workspace (only ownWorkspace, #1899)
	agents       sync.Map // key: "namespace/name" -> *corev1alpha1.AgentRuntime (only ownNamespace, #1899)
	pollInterval time.Duration
	log          logr.Logger
	onChange     PolicyChangeCallback
}

// OnPolicyChange installs a callback invoked on each policy add/update/delete
// observed by the watcher's reconcile loop. Zero or one callback is supported;
// a later call replaces the previous one.
func (w *PolicyWatcher) OnPolicyChange(cb PolicyChangeCallback) {
	w.onChange = cb
}

// NewPolicyWatcher creates a watcher that observes privacy-related CRDs
// using a controller-runtime client. ownWorkspaceName is the name of the
// Workspace this service instance belongs to, and ownNamespace is the
// namespace its own pod runs in (memory-api/session-api are deployed
// one-per-workspace, #1899). The watcher only ever reads that one Workspace
// and lists AgentRuntime/SessionPrivacyPolicy within that one namespace —
// plus the cluster-wide default SessionPrivacyPolicy at
// omnia-system/default, which it Gets explicitly — instead of listing
// cluster-wide.
func NewPolicyWatcher(k8sClient client.Client, log logr.Logger, ownWorkspaceName, ownNamespace string) *PolicyWatcher {
	return &PolicyWatcher{
		client:       k8sClient,
		ownWorkspace: ownWorkspaceName,
		ownNamespace: ownNamespace,
		pollInterval: 30 * time.Second,
		log:          log.WithName("policy-watcher"),
	}
}

// SetPollInterval overrides the default poll interval (for testing).
func (w *PolicyWatcher) SetPollInterval(d time.Duration) {
	w.pollInterval = d
}

// Start begins watching for changes. It performs an initial load, then polls
// periodically. Blocks until ctx is cancelled.
func (w *PolicyWatcher) Start(ctx context.Context) error {
	if err := w.loadAll(ctx); err != nil {
		return fmt.Errorf("initial policy load failed: %w", err)
	}
	w.log.Info("policy watcher synced")

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := w.loadAll(ctx); err != nil {
				w.log.Error(err, "policy reload failed")
			}
		}
	}
}

func (w *PolicyWatcher) loadAll(ctx context.Context) error {
	if err := w.loadPolicies(ctx); err != nil {
		return err
	}
	if err := w.loadWorkspaces(ctx); err != nil {
		return err
	}
	if err := w.loadAgentRuntimes(ctx); err != nil {
		return err
	}
	return nil
}

// loadPolicies lists SessionPrivacyPolicy resources in the watcher's own
// namespace and additionally Gets the cluster-wide default policy at
// omnia-system/default — the resolution chain's final fallback (#1899) — and
// merges both into the cache. If ownNamespace is unset, the own-namespace
// list is skipped (empty cache for that part), matching loadWorkspaces'
// own-Workspace guard; the global default is still fetched regardless.
func (w *PolicyWatcher) loadPolicies(ctx context.Context) error {
	seen := make(map[string]bool)

	if w.ownNamespace != "" {
		var list omniav1alpha1.SessionPrivacyPolicyList
		if err := w.client.List(ctx, &list, client.InNamespace(w.ownNamespace)); err != nil {
			return fmt.Errorf("listing policies: %w", err)
		}
		for i := range list.Items {
			w.cachePolicy(&list.Items[i], seen)
		}
	}

	var globalDefault omniav1alpha1.SessionPrivacyPolicy
	key := client.ObjectKey{Namespace: globalPolicyNamespace, Name: globalPolicyName}
	switch err := w.client.Get(ctx, key, &globalDefault); {
	case apierrors.IsNotFound(err):
		// no cluster-wide default configured — nothing to cache, not an error
	case err != nil:
		return fmt.Errorf("get global default policy %s/%s: %w", globalPolicyNamespace, globalPolicyName, err)
	default:
		w.cachePolicy(&globalDefault, seen)
	}

	w.policies.Range(func(k, v any) bool {
		if !seen[k.(string)] {
			w.policies.Delete(k)
			w.log.V(2).Info("policy evicted", "key", k)
			if w.onChange != nil {
				oldPolicy := v.(*omniav1alpha1.SessionPrivacyPolicy)
				w.onChange(oldPolicy, nil)
			}
		}
		return true
	})
	return nil
}

// cachePolicy stores p in the cache keyed by "namespace/name", marks that
// key seen, and invokes the onChange callback on first observation or a
// genuine spec change (never on a no-op reload).
func (w *PolicyWatcher) cachePolicy(p *omniav1alpha1.SessionPrivacyPolicy, seen map[string]bool) {
	key := policyKey(p)
	seen[key] = true
	newVal := p.DeepCopy()

	// Capture old value before storing, then invoke callback outside any lock.
	var oldPolicy *omniav1alpha1.SessionPrivacyPolicy
	if prev, ok := w.policies.Load(key); ok {
		oldPolicy = prev.(*omniav1alpha1.SessionPrivacyPolicy)
	}
	w.policies.Store(key, newVal)
	w.log.V(2).Info("policy cached", "key", key)
	if w.onChange != nil && (oldPolicy == nil || !reflect.DeepEqual(oldPolicy.Spec, newVal.Spec)) {
		w.onChange(oldPolicy, newVal)
	}
}

// loadWorkspaces Gets the watcher's OWN Workspace (memory-api/session-api are
// per-workspace) and caches just that one, instead of listing every Workspace
// to reverse-map namespaces client-side (#1899). resolveServiceGroupPolicy
// ranges this now-single-entry cache unchanged.
func (w *PolicyWatcher) loadWorkspaces(ctx context.Context) error {
	seen := make(map[string]bool, 1)
	if w.ownWorkspace != "" {
		var ws corev1alpha1.Workspace
		err := w.client.Get(ctx, client.ObjectKey{Name: w.ownWorkspace}, &ws)
		switch {
		case apierrors.IsNotFound(err):
			// own Workspace absent (transient at boot) — cache nothing this pass
		case err != nil:
			return fmt.Errorf("get own workspace %q: %w", w.ownWorkspace, err)
		default:
			seen[ws.Name] = true
			w.workspaces.Store(ws.Name, ws.DeepCopy())
			w.log.V(2).Info("workspace cached", "name", ws.Name)
		}
	}
	w.workspaces.Range(func(k, _ any) bool {
		if !seen[k.(string)] {
			w.workspaces.Delete(k)
		}
		return true
	})
	return nil
}

// loadAgentRuntimes lists AgentRuntime resources in the watcher's own
// namespace and updates the cache (#1899). If ownNamespace is unset, the
// list is skipped and the cache is left empty, matching loadWorkspaces'
// own-Workspace guard.
func (w *PolicyWatcher) loadAgentRuntimes(ctx context.Context) error {
	seen := make(map[string]bool)

	if w.ownNamespace != "" {
		var list corev1alpha1.AgentRuntimeList
		if err := w.client.List(ctx, &list, client.InNamespace(w.ownNamespace)); err != nil {
			return fmt.Errorf("listing agentruntimes: %w", err)
		}
		for i := range list.Items {
			ar := &list.Items[i]
			key := ar.Namespace + "/" + ar.Name
			seen[key] = true
			w.agents.Store(key, ar.DeepCopy())
			w.log.V(2).Info("agentruntime cached", "key", key)
		}
	}
	w.agents.Range(func(k, _ any) bool {
		if !seen[k.(string)] {
			w.agents.Delete(k)
		}
		return true
	})
	return nil
}

// GetEffectivePolicy resolves the privacy policy for the given namespace/agent
// using deterministic lookup (no merge semantics):
//  1. AgentRuntime override (spec.privacyPolicyRef)
//  2. Workspace service group (matched by namespace and agent's serviceGroup)
//  3. Global default at omnia-system/default
//  4. nil
func (w *PolicyWatcher) GetEffectivePolicy(namespace, agentName string) *EffectivePolicy {
	// 1. Agent-level override
	if namespace != "" && agentName != "" {
		agentKey := namespace + "/" + agentName
		if val, ok := w.agents.Load(agentKey); ok {
			ar := val.(*corev1alpha1.AgentRuntime)
			if ar.Spec.PrivacyPolicyRef != nil {
				if p := w.lookupPolicy(namespace, ar.Spec.PrivacyPolicyRef.Name); p != nil {
					return w.toEffective(p)
				}
			}
		}
	}

	// 2. Workspace service group
	if namespace != "" {
		if p := w.resolveServiceGroupPolicy(namespace, agentName); p != nil {
			return w.toEffective(p)
		}
	}

	// 3. Global default
	if p := w.lookupPolicy(globalPolicyNamespace, globalPolicyName); p != nil {
		return w.toEffective(p)
	}

	return nil
}

// resolveServiceGroupPolicy finds the policy bound to the agent's service group
// within the workspace that owns the given namespace.
func (w *PolicyWatcher) resolveServiceGroupPolicy(namespace, agentName string) *omniav1alpha1.SessionPrivacyPolicy {
	// Find the agent's service group name (defaults to "default").
	serviceGroupName := "default"
	if agentName != "" {
		agentKey := namespace + "/" + agentName
		if val, ok := w.agents.Load(agentKey); ok {
			if ar := val.(*corev1alpha1.AgentRuntime); ar.Spec.ServiceGroup != "" {
				serviceGroupName = ar.Spec.ServiceGroup
			}
		}
	}

	// Find the workspace whose Spec.Namespace.Name matches the agent's namespace.
	var found *omniav1alpha1.SessionPrivacyPolicy
	w.workspaces.Range(func(_, val any) bool {
		ws := val.(*corev1alpha1.Workspace)
		if ws.Spec.Namespace.Name != namespace {
			return true // continue
		}
		for _, sg := range ws.Spec.Services {
			if sg.Name == serviceGroupName && sg.PrivacyPolicyRef != nil {
				found = w.lookupPolicy(namespace, sg.PrivacyPolicyRef.Name)
				return false // stop
			}
		}
		return true
	})
	return found
}

// lookupPolicy retrieves a policy from the cache by namespace/name.
func (w *PolicyWatcher) lookupPolicy(namespace, name string) *omniav1alpha1.SessionPrivacyPolicy {
	if val, ok := w.policies.Load(namespace + "/" + name); ok {
		return val.(*omniav1alpha1.SessionPrivacyPolicy)
	}
	return nil
}

// toEffective converts a SessionPrivacyPolicy to an EffectivePolicy.
func (w *PolicyWatcher) toEffective(p *omniav1alpha1.SessionPrivacyPolicy) *EffectivePolicy {
	eff := &EffectivePolicy{
		Recording:  p.Spec.Recording,
		UserOptOut: p.Spec.UserOptOut,
	}
	if p.Spec.Encryption != nil {
		eff.Encryption = *p.Spec.Encryption
	}
	return eff
}

func policyKey(p *omniav1alpha1.SessionPrivacyPolicy) string {
	return p.Namespace + "/" + p.Name
}
