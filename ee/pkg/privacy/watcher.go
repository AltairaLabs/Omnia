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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// EffectivePolicy contains the computed privacy policy fields relevant to
// the session-api's redaction and opt-out logic.
type EffectivePolicy struct {
	// Recording holds the effective recording config (PII, facadeData, richData).
	Recording omniav1alpha1.RecordingConfig
	// UserOptOut holds the effective user opt-out config.
	UserOptOut *omniav1alpha1.UserOptOutConfig
}

// PolicyWatcher uses a shared informer to watch SessionPrivacyPolicy CRDs
// and maintains an in-memory cache of effective policies keyed by scope.
type PolicyWatcher struct {
	informer cache.SharedIndexInformer
	policies sync.Map // key: string (namespace/name) -> *omniav1alpha1.SessionPrivacyPolicy
	log      logr.Logger
}

// NewPolicyWatcher creates a watcher that observes SessionPrivacyPolicy CRDs.
// It uses the provided REST config to communicate with the K8s API server.
func NewPolicyWatcher(config *rest.Config, log logr.Logger) (*PolicyWatcher, error) {
	scheme := runtime.NewScheme()
	metav1.AddToGroupVersion(scheme, omniav1alpha1.GroupVersion)
	if err := omniav1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("adding EE scheme: %w", err)
	}

	codecs := serializer.NewCodecFactory(scheme)
	cfg := *config
	cfg.APIPath = "/apis"
	cfg.GroupVersion = &omniav1alpha1.GroupVersion
	cfg.NegotiatedSerializer = codecs.WithoutConversion()

	client, err := rest.RESTClientFor(&cfg)
	if err != nil {
		return nil, fmt.Errorf("creating REST client: %w", err)
	}

	listWatcher := &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			result := &omniav1alpha1.SessionPrivacyPolicyList{}
			err := client.Get().
				Resource("sessionprivacypolicies").
				VersionedParams(&opts, runtime.NewParameterCodec(scheme)).
				Do(context.Background()).
				Into(result)
			return result, err
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			return client.Get().
				Resource("sessionprivacypolicies").
				VersionedParams(&opts, runtime.NewParameterCodec(scheme)).
				Watch(context.Background())
		},
	}

	informer := cache.NewSharedIndexInformer(
		listWatcher,
		&omniav1alpha1.SessionPrivacyPolicy{},
		10*time.Minute, // resync period
		cache.Indexers{},
	)

	w := &PolicyWatcher{
		informer: informer,
		log:      log.WithName("policy-watcher"),
	}

	// Register event handlers to maintain the in-memory cache.
	_, _ = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj any) { w.onAdd(obj) },
		UpdateFunc: func(_, newObj any) { w.onAdd(newObj) },
		DeleteFunc: func(obj any) { w.onDelete(obj) },
	})

	return w, nil
}

// Start begins watching for SessionPrivacyPolicy changes. Blocks until ctx is
// cancelled or the informer fails to sync.
func (w *PolicyWatcher) Start(ctx context.Context) error {
	go w.informer.Run(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), w.informer.HasSynced) {
		return fmt.Errorf("policy watcher cache sync failed")
	}
	w.log.Info("policy watcher synced")
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
	return &EffectivePolicy{
		Recording:  spec.Recording,
		UserOptOut: spec.UserOptOut,
	}
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

func (w *PolicyWatcher) onAdd(obj any) {
	p, ok := obj.(*omniav1alpha1.SessionPrivacyPolicy)
	if !ok {
		return
	}
	key := policyKey(p)
	w.policies.Store(key, p.DeepCopy())
	w.log.V(1).Info("policy cached", "name", p.Name, "level", p.Spec.Level)
}

func (w *PolicyWatcher) onDelete(obj any) {
	p, ok := obj.(*omniav1alpha1.SessionPrivacyPolicy)
	if !ok {
		// Handle DeletedFinalStateUnknown
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			return
		}
		p, ok = tombstone.Obj.(*omniav1alpha1.SessionPrivacyPolicy)
		if !ok {
			return
		}
	}
	key := policyKey(p)
	w.policies.Delete(key)
	w.log.V(1).Info("policy removed", "name", p.Name)
}

func policyKey(p *omniav1alpha1.SessionPrivacyPolicy) string {
	return p.Namespace + "/" + p.Name
}
