/*
Copyright 2026 Altaira Labs.

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

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func testScheme() *k8sruntime.Scheme {
	s := k8sruntime.NewScheme()
	_ = omniav1alpha1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)
	_ = rbacv1.AddToScheme(s)
	return s
}

func newTestWorkspace(name, namespace string, services []omniav1alpha1.WorkspaceServiceGroup) *omniav1alpha1.Workspace {
	return &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			UID:  types.UID("test-uid-" + name),
		},
		Spec: omniav1alpha1.WorkspaceSpec{
			Namespace: omniav1alpha1.NamespaceConfig{
				Name:   namespace,
				Create: true,
			},
			Services: services,
		},
	}
}

func newTestReconciler(scheme *k8sruntime.Scheme, objs ...k8sruntime.Object) *WorkspaceReconciler {
	clientObjs := make([]k8sruntime.Object, len(objs))
	copy(clientObjs, objs)
	cl := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(clientObjs...).Build()
	sb := &ServiceBuilder{
		SessionImage:           "ghcr.io/altairalabs/omnia-session-api:test",
		SessionImagePullPolicy: corev1.PullIfNotPresent,
		MemoryImage:            "ghcr.io/altairalabs/omnia-memory-api:test",
		MemoryImagePullPolicy:  corev1.PullIfNotPresent,
	}
	return &WorkspaceReconciler{
		Client:                          cl,
		Scheme:                          scheme,
		ServiceBuilder:                  sb,
		AgentWorkspaceReaderClusterRole: "omnia-agent-workspace-reader",
	}
}

func TestReconcileServices_ManagedCreatesDeploymentsAndServices(t *testing.T) {
	g := NewWithT(t)
	scheme := testScheme()

	ws := newTestWorkspace("myws", "myws-ns", []omniav1alpha1.WorkspaceServiceGroup{
		{
			Name: "primary",
			Mode: omniav1alpha1.ServiceModeManaged,
			Memory: &omniav1alpha1.MemoryServiceConfig{
				Database: omniav1alpha1.DatabaseConfig{
					SecretRef: corev1.LocalObjectReference{Name: "db-secret"},
				},
			},
			Session: &omniav1alpha1.SessionServiceConfig{
				Database: omniav1alpha1.DatabaseConfig{
					SecretRef: corev1.LocalObjectReference{Name: "db-secret"},
				},
			},
		},
	})

	// Create the namespace so the fake client can create namespaced resources
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "myws-ns"}}
	r := newTestReconciler(scheme, ws, ns)

	ctx := context.Background()
	err := r.reconcileServices(ctx, ws)
	g.Expect(err).NotTo(HaveOccurred())

	// Verify session deployment exists
	sessionDep := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: "session-myws-primary", Namespace: "myws-ns"}, sessionDep)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(sessionDep.Labels[labelComponent]).To(Equal("session-api"))
	g.Expect(sessionDep.Labels[labelWorkspace]).To(Equal("myws"))
	g.Expect(sessionDep.Labels[labelServiceGroup]).To(Equal("primary"))

	// Verify memory deployment exists
	memoryDep := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: "memory-myws-primary", Namespace: "myws-ns"}, memoryDep)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(memoryDep.Labels[labelComponent]).To(Equal("memory-api"))

	// Verify session service exists
	sessionSvc := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: "session-myws-primary", Namespace: "myws-ns"}, sessionSvc)
	g.Expect(err).NotTo(HaveOccurred())

	// Verify memory service exists
	memorySvc := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: "memory-myws-primary", Namespace: "myws-ns"}, memorySvc)
	g.Expect(err).NotTo(HaveOccurred())

	// Verify status
	g.Expect(ws.Status.Services).To(HaveLen(1))
	g.Expect(ws.Status.Services[0].Name).To(Equal("primary"))
	g.Expect(ws.Status.Services[0].SessionURL).To(Equal("http://session-myws-primary.myws-ns:8080"))
	g.Expect(ws.Status.Services[0].MemoryURL).To(Equal("http://memory-myws-primary.myws-ns:8080"))
	// Not ready because fake client doesn't set ReadyReplicas
	g.Expect(ws.Status.Services[0].Ready).To(BeFalse())
}

func TestReconcileServices_ManagedUpdatesExistingResources(t *testing.T) {
	g := NewWithT(t)
	scheme := testScheme()

	ws := newTestWorkspace("myws", "myws-ns", []omniav1alpha1.WorkspaceServiceGroup{
		{
			Name: "primary",
			Mode: omniav1alpha1.ServiceModeManaged,
			Memory: &omniav1alpha1.MemoryServiceConfig{
				Database: omniav1alpha1.DatabaseConfig{
					SecretRef: corev1.LocalObjectReference{Name: "db-secret"},
				},
			},
			Session: &omniav1alpha1.SessionServiceConfig{
				Database: omniav1alpha1.DatabaseConfig{
					SecretRef: corev1.LocalObjectReference{Name: "db-secret"},
				},
			},
		},
	})

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "myws-ns"}}
	r := newTestReconciler(scheme, ws, ns)
	ctx := context.Background()

	// First reconcile creates resources
	err := r.reconcileServices(ctx, ws)
	g.Expect(err).NotTo(HaveOccurred())

	// Second reconcile updates existing resources (no error)
	err = r.reconcileServices(ctx, ws)
	g.Expect(err).NotTo(HaveOccurred())

	// Still one session deployment
	sessionDep := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: "session-myws-primary", Namespace: "myws-ns"}, sessionDep)
	g.Expect(err).NotTo(HaveOccurred())
}

func TestReconcileServices_ExternalWritesURLsToStatus(t *testing.T) {
	g := NewWithT(t)
	scheme := testScheme()

	ws := newTestWorkspace("myws", "myws-ns", []omniav1alpha1.WorkspaceServiceGroup{
		{
			Name: "external-group",
			Mode: omniav1alpha1.ServiceModeExternal,
			External: &omniav1alpha1.ExternalEndpoints{
				SessionURL: "https://session.example.com",
				MemoryURL:  "https://memory.example.com",
			},
		},
	})

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "myws-ns"}}
	r := newTestReconciler(scheme, ws, ns)

	ctx := context.Background()
	err := r.reconcileServices(ctx, ws)
	g.Expect(err).NotTo(HaveOccurred())

	// Verify no deployments were created
	depList := &appsv1.DeploymentList{}
	err = r.List(ctx, depList)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(depList.Items).To(BeEmpty())

	// Verify status
	g.Expect(ws.Status.Services).To(HaveLen(1))
	g.Expect(ws.Status.Services[0].Name).To(Equal("external-group"))
	g.Expect(ws.Status.Services[0].SessionURL).To(Equal("https://session.example.com"))
	g.Expect(ws.Status.Services[0].MemoryURL).To(Equal("https://memory.example.com"))
	g.Expect(ws.Status.Services[0].Ready).To(BeTrue())
}

func TestReconcileServices_NoServicesBlock(t *testing.T) {
	g := NewWithT(t)
	scheme := testScheme()

	ws := newTestWorkspace("myws", "myws-ns", nil)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "myws-ns"}}
	r := newTestReconciler(scheme, ws, ns)

	ctx := context.Background()
	err := r.reconcileServices(ctx, ws)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ws.Status.Services).To(BeEmpty())
}

func TestReconcileServices_CleanupRemovedGroups(t *testing.T) {
	g := NewWithT(t)
	scheme := testScheme()

	ws := newTestWorkspace("myws", "myws-ns", []omniav1alpha1.WorkspaceServiceGroup{
		{
			Name: "primary",
			Mode: omniav1alpha1.ServiceModeManaged,
			Memory: &omniav1alpha1.MemoryServiceConfig{
				Database: omniav1alpha1.DatabaseConfig{
					SecretRef: corev1.LocalObjectReference{Name: "db-secret"},
				},
			},
			Session: &omniav1alpha1.SessionServiceConfig{
				Database: omniav1alpha1.DatabaseConfig{
					SecretRef: corev1.LocalObjectReference{Name: "db-secret"},
				},
			},
		},
	})

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "myws-ns"}}
	r := newTestReconciler(scheme, ws, ns)
	ctx := context.Background()

	// First reconcile creates resources
	err := r.reconcileServices(ctx, ws)
	g.Expect(err).NotTo(HaveOccurred())

	// Verify resources exist
	sessionDep := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: "session-myws-primary", Namespace: "myws-ns"}, sessionDep)
	g.Expect(err).NotTo(HaveOccurred())

	// Remove the service group from spec
	ws.Spec.Services = nil

	// Reconcile again
	err = r.reconcileServices(ctx, ws)
	g.Expect(err).NotTo(HaveOccurred())

	// Verify deployments and services are deleted
	err = r.Get(ctx, types.NamespacedName{Name: "session-myws-primary", Namespace: "myws-ns"}, &appsv1.Deployment{})
	g.Expect(err).To(HaveOccurred()) // NotFound

	err = r.Get(ctx, types.NamespacedName{Name: "memory-myws-primary", Namespace: "myws-ns"}, &appsv1.Deployment{})
	g.Expect(err).To(HaveOccurred()) // NotFound

	err = r.Get(ctx, types.NamespacedName{Name: "session-myws-primary", Namespace: "myws-ns"}, &corev1.Service{})
	g.Expect(err).To(HaveOccurred()) // NotFound

	err = r.Get(ctx, types.NamespacedName{Name: "memory-myws-primary", Namespace: "myws-ns"}, &corev1.Service{})
	g.Expect(err).To(HaveOccurred()) // NotFound

	g.Expect(ws.Status.Services).To(BeEmpty())
}

func TestReconcileServices_MixedManagedAndExternal(t *testing.T) {
	g := NewWithT(t)
	scheme := testScheme()

	ws := newTestWorkspace("myws", "myws-ns", []omniav1alpha1.WorkspaceServiceGroup{
		{
			Name: "managed-group",
			Mode: omniav1alpha1.ServiceModeManaged,
			Memory: &omniav1alpha1.MemoryServiceConfig{
				Database: omniav1alpha1.DatabaseConfig{
					SecretRef: corev1.LocalObjectReference{Name: "db-secret"},
				},
			},
			Session: &omniav1alpha1.SessionServiceConfig{
				Database: omniav1alpha1.DatabaseConfig{
					SecretRef: corev1.LocalObjectReference{Name: "db-secret"},
				},
			},
		},
		{
			Name: "ext-group",
			Mode: omniav1alpha1.ServiceModeExternal,
			External: &omniav1alpha1.ExternalEndpoints{
				SessionURL: "https://session.ext.com",
				MemoryURL:  "https://memory.ext.com",
			},
		},
	})

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "myws-ns"}}
	r := newTestReconciler(scheme, ws, ns)

	ctx := context.Background()
	err := r.reconcileServices(ctx, ws)
	g.Expect(err).NotTo(HaveOccurred())

	// Verify managed resources exist
	sessionDep := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: "session-myws-managed-group", Namespace: "myws-ns"}, sessionDep)
	g.Expect(err).NotTo(HaveOccurred())

	// Verify status has both groups
	g.Expect(ws.Status.Services).To(HaveLen(2))

	// Find managed group status
	var managedStatus, extStatus omniav1alpha1.ServiceGroupStatus
	for _, s := range ws.Status.Services {
		if s.Name == "managed-group" {
			managedStatus = s
		}
		if s.Name == "ext-group" {
			extStatus = s
		}
	}

	g.Expect(managedStatus.SessionURL).To(Equal("http://session-myws-managed-group.myws-ns:8080"))
	g.Expect(extStatus.SessionURL).To(Equal("https://session.ext.com"))
	g.Expect(extStatus.Ready).To(BeTrue())
}

func TestReconcileServices_DifferentWorkspaceName(t *testing.T) {
	g := NewWithT(t)
	scheme := testScheme()

	ws := newTestWorkspace("other-ws", "other-ns", []omniav1alpha1.WorkspaceServiceGroup{
		{
			Name: "default",
			Mode: omniav1alpha1.ServiceModeExternal,
			External: &omniav1alpha1.ExternalEndpoints{
				SessionURL: "https://session.other.com",
				MemoryURL:  "https://memory.other.com",
			},
		},
	})

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "other-ns"}}
	r := newTestReconciler(scheme, ws, ns)

	ctx := context.Background()
	err := r.reconcileServices(ctx, ws)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ws.Status.Services).To(HaveLen(1))
	g.Expect(ws.Status.Services[0].SessionURL).To(Equal("https://session.other.com"))
}

func TestIsDeploymentReady(t *testing.T) {
	g := NewWithT(t)
	scheme := testScheme()

	// Create a deployment with ReadyReplicas > 0
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dep",
			Namespace: "test-ns",
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: 1,
		},
	}
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}}
	r := newTestReconciler(scheme, dep, ns)

	ctx := context.Background()
	g.Expect(r.isDeploymentReady(ctx, "test-dep", "test-ns")).To(BeTrue())
	g.Expect(r.isDeploymentReady(ctx, "nonexistent", "test-ns")).To(BeFalse())
}

func TestSetServicesReadyCondition_AllReady(t *testing.T) {
	g := NewWithT(t)
	var conditions []metav1.Condition
	services := []omniav1alpha1.ServiceGroupStatus{
		{Name: "a", Ready: true},
		{Name: "b", Ready: true},
	}
	setServicesReadyCondition(&conditions, 1, services)
	g.Expect(conditions).To(HaveLen(1))
	g.Expect(conditions[0].Type).To(Equal(ConditionTypeServicesReady))
	g.Expect(conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(conditions[0].Reason).To(Equal("ServicesReady"))
}

func TestSetServicesReadyCondition_NotReady(t *testing.T) {
	g := NewWithT(t)
	var conditions []metav1.Condition
	services := []omniav1alpha1.ServiceGroupStatus{
		{Name: "a", Ready: true},
		{Name: "b", Ready: false},
	}
	setServicesReadyCondition(&conditions, 1, services)
	g.Expect(conditions).To(HaveLen(1))
	g.Expect(conditions[0].Type).To(Equal(ConditionTypeServicesReady))
	g.Expect(conditions[0].Status).To(Equal(metav1.ConditionFalse))
	g.Expect(conditions[0].Reason).To(Equal("ServicesNotReady"))
}

func TestSetServicesReadyCondition_Empty(t *testing.T) {
	g := NewWithT(t)
	var conditions []metav1.Condition
	setServicesReadyCondition(&conditions, 1, nil)
	g.Expect(conditions).To(BeEmpty())
}

func TestSetReconcileError(t *testing.T) {
	g := NewWithT(t)
	scheme := testScheme()

	ws := newTestWorkspace("myws", "myws-ns", nil)
	r := newTestReconciler(scheme, ws)

	ctx := context.Background()
	testErr := fmt.Errorf("something failed")
	log := logf.FromContext(ctx)

	result, err := r.setReconcileError(ctx, ws, ConditionTypeServicesReady, "ServicesFailed", testErr, log)
	g.Expect(err).To(MatchError("something failed"))
	g.Expect(result).To(Equal(ctrl.Result{}))
	g.Expect(ws.Status.Phase).To(Equal(omniav1alpha1.WorkspacePhaseError))

	// Condition should be set on the workspace
	var found bool
	for _, c := range ws.Status.Conditions {
		if c.Type == ConditionTypeServicesReady {
			g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(c.Reason).To(Equal("ServicesFailed"))
			found = true
		}
	}
	g.Expect(found).To(BeTrue())
}
