/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package policy

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// Watcher watches ToolPolicy CRDs and keeps the Evaluator up to date.
type Watcher struct {
	evaluator *Evaluator
	client    client.Client
	logger    logr.Logger
	namespace string
	scheme    *runtime.Scheme

	// metrics is optional (nil-safe): when set, the active_policies gauge is
	// refreshed from the evaluator's compiled-policy count on every load
	// (initial load and each poll cycle), so it self-corrects on reload.
	metrics *Metrics
}

// NewWatcher creates a new ToolPolicy watcher.
func NewWatcher(
	evaluator *Evaluator,
	k8sClient client.Client,
	scheme *runtime.Scheme,
	namespace string,
	logger logr.Logger,
) *Watcher {
	return &Watcher{
		evaluator: evaluator,
		client:    k8sClient,
		scheme:    scheme,
		namespace: namespace,
		logger:    logger,
	}
}

// SetMetrics attaches Prometheus metrics to the watcher. Nil-safe: when never
// called, initialLoad skips updating the active_policies gauge.
func (w *Watcher) SetMetrics(metrics *Metrics) {
	w.metrics = metrics
}

// Start begins watching ToolPolicy resources and blocks until the context is cancelled.
func (w *Watcher) Start(ctx context.Context) error {
	if err := w.initialLoad(ctx); err != nil {
		return fmt.Errorf("initial ToolPolicy load failed: %w", err)
	}
	return w.pollLoop(ctx)
}

// initialLoad lists all ToolPolicy resources and compiles them.
func (w *Watcher) initialLoad(ctx context.Context) error {
	var list omniav1alpha1.ToolPolicyList
	opts := w.listOptions()
	if err := w.client.List(ctx, &list, opts...); err != nil {
		return fmt.Errorf("failed to list ToolPolicies: %w", err)
	}

	for i := range list.Items {
		policy := &list.Items[i]
		if err := w.evaluator.CompilePolicy(policy); err != nil {
			w.logger.Error(err, "failed to compile ToolPolicy on load",
				"name", policy.Name,
				"namespace", policy.Namespace)
			continue
		}
		w.logger.Info("compiled ToolPolicy",
			"name", policy.Name,
			"namespace", policy.Namespace,
			"rules", len(policy.Spec.Rules))
	}

	count := w.evaluator.PolicyCount()
	w.logger.Info("initial ToolPolicy load complete", "count", count)
	if w.metrics != nil {
		w.metrics.SetActivePolicies(count)
	}
	return nil
}

// listOptions returns the list options for ToolPolicy queries.
func (w *Watcher) listOptions() []client.ListOption {
	if w.namespace != "" {
		return []client.ListOption{client.InNamespace(w.namespace)}
	}
	return nil
}

// pollLoop periodically re-lists ToolPolicy resources to detect changes.
func (w *Watcher) pollLoop(ctx context.Context) error {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := w.initialLoad(ctx); err != nil {
				w.logger.Error(err, "poll cycle failed")
			}
		}
	}
}

// HandleEvent processes a ToolPolicy watch event.
func (w *Watcher) HandleEvent(eventType watch.EventType, policy *omniav1alpha1.ToolPolicy) {
	switch eventType {
	case watch.Added, watch.Modified:
		w.handleAddOrUpdate(policy)
	case watch.Deleted:
		w.handleDelete(policy)
	}
}

// handleAddOrUpdate compiles or recompiles a ToolPolicy.
func (w *Watcher) handleAddOrUpdate(policy *omniav1alpha1.ToolPolicy) {
	if err := w.evaluator.CompilePolicy(policy); err != nil {
		w.logger.Error(err, "failed to compile ToolPolicy",
			"name", policy.Name,
			"namespace", policy.Namespace)
		return
	}
	w.logger.Info("compiled ToolPolicy",
		"name", policy.Name,
		"namespace", policy.Namespace,
		"rules", len(policy.Spec.Rules))
}

// handleDelete removes a ToolPolicy from the evaluator.
func (w *Watcher) handleDelete(policy *omniav1alpha1.ToolPolicy) {
	w.evaluator.RemovePolicy(policy.Namespace, policy.Name)
	w.logger.Info("removed ToolPolicy",
		"name", policy.Name,
		"namespace", policy.Namespace)
}

// InformerWatcher creates a proper Kubernetes informer-based watcher.
// This is the production implementation that provides efficient event-driven updates.
type InformerWatcher struct {
	evaluator *Evaluator
	logger    logr.Logger
	informer  cache.SharedIndexInformer
}

// NewInformerWatcher creates a watcher backed by a Kubernetes SharedInformer.
func NewInformerWatcher(
	evaluator *Evaluator,
	informer cache.SharedIndexInformer,
	logger logr.Logger,
) *InformerWatcher {
	iw := &InformerWatcher{
		evaluator: evaluator,
		logger:    logger,
		informer:  informer,
	}

	_, _ = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    iw.onAdd,
		UpdateFunc: iw.onUpdate,
		DeleteFunc: iw.onDelete,
	})

	return iw
}

// Start runs the informer until the context is cancelled.
func (iw *InformerWatcher) Start(ctx context.Context) error {
	iw.informer.Run(ctx.Done())
	return nil
}

func (iw *InformerWatcher) onAdd(obj interface{}) {
	policy, ok := obj.(*omniav1alpha1.ToolPolicy)
	if !ok {
		iw.logger.V(1).Info("received non-ToolPolicy object in add handler")
		return
	}
	iw.compilePolicy(policy)
}

func (iw *InformerWatcher) onUpdate(_ interface{}, newObj interface{}) {
	policy, ok := newObj.(*omniav1alpha1.ToolPolicy)
	if !ok {
		iw.logger.V(1).Info("received non-ToolPolicy object in update handler")
		return
	}
	iw.compilePolicy(policy)
}

func (iw *InformerWatcher) onDelete(obj interface{}) {
	policy, ok := extractDeletedObject(obj)
	if !ok {
		iw.logger.V(1).Info("received unexpected object in delete handler")
		return
	}
	iw.evaluator.RemovePolicy(policy.Namespace, policy.Name)
	iw.logger.Info("removed ToolPolicy", "name", policy.Name, "namespace", policy.Namespace)
}

// extractDeletedObject handles both direct and tombstone delete events.
func extractDeletedObject(obj interface{}) (*omniav1alpha1.ToolPolicy, bool) {
	policy, ok := obj.(*omniav1alpha1.ToolPolicy)
	if ok {
		return policy, true
	}
	tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
	if !ok {
		return nil, false
	}
	policy, ok = tombstone.Obj.(*omniav1alpha1.ToolPolicy)
	return policy, ok
}

func (iw *InformerWatcher) compilePolicy(policy *omniav1alpha1.ToolPolicy) {
	if err := iw.evaluator.CompilePolicy(policy); err != nil {
		iw.logger.Error(err, "failed to compile ToolPolicy",
			"name", policy.Name,
			"namespace", policy.Namespace)
		return
	}
	iw.logger.Info("compiled ToolPolicy",
		"name", policy.Name,
		"namespace", policy.Namespace,
		"rules", len(policy.Spec.Rules))
}
