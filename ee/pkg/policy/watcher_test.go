/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package policy

import (
	"context"
	"io"
	"log/slog"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func makeTestPolicy(name, celExpr string) *omniav1alpha1.ToolPolicy {
	return &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{
				Registry: "test-registry",
			},
			Rules: []omniav1alpha1.PolicyRule{
				{
					Name: "test-rule",
					Deny: omniav1alpha1.PolicyRuleDeny{
						CEL:     celExpr,
						Message: "test denial",
					},
				},
			},
			Mode:      omniav1alpha1.PolicyModeEnforce,
			OnFailure: omniav1alpha1.OnFailureDeny,
		},
	}
}

func TestWatcher_HandleEvent_Add(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	w := &Watcher{
		evaluator: eval,
		logger:    discardLogger(),
	}

	policy := makeTestPolicy("add-test", "true")
	w.HandleEvent(watch.Added, policy)

	if eval.PolicyCount() != 1 {
		t.Errorf("PolicyCount() = %d, want 1", eval.PolicyCount())
	}
}

func TestWatcher_HandleEvent_Modified(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	w := &Watcher{
		evaluator: eval,
		logger:    discardLogger(),
	}

	policy := makeTestPolicy("mod-test", "true")
	w.HandleEvent(watch.Added, policy)
	if eval.PolicyCount() != 1 {
		t.Fatalf("PolicyCount() = %d, want 1 after add", eval.PolicyCount())
	}

	policy.Spec.Rules[0].Deny.CEL = "false"
	w.HandleEvent(watch.Modified, policy)
	if eval.PolicyCount() != 1 {
		t.Errorf("PolicyCount() = %d, want 1 after modify", eval.PolicyCount())
	}

	headers := map[string]string{
		HeaderToolName:     "any",
		HeaderToolRegistry: "test-registry",
	}
	decision := eval.Evaluate(headers, nil)
	if !decision.Allowed {
		t.Error("want allowed after modifying rule to 'false'")
	}
}

func TestWatcher_HandleEvent_Deleted(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	w := &Watcher{
		evaluator: eval,
		logger:    discardLogger(),
	}

	policy := makeTestPolicy("del-test", "true")
	w.HandleEvent(watch.Added, policy)
	if eval.PolicyCount() != 1 {
		t.Fatalf("PolicyCount() = %d, want 1", eval.PolicyCount())
	}

	w.HandleEvent(watch.Deleted, policy)
	if eval.PolicyCount() != 0 {
		t.Errorf("PolicyCount() = %d, want 0 after delete", eval.PolicyCount())
	}
}

func TestWatcher_HandleEvent_InvalidCEL(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	w := &Watcher{
		evaluator: eval,
		logger:    discardLogger(),
	}

	policy := makeTestPolicy("bad-policy", "invalid CEL %%%")
	w.HandleEvent(watch.Added, policy)

	if eval.PolicyCount() != 0 {
		t.Errorf("PolicyCount() = %d, want 0 for invalid CEL", eval.PolicyCount())
	}
}

func TestExtractDeletedObject_Direct(t *testing.T) {
	policy := makeTestPolicy("test", "true")
	result, ok := extractDeletedObject(policy)
	if !ok {
		t.Fatal("extractDeletedObject() returned false for direct object")
	}
	if result.Name != "test" {
		t.Errorf("name = %q, want %q", result.Name, "test")
	}
}

func TestExtractDeletedObject_Tombstone(t *testing.T) {
	policy := makeTestPolicy("tombstone-test", "true")
	tombstone := cache.DeletedFinalStateUnknown{
		Key: "default/tombstone-test",
		Obj: policy,
	}
	result, ok := extractDeletedObject(tombstone)
	if !ok {
		t.Fatal("extractDeletedObject() returned false for tombstone")
	}
	if result.Name != "tombstone-test" {
		t.Errorf("name = %q, want %q", result.Name, "tombstone-test")
	}
}

func TestExtractDeletedObject_Unknown(t *testing.T) {
	_, ok := extractDeletedObject("not a policy")
	if ok {
		t.Error("extractDeletedObject() returned true for unknown object type")
	}
}

