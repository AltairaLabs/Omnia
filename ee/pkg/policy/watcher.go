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

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	"github.com/go-logr/logr"
)

// Watcher resync interval.
const defaultResyncPeriod = 30 * time.Second

// ToolPolicy field constants to avoid duplication.
const (
	fieldSpec            = "spec"
	fieldSelector        = "selector"
	fieldRegistry        = "registry"
	fieldTools           = "tools"
	fieldRules           = "rules"
	fieldName            = "name"
	fieldDeny            = "deny"
	fieldCEL             = "cel"
	fieldMessage         = "message"
	fieldMode            = "mode"
	fieldOnFailure       = "onFailure"
	fieldMetadata        = "metadata"
	fieldMetadataName    = "name"
	defaultPolicyMode    = "enforce"
	defaultOnFailure     = "deny"
	toolPolicyResource   = "toolpolicies"
	toolPolicyAPIGroup   = "omnia.altairalabs.ai"
	toolPolicyAPIVersion = "v1alpha1"
)

// ToolPolicyGVR is the GroupVersionResource for ToolPolicy CRDs.
var ToolPolicyGVR = schema.GroupVersionResource{
	Group:    toolPolicyAPIGroup,
	Version:  toolPolicyAPIVersion,
	Resource: toolPolicyResource,
}

// Watcher watches ToolPolicy CRDs and updates the evaluator.
type Watcher struct {
	client    dynamic.Interface
	evaluator *Evaluator
	log       logr.Logger
	stopCh    chan struct{}
}

// NewWatcher creates a new ToolPolicy watcher.
func NewWatcher(client dynamic.Interface, evaluator *Evaluator, log logr.Logger) *Watcher {
	return &Watcher{
		client:    client,
		evaluator: evaluator,
		log:       log.WithName("toolpolicy-watcher"),
		stopCh:    make(chan struct{}),
	}
}

// Start begins watching ToolPolicy CRDs. It blocks until the context is cancelled.
func (w *Watcher) Start(ctx context.Context) error {
	factory := dynamicinformer.NewDynamicSharedInformerFactory(w.client, defaultResyncPeriod)
	informer := factory.ForResource(ToolPolicyGVR).Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    w.onAdd,
		UpdateFunc: w.onUpdate,
		DeleteFunc: w.onDelete,
	})
	if err != nil {
		return fmt.Errorf("adding event handler: %w", err)
	}

	go informer.Run(w.stopCh)

	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		return fmt.Errorf("timed out waiting for informer cache sync")
	}

	w.log.Info("ToolPolicy watcher started and synced")
	<-ctx.Done()
	close(w.stopCh)
	return nil
}

func (w *Watcher) onAdd(obj interface{}) {
	w.syncPolicy(obj)
}

func (w *Watcher) onUpdate(_, newObj interface{}) {
	w.syncPolicy(newObj)
}

func (w *Watcher) onDelete(obj interface{}) {
	u, ok := toUnstructured(obj)
	if !ok {
		w.log.Error(nil, "unexpected object type on delete")
		return
	}
	name := u.GetName()
	namespace := u.GetNamespace()
	w.evaluator.RemovePolicy(namespace, name)
	w.log.Info("removed ToolPolicy", "name", name, "namespace", namespace)
}

func (w *Watcher) syncPolicy(obj interface{}) {
	u, ok := toUnstructured(obj)
	if !ok {
		w.log.Error(nil, "unexpected object type")
		return
	}

	name := u.GetName()
	namespace := u.GetNamespace()
	content := u.UnstructuredContent()

	registry, tools, rules, mode, onFailure, err := extractPolicyFields(content)
	if err != nil {
		w.log.Error(err, "failed to extract policy fields", "name", name, "namespace", namespace)
		return
	}

	if err := w.evaluator.SetPolicy(name, namespace, registry, tools, rules, mode, onFailure); err != nil {
		w.log.Error(err, "failed to compile ToolPolicy", "name", name, "namespace", namespace)
		return
	}

	w.log.Info("synced ToolPolicy", "name", name, "namespace", namespace, "rules", len(rules))
}

