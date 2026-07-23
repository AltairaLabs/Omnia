/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package consolidation_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	memoryv1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/memory/consolidation"
)

func newFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := memoryv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func wsOptingInto(name, policy string) *memoryv1.Workspace {
	return &memoryv1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID("uid-" + name)},
		Spec: memoryv1.WorkspaceSpec{
			Services: []memoryv1.WorkspaceServiceGroup{{
				Name: "default",
				Memory: &memoryv1.MemoryServiceConfig{
					PolicyRef: &corev1.LocalObjectReference{Name: policy},
				},
			}},
		},
	}
}

func TestForPolicy_OwnWorkspaceOptsIn_ReturnsIt(t *testing.T) {
	c := newFakeClient(t, wsOptingInto("demo", "p1"))
	l := consolidation.NewK8sWorkspaceLister(c, "demo")
	got, err := l.ForPolicy(context.Background(), "p1")
	if err != nil {
		t.Fatalf("ForPolicy: %v", err)
	}
	if len(got) != 1 || got[0].Name != "demo" || got[0].UID != "uid-demo" {
		t.Fatalf("want [demo/uid-demo], got %+v", got)
	}
}

func TestForPolicy_OwnWorkspaceOptsOut_ReturnsEmpty(t *testing.T) {
	c := newFakeClient(t, wsOptingInto("demo", "other-policy"))
	l := consolidation.NewK8sWorkspaceLister(c, "demo")
	got, err := l.ForPolicy(context.Background(), "p1")
	if err != nil || len(got) != 0 {
		t.Fatalf("want empty no-error, got %+v err=%v", got, err)
	}
}

func TestForPolicy_OwnWorkspaceNotFound_ReturnsEmptyNoError(t *testing.T) {
	c := newFakeClient(t) // no workspaces
	l := consolidation.NewK8sWorkspaceLister(c, "demo")
	got, err := l.ForPolicy(context.Background(), "p1")
	if err != nil || len(got) != 0 {
		t.Fatalf("not-found must be non-fatal: got %+v err=%v", got, err)
	}
}

func TestForPolicy_DoesNotSeeOtherWorkspaces(t *testing.T) {
	// A different workspace opts into p1; ours does not exist -> empty.
	c := newFakeClient(t, wsOptingInto("other", "p1"))
	l := consolidation.NewK8sWorkspaceLister(c, "demo")
	got, err := l.ForPolicy(context.Background(), "p1")
	if err != nil || len(got) != 0 {
		t.Fatalf("must not enumerate other workspaces: got %+v err=%v", got, err)
	}
}

// TestForPolicy_EmptyOwnWorkspace_ReturnsEmptyWithoutClientCall verifies the
// ownWorkspace=="" guard short-circuits before ever calling the client — an
// interceptor that fails the test on Get proves the branch never falls through.
func TestForPolicy_EmptyOwnWorkspace_ReturnsEmptyWithoutClientCall(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := memoryv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cw client.WithWatch, key client.ObjectKey,
				obj client.Object, opts ...client.GetOption) error {
				t.Fatal("Get must not be called when ownWorkspace is empty")
				return nil
			},
		}).
		Build()

	l := consolidation.NewK8sWorkspaceLister(c, "")
	got, err := l.ForPolicy(context.Background(), "p1")
	if err != nil || len(got) != 0 {
		t.Fatalf("want empty no-error, got %+v err=%v", got, err)
	}
}

// TestForPolicy_GetError_ReturnsWrappedError verifies a non-NotFound Get
// error is wrapped and returned (not swallowed like IsNotFound is).
func TestForPolicy_GetError_ReturnsWrappedError(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := memoryv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cw client.WithWatch, key client.ObjectKey,
				obj client.Object, opts ...client.GetOption) error {
				return errors.New("boom")
			},
		}).
		Build()

	l := consolidation.NewK8sWorkspaceLister(c, "demo")
	got, err := l.ForPolicy(context.Background(), "p1")
	if err == nil {
		t.Fatalf("want error, got nil (got=%+v)", got)
	}
	if !strings.Contains(err.Error(), "demo") {
		t.Fatalf("want error to mention workspace name %q, got: %v", "demo", err)
	}
}