func TestInformerWatcher_OnAdd(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	iw := &InformerWatcher{
		evaluator: eval,
		logger:    discardLogger(),
	}

	policy := makeTestPolicy("informer-add", "true")
	iw.onAdd(policy)

	if eval.PolicyCount() != 1 {
		t.Errorf("PolicyCount() = %d, want 1", eval.PolicyCount())
	}
}

func TestInformerWatcher_OnUpdate(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	iw := &InformerWatcher{
		evaluator: eval,
		logger:    discardLogger(),
	}

	old := makeTestPolicy("informer-update", "true")
	iw.onAdd(old)

	updated := makeTestPolicy("informer-update", "false")
	iw.onUpdate(old, updated)

	if eval.PolicyCount() != 1 {
		t.Errorf("PolicyCount() = %d, want 1", eval.PolicyCount())
	}
}

func TestInformerWatcher_OnDelete(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	iw := &InformerWatcher{
		evaluator: eval,
		logger:    discardLogger(),
	}

	policy := makeTestPolicy("informer-del", "true")
	iw.onAdd(policy)
	if eval.PolicyCount() != 1 {
		t.Fatalf("PolicyCount() = %d, want 1", eval.PolicyCount())
	}

	iw.onDelete(policy)
	if eval.PolicyCount() != 0 {
		t.Errorf("PolicyCount() = %d, want 0", eval.PolicyCount())
	}
}

func TestInformerWatcher_OnAdd_NonToolPolicy(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	iw := &InformerWatcher{
		evaluator: eval,
		logger:    discardLogger(),
	}

	iw.onAdd("not a policy")
	if eval.PolicyCount() != 0 {
		t.Errorf("PolicyCount() = %d, want 0", eval.PolicyCount())
	}
}

func TestInformerWatcher_OnUpdate_NonToolPolicy(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	iw := &InformerWatcher{
		evaluator: eval,
		logger:    discardLogger(),
	}

	iw.onUpdate("not a policy", "also not a policy")
	if eval.PolicyCount() != 0 {
		t.Errorf("PolicyCount() = %d, want 0", eval.PolicyCount())
	}
}

func TestInformerWatcher_OnDelete_NonToolPolicy(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	iw := &InformerWatcher{
		evaluator: eval,
		logger:    discardLogger(),
	}

	iw.onDelete("not a policy")
}

func newFakeScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)
	return scheme
}

func newFakeClient(
	scheme *runtime.Scheme,
	objs ...client.Object,
) client.Client {
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&omniav1alpha1.ToolPolicy{}).
		Build()
}

func TestNewWatcher(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	scheme := newFakeScheme()
	fc := newFakeClient(scheme)
	logger := discardLogger()

	w := NewWatcher(eval, fc, scheme, "test-ns", logger)
	if w == nil {
		t.Fatal("NewWatcher() returned nil")
	}
	if w.evaluator != eval {
		t.Error("evaluator not set")
	}
	if w.namespace != "test-ns" {
		t.Errorf("namespace = %q, want %q", w.namespace, "test-ns")
	}
}

func TestWatcher_ListOptions_WithNamespace(t *testing.T) {
	w := &Watcher{namespace: "my-ns"}
	opts := w.listOptions()
	if len(opts) != 1 {
		t.Fatalf("listOptions() returned %d opts, want 1", len(opts))
	}
}

func TestWatcher_ListOptions_ClusterWide(t *testing.T) {
	w := &Watcher{namespace: ""}
	opts := w.listOptions()
	if opts != nil {
		t.Errorf("listOptions() = %v, want nil for cluster-wide", opts)
	}
}

func TestWatcher_InitialLoad_EmptyList(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	scheme := newFakeScheme()
	fc := newFakeClient(scheme)

	w := NewWatcher(eval, fc, scheme, "default", discardLogger())
	if err := w.initialLoad(context.Background()); err != nil {
		t.Fatalf("initialLoad() error = %v", err)
	}
	if eval.PolicyCount() != 0 {
		t.Errorf("PolicyCount() = %d, want 0", eval.PolicyCount())
	}
}

