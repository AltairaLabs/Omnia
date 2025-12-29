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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

var _ = Describe("ToolRegistry Controller", func() {
	const (
		registryName      = "test-registry"
		registryNamespace = "default"
	)

	ctx := context.Background()

	Context("When reconciling a ToolRegistry with inline URL endpoint", func() {
		var toolRegistry *omniav1alpha1.ToolRegistry

		BeforeEach(func() {
			By("creating the ToolRegistry with inline URL")
			toolURL := "https://api.example.com/tool"
			toolRegistry = &omniav1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      registryName,
					Namespace: registryNamespace,
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Tools: []omniav1alpha1.ToolDefinition{
						{
							Name: "test-tool",
							Type: omniav1alpha1.ToolTypeHTTP,
							Endpoint: omniav1alpha1.ToolEndpoint{
								URL: &toolURL,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, toolRegistry)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up the ToolRegistry")
			resource := &omniav1alpha1.ToolRegistry{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      registryName,
				Namespace: registryNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should discover the tool and set Ready phase", func() {
			By("reconciling the ToolRegistry")
			reconciler := &ToolRegistryReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      registryName,
					Namespace: registryNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedTR := &omniav1alpha1.ToolRegistry{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      registryName,
				Namespace: registryNamespace,
			}, updatedTR)).To(Succeed())

			Expect(updatedTR.Status.Phase).To(Equal(omniav1alpha1.ToolRegistryPhaseReady))
			Expect(updatedTR.Status.DiscoveredToolsCount).To(Equal(int32(1)))
			Expect(updatedTR.Status.DiscoveredTools).To(HaveLen(1))
			Expect(updatedTR.Status.DiscoveredTools[0].Name).To(Equal("test-tool"))
			Expect(updatedTR.Status.DiscoveredTools[0].Endpoint).To(Equal("https://api.example.com/tool"))
			Expect(updatedTR.Status.DiscoveredTools[0].Status).To(Equal(omniav1alpha1.ToolStatusAvailable))

			By("checking the ToolsDiscovered condition")
			condition := meta.FindStatusCondition(updatedTR.Status.Conditions, ToolRegistryConditionTypeToolsDiscovered)
			Expect(condition).NotTo(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			Expect(condition.Reason).To(Equal("ToolsDiscovered"))
		})
	})

	Context("When reconciling a ToolRegistry with service selector", func() {
		var (
			toolRegistry *omniav1alpha1.ToolRegistry
			service      *corev1.Service
		)

		BeforeEach(func() {
			By("creating a Service with matching labels")
			service = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tool-service",
					Namespace: registryNamespace,
					Labels: map[string]string{
						"app": "my-tool",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, service)).To(Succeed())

			By("creating the ToolRegistry with selector")
			toolRegistry = &omniav1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "selector-registry",
					Namespace: registryNamespace,
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Tools: []omniav1alpha1.ToolDefinition{
						{
							Name: "discovered-tool",
							Type: omniav1alpha1.ToolTypeHTTP,
							Endpoint: omniav1alpha1.ToolEndpoint{
								Selector: &omniav1alpha1.ToolSelector{
									MatchLabels: map[string]string{
										"app": "my-tool",
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, toolRegistry)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			tr := &omniav1alpha1.ToolRegistry{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "selector-registry",
				Namespace: registryNamespace,
			}, tr)
			if err == nil {
				Expect(k8sClient.Delete(ctx, tr)).To(Succeed())
			}

			svc := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "tool-service",
				Namespace: registryNamespace,
			}, svc)
			if err == nil {
				Expect(k8sClient.Delete(ctx, svc)).To(Succeed())
			}
		})

		It("should discover the tool via service selector", func() {
			By("reconciling the ToolRegistry")
			reconciler := &ToolRegistryReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "selector-registry",
					Namespace: registryNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedTR := &omniav1alpha1.ToolRegistry{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "selector-registry",
				Namespace: registryNamespace,
			}, updatedTR)).To(Succeed())

			Expect(updatedTR.Status.Phase).To(Equal(omniav1alpha1.ToolRegistryPhaseReady))
			Expect(updatedTR.Status.DiscoveredToolsCount).To(Equal(int32(1)))
			Expect(updatedTR.Status.DiscoveredTools).To(HaveLen(1))
			Expect(updatedTR.Status.DiscoveredTools[0].Name).To(Equal("discovered-tool"))
			Expect(updatedTR.Status.DiscoveredTools[0].Endpoint).To(ContainSubstring("tool-service"))
			Expect(updatedTR.Status.DiscoveredTools[0].Endpoint).To(ContainSubstring("8080"))
			Expect(updatedTR.Status.DiscoveredTools[0].Status).To(Equal(omniav1alpha1.ToolStatusAvailable))
		})
	})

	Context("When reconciling a ToolRegistry with no matching services", func() {
		var toolRegistry *omniav1alpha1.ToolRegistry

		BeforeEach(func() {
			By("creating the ToolRegistry with selector that won't match")
			toolRegistry = &omniav1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-match-registry",
					Namespace: registryNamespace,
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Tools: []omniav1alpha1.ToolDefinition{
						{
							Name: "missing-tool",
							Type: omniav1alpha1.ToolTypeHTTP,
							Endpoint: omniav1alpha1.ToolEndpoint{
								Selector: &omniav1alpha1.ToolSelector{
									MatchLabels: map[string]string{
										"app": "nonexistent",
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, toolRegistry)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up the ToolRegistry")
			resource := &omniav1alpha1.ToolRegistry{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "no-match-registry",
				Namespace: registryNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should set tool as unavailable and phase as Failed", func() {
			By("reconciling the ToolRegistry")
			reconciler := &ToolRegistryReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "no-match-registry",
					Namespace: registryNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedTR := &omniav1alpha1.ToolRegistry{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "no-match-registry",
				Namespace: registryNamespace,
			}, updatedTR)).To(Succeed())

			Expect(updatedTR.Status.Phase).To(Equal(omniav1alpha1.ToolRegistryPhaseFailed))
			Expect(updatedTR.Status.DiscoveredToolsCount).To(Equal(int32(1)))
			Expect(updatedTR.Status.DiscoveredTools[0].Status).To(Equal(omniav1alpha1.ToolStatusUnavailable))
		})
	})

	Context("When reconciling a ToolRegistry with mixed availability", func() {
		var toolRegistry *omniav1alpha1.ToolRegistry

		BeforeEach(func() {
			By("creating the ToolRegistry with one available and one unavailable tool")
			toolURL := "https://api.example.com/available"
			toolRegistry = &omniav1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mixed-registry",
					Namespace: registryNamespace,
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Tools: []omniav1alpha1.ToolDefinition{
						{
							Name: "available-tool",
							Type: omniav1alpha1.ToolTypeHTTP,
							Endpoint: omniav1alpha1.ToolEndpoint{
								URL: &toolURL,
							},
						},
						{
							Name: "unavailable-tool",
							Type: omniav1alpha1.ToolTypeHTTP,
							Endpoint: omniav1alpha1.ToolEndpoint{
								Selector: &omniav1alpha1.ToolSelector{
									MatchLabels: map[string]string{
										"app": "nonexistent",
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, toolRegistry)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up the ToolRegistry")
			resource := &omniav1alpha1.ToolRegistry{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "mixed-registry",
				Namespace: registryNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should set phase as Degraded", func() {
			By("reconciling the ToolRegistry")
			reconciler := &ToolRegistryReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "mixed-registry",
					Namespace: registryNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the updated status")
			updatedTR := &omniav1alpha1.ToolRegistry{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "mixed-registry",
				Namespace: registryNamespace,
			}, updatedTR)).To(Succeed())

			Expect(updatedTR.Status.Phase).To(Equal(omniav1alpha1.ToolRegistryPhaseDegraded))
			Expect(updatedTR.Status.DiscoveredToolsCount).To(Equal(int32(2)))
		})
	})

	Context("When reconciling a non-existent ToolRegistry", func() {
		It("should return without error", func() {
			By("reconciling a non-existent ToolRegistry")
			reconciler := &ToolRegistryReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "nonexistent-registry",
					Namespace: registryNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When testing service endpoint building", func() {
		var (
			toolRegistry *omniav1alpha1.ToolRegistry
			service      *corev1.Service
		)

		BeforeEach(func() {
			By("creating a Service with path annotation")
			service = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "annotated-service",
					Namespace: registryNamespace,
					Labels: map[string]string{
						"app": "annotated-tool",
					},
					Annotations: map[string]string{
						AnnotationToolPath: "/api/v1/tool",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       9090,
							TargetPort: intstr.FromInt(9090),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, service)).To(Succeed())

			By("creating the ToolRegistry")
			toolRegistry = &omniav1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "annotated-registry",
					Namespace: registryNamespace,
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Tools: []omniav1alpha1.ToolDefinition{
						{
							Name: "annotated-tool",
							Type: omniav1alpha1.ToolTypeHTTP,
							Endpoint: omniav1alpha1.ToolEndpoint{
								Selector: &omniav1alpha1.ToolSelector{
									MatchLabels: map[string]string{
										"app": "annotated-tool",
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, toolRegistry)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			tr := &omniav1alpha1.ToolRegistry{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "annotated-registry",
				Namespace: registryNamespace,
			}, tr)
			if err == nil {
				Expect(k8sClient.Delete(ctx, tr)).To(Succeed())
			}

			svc := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "annotated-service",
				Namespace: registryNamespace,
			}, svc)
			if err == nil {
				Expect(k8sClient.Delete(ctx, svc)).To(Succeed())
			}
		})

		It("should include path annotation in endpoint", func() {
			By("reconciling the ToolRegistry")
			reconciler := &ToolRegistryReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "annotated-registry",
					Namespace: registryNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the endpoint includes path")
			updatedTR := &omniav1alpha1.ToolRegistry{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "annotated-registry",
				Namespace: registryNamespace,
			}, updatedTR)).To(Succeed())

			Expect(updatedTR.Status.DiscoveredTools[0].Endpoint).To(ContainSubstring("/api/v1/tool"))
			Expect(updatedTR.Status.DiscoveredTools[0].Endpoint).To(ContainSubstring("9090"))
		})
	})

	Context("When testing gRPC tool type", func() {
		var toolRegistry *omniav1alpha1.ToolRegistry
		var service *corev1.Service

		BeforeEach(func() {
			By("creating a gRPC Service")
			service = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "grpc-service",
					Namespace: registryNamespace,
					Labels: map[string]string{
						"app": "grpc-tool",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "grpc",
							Port:       50051,
							TargetPort: intstr.FromInt(50051),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, service)).To(Succeed())

			By("creating the ToolRegistry for gRPC")
			toolRegistry = &omniav1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "grpc-registry",
					Namespace: registryNamespace,
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Tools: []omniav1alpha1.ToolDefinition{
						{
							Name: "grpc-tool",
							Type: omniav1alpha1.ToolTypeGRPC,
							Endpoint: omniav1alpha1.ToolEndpoint{
								Selector: &omniav1alpha1.ToolSelector{
									MatchLabels: map[string]string{
										"app": "grpc-tool",
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, toolRegistry)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			tr := &omniav1alpha1.ToolRegistry{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "grpc-registry",
				Namespace: registryNamespace,
			}, tr)
			if err == nil {
				Expect(k8sClient.Delete(ctx, tr)).To(Succeed())
			}

			svc := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "grpc-service",
				Namespace: registryNamespace,
			}, svc)
			if err == nil {
				Expect(k8sClient.Delete(ctx, svc)).To(Succeed())
			}
		})

		It("should use grpc protocol in endpoint", func() {
			By("reconciling the ToolRegistry")
			reconciler := &ToolRegistryReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "grpc-registry",
					Namespace: registryNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the endpoint uses grpc protocol")
			updatedTR := &omniav1alpha1.ToolRegistry{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "grpc-registry",
				Namespace: registryNamespace,
			}, updatedTR)).To(Succeed())

			Expect(updatedTR.Status.DiscoveredTools[0].Endpoint).To(HavePrefix("grpc://"))
		})
	})

	Context("When testing findToolRegistriesForService", func() {
		var (
			toolRegistry *omniav1alpha1.ToolRegistry
			service      *corev1.Service
		)

		BeforeEach(func() {
			By("creating a Service")
			service = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "watch-service",
					Namespace: registryNamespace,
					Labels: map[string]string{
						"app": "watched-tool",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port: 8080,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, service)).To(Succeed())

			By("creating the ToolRegistry")
			toolRegistry = &omniav1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "watch-registry",
					Namespace: registryNamespace,
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Tools: []omniav1alpha1.ToolDefinition{
						{
							Name: "watched-tool",
							Type: omniav1alpha1.ToolTypeHTTP,
							Endpoint: omniav1alpha1.ToolEndpoint{
								Selector: &omniav1alpha1.ToolSelector{
									MatchLabels: map[string]string{
										"app": "watched-tool",
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, toolRegistry)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			tr := &omniav1alpha1.ToolRegistry{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "watch-registry",
				Namespace: registryNamespace,
			}, tr)
			if err == nil {
				Expect(k8sClient.Delete(ctx, tr)).To(Succeed())
			}

			svc := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "watch-service",
				Namespace: registryNamespace,
			}, svc)
			if err == nil {
				Expect(k8sClient.Delete(ctx, svc)).To(Succeed())
			}
		})

		It("should return reconcile requests for matching ToolRegistries", func() {
			By("calling findToolRegistriesForService")
			reconciler := &ToolRegistryReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			requests := reconciler.findToolRegistriesForService(ctx, service)
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Name).To(Equal("watch-registry"))
			Expect(requests[0].Namespace).To(Equal(registryNamespace))
		})
	})

	Context("When testing specific port selection", func() {
		var (
			toolRegistry *omniav1alpha1.ToolRegistry
			service      *corev1.Service
		)

		BeforeEach(func() {
			By("creating a Service with multiple ports")
			service = &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multiport-service",
					Namespace: registryNamespace,
					Labels: map[string]string{
						"app": "multiport-tool",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						},
						{
							Name:       "admin",
							Port:       9090,
							TargetPort: intstr.FromInt(9090),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, service)).To(Succeed())

			By("creating the ToolRegistry with specific port")
			portName := "admin"
			toolRegistry = &omniav1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multiport-registry",
					Namespace: registryNamespace,
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Tools: []omniav1alpha1.ToolDefinition{
						{
							Name: "multiport-tool",
							Type: omniav1alpha1.ToolTypeHTTP,
							Endpoint: omniav1alpha1.ToolEndpoint{
								Selector: &omniav1alpha1.ToolSelector{
									MatchLabels: map[string]string{
										"app": "multiport-tool",
									},
									Port: &portName,
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, toolRegistry)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up resources")
			tr := &omniav1alpha1.ToolRegistry{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "multiport-registry",
				Namespace: registryNamespace,
			}, tr)
			if err == nil {
				Expect(k8sClient.Delete(ctx, tr)).To(Succeed())
			}

			svc := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "multiport-service",
				Namespace: registryNamespace,
			}, svc)
			if err == nil {
				Expect(k8sClient.Delete(ctx, svc)).To(Succeed())
			}
		})

		It("should use the specified port name", func() {
			By("reconciling the ToolRegistry")
			reconciler := &ToolRegistryReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "multiport-registry",
					Namespace: registryNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("checking the endpoint uses the admin port (9090)")
			updatedTR := &omniav1alpha1.ToolRegistry{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "multiport-registry",
				Namespace: registryNamespace,
			}, updatedTR)).To(Succeed())

			Expect(updatedTR.Status.DiscoveredTools[0].Endpoint).To(ContainSubstring("9090"))
		})
	})
})

// Ensure unused import doesn't cause issues
var _ = errors.IsNotFound
