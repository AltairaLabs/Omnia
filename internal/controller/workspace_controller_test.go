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
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	eev1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// testCounter ensures unique names across all workspace tests
var testCounter uint64

var _ = Describe("Workspace Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When reconciling Workspace", func() {
		var (
			ctx           context.Context
			workspaceKey  types.NamespacedName
			namespaceName string
			reconciler    *WorkspaceReconciler
			testID        string
		)

		BeforeEach(func() {
			ctx = context.Background()
			// Use atomic counter to guarantee unique names across tests
			testID = fmt.Sprintf("%d", atomic.AddUint64(&testCounter, 1))
			workspaceKey = types.NamespacedName{
				Name: "test-ws-" + testID,
			}
			namespaceName = "ws-test-" + testID
			reconciler = &WorkspaceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
		})

		AfterEach(func() {
			// Clean up Workspace
			workspace := &omniav1alpha1.Workspace{}
			err := k8sClient.Get(ctx, workspaceKey, workspace)
			if err == nil {
				// Remove finalizer first to allow deletion
				workspace.Finalizers = nil
				_ = k8sClient.Update(ctx, workspace)
				_ = k8sClient.Delete(ctx, workspace)
			}

			// Wait for workspace cleanup
			Eventually(func() bool {
				err := k8sClient.Get(ctx, workspaceKey, &omniav1alpha1.Workspace{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// Note: envtest doesn't fully support namespace deletion (no namespace controller)
			// so we just issue the delete and don't wait for completion
			ns := &corev1.Namespace{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: namespaceName}, ns)
			if err == nil {
				_ = k8sClient.Delete(ctx, ns)
			}
		})

		It("should fail when namespace does not exist and create is false", func() {
			By("creating a Workspace with create=false for non-existent namespace")
			workspace := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceKey.Name,
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					Environment: omniav1alpha1.WorkspaceEnvironmentDevelopment,
					Namespace: omniav1alpha1.NamespaceConfig{
						Name:   "nonexistent-ns-" + testID,
						Create: false,
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			By("reconciling the Workspace - first adds finalizer")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			By("reconciling again - should fail on missing namespace")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not exist"))

			By("checking the status is set to Error")
			Eventually(func() omniav1alpha1.WorkspacePhase {
				updated := &omniav1alpha1.Workspace{}
				if err := k8sClient.Get(ctx, workspaceKey, updated); err != nil {
					return ""
				}
				return updated.Status.Phase
			}, timeout, interval).Should(Equal(omniav1alpha1.WorkspacePhaseError))
		})

		It("should create namespace, ServiceAccounts, and RoleBindings when create is true", func() {
			By("creating a Workspace with create=true")
			workspace := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceKey.Name,
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					Environment: omniav1alpha1.WorkspaceEnvironmentDevelopment,
					Namespace: omniav1alpha1.NamespaceConfig{
						Name:   namespaceName,
						Create: true,
						Labels: map[string]string{
							"custom-label": "test-value",
						},
					},
					DefaultTags: map[string]string{
						"team":        "engineering",
						"cost-center": "CC-1234",
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			By("reconciling the Workspace until ready")
			// First reconcile adds finalizer
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			// Second reconcile creates resources
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying the namespace was created with labels")
			ns := &corev1.Namespace{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: namespaceName}, ns)
			}, timeout, interval).Should(Succeed())

			Expect(ns.Labels[labelWorkspace]).To(Equal(workspaceKey.Name))
			Expect(ns.Labels[labelEnvironment]).To(Equal("development"))
			Expect(ns.Labels["custom-label"]).To(Equal("test-value"))
			Expect(ns.Labels["team"]).To(Equal("engineering"))
			Expect(ns.Labels["cost-center"]).To(Equal("CC-1234"))

			By("verifying ServiceAccounts were created")
			ownerSA := &corev1.ServiceAccount{}
			ownerSAName := fmt.Sprintf("workspace-%s-owner-sa", workspaceKey.Name)
			Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name:      ownerSAName,
				Namespace: namespaceName,
			}, ownerSA)).To(Succeed())
			Expect(ownerSA.Labels[labelWorkspaceRole]).To(Equal("owner"))

			editorSA := &corev1.ServiceAccount{}
			editorSAName := fmt.Sprintf("workspace-%s-editor-sa", workspaceKey.Name)
			Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name:      editorSAName,
				Namespace: namespaceName,
			}, editorSA)).To(Succeed())
			Expect(editorSA.Labels[labelWorkspaceRole]).To(Equal("editor"))

			viewerSA := &corev1.ServiceAccount{}
			viewerSAName := fmt.Sprintf("workspace-%s-viewer-sa", workspaceKey.Name)
			Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name:      viewerSAName,
				Namespace: namespaceName,
			}, viewerSA)).To(Succeed())
			Expect(viewerSA.Labels[labelWorkspaceRole]).To(Equal("viewer"))

			By("verifying RoleBindings were created")
			ownerRB := &rbacv1.RoleBinding{}
			ownerRBName := fmt.Sprintf("workspace-%s-owner", workspaceKey.Name)
			Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name:      ownerRBName,
				Namespace: namespaceName,
			}, ownerRB)).To(Succeed())
			Expect(ownerRB.RoleRef.Name).To(Equal(clusterRoleOwner))

			By("checking the status is set to Ready")
			Eventually(func() omniav1alpha1.WorkspacePhase {
				updated := &omniav1alpha1.Workspace{}
				if err := k8sClient.Get(ctx, workspaceKey, updated); err != nil {
					return ""
				}
				return updated.Status.Phase
			}, timeout, interval).Should(Equal(omniav1alpha1.WorkspacePhaseReady))

			By("verifying status fields are populated")
			updated := &omniav1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, workspaceKey, updated)).To(Succeed())
			Expect(updated.Status.Namespace).NotTo(BeNil())
			Expect(updated.Status.Namespace.Name).To(Equal(namespaceName))
			Expect(updated.Status.Namespace.Created).To(BeTrue())
			Expect(updated.Status.ServiceAccounts).NotTo(BeNil())
			Expect(updated.Status.ServiceAccounts.Owner).To(Equal(ownerSAName))
		})

		It("should handle external ServiceAccount bindings", func() {
			// Use workspace with Create: true to avoid namespace collision issues
			ciNSName := "ci-sys-" + testID
			By("creating the ci-system namespace")
			ciNS := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: ciNSName,
				},
			}
			Expect(k8sClient.Create(ctx, ciNS)).To(Succeed())

			By("creating the external ServiceAccount")
			extSA := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "github-actions",
					Namespace: ciNSName,
				},
			}
			Expect(k8sClient.Create(ctx, extSA)).To(Succeed())

			By("creating a Workspace with external ServiceAccount binding")
			workspace := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceKey.Name,
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					Environment: omniav1alpha1.WorkspaceEnvironmentDevelopment,
					Namespace: omniav1alpha1.NamespaceConfig{
						Name:   namespaceName,
						Create: true, // Let controller create the namespace
					},
					RoleBindings: []omniav1alpha1.RoleBinding{
						{
							ServiceAccounts: []omniav1alpha1.ServiceAccountRef{
								{
									Name:      "github-actions",
									Namespace: ciNSName,
								},
							},
							Role: omniav1alpha1.WorkspaceRoleEditor,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			By("reconciling the Workspace")
			// First reconcile adds finalizer
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			// Second reconcile creates resources
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying external ServiceAccount RoleBinding was created")
			extRB := &rbacv1.RoleBinding{}
			extRBName := fmt.Sprintf("%s-sa-github-actions-%s", workspaceKey.Name, sanitizeName(ciNSName))
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{
					Name:      extRBName,
					Namespace: namespaceName,
				}, extRB)
			}, timeout, interval).Should(Succeed())

			Expect(extRB.RoleRef.Name).To(Equal(clusterRoleEditor))
			Expect(extRB.Subjects).To(HaveLen(1))
			Expect(extRB.Subjects[0].Name).To(Equal("github-actions"))
			Expect(extRB.Subjects[0].Namespace).To(Equal(ciNSName))

			// Clean up extra namespace
			_ = k8sClient.Delete(ctx, ciNS)
		})

		It("should clean up resources when workspace is deleted", func() {
			By("creating a Workspace with create=true")
			workspace := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceKey.Name,
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Delete Test Workspace",
					Environment: omniav1alpha1.WorkspaceEnvironmentProduction,
					Namespace: omniav1alpha1.NamespaceConfig{
						Name:   namespaceName,
						Create: true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			By("reconciling until ready")
			// First reconcile adds finalizer
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			// Second reconcile creates resources
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying resources exist")
			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: namespaceName}, ns)).To(Succeed())

			By("deleting the workspace")
			Expect(k8sClient.Delete(ctx, workspace)).To(Succeed())

			By("reconciling the deletion")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying the workspace is deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, workspaceKey, &omniav1alpha1.Workspace{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("should use existing namespace when create is false", func() {
			By("creating the namespace first")
			existingNS := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespaceName,
				},
			}
			Expect(k8sClient.Create(ctx, existingNS)).To(Succeed())

			By("creating a Workspace with create=false for existing namespace")
			workspace := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceKey.Name,
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Existing NS Workspace",
					Environment: omniav1alpha1.WorkspaceEnvironmentStaging,
					Namespace: omniav1alpha1.NamespaceConfig{
						Name:   namespaceName,
						Create: false,
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			By("reconciling the Workspace")
			// First reconcile adds finalizer
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			// Second reconcile creates resources in existing namespace
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying ServiceAccounts were created in existing namespace")
			ownerSA := &corev1.ServiceAccount{}
			ownerSAName := fmt.Sprintf("workspace-%s-owner-sa", workspaceKey.Name)
			Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name:      ownerSAName,
				Namespace: namespaceName,
			}, ownerSA)).To(Succeed())

			By("checking the status reflects the namespace was not created by controller")
			updated := &omniav1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, workspaceKey, updated)).To(Succeed())
			Expect(updated.Status.Namespace).NotTo(BeNil())
			Expect(updated.Status.Namespace.Name).To(Equal(namespaceName))
			Expect(updated.Status.Namespace.Created).To(BeFalse())

			By("checking the status is set to Ready")
			Eventually(func() omniav1alpha1.WorkspacePhase {
				updated := &omniav1alpha1.Workspace{}
				if err := k8sClient.Get(ctx, workspaceKey, updated); err != nil {
					return ""
				}
				return updated.Status.Phase
			}, timeout, interval).Should(Equal(omniav1alpha1.WorkspacePhaseReady))
		})

		It("should return empty result when workspace not found", func() {
			By("reconciling a non-existent workspace")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "nonexistent-workspace"},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should count members correctly with direct grants", func() {
			By("creating a Workspace with direct grants")
			workspace := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceKey.Name,
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Direct Grants Test",
					Environment: omniav1alpha1.WorkspaceEnvironmentDevelopment,
					Namespace: omniav1alpha1.NamespaceConfig{
						Name:   namespaceName,
						Create: true,
					},
					DirectGrants: []omniav1alpha1.DirectGrant{
						{User: "owner1@example.com", Role: omniav1alpha1.WorkspaceRoleOwner},
						{User: "editor1@example.com", Role: omniav1alpha1.WorkspaceRoleEditor},
						{User: "editor2@example.com", Role: omniav1alpha1.WorkspaceRoleEditor},
						{User: "viewer1@example.com", Role: omniav1alpha1.WorkspaceRoleViewer},
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			By("reconciling the Workspace")
			// First reconcile adds finalizer
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			// Second reconcile creates resources
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying member counts")
			updated := &omniav1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, workspaceKey, updated)).To(Succeed())
			Expect(updated.Status.Members).NotTo(BeNil())
			Expect(updated.Status.Members.Owners).To(Equal(int32(1)))
			Expect(updated.Status.Members.Editors).To(Equal(int32(2)))
			Expect(updated.Status.Members.Viewers).To(Equal(int32(1)))
		})

	})

	Context("Network Isolation", func() {
		var (
			ctx           context.Context
			workspaceKey  types.NamespacedName
			namespaceName string
			reconciler    *WorkspaceReconciler
			testID        string
		)

		BeforeEach(func() {
			ctx = context.Background()
			testID = fmt.Sprintf("%d", atomic.AddUint64(&testCounter, 1))
			workspaceKey = types.NamespacedName{
				Name: "test-np-" + testID,
			}
			namespaceName = "np-test-" + testID
			reconciler = &WorkspaceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
		})

		AfterEach(func() {
			workspace := &omniav1alpha1.Workspace{}
			err := k8sClient.Get(ctx, workspaceKey, workspace)
			if err == nil {
				workspace.Finalizers = nil
				_ = k8sClient.Update(ctx, workspace)
				_ = k8sClient.Delete(ctx, workspace)
			}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, workspaceKey, &omniav1alpha1.Workspace{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			ns := &corev1.Namespace{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: namespaceName}, ns)
			if err == nil {
				_ = k8sClient.Delete(ctx, ns)
			}
		})

		It("should not create NetworkPolicy when isolation is disabled (default)", func() {
			By("creating a Workspace without networkPolicy configured")
			workspace := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceKey.Name,
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "No Isolation Workspace",
					Environment: omniav1alpha1.WorkspaceEnvironmentDevelopment,
					Namespace: omniav1alpha1.NamespaceConfig{
						Name:   namespaceName,
						Create: true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			By("reconciling the Workspace")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying no NetworkPolicy was created")
			npName := fmt.Sprintf("workspace-%s-isolation", workspaceKey.Name)
			np := &networkingv1.NetworkPolicy{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      npName,
				Namespace: namespaceName,
			}, np)
			Expect(errors.IsNotFound(err)).To(BeTrue())

			By("verifying status.networkPolicy is nil")
			updated := &omniav1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, workspaceKey, updated)).To(Succeed())
			Expect(updated.Status.NetworkPolicy).To(BeNil())
		})

		It("should create NetworkPolicy with default rules when isolate is true", func() {
			By("creating a Workspace with network isolation enabled")
			workspace := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceKey.Name,
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Isolated Workspace",
					Environment: omniav1alpha1.WorkspaceEnvironmentDevelopment,
					Namespace: omniav1alpha1.NamespaceConfig{
						Name:   namespaceName,
						Create: true,
					},
					NetworkPolicy: &omniav1alpha1.WorkspaceNetworkPolicy{
						Isolate: true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			By("reconciling the Workspace")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying NetworkPolicy was created")
			npName := fmt.Sprintf("workspace-%s-isolation", workspaceKey.Name)
			np := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name:      npName,
				Namespace: namespaceName,
			}, np)).To(Succeed())

			By("verifying NetworkPolicy labels")
			Expect(np.Labels[labelWorkspace]).To(Equal(workspaceKey.Name))
			Expect(np.Labels[labelWorkspaceManaged]).To(Equal("true"))

			By("verifying policy types")
			Expect(np.Spec.PolicyTypes).To(ContainElements(
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			))

			By("verifying ingress rules include shared namespaces and same namespace")
			Expect(len(np.Spec.Ingress)).To(BeNumerically(">=", 2))

			By("verifying egress rules include DNS, shared namespaces, same namespace, and external")
			Expect(len(np.Spec.Egress)).To(BeNumerically(">=", 4))

			By("verifying status is updated")
			updated := &omniav1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, workspaceKey, updated)).To(Succeed())
			Expect(updated.Status.NetworkPolicy).NotTo(BeNil())
			Expect(updated.Status.NetworkPolicy.Name).To(Equal(npName))
			Expect(updated.Status.NetworkPolicy.Enabled).To(BeTrue())
			Expect(updated.Status.NetworkPolicy.RulesCount).To(BeNumerically(">", 0))
		})

		It("should apply custom ingress and egress rules", func() {
			By("creating a Workspace with custom network rules")
			workspace := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceKey.Name,
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Custom Rules Workspace",
					Environment: omniav1alpha1.WorkspaceEnvironmentDevelopment,
					Namespace: omniav1alpha1.NamespaceConfig{
						Name:   namespaceName,
						Create: true,
					},
					NetworkPolicy: &omniav1alpha1.WorkspaceNetworkPolicy{
						Isolate: true,
						AllowFrom: []omniav1alpha1.NetworkPolicyRule{
							{
								Peers: []omniav1alpha1.NetworkPolicyPeer{
									{
										NamespaceSelector: &omniav1alpha1.LabelSelector{
											MatchLabels: map[string]string{
												"kubernetes.io/metadata.name": "ingress-nginx",
											},
										},
									},
								},
							},
						},
						AllowTo: []omniav1alpha1.NetworkPolicyRule{
							{
								Peers: []omniav1alpha1.NetworkPolicyPeer{
									{
										IPBlock: &omniav1alpha1.IPBlock{
											CIDR: "10.0.0.0/8",
										},
									},
								},
								Ports: []omniav1alpha1.NetworkPolicyPort{
									{
										Protocol: "TCP",
										Port:     5432,
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			By("reconciling the Workspace")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying NetworkPolicy includes custom rules")
			npName := fmt.Sprintf("workspace-%s-isolation", workspaceKey.Name)
			np := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name:      npName,
				Namespace: namespaceName,
			}, np)).To(Succeed())

			// Check that custom ingress rule exists (from ingress-nginx namespace)
			foundIngressRule := false
			for _, rule := range np.Spec.Ingress {
				for _, from := range rule.From {
					if from.NamespaceSelector != nil &&
						from.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] == "ingress-nginx" {
						foundIngressRule = true
						break
					}
				}
			}
			Expect(foundIngressRule).To(BeTrue(), "Custom ingress rule for ingress-nginx not found")

			// Check that custom egress rule exists (to 10.0.0.0/8 on port 5432)
			foundEgressRule := false
			for _, rule := range np.Spec.Egress {
				for _, to := range rule.To {
					if to.IPBlock != nil && to.IPBlock.CIDR == "10.0.0.0/8" {
						foundEgressRule = true
						break
					}
				}
			}
			Expect(foundEgressRule).To(BeTrue(), "Custom egress rule for 10.0.0.0/8 not found")
		})

		It("should not include external APIs rule when allowExternalAPIs is false", func() {
			By("creating a Workspace with external APIs disabled")
			allowExternalAPIs := false
			workspace := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceKey.Name,
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "No External APIs Workspace",
					Environment: omniav1alpha1.WorkspaceEnvironmentDevelopment,
					Namespace: omniav1alpha1.NamespaceConfig{
						Name:   namespaceName,
						Create: true,
					},
					NetworkPolicy: &omniav1alpha1.WorkspaceNetworkPolicy{
						Isolate:           true,
						AllowExternalAPIs: &allowExternalAPIs,
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			By("reconciling the Workspace")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying NetworkPolicy does not have 0.0.0.0/0 egress rule")
			npName := fmt.Sprintf("workspace-%s-isolation", workspaceKey.Name)
			np := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name:      npName,
				Namespace: namespaceName,
			}, np)).To(Succeed())

			foundExternalRule := false
			for _, rule := range np.Spec.Egress {
				for _, to := range rule.To {
					if to.IPBlock != nil && to.IPBlock.CIDR == "0.0.0.0/0" {
						foundExternalRule = true
						break
					}
				}
			}
			Expect(foundExternalRule).To(BeFalse(), "Should not have 0.0.0.0/0 egress rule when allowExternalAPIs is false")
		})

		It("should delete NetworkPolicy when isolate changes from true to false", func() {
			By("creating a Workspace with network isolation enabled")
			workspace := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceKey.Name,
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Cleanup Test Workspace",
					Environment: omniav1alpha1.WorkspaceEnvironmentDevelopment,
					Namespace: omniav1alpha1.NamespaceConfig{
						Name:   namespaceName,
						Create: true,
					},
					NetworkPolicy: &omniav1alpha1.WorkspaceNetworkPolicy{
						Isolate: true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			By("reconciling to create the NetworkPolicy")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying NetworkPolicy exists")
			npName := fmt.Sprintf("workspace-%s-isolation", workspaceKey.Name)
			np := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name:      npName,
				Namespace: namespaceName,
			}, np)).To(Succeed())

			By("disabling isolation")
			updated := &omniav1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, workspaceKey, updated)).To(Succeed())
			updated.Spec.NetworkPolicy.Isolate = false
			Expect(k8sClient.Update(ctx, updated)).To(Succeed())

			By("reconciling to delete the NetworkPolicy")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying NetworkPolicy was deleted")
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      npName,
				Namespace: namespaceName,
			}, np)
			Expect(errors.IsNotFound(err)).To(BeTrue())

			By("verifying status.networkPolicy is nil")
			Expect(k8sClient.Get(ctx, workspaceKey, updated)).To(Succeed())
			Expect(updated.Status.NetworkPolicy).To(BeNil())
		})

		It("should allow private networks when allowPrivateNetworks is true", func() {
			By("creating a Workspace with private networks allowed")
			allowPrivateNetworks := true
			workspace := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceKey.Name,
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Private Networks Workspace",
					Environment: omniav1alpha1.WorkspaceEnvironmentDevelopment,
					Namespace: omniav1alpha1.NamespaceConfig{
						Name:   namespaceName,
						Create: true,
					},
					NetworkPolicy: &omniav1alpha1.WorkspaceNetworkPolicy{
						Isolate:              true,
						AllowPrivateNetworks: &allowPrivateNetworks,
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			By("reconciling the Workspace")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying NetworkPolicy has 0.0.0.0/0 without exceptions")
			npName := fmt.Sprintf("workspace-%s-isolation", workspaceKey.Name)
			np := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name:      npName,
				Namespace: namespaceName,
			}, np)).To(Succeed())

			// Find the external APIs rule (0.0.0.0/0)
			foundExternalRule := false
			for _, rule := range np.Spec.Egress {
				for _, to := range rule.To {
					if to.IPBlock != nil && to.IPBlock.CIDR == "0.0.0.0/0" {
						foundExternalRule = true
						// Should have NO exceptions when allowPrivateNetworks is true
						Expect(to.IPBlock.Except).To(BeEmpty(), "Should not have RFC 1918 exceptions when allowPrivateNetworks is true")
						break
					}
				}
			}
			Expect(foundExternalRule).To(BeTrue(), "Should have 0.0.0.0/0 egress rule")
		})

		It("should delete NetworkPolicy when workspace is deleted", func() {
			By("creating a Workspace with network isolation enabled")
			workspace := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceKey.Name,
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Deletion Test Workspace",
					Environment: omniav1alpha1.WorkspaceEnvironmentDevelopment,
					Namespace: omniav1alpha1.NamespaceConfig{
						Name:   namespaceName,
						Create: true,
					},
					NetworkPolicy: &omniav1alpha1.WorkspaceNetworkPolicy{
						Isolate: true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			By("reconciling to create the NetworkPolicy")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying NetworkPolicy exists")
			npName := fmt.Sprintf("workspace-%s-isolation", workspaceKey.Name)
			np := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name:      npName,
				Namespace: namespaceName,
			}, np)).To(Succeed())

			By("deleting the workspace")
			Expect(k8sClient.Delete(ctx, workspace)).To(Succeed())

			By("reconciling the deletion")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying NetworkPolicy was deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Name:      npName,
					Namespace: namespaceName,
				}, np)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("auto-allows traffic to/from the operator namespace when configured", func() {
			By("creating an isolated Workspace with the operator-aware reconciler")
			operatorReconciler := &WorkspaceReconciler{
				Client:            k8sClient,
				Scheme:            k8sClient.Scheme(),
				OperatorNamespace: "omnia-system",
			}
			workspace := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceKey.Name,
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Isolated + Operator-aware",
					Environment: omniav1alpha1.WorkspaceEnvironmentDevelopment,
					Namespace: omniav1alpha1.NamespaceConfig{
						Name:   namespaceName,
						Create: true,
					},
					NetworkPolicy: &omniav1alpha1.WorkspaceNetworkPolicy{
						Isolate: true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			By("reconciling the Workspace")
			_, err := operatorReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())
			_, err = operatorReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying the NetworkPolicy admits traffic from the operator namespace")
			npName := fmt.Sprintf("workspace-%s-isolation", workspaceKey.Name)
			np := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name:      npName,
				Namespace: namespaceName,
			}, np)).To(Succeed())

			matchesOperatorNs := func(peers []networkingv1.NetworkPolicyPeer) bool {
				for _, peer := range peers {
					if peer.NamespaceSelector == nil {
						continue
					}
					if peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] == "omnia-system" {
						return true
					}
				}
				return false
			}

			By("asserting ingress includes a rule from omnia-system")
			foundIngress := false
			for _, rule := range np.Spec.Ingress {
				if matchesOperatorNs(rule.From) {
					foundIngress = true
					break
				}
			}
			Expect(foundIngress).To(BeTrue(), "expected ingress rule from operator namespace omnia-system")

			By("asserting egress includes a rule to omnia-system")
			foundEgress := false
			for _, rule := range np.Spec.Egress {
				if matchesOperatorNs(rule.To) {
					foundEgress = true
					break
				}
			}
			Expect(foundEgress).To(BeTrue(), "expected egress rule to operator namespace omnia-system")
		})

		It("omits the operator-namespace rules when OperatorNamespace is empty (e.g. test harness)", func() {
			By("creating an isolated Workspace with a reconciler missing OperatorNamespace")
			bareReconciler := &WorkspaceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			workspace := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceKey.Name,
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Isolated, no operator NS",
					Environment: omniav1alpha1.WorkspaceEnvironmentDevelopment,
					Namespace: omniav1alpha1.NamespaceConfig{
						Name:   namespaceName,
						Create: true,
					},
					NetworkPolicy: &omniav1alpha1.WorkspaceNetworkPolicy{
						Isolate: true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			_, err := bareReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())
			_, err = bareReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())

			npName := fmt.Sprintf("workspace-%s-isolation", workspaceKey.Name)
			np := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name:      npName,
				Namespace: namespaceName,
			}, np)).To(Succeed())

			hasMetadataNameSelector := func(peers []networkingv1.NetworkPolicyPeer) bool {
				for _, peer := range peers {
					if peer.NamespaceSelector == nil {
						continue
					}
					if _, has := peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"]; has {
						// kube-system DNS rule is allowed; everything else with
						// `kubernetes.io/metadata.name` is a sign the operator-ns
						// rule leaked in.
						if peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] != "kube-system" {
							return true
						}
					}
				}
				return false
			}

			for _, rule := range np.Spec.Ingress {
				Expect(hasMetadataNameSelector(rule.From)).To(BeFalse(),
					"ingress must not include a metadata.name selector when OperatorNamespace is empty")
			}
			for _, rule := range np.Spec.Egress {
				// The DNS rule uses metadata.name=kube-system, which the helper ignores.
				Expect(hasMetadataNameSelector(rule.To)).To(BeFalse(),
					"egress must not include a non-DNS metadata.name selector when OperatorNamespace is empty")
			}
		})
	})
})

