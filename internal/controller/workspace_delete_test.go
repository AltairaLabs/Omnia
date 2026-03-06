/*
Copyright 2025.

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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func newDeleteTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = networkingv1.AddToScheme(s)
	_ = rbacv1.AddToScheme(s)
	return s
}

const (
	testWSName = "test-ws"
	testWSNS   = "ws-namespace"
)

func wsLabels(wsName string) map[string]string {
	return map[string]string{
		labelWorkspace:        wsName,
		labelWorkspaceManaged: labelValueTrue,
	}
}

func TestReconcileDelete_FinalizerRetainedOnError(t *testing.T) {
	scheme := newDeleteTestScheme()
	wsName := testWSName
	ns := testWSNS

	workspace := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:              wsName,
			Finalizers:        []string{WorkspaceFinalizerName},
			DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
		},
		Spec: omniav1alpha1.WorkspaceSpec{
			Namespace: omniav1alpha1.NamespaceConfig{
				Name: ns,
			},
		},
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: ns,
			Labels:    wsLabels(wsName),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(workspace, pvc).
		WithStatusSubresource(workspace).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				if _, ok := obj.(*corev1.PersistentVolumeClaim); ok {
					return fmt.Errorf("simulated delete failure")
				}
				return c.Delete(ctx, obj, opts...)
			},
		}).
		Build()

	r := &WorkspaceReconciler{Client: fakeClient, Scheme: scheme}
	result, err := r.reconcileDelete(context.Background(), workspace)

	// Should return error and requeue
	assert.Error(t, err)
	assert.NotZero(t, result.RequeueAfter)

	// Finalizer should still be present
	updatedWS := &omniav1alpha1.Workspace{}
	require.NoError(t, fakeClient.Get(context.Background(), types.NamespacedName{Name: wsName}, updatedWS))
	assert.Contains(t, updatedWS.Finalizers, WorkspaceFinalizerName)
}

func TestReconcileDelete_FinalizerRemovedOnSuccess(t *testing.T) {
	scheme := newDeleteTestScheme()
	wsName := testWSName
	ns := testWSNS

	workspace := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:              wsName,
			Finalizers:        []string{WorkspaceFinalizerName},
			DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
		},
		Spec: omniav1alpha1.WorkspaceSpec{
			Namespace: omniav1alpha1.NamespaceConfig{
				Name: ns,
			},
		},
	}

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sa",
			Namespace: ns,
			Labels:    wsLabels(wsName),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(workspace, sa).
		WithStatusSubresource(workspace).
		Build()

	r := &WorkspaceReconciler{Client: fakeClient, Scheme: scheme}
	result, err := r.reconcileDelete(context.Background(), workspace)

	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Finalizer should be removed from the workspace object
	assert.NotContains(t, workspace.Finalizers, WorkspaceFinalizerName)

	// ServiceAccount should be deleted
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "test-sa", Namespace: ns}, &corev1.ServiceAccount{})
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcileDelete_NamespaceDeletedIfCreated(t *testing.T) {
	scheme := newDeleteTestScheme()
	wsName := testWSName
	ns := testWSNS

	workspace := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:              wsName,
			Finalizers:        []string{WorkspaceFinalizerName},
			DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
		},
		Spec: omniav1alpha1.WorkspaceSpec{
			Namespace: omniav1alpha1.NamespaceConfig{
				Name: ns,
			},
		},
		Status: omniav1alpha1.WorkspaceStatus{
			Namespace: &omniav1alpha1.NamespaceStatus{
				Name:    ns,
				Created: true,
			},
		},
	}

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   ns,
			Labels: map[string]string{labelWorkspace: wsName},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(workspace, namespace).
		WithStatusSubresource(workspace).
		Build()

	r := &WorkspaceReconciler{Client: fakeClient, Scheme: scheme}
	_, err := r.reconcileDelete(context.Background(), workspace)
	assert.NoError(t, err)

	// Namespace should be deleted
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: ns}, &corev1.Namespace{})
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcileDelete_RetainsStorageOnRetainPolicy(t *testing.T) {
	scheme := newDeleteTestScheme()
	wsName := testWSName
	ns := testWSNS

	workspace := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:              wsName,
			Finalizers:        []string{WorkspaceFinalizerName},
			DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
		},
		Spec: omniav1alpha1.WorkspaceSpec{
			Namespace: omniav1alpha1.NamespaceConfig{Name: ns},
			Storage: &omniav1alpha1.WorkspaceStorageConfig{
				RetentionPolicy: "Retain",
			},
		},
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "retained-pvc",
			Namespace: ns,
			Labels:    wsLabels(wsName),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(workspace, pvc).
		WithStatusSubresource(workspace).
		Build()

	r := &WorkspaceReconciler{Client: fakeClient, Scheme: scheme}
	_, err := r.reconcileDelete(context.Background(), workspace)
	assert.NoError(t, err)

	// PVC should still exist (retained)
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "retained-pvc", Namespace: ns}, &corev1.PersistentVolumeClaim{})
	assert.NoError(t, err)
}

func TestReconcileDelete_NamespaceRetainedIfLabelMismatch(t *testing.T) {
	scheme := newDeleteTestScheme()
	wsName := testWSName
	ns := testWSNS

	workspace := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:              wsName,
			Finalizers:        []string{WorkspaceFinalizerName},
			DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
		},
		Spec: omniav1alpha1.WorkspaceSpec{
			Namespace: omniav1alpha1.NamespaceConfig{
				Name: ns,
			},
		},
		Status: omniav1alpha1.WorkspaceStatus{
			Namespace: &omniav1alpha1.NamespaceStatus{
				Name:    ns,
				Created: true,
			},
		},
	}

	// Namespace exists but has a different workspace label
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   ns,
			Labels: map[string]string{labelWorkspace: "other-workspace"},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(workspace, namespace).
		WithStatusSubresource(workspace).
		Build()

	r := &WorkspaceReconciler{Client: fakeClient, Scheme: scheme}
	_, err := r.reconcileDelete(context.Background(), workspace)
	assert.NoError(t, err)

	// Namespace should NOT be deleted (label mismatch protects it)
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: ns}, &corev1.Namespace{})
	assert.NoError(t, err)
}

func TestReconcileDelete_NamespaceNotCreatedSkipsDelete(t *testing.T) {
	scheme := newDeleteTestScheme()
	wsName := testWSName
	ns := testWSNS

	workspace := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:              wsName,
			Finalizers:        []string{WorkspaceFinalizerName},
			DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
		},
		Spec: omniav1alpha1.WorkspaceSpec{
			Namespace: omniav1alpha1.NamespaceConfig{Name: ns},
		},
		// No Status.Namespace — namespace was not created by the controller
	}

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   ns,
			Labels: map[string]string{labelWorkspace: wsName},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(workspace, namespace).
		WithStatusSubresource(workspace).
		Build()

	r := &WorkspaceReconciler{Client: fakeClient, Scheme: scheme}
	_, err := r.reconcileDelete(context.Background(), workspace)
	assert.NoError(t, err)

	// Namespace should still exist (controller didn't create it)
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: ns}, &corev1.Namespace{})
	assert.NoError(t, err)
}

func TestDeletePageSize(t *testing.T) {
	assert.Equal(t, 100, deletePageSize)
}
