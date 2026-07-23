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
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func testScheme() *k8sruntime.Scheme {
	s := k8sruntime.NewScheme()
	_ = omniav1alpha1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)
	_ = networkingv1.AddToScheme(s)
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
		Client:                     cl,
		Scheme:                     scheme,
		ServiceBuilder:             sb,
		WorkspaceReaderRBACEnabled: true,
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

// TestReconcileServices_ServicePodBindsPerWorkspaceReader asserts that the
// per-workspace service pods (session-api, memory-api) bind the get-only
// per-workspace Workspace reader ClusterRole (resourceNames-scoped to their own
// workspace), NOT the cluster-wide agent-workspace-reader. A pod in workspace
// "demo" must not be able to enumerate the config of other workspaces (#1899).
// The workspace is named "demo" and owns the distinct namespace "omnia-demo"
// (the #1875 name/namespace convention).
func TestReconcileServices_ServicePodBindsPerWorkspaceReader(t *testing.T) {
	g := NewWithT(t)
	scheme := testScheme()

	ws := newTestWorkspace("demo", "omnia-demo", []omniav1alpha1.WorkspaceServiceGroup{
		{
			Name: "default",
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

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "omnia-demo"}}
	r := newTestReconciler(scheme, ws, ns)
	r.WorkspaceReaderRBACEnabled = true

	ctx := context.Background()
	g.Expect(r.reconcileServices(ctx, ws)).To(Succeed())

	crb := &rbacv1.ClusterRoleBinding{}
	key := client.ObjectKey{Name: fmt.Sprintf("service-%s-%s", "omnia-demo", "memory-demo-default")}
	g.Expect(r.Get(ctx, key, crb)).To(Succeed())
	g.Expect(crb.RoleRef.Name).To(Equal(WorkspaceReaderClusterRoleName("demo")))
	g.Expect(crb.RoleRef.Name).NotTo(Equal("omnia-agent-workspace-reader"),
		"service pods must no longer bind the cluster-wide reader")
	g.Expect(crb.Subjects).To(HaveLen(1))
	g.Expect(crb.Subjects[0].Name).To(Equal("memory-demo-default"))
	g.Expect(crb.Subjects[0].Namespace).To(Equal("omnia-demo"))
}

// TestReconcileServices_GroupRedisWiredIntoDeployments is a WIRING test: it
// drives the full operator path (reconcileServices → reconcileManagedServiceGroup
// → ServiceBuilder.Build*Deployment → client Create) and asserts the group-level
// redis lands as --redis-url on BOTH the created session-api and memory-api pods.
//
// The builder unit tests prove Build*Deployment reads sg.Redis in isolation; this
// proves the reconciler actually passes sg (with its Redis block) through to the
// builder and persists the result — the "is it hooked up end to end" guarantee.
// Uses serviceRef with an explicit namespace so the assertion also covers
// cross-namespace resolution flowing through the reconciler.
func TestReconcileServices_GroupRedisWiredIntoDeployments(t *testing.T) {
	g := NewWithT(t)
	scheme := testScheme()

	ws := newTestWorkspace("myws", "myws-ns", []omniav1alpha1.WorkspaceServiceGroup{
		{
			Name: "primary",
			Mode: omniav1alpha1.ServiceModeManaged,
			Redis: &omniav1alpha1.RedisConfig{
				ServiceRef: &omniav1alpha1.RedisServiceRef{Name: "redis", Namespace: "cache"},
			},
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
	err := r.reconcileServices(ctx, ws)
	g.Expect(err).NotTo(HaveOccurred())

	const wantArg = "--redis-url=redis://redis.cache:6379"

	sessionDep := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: "session-myws-primary", Namespace: "myws-ns"}, sessionDep)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(sessionDep.Spec.Template.Spec.Containers).To(HaveLen(1))
	g.Expect(sessionDep.Spec.Template.Spec.Containers[0].Args).To(ContainElement(wantArg),
		"group-level redis must be wired into the session-api pod via the reconciler")

	memoryDep := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: "memory-myws-primary", Namespace: "myws-ns"}, memoryDep)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(memoryDep.Spec.Template.Spec.Containers).To(HaveLen(1))
	g.Expect(memoryDep.Spec.Template.Spec.Containers[0].Args).To(ContainElement(wantArg),
		"group-level redis must be wired into the memory-api pod via the reconciler")
}

