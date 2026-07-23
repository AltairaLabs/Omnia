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
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// authTestScheme extends the base test scheme with networking + the
// PeerAuthentication unstructured GVK so the fake client can persist them.
func authTestScheme() *k8sruntime.Scheme {
	s := testScheme()
	_ = networkingv1.AddToScheme(s)
	s.AddKnownTypeWithName(peerAuthenticationGVK, &unstructured.Unstructured{})
	listGVK := peerAuthenticationGVK
	listGVK.Kind += "List"
	s.AddKnownTypeWithName(listGVK, &unstructured.UnstructuredList{})
	return s
}

func newAuthReconciler(t *testing.T, auth ServiceAuthConfig) *WorkspaceReconciler {
	t.Helper()
	scheme := authTestScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	sb := &ServiceBuilder{
		SessionImage:           "ghcr.io/altairalabs/omnia-session-api:test",
		SessionImagePullPolicy: corev1.PullIfNotPresent,
		MemoryImage:            "ghcr.io/altairalabs/omnia-memory-api:test",
		MemoryImagePullPolicy:  corev1.PullIfNotPresent,
		ServiceAuth:            auth,
	}
	return &WorkspaceReconciler{
		Client:                           cl,
		Scheme:                           scheme,
		ServiceBuilder:                   sb,
		WorkspaceReaderRBACEnabled:       true,
		OperatorNamespace:                "omnia-system",
		SessionAPITokenReviewClusterRole: "omnia-session-api-tokenreview",
	}
}

func TestNetworkHardening_DisabledIsNoop(t *testing.T) {
	g := NewWithT(t)
	r := newAuthReconciler(t, ServiceAuthConfig{Enabled: false})
	ws := newTestWorkspace("acme", testAuthNS, nil)

	g.Expect(r.reconcileServiceAuthNetworkHardening(context.Background(), ws, testAuthNS)).To(Succeed())

	np := &networkingv1.NetworkPolicy{}
	err := r.Get(context.Background(), types.NamespacedName{Name: testServiceAuthNetpolName, Namespace: testAuthNS}, np)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
}

func TestNetworkHardening_CreatesDefaultDenyNetworkPolicy(t *testing.T) {
	g := NewWithT(t)
	r := newAuthReconciler(t, ServiceAuthConfig{Enabled: true, Audience: testAudience})
	ws := newTestWorkspace("acme", testAuthNS, nil)
	ctx := context.Background()

	g.Expect(r.reconcileServiceAuthNetworkHardening(ctx, ws, testAuthNS)).To(Succeed())

	np := &networkingv1.NetworkPolicy{}
	g.Expect(r.Get(ctx, types.NamespacedName{Name: testServiceAuthNetpolName, Namespace: testAuthNS}, np)).To(Succeed())
	g.Expect(np.Spec.PolicyTypes).To(ContainElement(networkingv1.PolicyTypeIngress))
	g.Expect(np.Spec.PodSelector.MatchLabels).To(HaveKeyWithValue(labelWorkspace, "acme"))
	g.Expect(np.Spec.Ingress).To(HaveLen(1))
	// same-namespace + operator-namespace peers.
	g.Expect(np.Spec.Ingress[0].From).To(HaveLen(2))

	// No PeerAuthentication when Istio mTLS is off.
	pa := &unstructured.Unstructured{}
	pa.SetGroupVersionKind(peerAuthenticationGVK)
	err := r.Get(ctx, types.NamespacedName{Name: testServiceAuthNetpolName, Namespace: testAuthNS}, pa)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
}

func TestNetworkHardening_IdempotentUpdate(t *testing.T) {
	g := NewWithT(t)
	r := newAuthReconciler(t, ServiceAuthConfig{Enabled: true, Audience: testAudience})
	ws := newTestWorkspace("acme", testAuthNS, nil)
	ctx := context.Background()

	g.Expect(r.reconcileServiceAuthNetworkHardening(ctx, ws, testAuthNS)).To(Succeed())
	// Second pass hits the update branch and must not error.
	g.Expect(r.reconcileServiceAuthNetworkHardening(ctx, ws, testAuthNS)).To(Succeed())
}