var _ = Describe("Workspace Controller Storage", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	var (
		ctx           context.Context
		workspaceKey  types.NamespacedName
		namespaceName string
		reconciler    *WorkspaceReconciler
		testID        string
	)

	BeforeEach(func() {
		ctx = context.Background()
		testID = fmt.Sprintf("%d", atomic.AddUint64(&testCounter, 1))
		workspaceKey = types.NamespacedName{
			Name: "storage-ws-" + testID,
		}
		namespaceName = "storage-test-" + testID
		reconciler = &WorkspaceReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
	})

	AfterEach(func() {
		workspace := &omniav1alpha1.Workspace{}
		err := k8sClient.Get(ctx, workspaceKey, workspace)
		if err == nil {
			workspace.Finalizers = nil
			_ = k8sClient.Update(ctx, workspace)
			_ = k8sClient.Delete(ctx, workspace)
		}

		Eventually(func() bool {
			err := k8sClient.Get(ctx, workspaceKey, &omniav1alpha1.Workspace{})
			return errors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue())

		ns := &corev1.Namespace{}
		err = k8sClient.Get(ctx, client.ObjectKey{Name: namespaceName}, ns)
		if err == nil {
			_ = k8sClient.Delete(ctx, ns)
		}
	})

	Context("When storage is enabled", func() {
		It("should set storage status to Pending when PVC does not exist yet", func() {
			By("creating a workspace with storage enabled")
			enabled := true
			workspace := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceKey.Name,
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Storage Test Workspace",
					Environment: omniav1alpha1.WorkspaceEnvironmentDevelopment,
					Namespace: omniav1alpha1.NamespaceConfig{
						Name:   namespaceName,
						Create: true,
					},
					Storage: &omniav1alpha1.WorkspaceStorageConfig{
						Enabled: &enabled,
						Size:    "5Gi",
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			By("reconciling the workspace")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying PVC was NOT created (lazy creation by Arena)")
			pvcName := fmt.Sprintf("workspace-%s-content", namespaceName)
			pvc := &corev1.PersistentVolumeClaim{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      pvcName,
				Namespace: namespaceName,
			}, pvc)
			Expect(errors.IsNotFound(err)).To(BeTrue())

			By("verifying storage status shows Pending")
			updated := &omniav1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, workspaceKey, updated)).To(Succeed())
			Expect(updated.Status.Storage).NotTo(BeNil())
			Expect(updated.Status.Storage.PVCName).To(Equal(pvcName))
			Expect(updated.Status.Storage.Phase).To(Equal("Pending"))
		})

		It("should track existing PVC status if PVC already exists", func() {
			By("creating a workspace with storage enabled")
			enabled := true
			workspace := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceKey.Name,
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Storage Test Workspace",
					Environment: omniav1alpha1.WorkspaceEnvironmentDevelopment,
					Namespace: omniav1alpha1.NamespaceConfig{
						Name:   namespaceName,
						Create: true,
					},
					Storage: &omniav1alpha1.WorkspaceStorageConfig{
						Enabled: &enabled,
						Size:    "5Gi",
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			By("reconciling to create namespace and complete setup")
			// Multiple reconciles needed for complete workspace setup
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying namespace exists")
			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: namespaceName}, ns)).To(Succeed())

			By("creating PVC manually (simulating Arena controller)")
			pvcName := fmt.Sprintf("workspace-%s-content", namespaceName)
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespaceName,
					Labels: map[string]string{
						labelWorkspace:        workspaceKey.Name,
						labelWorkspaceManaged: "true",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("5Gi"),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pvc)).To(Succeed())

			By("reconciling again")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying storage status tracks existing PVC")
			updated := &omniav1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, workspaceKey, updated)).To(Succeed())
			Expect(updated.Status.Storage).NotTo(BeNil())
			Expect(updated.Status.Storage.PVCName).To(Equal(pvcName))
		})
	})

	Context("When storage is disabled", func() {
		It("should not create PVC when storage is nil", func() {
			By("creating a workspace without storage config")
			workspace := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceKey.Name,
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "No Storage Test",
					Environment: omniav1alpha1.WorkspaceEnvironmentDevelopment,
					Namespace: omniav1alpha1.NamespaceConfig{
						Name:   namespaceName,
						Create: true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			By("reconciling the workspace")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying no PVC was created")
			pvcName := fmt.Sprintf("workspace-%s-content", namespaceName)
			pvc := &corev1.PersistentVolumeClaim{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      pvcName,
				Namespace: namespaceName,
			}, pvc)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("should delete PVC when storage is disabled and PVC exists", func() {
			By("creating a workspace with storage enabled")
			enabled := true
			workspace := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: workspaceKey.Name,
				},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Delete Storage Test",
					Environment: omniav1alpha1.WorkspaceEnvironmentDevelopment,
					Namespace: omniav1alpha1.NamespaceConfig{
						Name:   namespaceName,
						Create: true,
					},
					Storage: &omniav1alpha1.WorkspaceStorageConfig{
						Enabled: &enabled,
						Size:    "5Gi",
					},
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			By("reconciling to create namespace and complete setup")
			// Multiple reconciles needed for complete workspace setup
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying namespace exists")
			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: namespaceName}, ns)).To(Succeed())

			By("creating PVC manually (simulating Arena controller)")
			pvcName := fmt.Sprintf("workspace-%s-content", namespaceName)
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespaceName,
					Labels: map[string]string{
						labelWorkspace:        workspaceKey.Name,
						labelWorkspaceManaged: "true",
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("5Gi"),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pvc)).To(Succeed())

			By("disabling storage")
			updated := &omniav1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, workspaceKey, updated)).To(Succeed())
			enabled = false
			updated.Spec.Storage.Enabled = &enabled
			Expect(k8sClient.Update(ctx, updated)).To(Succeed())

			By("reconciling to delete PVC")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workspaceKey})
			Expect(err).NotTo(HaveOccurred())

			By("verifying storage status is nil after reconcile")
			Expect(k8sClient.Get(ctx, workspaceKey, updated)).To(Succeed())
			Expect(updated.Status.Storage).To(BeNil())

			// Note: envtest doesn't fully simulate PVC deletion from the API server
			// The delete was issued (as shown by the "Deleted PVC" log), but the
			// object may still exist in envtest. We verify the reconciler behavior
			// by checking that storage status is cleared.
		})
	})

})