// TestReconcileServices_TokenReviewBindingsForBothSAs is a WIRING test: it drives
// the full operator path (reconcileServices → reconcileManagedServiceGroup) with
// internal service auth enabled and asserts that BOTH the session-api AND the
// memory-api ServiceAccounts get a ClusterRoleBinding to the install-wide
// tokenreview ClusterRole. memory-api validates caller tokens via the Kubernetes
// TokenReview API just like session-api; without this binding its TokenReview
// calls 403 and it rejects all callers. The session binding was already wired;
// this proves the memory binding is too (call site in reconcileManagedServiceGroup).
func TestReconcileServices_TokenReviewBindingsForBothSAs(t *testing.T) {
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
	r.ServiceBuilder.ServiceAuth = ServiceAuthConfig{Enabled: true}
	r.SessionAPITokenReviewClusterRole = "omnia-session-api-tokenreview"

	ctx := context.Background()
	err := r.reconcileServices(ctx, ws)
	g.Expect(err).NotTo(HaveOccurred())

	// session-api SA must be bound to the tokenreview ClusterRole.
	sessionCRB := &rbacv1.ClusterRoleBinding{}
	err = r.Get(ctx, types.NamespacedName{Name: "session-tokenreview-myws-ns-session-myws-primary"}, sessionCRB)
	g.Expect(err).NotTo(HaveOccurred(), "session-api SA must be bound to the tokenreview ClusterRole")
	g.Expect(sessionCRB.RoleRef.Name).To(Equal("omnia-session-api-tokenreview"))
	g.Expect(sessionCRB.Subjects).To(HaveLen(1))
	g.Expect(sessionCRB.Subjects[0].Name).To(Equal("session-myws-primary"))

	// memory-api SA must ALSO be bound to the tokenreview ClusterRole (#1730).
	memoryCRB := &rbacv1.ClusterRoleBinding{}
	err = r.Get(ctx, types.NamespacedName{Name: "session-tokenreview-myws-ns-memory-myws-primary"}, memoryCRB)
	g.Expect(err).NotTo(HaveOccurred(), "memory-api SA must be bound to the tokenreview ClusterRole")
	g.Expect(memoryCRB.RoleRef.Name).To(Equal("omnia-session-api-tokenreview"))
	g.Expect(memoryCRB.Subjects).To(HaveLen(1))
	g.Expect(memoryCRB.Subjects[0].Name).To(Equal("memory-myws-primary"))
	g.Expect(memoryCRB.Subjects[0].Namespace).To(Equal("myws-ns"))
}