// unstructuredObj is the interface for accessing unstructured Kubernetes objects.
type unstructuredObj interface {
	GetName() string
	GetNamespace() string
	UnstructuredContent() map[string]interface{}
}

// toUnstructured extracts unstructured data from an informer event object.
func toUnstructured(obj interface{}) (unstructuredObj, bool) {
	// Handle tombstone objects from the delete handler.
	if d, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = d.Obj
	}

	u, ok := obj.(unstructuredObj)
	return u, ok
}

// extractPolicyFields extracts policy data from unstructured content.
func extractPolicyFields(content map[string]interface{}) (
	registry string,
	tools []string,
	rules []RuleInput,
	mode string,
	onFailure string,
	err error,
) {
	spec, ok := nestedMap(content, fieldSpec)
	if !ok {
		return "", nil, nil, "", "", fmt.Errorf("missing spec")
	}

	registry, tools, err = extractSelector(spec)
	if err != nil {
		return "", nil, nil, "", "", err
	}

	rules, err = extractRules(spec)
	if err != nil {
		return "", nil, nil, "", "", err
	}

	mode = stringField(spec, fieldMode, defaultPolicyMode)
	onFailure = stringField(spec, fieldOnFailure, defaultOnFailure)

	return registry, tools, rules, mode, onFailure, nil
}

// extractSelector extracts the selector fields from the spec.
func extractSelector(spec map[string]interface{}) (string, []string, error) {
	selector, ok := nestedMap(spec, fieldSelector)
	if !ok {
		return "", nil, fmt.Errorf("missing spec.selector")
	}

	registry, ok := selector[fieldRegistry].(string)
	if !ok || registry == "" {
		return "", nil, fmt.Errorf("missing spec.selector.registry")
	}

	var tools []string
	if toolsRaw, ok := selector[fieldTools]; ok {
		if toolsList, ok := toolsRaw.([]interface{}); ok {
			for _, t := range toolsList {
				if s, ok := t.(string); ok {
					tools = append(tools, s)
				}
			}
		}
	}

	return registry, tools, nil
}

// extractRules extracts the rules from the spec.
func extractRules(spec map[string]interface{}) ([]RuleInput, error) {
	rulesRaw, ok := spec[fieldRules]
	if !ok {
		return nil, fmt.Errorf("missing spec.rules")
	}

	rulesList, ok := rulesRaw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("spec.rules is not a list")
	}

	rules := make([]RuleInput, 0, len(rulesList))
	for _, raw := range rulesList {
		rule, err := extractSingleRule(raw)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

// extractSingleRule extracts a single rule from unstructured data.
func extractSingleRule(raw interface{}) (RuleInput, error) {
	ruleMap, ok := raw.(map[string]interface{})
	if !ok {
		return RuleInput{}, fmt.Errorf("rule is not a map")
	}

	name, _ := ruleMap[fieldName].(string)
	deny, ok := nestedMap(ruleMap, fieldDeny)
	if !ok {
		return RuleInput{}, fmt.Errorf("rule %q missing deny", name)
	}

	celExpr, _ := deny[fieldCEL].(string)
	message, _ := deny[fieldMessage].(string)

	if celExpr == "" {
		return RuleInput{}, fmt.Errorf("rule %q missing deny.cel", name)
	}

	return RuleInput{
		Name:    name,
		CEL:     celExpr,
		Message: message,
	}, nil
}

// nestedMap retrieves a nested map from unstructured data.
func nestedMap(m map[string]interface{}, key string) (map[string]interface{}, bool) {
	val, ok := m[key]
	if !ok {
		return nil, false
	}
	result, ok := val.(map[string]interface{})
	return result, ok
}

// stringField retrieves a string field with a default value.
func stringField(m map[string]interface{}, key, defaultVal string) string {
	val, ok := m[key].(string)
	if !ok || val == "" {
		return defaultVal
	}
	return val
}
