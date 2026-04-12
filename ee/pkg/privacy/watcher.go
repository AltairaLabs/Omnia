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
	"sync"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

// PolicyWatcher polls SessionPrivacyPolicy CRDs and maintains an in-memory
// cache of policies for fast lookup by the privacy middleware.
type PolicyWatcher struct {
	client       client.Client
	policies     sync.Map // key: string (namespace/name) -> *omniav1alpha1.SessionPrivacyPolicy
	pollInterval time.Duration
	log          logr.Logger
}

// NewPolicyWatcher creates a watcher that observes SessionPrivacyPolicy CRDs
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

// Start begins watching for SessionPrivacyPolicy changes. It performs an
// initial list, then polls periodically. Blocks until ctx is cancelled.
func (w *PolicyWatcher) Start(ctx context.Context) error {
	if err := w.loadPolicies(ctx); err != nil {
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
			if err := w.loadPolicies(ctx); err != nil {
				w.log.Error(err, "policy reload failed")
			}
		}
	}
}

// loadPolicies lists all SessionPrivacyPolicy resources and updates the cache.
func (w *PolicyWatcher) loadPolicies(ctx context.Context) error {
	var list omniav1alpha1.SessionPrivacyPolicyList
	if err := w.client.List(ctx, &list); err != nil {
		return fmt.Errorf("listing policies: %w", err)
	}

	// Track which keys are still present.
	seen := make(map[string]bool, len(list.Items))

	for i := range list.Items {
		p := &list.Items[i]
		key := policyKey(p)
		seen[key] = true
		w.policies.Store(key, p.DeepCopy())
		w.log.V(1).Info("policy cached", "name", p.Name, "level", p.Spec.Level)
	}

	// Remove policies no longer present.
	w.policies.Range(func(k, _ any) bool {
		if !seen[k.(string)] {
			w.policies.Delete(k)
			w.log.V(1).Info("policy removed", "key", k)
		}
		return true
	})

	return nil
}

// GetEffectivePolicy computes the merged policy for the given namespace/agent
// by walking the inheritance chain: agent → workspace → global.
func (w *PolicyWatcher) GetEffectivePolicy(namespace, agentName string) *EffectivePolicy {
	allPolicies := w.collectPolicies()
	chain := buildPolicyChain(allPolicies, namespace, agentName)

	if len(chain) == 0 {
		return nil
	}

	spec := ComputeEffectivePolicy(chain)
	eff := &EffectivePolicy{
		Recording:  spec.Recording,
		UserOptOut: spec.UserOptOut,
	}
	if spec.Encryption != nil {
		eff.Encryption = *spec.Encryption
	}
	return eff
}

// collectPolicies returns all cached policies as a slice.
func (w *PolicyWatcher) collectPolicies() []*omniav1alpha1.SessionPrivacyPolicy {
	var result []*omniav1alpha1.SessionPrivacyPolicy
	w.policies.Range(func(_, value any) bool {
		if p, ok := value.(*omniav1alpha1.SessionPrivacyPolicy); ok {
			result = append(result, p)
		}
		return true
	})
	return result
}

// buildPolicyChain constructs the inheritance chain from global → workspace → agent.
func buildPolicyChain(
	policies []*omniav1alpha1.SessionPrivacyPolicy,
	namespace, agentName string,
) []*omniav1alpha1.SessionPrivacyPolicy {
	var chain []*omniav1alpha1.SessionPrivacyPolicy

	if p := findByLevel(policies, omniav1alpha1.PolicyLevelGlobal, "", ""); p != nil {
		chain = append(chain, p)
	}
	if namespace != "" {
		if p := findByLevel(policies, omniav1alpha1.PolicyLevelWorkspace, namespace, ""); p != nil {
			chain = append(chain, p)
		}
	}
	if agentName != "" && namespace != "" {
		if p := findByLevel(policies, omniav1alpha1.PolicyLevelAgent, namespace, agentName); p != nil {
			chain = append(chain, p)
		}
	}
	return chain
}

// findByLevel locates the first policy matching level and optional scope.
func findByLevel(
	policies []*omniav1alpha1.SessionPrivacyPolicy,
	level omniav1alpha1.PolicyLevel,
	namespace, agentName string,
) *omniav1alpha1.SessionPrivacyPolicy {
	for _, p := range policies {
		if p.Spec.Level != level {
			continue
		}
		switch level {
		case omniav1alpha1.PolicyLevelWorkspace:
			if p.Spec.WorkspaceRef != nil && p.Spec.WorkspaceRef.Name == namespace {
				return p
			}
		case omniav1alpha1.PolicyLevelAgent:
			if p.Spec.AgentRef != nil &&
				p.Spec.AgentRef.Name == agentName &&
				p.Spec.AgentRef.Namespace == namespace {
				return p
			}
		default:
			return p
		}
	}
	return nil
}

func policyKey(p *omniav1alpha1.SessionPrivacyPolicy) string {
	return p.Namespace + "/" + p.Name
}