// TestReconcileServices_BindingsTargetOverriddenServiceAccount verifies that
// when a service group overrides its pod ServiceAccount (e.g. memory-api runs
// as a Workload-Identity SA for keyless embeddings), the tokenreview and
// enterprise-reader ClusterRoleBindings target the OVERRIDDEN SA the pod
// actually runs as — not the default per-deployment SA, which the pod never
// uses. Binding the default SA leaves the real SA without the grant, failing
// service auth (TokenReview 401) and the privacy-policy watcher closed (#1817).
func TestReconcileServices_BindingsTargetOverriddenServiceAccount(t *testing.T) {
	g := NewWithT(t)
	scheme := testScheme()

	const wiSA = "omnia-runtime-wi"
	ws := newTestWorkspace("myws", "myws-ns", []omniav1alpha1.WorkspaceServiceGroup{
		{
			Name: "primary",
			Mode: omniav1alpha1.ServiceModeManaged,
			Memory: &omniav1alpha1.MemoryServiceConfig{
				Database: omniav1alpha1.DatabaseConfig{
					SecretRef: corev1.LocalObjectReference{Name: "db-secret"},
				},
				PodOverrides: &omniav1alpha1.PodOverrides{ServiceAccountName: wiSA},
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
	r.ServiceBuilder.ServiceAuth = ServiceAuthConfig{Enabled: true}
	r.SessionAPITokenReviewClusterRole = "omnia-session-api-tokenreview"
	r.MemoryEnterpriseReaderClusterRole = "omnia-memory-enterprise-reader"

	ctx := context.Background()
	err := r.reconcileServices(ctx, ws)
	g.Expect(err).NotTo(HaveOccurred())

	// memory-api's tokenreview binding must target the overridden WI SA, keyed
	// by that SA name — NOT the default memory-myws-primary SA.
	memoryTR := &rbacv1.ClusterRoleBinding{}
	err = r.Get(ctx, types.NamespacedName{Name: "session-tokenreview-myws-ns-" + wiSA}, memoryTR)
	g.Expect(err).NotTo(HaveOccurred(), "tokenreview binding must target the overridden memory-api SA")
	g.Expect(memoryTR.Subjects).To(HaveLen(1))
	g.Expect(memoryTR.Subjects[0].Name).To(Equal(wiSA))
	g.Expect(memoryTR.Subjects[0].Namespace).To(Equal("myws-ns"))

	// ...and its enterprise-reader binding likewise targets the WI SA.
	memoryER := &rbacv1.ClusterRoleBinding{}
	err = r.Get(ctx, types.NamespacedName{Name: "memory-enterprise-reader-myws-ns-" + wiSA}, memoryER)
	g.Expect(err).NotTo(HaveOccurred(), "enterprise-reader binding must target the overridden memory-api SA")
	g.Expect(memoryER.Subjects[0].Name).To(Equal(wiSA))

	// No binding must be created against the unused default memory SA.
	staleTR := &rbacv1.ClusterRoleBinding{}
	err = r.Get(ctx, types.NamespacedName{Name: "session-tokenreview-myws-ns-memory-myws-primary"}, staleTR)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "no tokenreview binding against the unused default memory SA")

	// session-api keeps its default SA (no override), so its binding is unchanged.
	sessionTR := &rbacv1.ClusterRoleBinding{}
	err = r.Get(ctx, types.NamespacedName{Name: "session-tokenreview-myws-ns-session-myws-primary"}, sessionTR)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(sessionTR.Subjects[0].Name).To(Equal("session-myws-primary"))
}

// TestReconcileServices_MemoryTokenReviewBindingCreateError verifies that when
// creating the memory-api ClusterRoleBinding fails (e.g. the API server
// rejects it), reconcileManagedServiceGroup surfaces the error wrapped with
// "memory tokenreview binding" rather than swallowing it. The fake client's
// interceptor is used to fail only the memory-api binding's Create call so
// the session-api binding (created first) succeeds, proving the error path
// is specific to the memory call site added in #1730.
func TestReconcileServices_MemoryTokenReviewBindingCreateError(t *testing.T) {
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

	wantErr := fmt.Errorf("simulated apiserver rejection")
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(ws, ns).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if crb, ok := obj.(*rbacv1.ClusterRoleBinding); ok &&
					crb.Name == "session-tokenreview-myws-ns-memory-myws-primary" {
					return wantErr
				}
				return c.Create(ctx, obj, opts...)
			},
		}).
		Build()

	sb := &ServiceBuilder{
		SessionImage:           "ghcr.io/altairalabs/omnia-session-api:test",
		SessionImagePullPolicy: corev1.PullIfNotPresent,
		MemoryImage:            "ghcr.io/altairalabs/omnia-memory-api:test",
		MemoryImagePullPolicy:  corev1.PullIfNotPresent,
		ServiceAuth:            ServiceAuthConfig{Enabled: true},
	}
	r := &WorkspaceReconciler{
		Client:                           cl,
		Scheme:                           scheme,
		ServiceBuilder:                   sb,
		WorkspaceReaderRBACEnabled:       true,
		SessionAPITokenReviewClusterRole: "omnia-session-api-tokenreview",
	}

	ctx := context.Background()
	err := r.reconcileServices(ctx, ws)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("memory tokenreview binding"))
	g.Expect(err.Error()).To(ContainSubstring(wantErr.Error()))

	// The session-api binding (created before the memory one) must still have
	// succeeded — proving the injected failure is specific to the memory CRB.
	sessionCRB := &rbacv1.ClusterRoleBinding{}
	getErr := r.Get(ctx, types.NamespacedName{Name: "session-tokenreview-myws-ns-session-myws-primary"}, sessionCRB)
	g.Expect(getErr).NotTo(HaveOccurred())
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

