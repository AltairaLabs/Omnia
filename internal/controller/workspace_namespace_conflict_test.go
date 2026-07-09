/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// fakeNamespaceReconciler builds a WorkspaceReconciler backed by a fake client
// seeded with objs, for exercising reconcileNamespace in isolation (#1821).
func fakeNamespaceReconciler(t *testing.T, objs ...client.Object) *WorkspaceReconciler {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, omniav1alpha1.AddToScheme(s))
	cl := fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
	return &WorkspaceReconciler{Client: cl, Scheme: s}
}

func conflictWorkspace(wsName, nsName string, create bool) *omniav1alpha1.Workspace {
	return &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: wsName},
		Spec: omniav1alpha1.WorkspaceSpec{
			Namespace: omniav1alpha1.NamespaceConfig{Name: nsName, Create: create},
		},
	}
}

func ownedNamespace(nsName, owner string) *corev1.Namespace {
	labels := map[string]string{}
	if owner != "" {
		labels[labelWorkspace] = owner
	}
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: nsName, Labels: labels},
	}
}

// TestReconcileNamespace_RejectsForeignOwner: an existing namespace already
// owned by a different Workspace is not adopted — the guard returns
// errNamespaceConflict and leaves the owner label untouched.
func TestReconcileNamespace_RejectsForeignOwner(t *testing.T) {
	r := fakeNamespaceReconciler(t, ownedNamespace("my-team", "my-team"))
	ws := conflictWorkspace("customer-support", "my-team", false)

	err := r.reconcileNamespace(context.Background(), ws)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errNamespaceConflict), "want errNamespaceConflict, got %v", err)
	assert.Contains(t, err.Error(), "my-team")

	// Owner label must NOT have been overwritten to the intruder.
	ns := &corev1.Namespace{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "my-team"}, ns))
	assert.Equal(t, "my-team", ns.Labels[labelWorkspace])
}

// TestReconcileNamespace_AdoptsSelfOwned: a namespace already owned by this
// Workspace reconciles cleanly (idempotent re-apply).
func TestReconcileNamespace_AdoptsSelfOwned(t *testing.T) {
	r := fakeNamespaceReconciler(t, ownedNamespace("my-team", "my-team"))
	ws := conflictWorkspace("my-team", "my-team", false)

	err := r.reconcileNamespace(context.Background(), ws)
	require.NoError(t, err)
	require.NotNil(t, ws.Status.Namespace)
	assert.False(t, ws.Status.Namespace.Created)
}

// TestReconcileNamespace_AdoptsUnowned: a pre-existing namespace with no owner
// label (e.g. a namespace.create=false install) is adopted and stamped.
func TestReconcileNamespace_AdoptsUnowned(t *testing.T) {
	r := fakeNamespaceReconciler(t, ownedNamespace("shared", ""))
	ws := conflictWorkspace("my-team", "shared", false)

	err := r.reconcileNamespace(context.Background(), ws)
	require.NoError(t, err)

	ns := &corev1.Namespace{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "shared"}, ns))
	assert.Equal(t, "my-team", ns.Labels[labelWorkspace])
}

// TestReconcileNamespace_CreatesWhenMissing: create=true provisions the namespace
// with the owner label.
func TestReconcileNamespace_CreatesWhenMissing(t *testing.T) {
	r := fakeNamespaceReconciler(t)
	ws := conflictWorkspace("my-team", "my-team", true)

	err := r.reconcileNamespace(context.Background(), ws)
	require.NoError(t, err)
	require.NotNil(t, ws.Status.Namespace)
	assert.True(t, ws.Status.Namespace.Created)

	ns := &corev1.Namespace{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "my-team"}, ns))
	assert.Equal(t, "my-team", ns.Labels[labelWorkspace])
}

// TestNamespaceConditionReason maps the sentinel (including %w-wrapped) to
// NamespaceConflict and every other error to NamespaceFailed.
func TestNamespaceConditionReason(t *testing.T) {
	assert.Equal(t, "NamespaceConflict", namespaceConditionReason(errNamespaceConflict))
	assert.Equal(t, "NamespaceConflict",
		namespaceConditionReason(fmt.Errorf("namespace foo: %w", errNamespaceConflict)))
	assert.Equal(t, "NamespaceFailed", namespaceConditionReason(errors.New("some other failure")))
}
