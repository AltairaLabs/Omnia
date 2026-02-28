/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package policy

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func makeUnstructuredToolPolicy(
	name, namespace string, tools []string, rules []map[string]interface{},
) *unstructured.Unstructured {
	return makeToolPolicyWithRegistry(name, namespace, "tools", tools, rules)
}

func makeToolPolicyWithRegistry(
	name, namespace, registry string,
	tools []string, rules []map[string]interface{},
) *unstructured.Unstructured {
	toolsI := make([]interface{}, len(tools))
	for i, t := range tools {
		toolsI[i] = t
	}
	rulesI := make([]interface{}, len(rules))
	for i, r := range rules {
		rulesI[i] = r
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{
					"registry": registry,
					"tools":    toolsI,
				},
				"rules":     rulesI,
				"mode":      "enforce",
				"onFailure": "deny",
			},
		},
	}
}

func makeRule(name, celExpr, message string) map[string]interface{} {
	return map[string]interface{}{
		"name": name,
		"deny": map[string]interface{}{
			"cel":     celExpr,
			"message": message,
		},
	}
}

func TestWatcher_SyncPolicy(t *testing.T) {
	e := newTestEvaluator(t)
	log := zap.New(zap.UseDevMode(true))
	w := NewWatcher(nil, e, log)

	obj := makeUnstructuredToolPolicy("test-policy", "default", []string{"refund"}, []map[string]interface{}{
		makeRule("r1", "int(headers['X-Amount']) > 1000", "too much"),
	})

	w.syncPolicy(obj)

	if e.PolicyCount() != 1 {
		t.Fatalf("expected 1 policy, got %d", e.PolicyCount())
	}

	// Verify evaluation works
	results := e.Evaluate("tools", "refund", &EvalContext{
		Headers: map[string]string{"X-Amount": "2000"},
	})
	if len(results) != 1 || !results[0].Denied {
		t.Error("expected denial for amount > 1000")
	}
}

func TestWatcher_OnDelete(t *testing.T) {
	e := newTestEvaluator(t)
	log := zap.New(zap.UseDevMode(true))
	w := NewWatcher(nil, e, log)

	obj := makeUnstructuredToolPolicy("test-policy", "default", nil, []map[string]interface{}{
		makeRule("r1", "true", "always deny"),
	})

	w.syncPolicy(obj)
	if e.PolicyCount() != 1 {
		t.Fatal("expected 1 policy after sync")
	}

	w.onDelete(obj)
	if e.PolicyCount() != 0 {
		t.Error("expected 0 policies after delete")
	}
}

func TestWatcher_OnDeleteTombstone(t *testing.T) {
	e := newTestEvaluator(t)
	log := zap.New(zap.UseDevMode(true))
	w := NewWatcher(nil, e, log)

	obj := makeUnstructuredToolPolicy("test-policy", "ns1", nil, []map[string]interface{}{
		makeRule("r1", "true", "deny"),
	})
	w.syncPolicy(obj)

	// Wrap in DeletedFinalStateUnknown (tombstone)
	tombstone := cache.DeletedFinalStateUnknown{
		Key: "ns1/test-policy",
		Obj: obj,
	}
	w.onDelete(tombstone)
	if e.PolicyCount() != 0 {
		t.Error("expected 0 policies after tombstone delete")
	}
}

func TestWatcher_OnUpdate(t *testing.T) {
	e := newTestEvaluator(t)
	log := zap.New(zap.UseDevMode(true))
	w := NewWatcher(nil, e, log)

	old := makeUnstructuredToolPolicy("p", "ns", nil, []map[string]interface{}{
		makeRule("r", "true", "v1"),
	})
	w.syncPolicy(old)

	updated := makeUnstructuredToolPolicy("p", "ns", nil, []map[string]interface{}{
		makeRule("r", "false", "v2"),
	})
	w.onUpdate(old, updated)

	results := e.Evaluate("tools", "any", &EvalContext{})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Denied {
		t.Error("updated policy should not deny (expression is false)")
	}
}

func TestWatcher_SyncPolicy_InvalidCEL(t *testing.T) {
	e := newTestEvaluator(t)
	log := zap.New(zap.UseDevMode(true))
	w := NewWatcher(nil, e, log)

	obj := makeUnstructuredToolPolicy("bad", "ns", nil, []map[string]interface{}{
		makeRule("r", "!@#invalid", "msg"),
	})

	w.syncPolicy(obj)

	// Policy should not be loaded on compile error
	if e.PolicyCount() != 0 {
		t.Errorf("expected 0 policies after invalid CEL, got %d", e.PolicyCount())
	}
}