// TestReconcilePrivacyService_CreatesDeploymentAndService verifies that when
// workspace.Spec.Privacy is set and PrivacyImage is configured, the reconciler
// creates the privacy-<ws> Deployment, Service, and ServiceAccount, and sets
// Status.PrivacyURL to the expected in-cluster URL.
func TestReconcilePrivacyService_CreatesDeploymentAndService(t *testing.T) {
	g := NewWithT(t)
	scheme := testScheme()

	ws := newTestWorkspace("myws", "myws-ns", nil)
	ws.Spec.Privacy = &omniav1alpha1.PrivacyServiceConfig{
		Database: omniav1alpha1.DatabaseConfig{
			SecretRef: corev1.LocalObjectReference{Name: "privacy-db"},
		},
	}

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "myws-ns"}}
	r := newTestReconciler(scheme, ws, ns)
	// Set a privacy image so the gate passes.
	r.ServiceBuilder.PrivacyImage = "ghcr.io/altairalabs/omnia-privacy-api:test"
	r.ServiceBuilder.PrivacyImagePullPolicy = corev1.PullIfNotPresent

	ctx := context.Background()
	err := r.reconcileServices(ctx, ws)
	g.Expect(err).NotTo(HaveOccurred())

	// Deployment should exist with the correct name.
	privDep := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: testPrivacyDeployName, Namespace: "myws-ns"}, privDep)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(privDep.Labels[labelComponent]).To(Equal("privacy-api"))
	g.Expect(privDep.Labels[labelWorkspace]).To(Equal("myws"))
	g.Expect(privDep.Labels[labelServiceGroup]).To(BeEmpty(),
		"privacy deployment must have empty service-group label (per-workspace deployment)")

	// Service should exist.
	privSvc := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: testPrivacyDeployName, Namespace: "myws-ns"}, privSvc)
	g.Expect(err).NotTo(HaveOccurred())

	// ServiceAccount should exist.
	privSA := &corev1.ServiceAccount{}
	err = r.Get(ctx, types.NamespacedName{Name: testPrivacyDeployName, Namespace: "myws-ns"}, privSA)
	g.Expect(err).NotTo(HaveOccurred())

	// PrivacyURL must be set.
	g.Expect(ws.Status.PrivacyURL).To(Equal("http://privacy-myws.myws-ns:8080"))
}

// TestReconcilePrivacyService_NilPrivacySpec verifies that when
// workspace.Spec.Privacy is nil, no privacy resources are created and
// Status.PrivacyURL is empty.
func TestReconcilePrivacyService_NilPrivacySpec(t *testing.T) {
	g := NewWithT(t)
	scheme := testScheme()

	ws := newTestWorkspace("myws", "myws-ns", nil)
	// Spec.Privacy is nil — no privacy-api should be deployed.

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "myws-ns"}}
	r := newTestReconciler(scheme, ws, ns)
	r.ServiceBuilder.PrivacyImage = "ghcr.io/altairalabs/omnia-privacy-api:test"

	ctx := context.Background()
	err := r.reconcileServices(ctx, ws)
	g.Expect(err).NotTo(HaveOccurred())

	// No privacy deployment should exist.
	privDep := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: testPrivacyDeployName, Namespace: "myws-ns"}, privDep)
	g.Expect(err).To(HaveOccurred(), "no privacy deployment should be created when Spec.Privacy is nil")

	// PrivacyURL must be cleared.
	g.Expect(ws.Status.PrivacyURL).To(BeEmpty())
}

