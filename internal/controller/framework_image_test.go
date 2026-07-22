/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// testFacadeImage is the shared facade image used across controller tests.
const testFacadeImage = "test-facade:v1.0.0"

// promptkitImage builds a FrameworkImages map mapping the promptkit framework
// to img — the common test setup (agents default to promptkit).
func promptkitImage(img string) map[string]string {
	return map[string]string{string(omniav1alpha1.FrameworkTypePromptKit): img}
}

func arWithFramework(typ omniav1alpha1.FrameworkType, image string) *omniav1alpha1.AgentRuntime {
	ar := &omniav1alpha1.AgentRuntime{}
	if typ != "" || image != "" {
		ar.Spec.Framework = &omniav1alpha1.FrameworkConfig{Type: typ, Image: image}
	}
	return ar
}

func TestResolveFrameworkImage(t *testing.T) {
	r := &AgentRuntimeReconciler{FrameworkImages: map[string]string{
		"promptkit": "ghcr.io/altairalabs/omnia-runtime:v1",
		"langchain": "ghcr.io/altairalabs/omnia-langchain-runtime:v1",
	}}
	cases := []struct {
		name      string
		ar        *omniav1alpha1.AgentRuntime
		wantImage string
		wantOK    bool
	}{
		{"explicit override wins", arWithFramework(omniav1alpha1.FrameworkTypeLangChain, "custom:tag"), "custom:tag", true},
		{"langchain from map", arWithFramework(omniav1alpha1.FrameworkTypeLangChain, ""), "ghcr.io/altairalabs/omnia-langchain-runtime:v1", true},
		{"promptkit from map", arWithFramework(omniav1alpha1.FrameworkTypePromptKit, ""), "ghcr.io/altairalabs/omnia-runtime:v1", true},
		{"nil framework -> promptkit", arWithFramework("", ""), "ghcr.io/altairalabs/omnia-runtime:v1", true},
		{"custom -> blocked", arWithFramework(omniav1alpha1.FrameworkTypeCustom, ""), "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img, ok := r.resolveFrameworkImage(tc.ar)
			if img != tc.wantImage || ok != tc.wantOK {
				t.Fatalf("got (%q,%v) want (%q,%v)", img, ok, tc.wantImage, tc.wantOK)
			}
		})
	}
}

// TestReconcileResources_UnresolvableFramework_Blocks proves the #1206 fix:
// a framework type with no resolvable image blocks loudly (condition + Event +
// no Deployment) instead of silently running PromptKit.
func TestReconcileResources_UnresolvableFramework_Blocks(t *testing.T) {
	scheme := newTestScheme(t)
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "custom-agent", Namespace: "fw1206-ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Framework: &omniav1alpha1.FrameworkConfig{Type: omniav1alpha1.FrameworkTypeCustom},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ar).
		WithStatusSubresource(&omniav1alpha1.AgentRuntime{}).
		Build()
	rec := record.NewFakeRecorder(10)
	// No FrameworkImages and no spec.framework.image: custom has no built-in image -> must block.
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme, Recorder: rec}

	dep, err := r.reconcileResources(context.Background(), logr.Discard(), ar, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for unresolvable framework image")
	}
	if dep != nil {
		t.Fatal("no Deployment should be built when the framework image is unavailable")
	}
	cond := findCondition(ar.Status.Conditions, ConditionTypeFrameworkReady)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != reasonFrameworkImageUnavailable {
		t.Fatalf("want FrameworkReady=False/FrameworkImageUnavailable, got %+v", cond)
	}
	select {
	case ev := <-rec.Events:
		if !strings.Contains(ev, "FrameworkImageUnavailable") {
			t.Fatalf("event %q missing FrameworkImageUnavailable", ev)
		}
	default:
		t.Fatal("expected a Warning event")
	}
	// dep == nil (asserted above) + the block returns before any
	// reconcileFacadeRBAC/reconcileDeployment proves no Deployment is built.
}