func TestExtractPolicyFields_MissingSpec(t *testing.T) {
	_, _, _, _, _, err := extractPolicyFields(map[string]interface{}{})
	if err == nil {
		t.Error("expected error for missing spec")
	}
}

func TestExtractPolicyFields_MissingSelector(t *testing.T) {
	_, _, _, _, _, err := extractPolicyFields(map[string]interface{}{
		"spec": map[string]interface{}{},
	})
	if err == nil {
		t.Error("expected error for missing selector")
	}
}

func TestExtractPolicyFields_MissingRegistry(t *testing.T) {
	_, _, _, _, _, err := extractPolicyFields(map[string]interface{}{
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{},
		},
	})
	if err == nil {
		t.Error("expected error for missing registry")
	}
}

func TestExtractPolicyFields_MissingRules(t *testing.T) {
	_, _, _, _, _, err := extractPolicyFields(map[string]interface{}{
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{
				"registry": "reg",
			},
		},
	})
	if err == nil {
		t.Error("expected error for missing rules")
	}
}

func TestExtractPolicyFields_InvalidRuleFormat(t *testing.T) {
	_, _, _, _, _, err := extractPolicyFields(map[string]interface{}{
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{
				"registry": "reg",
			},
			"rules": "not-a-list",
		},
	})
	if err == nil {
		t.Error("expected error for invalid rules format")
	}
}

func TestExtractSingleRule_NotAMap(t *testing.T) {
	_, err := extractSingleRule("not a map")
	if err == nil {
		t.Error("expected error")
	}
}

func TestExtractSingleRule_MissingDeny(t *testing.T) {
	_, err := extractSingleRule(map[string]interface{}{
		"name": "r1",
	})
	if err == nil {
		t.Error("expected error for missing deny")
	}
}

func TestExtractSingleRule_MissingCEL(t *testing.T) {
	_, err := extractSingleRule(map[string]interface{}{
		"name": "r1",
		"deny": map[string]interface{}{
			"message": "msg",
		},
	})
	if err == nil {
		t.Error("expected error for missing cel")
	}
}

func TestStringField(t *testing.T) {
	m := map[string]interface{}{"key": "val"}
	if got := stringField(m, "key", "default"); got != "val" {
		t.Errorf("expected 'val', got %q", got)
	}
	if got := stringField(m, "missing", "default"); got != "default" {
		t.Errorf("expected 'default', got %q", got)
	}
}

func TestNestedMap(t *testing.T) {
	m := map[string]interface{}{
		"nested": map[string]interface{}{"a": "b"},
		"scalar": "value",
	}
	if _, ok := nestedMap(m, "nested"); !ok {
		t.Error("expected nested map to be found")
	}
	if _, ok := nestedMap(m, "scalar"); ok {
		t.Error("scalar should not be a nested map")
	}
	if _, ok := nestedMap(m, "missing"); ok {
		t.Error("missing key should not be found")
	}
}

func TestExtractPolicyFields_DefaultModeAndOnFailure(t *testing.T) {
	content := map[string]interface{}{
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{
				"registry": "reg",
			},
			"rules": []interface{}{
				map[string]interface{}{
					"name": "r1",
					"deny": map[string]interface{}{
						"cel":     "true",
						"message": "msg",
					},
				},
			},
		},
	}

	_, _, _, mode, onFailure, err := extractPolicyFields(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != defaultPolicyMode {
		t.Errorf("expected mode %q, got %q", defaultPolicyMode, mode)
	}
	if onFailure != defaultOnFailure {
		t.Errorf("expected onFailure %q, got %q", defaultOnFailure, onFailure)
	}
}

func TestToUnstructured_InvalidType(t *testing.T) {
	_, ok := toUnstructured("not an object")
	if ok {
		t.Error("expected false for non-unstructured object")
	}
}

func TestWatcher_OnDelete_InvalidObj(t *testing.T) {
	e := newTestEvaluator(t)
	log := zap.New(zap.UseDevMode(true))
	w := NewWatcher(nil, e, log)

	// Should not panic on invalid object
	w.onDelete("not an object")
}

func TestWatcher_SyncPolicy_InvalidObj(t *testing.T) {
	e := newTestEvaluator(t)
	log := zap.New(zap.UseDevMode(true))
	w := NewWatcher(nil, e, log)

	// Should not panic on invalid object
	w.syncPolicy("not an object")
}

func TestWatcher_SyncPolicy_MissingSpec(t *testing.T) {
	e := newTestEvaluator(t)
	log := zap.New(zap.UseDevMode(true))
	w := NewWatcher(nil, e, log)

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "test",
				"namespace": "ns",
			},
		},
	}

	w.syncPolicy(obj)
	if e.PolicyCount() != 0 {
		t.Errorf("expected 0 policies for missing spec, got %d", e.PolicyCount())
	}
}