// TestReconcilePrivacyService_RemovalTearsDownResources verifies that when
// workspace.Spec.Privacy is removed from a live Workspace, the operator
// deletes all privacy-<ws> resources: Deployment, Service, ServiceAccount,
// and both ClusterRoleBindings (tokenreview + enterprise-reader).
func TestReconcilePrivacyService_RemovalTearsDownResources(t *testing.T) {
	g := NewWithT(t)
	scheme := testScheme()

	ws := newTestWorkspace("myws", "myws-ns", nil)
	ws.Spec.Privacy = &omniav1alpha1.PrivacyServiceConfig{
		Database: omniav1alpha1.DatabaseConfig{
			SecretRef: corev1.LocalObjectReference{Name: "privacy-db"},
		},
	}

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "myws-ns"}}
	r := newTestReconciler(scheme, ws, ns)
	r.ServiceBuilder.PrivacyImage = "ghcr.io/altairalabs/omnia-privacy-api:test"
	r.ServiceBuilder.PrivacyImagePullPolicy = corev1.PullIfNotPresent
	r.ServiceBuilder.ServiceAuth = ServiceAuthConfig{Enabled: true}
	r.SessionAPITokenReviewClusterRole = "omnia-session-tokenreview"
	r.MemoryEnterpriseReaderClusterRole = "omnia-enterprise-reader"

	ctx := context.Background()

	// Phase 1: reconcile with Privacy set all resources should exist.
	err := r.reconcileServices(ctx, ws)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(r.Get(ctx, types.NamespacedName{Name: testPrivacyDeployName, Namespace: "myws-ns"}, &appsv1.Deployment{})).To(Succeed())
	g.Expect(r.Get(ctx, types.NamespacedName{Name: testPrivacyDeployName, Namespace: "myws-ns"}, &corev1.Service{})).To(Succeed())
	g.Expect(r.Get(ctx, types.NamespacedName{Name: testPrivacyDeployName, Namespace: "myws-ns"}, &corev1.ServiceAccount{})).To(Succeed())

	tokenReviewCRBName := "session-tokenreview-myws-ns-" + testPrivacyDeployName
	g.Expect(r.Get(ctx, types.NamespacedName{Name: tokenReviewCRBName}, &rbacv1.ClusterRoleBinding{})).To(Succeed())

	enterpriseReaderCRBName := "memory-enterprise-reader-myws-ns-" + testPrivacyDeployName
	g.Expect(r.Get(ctx, types.NamespacedName{Name: enterpriseReaderCRBName}, &rbacv1.ClusterRoleBinding{})).To(Succeed())

	// Phase 2: remove Spec.Privacy and reconcile again.
	ws.Spec.Privacy = nil
	err = r.reconcileServices(ctx, ws)
	g.Expect(err).NotTo(HaveOccurred())

	// All privacy resources must be gone.
	g.Expect(r.Get(ctx, types.NamespacedName{Name: testPrivacyDeployName, Namespace: "myws-ns"}, &appsv1.Deployment{})).
		To(MatchError(ContainSubstring("not found")), "privacy Deployment must be deleted")

	g.Expect(r.Get(ctx, types.NamespacedName{Name: testPrivacyDeployName, Namespace: "myws-ns"}, &corev1.Service{})).
		To(MatchError(ContainSubstring("not found")), "privacy Service must be deleted")

	g.Expect(r.Get(ctx, types.NamespacedName{Name: testPrivacyDeployName, Namespace: "myws-ns"}, &corev1.ServiceAccount{})).
		To(MatchError(ContainSubstring("not found")), "privacy ServiceAccount must be deleted")

	g.Expect(r.Get(ctx, types.NamespacedName{Name: tokenReviewCRBName}, &rbacv1.ClusterRoleBinding{})).
		To(MatchError(ContainSubstring("not found")), "privacy tokenreview CRB must be deleted")

	g.Expect(r.Get(ctx, types.NamespacedName{Name: enterpriseReaderCRBName}, &rbacv1.ClusterRoleBinding{})).
		To(MatchError(ContainSubstring("not found")), "privacy enterprise-reader CRB must be deleted")

	// PrivacyURL must be cleared.
	g.Expect(ws.Status.PrivacyURL).To(BeEmpty())
}