// TestReconcileResources_CrossNamespaceToolRegistry_Blocks proves #1874: an
// AgentRuntime whose toolRegistryRef names a foreign namespace is rejected
// loudly (condition + Event + no Deployment) rather than left running with
// registry-scoped ToolPolicies silently disabled.
func TestReconcileResources_CrossNamespaceToolRegistry_Blocks(t *testing.T) {
	scheme := newTestScheme(t)
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add appsv1 to scheme: %v", err)
	}
	otherNS := "other-ns"
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "xns-agent", Namespace: "agent-ns"},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ToolRegistryRef: &omniav1alpha1.ToolRegistryRef{
				Name:      "orders",
				Namespace: &otherNS,
			},
		},
	}
	// A Deployment already reconciled under the pre-change fail-open — the
	// rejection must stop it, not leave it running with policy silently off.
	existing := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "xns-agent", Namespace: "agent-ns"},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ar, existing).
		WithStatusSubresource(&omniav1alpha1.AgentRuntime{}).
		Build()
	rec := record.NewFakeRecorder(10)
	r := &AgentRuntimeReconciler{
		Client: c, Scheme: scheme, Recorder: rec,
		FrameworkImages: promptkitImage("test-runtime:v1"),
		FacadeImage:     testFacadeImage,
	}

	dep, err := r.reconcileResources(context.Background(), logr.Discard(), ar, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for cross-namespace toolRegistryRef")
	}
	if dep != nil {
		t.Fatal("no Deployment should be built for a cross-namespace toolRegistryRef")
	}
	// The pre-existing Deployment must have been stopped.
	got := &appsv1.Deployment{}
	if getErr := c.Get(context.Background(), types.NamespacedName{Name: "xns-agent", Namespace: "agent-ns"}, got); !apierrors.IsNotFound(getErr) {
		t.Fatalf("existing Deployment should have been deleted, got err=%v", getErr)
	}
	cond := findCondition(ar.Status.Conditions, ConditionTypeToolRegistryReady)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != reasonToolRegistryCrossNamespace {
		t.Fatalf("want ToolRegistryReady=False/%s, got %+v", reasonToolRegistryCrossNamespace, cond)
	}
	select {
	case ev := <-rec.Events:
		if !strings.Contains(ev, reasonToolRegistryCrossNamespace) {
			t.Fatalf("event %q missing %s", ev, reasonToolRegistryCrossNamespace)
		}
	default:
		t.Fatal("expected a Warning event")
	}
}

// TestReconcileResources_SameNamespaceToolRegistry_Allowed proves an explicit
// namespace equal to the AgentRuntime's own does not trip the cross-namespace
// guard. Other reconcile errors (missing registry/providers) are tolerated —
// only the cross-namespace rejection must not fire.
func TestReconcileResources_SameNamespaceToolRegistry_Allowed(t *testing.T) {
	scheme := newTestScheme(t)
	sameNS := "agent-ns"
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "same-ns-agent", Namespace: sameNS},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			ToolRegistryRef: &omniav1alpha1.ToolRegistryRef{
				Name:      "orders",
				Namespace: &sameNS,
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(ar).
		WithStatusSubresource(&omniav1alpha1.AgentRuntime{}).
		Build()
	r := &AgentRuntimeReconciler{
		Client: c, Scheme: scheme, Recorder: record.NewFakeRecorder(10),
		FrameworkImages: promptkitImage("test-runtime:v1"),
		FacadeImage:     testFacadeImage,
	}

	if _, err := r.reconcileResources(context.Background(), logr.Discard(), ar, nil, nil, nil); err != nil {
		if strings.Contains(err.Error(), "cross-namespace") {
			t.Fatalf("same-namespace ref must not be rejected: %v", err)
		}
	}
}

func TestResolveFrameworkImage_BareDevFallback(t *testing.T) {
	// No map configured (bare operator run) -> only promptkit has a built-in.
	r := &AgentRuntimeReconciler{}
	img, ok := r.resolveFrameworkImage(arWithFramework(omniav1alpha1.FrameworkTypePromptKit, ""))
	if !ok || img != DefaultFrameworkImage {
		t.Fatalf("bare-dev promptkit: got (%q,%v) want (%q,true)", img, ok, DefaultFrameworkImage)
	}
	// langchain has no built-in: it must block rather than silently run a
	// stale :latest community image (custom-runtime wave 1).
	if _, ok := r.resolveFrameworkImage(arWithFramework(omniav1alpha1.FrameworkTypeLangChain, "")); ok {
		t.Fatal("langchain must block with no explicit image configured")
	}
	// custom has no built-in -> blocked even bare (no map, no explicit image).
	if _, ok := r.resolveFrameworkImage(arWithFramework(omniav1alpha1.FrameworkTypeCustom, "")); ok {
		t.Fatal("custom must block with no explicit image configured")
	}
}
