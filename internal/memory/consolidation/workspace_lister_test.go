/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package consolidation_test

import (
	"context"
	"sort"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/memory/consolidation"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := omniav1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	return s
}

func wsWithPolicy(name, uid, policy string) *omniav1alpha1.Workspace {
	w := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID(uid)},
		Spec: omniav1alpha1.WorkspaceSpec{Services: []omniav1alpha1.WorkspaceServiceGroup{
			{
				Name: "default",
				Memory: &omniav1alpha1.MemoryServiceConfig{
					PolicyRef: &corev1.LocalObjectReference{Name: policy},
				},
			},
		}},
	}
	return w
}

func TestK8sWorkspaceLister_ForPolicy_FindsOptedIn(t *testing.T) {
	scheme := newScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		wsWithPolicy("alpha", "uid-alpha", "research"),
		wsWithPolicy("beta", "uid-beta", "research"),
		wsWithPolicy("gamma", "uid-gamma", "marketing"),
	).Build()

	l := consolidation.NewK8sWorkspaceLister(client)
	got, err := l.ForPolicy(context.Background(), "research")
	if err != nil {
		t.Fatalf("ForPolicy: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 workspaces, got %d (%+v)", len(got), got)
	}
	sort.Slice(got, func(i, j int) bool { return got[i].UID < got[j].UID })
	if got[0].UID != "uid-alpha" || got[1].UID != "uid-beta" {
		t.Errorf("wrong workspaces: %+v", got)
	}
	if got[0].Name != "alpha" || got[1].Name != "beta" {
		t.Errorf("wrong workspace names: %+v", got)
	}
}

func TestK8sWorkspaceLister_ForPolicy_NoMatches(t *testing.T) {
	scheme := newScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		wsWithPolicy("solo", "uid-solo", "other"),
	).Build()
	l := consolidation.NewK8sWorkspaceLister(client)
	got, err := l.ForPolicy(context.Background(), "research")
	if err != nil {
		t.Fatalf("ForPolicy: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want 0 workspaces, got %d", len(got))
	}
}

func TestK8sWorkspaceLister_ForPolicy_IgnoresNilMemory(t *testing.T) {
	scheme := newScheme(t)
	w := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "noMemory", UID: types.UID("uid-none")},
		Spec: omniav1alpha1.WorkspaceSpec{Services: []omniav1alpha1.WorkspaceServiceGroup{
			{Name: "default"}, // no Memory block
		}},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(w).Build()
	l := consolidation.NewK8sWorkspaceLister(client)
	got, err := l.ForPolicy(context.Background(), "research")
	if err != nil {
		t.Fatalf("ForPolicy: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want 0 (nil memory), got %d", len(got))
	}
}

func TestK8sWorkspaceLister_ForPolicy_MultipleServiceGroupsOneMatches(t *testing.T) {
	scheme := newScheme(t)
	w := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "multi", UID: types.UID("uid-multi")},
		Spec: omniav1alpha1.WorkspaceSpec{Services: []omniav1alpha1.WorkspaceServiceGroup{
			{Name: "primary", Memory: &omniav1alpha1.MemoryServiceConfig{
				PolicyRef: &corev1.LocalObjectReference{Name: "other"},
			}},
			{Name: "secondary", Memory: &omniav1alpha1.MemoryServiceConfig{
				PolicyRef: &corev1.LocalObjectReference{Name: "research"},
			}},
		}},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(w).Build()
	l := consolidation.NewK8sWorkspaceLister(client)
	got, err := l.ForPolicy(context.Background(), "research")
	if err != nil {
		t.Fatalf("ForPolicy: %v", err)
	}
	if len(got) != 1 || got[0].UID != "uid-multi" {
		t.Errorf("want one match for uid-multi, got %+v", got)
	}
}