func TestNetworkHardening_NoOperatorNamespace(t *testing.T) {
	g := NewWithT(t)
	r := newAuthReconciler(t, ServiceAuthConfig{Enabled: true})
	r.OperatorNamespace = ""
	ws := newTestWorkspace("acme", testAuthNS, nil)
	ctx := context.Background()

	g.Expect(r.reconcileServiceAuthNetworkHardening(ctx, ws, testAuthNS)).To(Succeed())
	np := &networkingv1.NetworkPolicy{}
	g.Expect(r.Get(ctx, types.NamespacedName{Name: testServiceAuthNetpolName, Namespace: testAuthNS}, np)).To(Succeed())
	g.Expect(np.Spec.Ingress[0].From).To(HaveLen(1))
}

func TestNetworkHardening_IstioPeerAuthentication(t *testing.T) {
	g := NewWithT(t)
	r := newAuthReconciler(t, ServiceAuthConfig{Enabled: true, IstioMTLS: true})
	ws := newTestWorkspace("acme", testAuthNS, nil)
	ctx := context.Background()

	g.Expect(r.reconcileServiceAuthNetworkHardening(ctx, ws, testAuthNS)).To(Succeed())

	pa := &unstructured.Unstructured{}
	pa.SetGroupVersionKind(peerAuthenticationGVK)
	g.Expect(r.Get(ctx, types.NamespacedName{Name: testServiceAuthNetpolName, Namespace: testAuthNS}, pa)).To(Succeed())
	mode, found, err := unstructured.NestedString(pa.Object, "spec", "mtls", "mode")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(mode).To(Equal("STRICT"))

	// Idempotent second pass (update branch).
	g.Expect(r.reconcileServiceAuthNetworkHardening(ctx, ws, testAuthNS)).To(Succeed())
}

func TestTokenReviewBinding_DisabledIsNoop(t *testing.T) {
	g := NewWithT(t)
	r := newAuthReconciler(t, ServiceAuthConfig{Enabled: false})

	g.Expect(r.reconcileSessionAPITokenReviewBinding(context.Background(), testAuthNS, testSessionSAName)).To(Succeed())
	assertNoTokenReviewBinding(t, r.Client)
}

func TestTokenReviewBinding_NoClusterRoleNameIsNoop(t *testing.T) {
	g := NewWithT(t)
	r := newAuthReconciler(t, ServiceAuthConfig{Enabled: true})
	r.SessionAPITokenReviewClusterRole = ""

	g.Expect(r.reconcileSessionAPITokenReviewBinding(context.Background(), testAuthNS, testSessionSAName)).To(Succeed())
	assertNoTokenReviewBinding(t, r.Client)
}

func TestTokenReviewBinding_CreatesClusterRoleBinding(t *testing.T) {
	g := NewWithT(t)
	r := newAuthReconciler(t, ServiceAuthConfig{Enabled: true})
	ctx := context.Background()

	g.Expect(r.reconcileSessionAPITokenReviewBinding(ctx, testAuthNS, testSessionSAName)).To(Succeed())

	crb := &rbacv1.ClusterRoleBinding{}
	g.Expect(r.Get(ctx, types.NamespacedName{Name: "session-tokenreview-acme-ns-session-acme-default"}, crb)).To(Succeed())
	g.Expect(crb.RoleRef.Name).To(Equal("omnia-session-api-tokenreview"))
	g.Expect(crb.Subjects).To(HaveLen(1))
	g.Expect(crb.Subjects[0].Name).To(Equal(testSessionSAName))
	g.Expect(crb.Subjects[0].Namespace).To(Equal(testAuthNS))

	// Idempotent: second call must not error (binding already exists).
	g.Expect(r.reconcileSessionAPITokenReviewBinding(ctx, testAuthNS, testSessionSAName)).To(Succeed())
}

func assertNoTokenReviewBinding(t *testing.T, cl client.Client) {
	t.Helper()
	g := NewWithT(t)
	crb := &rbacv1.ClusterRoleBinding{}
	err := cl.Get(context.Background(), types.NamespacedName{Name: "session-tokenreview-acme-ns-session-acme-default"}, crb)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
}
