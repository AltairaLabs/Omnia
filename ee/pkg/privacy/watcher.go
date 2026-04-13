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
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// EffectivePolicy contains the computed privacy policy fields relevant to
// the session-api's redaction and opt-out logic.
type EffectivePolicy struct {
	// Recording holds the effective recording config (PII, facadeData, richData).
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
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=workspaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentruntimes,verbs=get;list;watch
type PolicyWatcher struct {
	client       client.Client
	policies     sync.Map // key: "namespace/name" -> *omniav1alpha1.SessionPrivacyPolicy
	workspaces   sync.Map // key: name -> *corev1alpha1.Workspace
	agents       sync.Map // key: "namespace/name" -> *corev1alpha1.AgentRuntime
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
// using a controller-runtime client.
func NewPolicyWatcher(k8sClient client.Client, log logr.Logger) *PolicyWatcher {
	return &PolicyWatcher{
		client:       k8sClient,
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

// loadPolicies lists all SessionPrivacyPolicy resources and updates the cache.
func (w *PolicyWatcher) loadPolicies(ctx context.Context) error {
	var list omniav1alpha1.SessionPrivacyPolicyList
	if err := w.client.List(ctx, &list); err != nil {
		return fmt.Errorf("listing policies: %w", err)
	}

	seen := make(map[string]bool, len(list.Items))
	for i := range list.Items {
		p := &list.Items[i]
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
		// Only notify on a genuine spec change to avoid churning downstream
		// caches on every 30s no-op reload.
		if w.onChange != nil && (oldPolicy == nil || !reflect.DeepEqual(oldPolicy.Spec, newVal.Spec)) {
			w.onChange(oldPolicy, newVal)
		}
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

// loadWorkspaces lists all Workspace resources and updates the cache.
func (w *PolicyWatcher) loadWorkspaces(ctx context.Context) error {
	var list corev1alpha1.WorkspaceList
	if err := w.client.List(ctx, &list); err != nil {
		return fmt.Errorf("listing workspaces: %w", err)
	}

	seen := make(map[string]bool, len(list.Items))
	for i := range list.Items {
		ws := &list.Items[i]
		seen[ws.Name] = true
		w.workspaces.Store(ws.Name, ws.DeepCopy())
		w.log.V(2).Info("workspace cached", "name", ws.Name)
	}
	w.workspaces.Range(func(k, _ any) bool {
		if !seen[k.(string)] {
			w.workspaces.Delete(k)
		}
		return true
	})
	return nil
}

// loadAgentRuntimes lists all AgentRuntime resources and updates the cache.
func (w *PolicyWatcher) loadAgentRuntimes(ctx context.Context) error {
	var list corev1alpha1.AgentRuntimeList
	if err := w.client.List(ctx, &list); err != nil {
		return fmt.Errorf("listing agentruntimes: %w", err)
	}

	seen := make(map[string]bool, len(list.Items))
	for i := range list.Items {
		ar := &list.Items[i]
		key := ar.Namespace + "/" + ar.Name
		seen[key] = true
		w.agents.Store(key, ar.DeepCopy())
		w.log.V(2).Info("agentruntime cached", "key", key)
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
	if p := w.lookupPolicy("omnia-system", "default"); p != nil {
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