func newToolPolicyObject(name, cel string) *omniav1alpha1.ToolPolicy {
	return &omniav1alpha1.ToolPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: omniav1alpha1.ToolPolicySpec{
			Selector: omniav1alpha1.ToolPolicySelector{
				Registry: "test-registry",
			},
			Rules: []omniav1alpha1.PolicyRule{
				{
					Name: "r1",
					Deny: omniav1alpha1.PolicyRuleDeny{
						CEL:     cel,
						Message: "test",
					},
				},
			},
			Mode:      omniav1alpha1.PolicyModeEnforce,
			OnFailure: omniav1alpha1.OnFailureDeny,
		},
	}
}

func TestWatcher_InitialLoad_WithPolicies(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	scheme := newFakeScheme()
	tp := newToolPolicyObject("test-policy", "true")
	fc := newFakeClient(scheme, tp)

	w := NewWatcher(eval, fc, scheme, "default", discardLogger())
	if err := w.initialLoad(context.Background()); err != nil {
		t.Fatalf("initialLoad() error = %v", err)
	}
	if eval.PolicyCount() != 1 {
		t.Errorf("PolicyCount() = %d, want 1", eval.PolicyCount())
	}
}

func TestWatcher_InitialLoad_InvalidCEL(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	scheme := newFakeScheme()
	tp := newToolPolicyObject("bad-policy", "invalid %%%")
	fc := newFakeClient(scheme, tp)

	w := NewWatcher(eval, fc, scheme, "default", discardLogger())
	// Should not return error; just logs and skips
	if err := w.initialLoad(context.Background()); err != nil {
		t.Fatalf("initialLoad() error = %v", err)
	}
	if eval.PolicyCount() != 0 {
		t.Errorf("PolicyCount() = %d, want 0", eval.PolicyCount())
	}
}

func TestWatcher_PollLoop_CancelledContext(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	scheme := newFakeScheme()
	fc := newFakeClient(scheme)

	w := NewWatcher(eval, fc, scheme, "default", discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err = w.pollLoop(ctx)
	if err == nil {
		t.Fatal("pollLoop() expected context error, got nil")
	}
}

func TestWatcher_Start_CancelledContext(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	scheme := newFakeScheme()
	fc := newFakeClient(scheme)

	w := NewWatcher(eval, fc, scheme, "default", discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err = w.Start(ctx)
	if err == nil {
		t.Fatal("Start() expected context error, got nil")
	}
}

// fakeInformer is a minimal SharedIndexInformer for testing.
type fakeInformer struct {
	cache.SharedIndexInformer
	handlers []cache.ResourceEventHandler
}

func (f *fakeInformer) AddEventHandler(
	handler cache.ResourceEventHandler,
) (cache.ResourceEventHandlerRegistration, error) {
	f.handlers = append(f.handlers, handler)
	return nil, nil
}

func (f *fakeInformer) Run(stopCh <-chan struct{}) {
	<-stopCh
}

func TestNewInformerWatcher(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	fi := &fakeInformer{}
	iw := NewInformerWatcher(eval, fi, discardLogger())
	if iw == nil {
		t.Fatal("NewInformerWatcher() returned nil")
	}
	if len(fi.handlers) != 1 {
		t.Errorf("handlers = %d, want 1", len(fi.handlers))
	}
}

func TestInformerWatcher_Start(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	fi := &fakeInformer{}
	iw := NewInformerWatcher(eval, fi, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- iw.Start(ctx)
	}()

	cancel()
	err = <-done
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
}

func TestInformerWatcher_CompilePolicy_Error(t *testing.T) {
	eval, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator() error = %v", err)
	}

	iw := &InformerWatcher{
		evaluator: eval,
		logger:    discardLogger(),
	}

	badPolicy := makeTestPolicy("bad", "invalid CEL %%%")
	iw.compilePolicy(badPolicy)

	if eval.PolicyCount() != 0 {
		t.Errorf("PolicyCount() = %d, want 0", eval.PolicyCount())
	}
}