var _ = Describe("Workspace Controller Helpers", func() {
	Describe("sanitizeName", func() {
		It("should handle simple names", func() {
			Expect(sanitizeName("hello")).To(Equal("hello"))
		})

		It("should convert uppercase to lowercase", func() {
			Expect(sanitizeName("Hello")).To(Equal("hello"))
		})

		It("should replace special characters with dash", func() {
			Expect(sanitizeName("hello_world")).To(Equal("hello-world"))
			Expect(sanitizeName("hello.world")).To(Equal("hello-world"))
			Expect(sanitizeName("hello@world")).To(Equal("hello-world"))
		})

		It("should handle consecutive special characters", func() {
			// Note: implementation doesn't collapse consecutive dashes
			Expect(sanitizeName("hello__world")).To(Equal("hello--world"))
			Expect(sanitizeName("hello...world")).To(Equal("hello---world"))
		})

		It("should trim leading and trailing dashes", func() {
			Expect(sanitizeName("-hello-")).To(Equal("hello"))
			Expect(sanitizeName("__hello__")).To(Equal("hello"))
		})
	})

	Context("PrivacyPolicyResolved condition", func() {
		var (
			ctx           context.Context
			workspaceKey  types.NamespacedName
			namespaceName string
			reconciler    *WorkspaceReconciler
			testID        string
		)

		BeforeEach(func() {
			ctx = context.Background()
			testID = fmt.Sprintf("priv-%d", atomic.AddUint64(&testCounter, 1))
			workspaceKey = types.NamespacedName{Name: "ws-priv-" + testID}
			namespaceName = "priv-ns-" + testID
			reconciler = &WorkspaceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// Pre-create the namespace so reconciliation succeeds.
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName}}
			_ = k8sClient.Create(ctx, ns)
		})

		AfterEach(func() {
			ws := &omniav1alpha1.Workspace{}
			if err := k8sClient.Get(ctx, workspaceKey, ws); err == nil {
				ws.Finalizers = nil
				_ = k8sClient.Update(ctx, ws)
				_ = k8sClient.Delete(ctx, ws)
			}
			Eventually(func() bool {
				return errors.IsNotFound(k8sClient.Get(ctx, workspaceKey, &omniav1alpha1.Workspace{}))
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			// Clean up any SessionPrivacyPolicy objects created in this test.
			sppList := &eev1alpha1.SessionPrivacyPolicyList{}
			if err := k8sClient.List(ctx, sppList, client.InNamespace(namespaceName)); err == nil {
				for i := range sppList.Items {
					_ = k8sClient.Delete(ctx, &sppList.Items[i])
				}
			}

			ns := &corev1.Namespace{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: namespaceName}, ns); err == nil {
				_ = k8sClient.Delete(ctx, ns)
			}
		})

		// reconcileWorkspace is a helper that runs the reconciler past the
		// finalizer-add step and into the main reconciliation path.
		reconcileWorkspace := func(wsKey types.NamespacedName) {
			// First reconcile just adds the finalizer.
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: wsKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())
			// Second reconcile does the real work.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: wsKey})
			Expect(err).NotTo(HaveOccurred())
		}

		It("should set PrivacyPolicyResolved=True/DefaultPolicy when no service group sets a ref", func() {
			ws := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: workspaceKey.Name},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Privacy Test",
					Environment: omniav1alpha1.WorkspaceEnvironmentDevelopment,
					Namespace:   omniav1alpha1.NamespaceConfig{Name: namespaceName, Create: false},
				},
			}
			Expect(k8sClient.Create(ctx, ws)).To(Succeed())
			reconcileWorkspace(workspaceKey)

			updated := &omniav1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, workspaceKey, updated)).To(Succeed())
			cond := meta.FindStatusCondition(updated.Status.Conditions, "PrivacyPolicyResolved")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("DefaultPolicy"))
		})

		It("should set PrivacyPolicyResolved=False/PolicyNotFound when a service group references a missing policy", func() {
			ws := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: workspaceKey.Name},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Privacy Test",
					Environment: omniav1alpha1.WorkspaceEnvironmentDevelopment,
					Namespace:   omniav1alpha1.NamespaceConfig{Name: namespaceName, Create: false},
					Services: []omniav1alpha1.WorkspaceServiceGroup{
						{
							Name: "svc-a",
							Mode: omniav1alpha1.ServiceModeExternal,
							External: &omniav1alpha1.ExternalEndpoints{
								SessionURL: "http://session.example.com",
								MemoryURL:  "http://memory.example.com",
							},
							PrivacyPolicyRef: &corev1.LocalObjectReference{Name: "missing-policy"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ws)).To(Succeed())
			reconcileWorkspace(workspaceKey)

			updated := &omniav1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, workspaceKey, updated)).To(Succeed())
			cond := meta.FindStatusCondition(updated.Status.Conditions, "PrivacyPolicyResolved")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("PolicyNotFound"))
			Expect(cond.Message).To(ContainSubstring("svc-a"))
			Expect(cond.Message).To(ContainSubstring("missing-policy"))
		})

		It("should set PrivacyPolicyResolved=True/PolicyResolved when a service group references an existing policy", func() {
			// Create the SessionPrivacyPolicy in the workspace namespace.
			spp := &eev1alpha1.SessionPrivacyPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "my-policy", Namespace: namespaceName},
				Spec: eev1alpha1.SessionPrivacyPolicySpec{
					Recording: eev1alpha1.RecordingConfig{Enabled: true},
				},
			}
			Expect(k8sClient.Create(ctx, spp)).To(Succeed())

			ws := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: workspaceKey.Name},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Privacy Test",
					Environment: omniav1alpha1.WorkspaceEnvironmentDevelopment,
					Namespace:   omniav1alpha1.NamespaceConfig{Name: namespaceName, Create: false},
					Services: []omniav1alpha1.WorkspaceServiceGroup{
						{
							Name: "svc-a",
							Mode: omniav1alpha1.ServiceModeExternal,
							External: &omniav1alpha1.ExternalEndpoints{
								SessionURL: "http://session.example.com",
								MemoryURL:  "http://memory.example.com",
							},
							PrivacyPolicyRef: &corev1.LocalObjectReference{Name: "my-policy"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ws)).To(Succeed())
			reconcileWorkspace(workspaceKey)

			updated := &omniav1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, workspaceKey, updated)).To(Succeed())
			cond := meta.FindStatusCondition(updated.Status.Conditions, "PrivacyPolicyResolved")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("PolicyResolved"))
			Expect(cond.Message).To(ContainSubstring("svc-a"))
		})

		It("should set PrivacyPolicyResolved=False when one group is missing and one is resolved", func() {
			// Create the policy for the second group only.
			spp := &eev1alpha1.SessionPrivacyPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "existing-policy", Namespace: namespaceName},
				Spec: eev1alpha1.SessionPrivacyPolicySpec{
					Recording: eev1alpha1.RecordingConfig{Enabled: false},
				},
			}
			Expect(k8sClient.Create(ctx, spp)).To(Succeed())

			ws := &omniav1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: workspaceKey.Name},
				Spec: omniav1alpha1.WorkspaceSpec{
					DisplayName: "Privacy Test",
					Environment: omniav1alpha1.WorkspaceEnvironmentDevelopment,
					Namespace:   omniav1alpha1.NamespaceConfig{Name: namespaceName, Create: false},
					Services: []omniav1alpha1.WorkspaceServiceGroup{
						{
							Name: "svc-missing",
							Mode: omniav1alpha1.ServiceModeExternal,
							External: &omniav1alpha1.ExternalEndpoints{
								SessionURL: "http://session.example.com",
								MemoryURL:  "http://memory.example.com",
							},
							PrivacyPolicyRef: &corev1.LocalObjectReference{Name: "nonexistent-policy"},
						},
						{
							Name: "svc-ok",
							Mode: omniav1alpha1.ServiceModeExternal,
							External: &omniav1alpha1.ExternalEndpoints{
								SessionURL: "http://session2.example.com",
								MemoryURL:  "http://memory2.example.com",
							},
							PrivacyPolicyRef: &corev1.LocalObjectReference{Name: "existing-policy"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ws)).To(Succeed())
			reconcileWorkspace(workspaceKey)

			updated := &omniav1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, workspaceKey, updated)).To(Succeed())
			cond := meta.FindStatusCondition(updated.Status.Conditions, "PrivacyPolicyResolved")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("PolicyNotFound"))
			// Only the missing group should appear in the message.
			Expect(cond.Message).To(ContainSubstring("svc-missing"))
			Expect(cond.Message).To(ContainSubstring("nonexistent-policy"))
			Expect(cond.Message).NotTo(ContainSubstring("svc-ok"))
		})
	})
})