// TestReconcilePrivacyService_ImageAbsentNoTeardown verifies that when
// workspace.Spec.Privacy is set but PrivacyImage is empty (operator
// misconfiguration), reconcilePrivacyService skips resource creation without
// error and clears Status.PrivacyURL. It does NOT tear down existing resources
// — an operator upgrade must not destroy a running deployment.
func TestReconcilePrivacyService_ImageAbsentNoTeardown(t *testing.T) {
	g := NewWithT(t)
	scheme := testScheme()

	ws := newTestWorkspace("myws", "myws-ns", nil)
	ws.Spec.Privacy = &omniav1alpha1.PrivacyServiceConfig{
		Database: omniav1alpha1.DatabaseConfig{
			SecretRef: corev1.LocalObjectReference{Name: "privacy-db"},
		},
	}

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "myws-ns"}}
	r := newTestReconciler(scheme, ws, ns)
	// PrivacyImage intentionally left empty — operator is misconfigured.
	r.ServiceBuilder.PrivacyImage = ""

	ctx := context.Background()
	err := r.reconcilePrivacyService(ctx, ws)
	g.Expect(err).NotTo(HaveOccurred())

	// No privacy deployment should be created.
	privDep := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: testPrivacyDeployName, Namespace: "myws-ns"}, privDep)
	g.Expect(err).To(HaveOccurred(), "no privacy deployment should be created when PrivacyImage is empty")

	// PrivacyURL must be cleared.
	g.Expect(ws.Status.PrivacyURL).To(BeEmpty())
}

// TestReconcilePrivacyService_CleanupError verifies that when cleanupPrivacyService
// returns an error (e.g. Delete returns non-NotFound), reconcilePrivacyService
// propagates it. This covers the `return err` branch in the nil-privacy path.
func TestReconcilePrivacyService_CleanupError(t *testing.T) {
	g := NewWithT(t)
	scheme := testScheme()

	fc := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.DeleteOption) error {
				return apierrors.NewServiceUnavailable("delete-unavailable")
			},
		}).
		Build()

	r := &WorkspaceReconciler{Client: fc, Scheme: scheme}
	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "myws"},
		Spec: omniav1alpha1.WorkspaceSpec{
			Namespace: omniav1alpha1.NamespaceConfig{Name: "ns"},
		},
	}
	// Spec.Privacy is nil so reconcilePrivacyService will call cleanupPrivacyService,
	// which will hit the intercepted Delete and return an error.
	ws.Spec.Privacy = nil

	ctx := context.Background()
	err := r.reconcilePrivacyService(ctx, ws)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("cleanup privacy resource"))
}

// TestCleanupPrivacyService_DeleteError verifies that when a Delete call
// returns a non-NotFound error, cleanupPrivacyService propagates it wrapped
// with "cleanup privacy resource".
func TestCleanupPrivacyService_DeleteError(t *testing.T) {
	g := NewWithT(t)
	scheme := testScheme()

	fc := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.DeleteOption) error {
				return apierrors.NewServiceUnavailable("boom")
			},
		}).
		Build()

	r := &WorkspaceReconciler{Client: fc, Scheme: scheme}
	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "myws"},
		Spec: omniav1alpha1.WorkspaceSpec{
			Namespace: omniav1alpha1.NamespaceConfig{Name: "ns"},
		},
	}

	ctx := context.Background()
	err := r.cleanupPrivacyService(ctx, ws)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("cleanup privacy resource"))
}
