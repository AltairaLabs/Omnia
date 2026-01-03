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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
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

	Context("When reconciling a ToolRegistry with HTTP handler", func() {
		var toolRegistry *omniav1alpha1.ToolRegistry

		BeforeEach(func() {
			By("creating the ToolRegistry with inline HTTP handler")
			toolRegistry = &omniav1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      registryName,
					Namespace: registryNamespace,
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name: "test-handler",
							Type: omniav1alpha1.HandlerTypeHTTP,
							HTTPConfig: &omniav1alpha1.HTTPConfig{
								Endpoint: "https://api.example.com/tool",
								Method:   "POST",
							},
							Tool: &omniav1alpha1.ToolDefinition{
								Name:        "test_tool",
								Description: "A test tool",
								InputSchema: apiextensionsv1.JSON{
									Raw: []byte(`{"type":"object","properties":{"input":{"type":"string"}}}`),
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
			Expect(updatedTR.Status.DiscoveredTools[0].Name).To(Equal("test_tool"))
			Expect(updatedTR.Status.DiscoveredTools[0].HandlerName).To(Equal("test-handler"))
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
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name: "discovered-handler",
							Type: omniav1alpha1.HandlerTypeHTTP,
							Selector: &omniav1alpha1.ServiceSelector{
								MatchLabels: map[string]string{
									"app": "my-tool",
								},
							},
							HTTPConfig: &omniav1alpha1.HTTPConfig{
								Endpoint: "", // Will be resolved from selector
							},
							Tool: &omniav1alpha1.ToolDefinition{
								Name:        "discovered_tool",
								Description: "A discovered tool",
								InputSchema: apiextensionsv1.JSON{
									Raw: []byte(`{"type":"object"}`),
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
			Expect(updatedTR.Status.DiscoveredTools[0].Name).To(Equal("discovered_tool"))
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
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name: "missing-handler",
							Type: omniav1alpha1.HandlerTypeHTTP,
							Selector: &omniav1alpha1.ServiceSelector{
								MatchLabels: map[string]string{
									"app": "nonexistent",
								},
							},
							HTTPConfig: &omniav1alpha1.HTTPConfig{
								Endpoint: "", // Will be resolved from selector
							},
							Tool: &omniav1alpha1.ToolDefinition{
								Name:        "missing_tool",
								Description: "A missing tool",
								InputSchema: apiextensionsv1.JSON{
									Raw: []byte(`{"type":"object"}`),
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
			By("creating the ToolRegistry with one available and one unavailable handler")
			toolRegistry = &omniav1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mixed-registry",
					Namespace: registryNamespace,
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name: "available-handler",
							Type: omniav1alpha1.HandlerTypeHTTP,
							HTTPConfig: &omniav1alpha1.HTTPConfig{
								Endpoint: "https://api.example.com/available",
							},
							Tool: &omniav1alpha1.ToolDefinition{
								Name:        "available_tool",
								Description: "An available tool",
								InputSchema: apiextensionsv1.JSON{
									Raw: []byte(`{"type":"object"}`),
								},
							},
						},
						{
							Name: "unavailable-handler",
							Type: omniav1alpha1.HandlerTypeHTTP,
							Selector: &omniav1alpha1.ServiceSelector{
								MatchLabels: map[string]string{
									"app": "nonexistent",
								},
							},
							HTTPConfig: &omniav1alpha1.HTTPConfig{
								Endpoint: "", // Will be resolved from selector
							},
							Tool: &omniav1alpha1.ToolDefinition{
								Name:        "unavailable_tool",
								Description: "An unavailable tool",
								InputSchema: apiextensionsv1.JSON{
									Raw: []byte(`{"type":"object"}`),
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
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name: "annotated-handler",
							Type: omniav1alpha1.HandlerTypeHTTP,
							Selector: &omniav1alpha1.ServiceSelector{
								MatchLabels: map[string]string{
									"app": "annotated-tool",
								},
							},
							HTTPConfig: &omniav1alpha1.HTTPConfig{
								Endpoint: "", // Will be resolved from selector
							},
							Tool: &omniav1alpha1.ToolDefinition{
								Name:        "annotated_tool",
								Description: "An annotated tool",
								InputSchema: apiextensionsv1.JSON{
									Raw: []byte(`{"type":"object"}`),
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

	Context("When testing gRPC handler type", func() {
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
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name: "grpc-handler",
							Type: omniav1alpha1.HandlerTypeGRPC,
							Selector: &omniav1alpha1.ServiceSelector{
								MatchLabels: map[string]string{
									"app": "grpc-tool",
								},
							},
							GRPCConfig: &omniav1alpha1.GRPCConfig{
								Endpoint: "grpc://placeholder:50051", // Required for validation, but will be overridden by selector
							},
							Tool: &omniav1alpha1.ToolDefinition{
								Name:        "grpc_tool",
								Description: "A gRPC tool",
								InputSchema: apiextensionsv1.JSON{
									Raw: []byte(`{"type":"object"}`),
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
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name: "watched-handler",
							Type: omniav1alpha1.HandlerTypeHTTP,
							Selector: &omniav1alpha1.ServiceSelector{
								MatchLabels: map[string]string{
									"app": "watched-tool",
								},
							},
							HTTPConfig: &omniav1alpha1.HTTPConfig{
								Endpoint: "", // Will be resolved from selector
							},
							Tool: &omniav1alpha1.ToolDefinition{
								Name:        "watched_tool",
								Description: "A watched tool",
								InputSchema: apiextensionsv1.JSON{
									Raw: []byte(`{"type":"object"}`),
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
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name: "multiport-handler",
							Type: omniav1alpha1.HandlerTypeHTTP,
							Selector: &omniav1alpha1.ServiceSelector{
								MatchLabels: map[string]string{
									"app": "multiport-tool",
								},
								Port: &portName,
							},
							HTTPConfig: &omniav1alpha1.HTTPConfig{
								Endpoint: "", // Will be resolved from selector
							},
							Tool: &omniav1alpha1.ToolDefinition{
								Name:        "multiport_tool",
								Description: "A multiport tool",
								InputSchema: apiextensionsv1.JSON{
									Raw: []byte(`{"type":"object"}`),
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

	Context("When testing MCP handler", func() {
		var toolRegistry *omniav1alpha1.ToolRegistry

		BeforeEach(func() {
			By("creating the ToolRegistry with MCP handler")
			endpoint := "http://mcp-server:8080"
			toolRegistry = &omniav1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mcp-registry",
					Namespace: registryNamespace,
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name: "mcp-handler",
							Type: omniav1alpha1.HandlerTypeMCP,
							MCPConfig: &omniav1alpha1.MCPConfig{
								Transport: omniav1alpha1.MCPTransportSSE,
								Endpoint:  &endpoint,
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
				Name:      "mcp-registry",
				Namespace: registryNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should be created successfully", func() {
			By("verifying the ToolRegistry was created")
			created := &omniav1alpha1.ToolRegistry{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "mcp-registry",
				Namespace: registryNamespace,
			}, created)).To(Succeed())

			Expect(created.Spec.Handlers).To(HaveLen(1))
			Expect(created.Spec.Handlers[0].Type).To(Equal(omniav1alpha1.HandlerTypeMCP))
			Expect(created.Spec.Handlers[0].MCPConfig).NotTo(BeNil())
			Expect(created.Spec.Handlers[0].MCPConfig.Transport).To(Equal(omniav1alpha1.MCPTransportSSE))
		})
	})

	Context("When testing OpenAPI handler", func() {
		var toolRegistry *omniav1alpha1.ToolRegistry

		BeforeEach(func() {
			By("creating the ToolRegistry with OpenAPI handler")
			baseURL := "http://api-server"
			toolRegistry = &omniav1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openapi-registry",
					Namespace: registryNamespace,
				},
				Spec: omniav1alpha1.ToolRegistrySpec{
					Handlers: []omniav1alpha1.HandlerDefinition{
						{
							Name: "openapi-handler",
							Type: omniav1alpha1.HandlerTypeOpenAPI,
							OpenAPIConfig: &omniav1alpha1.OpenAPIConfig{
								SpecURL: "http://api-server/openapi.json",
								BaseURL: &baseURL,
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
				Name:      "openapi-registry",
				Namespace: registryNamespace,
			}, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should be created successfully", func() {
			By("verifying the ToolRegistry was created")
			created := &omniav1alpha1.ToolRegistry{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "openapi-registry",
				Namespace: registryNamespace,
			}, created)).To(Succeed())

			Expect(created.Spec.Handlers).To(HaveLen(1))
			Expect(created.Spec.Handlers[0].Type).To(Equal(omniav1alpha1.HandlerTypeOpenAPI))
			Expect(created.Spec.Handlers[0].OpenAPIConfig).NotTo(BeNil())
			Expect(created.Spec.Handlers[0].OpenAPIConfig.SpecURL).To(Equal("http://api-server/openapi.json"))
		})
	})

	Context("When validating handler configurations", func() {
		var reconciler *ToolRegistryReconciler

		BeforeEach(func() {
			reconciler = &ToolRegistryReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
		})

		It("should reject gRPC handler without grpcConfig", func() {
			handler := &omniav1alpha1.HandlerDefinition{
				Name: "invalid-grpc",
				Type: omniav1alpha1.HandlerTypeGRPC,
			}
			err := reconciler.validateHandler(handler)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("grpcConfig is required"))
		})

		It("should reject gRPC handler without tool definition", func() {
			handler := &omniav1alpha1.HandlerDefinition{
				Name: "grpc-no-tool",
				Type: omniav1alpha1.HandlerTypeGRPC,
				GRPCConfig: &omniav1alpha1.GRPCConfig{
					Endpoint: "localhost:50051",
				},
			}
			err := reconciler.validateHandler(handler)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("tool definition is required"))
		})

		It("should reject MCP handler without mcpConfig", func() {
			handler := &omniav1alpha1.HandlerDefinition{
				Name: "invalid-mcp",
				Type: omniav1alpha1.HandlerTypeMCP,
			}
			err := reconciler.validateHandler(handler)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("mcpConfig is required"))
		})

		It("should reject MCP SSE handler without endpoint", func() {
			handler := &omniav1alpha1.HandlerDefinition{
				Name: "mcp-sse-no-endpoint",
				Type: omniav1alpha1.HandlerTypeMCP,
				MCPConfig: &omniav1alpha1.MCPConfig{
					Transport: omniav1alpha1.MCPTransportSSE,
				},
			}
			err := reconciler.validateHandler(handler)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("endpoint is required"))
		})

		It("should reject MCP stdio handler without command", func() {
			handler := &omniav1alpha1.HandlerDefinition{
				Name: "mcp-stdio-no-command",
				Type: omniav1alpha1.HandlerTypeMCP,
				MCPConfig: &omniav1alpha1.MCPConfig{
					Transport: omniav1alpha1.MCPTransportStdio,
				},
			}
			err := reconciler.validateHandler(handler)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("command is required"))
		})

		It("should reject OpenAPI handler without openAPIConfig", func() {
			handler := &omniav1alpha1.HandlerDefinition{
				Name: "invalid-openapi",
				Type: omniav1alpha1.HandlerTypeOpenAPI,
			}
			err := reconciler.validateHandler(handler)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("openAPIConfig is required"))
		})

		It("should reject unknown handler type", func() {
			handler := &omniav1alpha1.HandlerDefinition{
				Name: "invalid-type",
				Type: "unknown",
			}
			err := reconciler.validateHandler(handler)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown handler type"))
		})

		It("should accept valid gRPC handler", func() {
			handler := &omniav1alpha1.HandlerDefinition{
				Name: "valid-grpc",
				Type: omniav1alpha1.HandlerTypeGRPC,
				GRPCConfig: &omniav1alpha1.GRPCConfig{
					Endpoint: "localhost:50051",
				},
				Tool: &omniav1alpha1.ToolDefinition{
					Name:        "grpc_tool",
					Description: "A gRPC tool",
					InputSchema: apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)},
				},
			}
			err := reconciler.validateHandler(handler)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept valid MCP SSE handler", func() {
			endpoint := "http://mcp-server/sse"
			handler := &omniav1alpha1.HandlerDefinition{
				Name: "valid-mcp-sse",
				Type: omniav1alpha1.HandlerTypeMCP,
				MCPConfig: &omniav1alpha1.MCPConfig{
					Transport: omniav1alpha1.MCPTransportSSE,
					Endpoint:  &endpoint,
				},
			}
			err := reconciler.validateHandler(handler)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept valid MCP stdio handler", func() {
			command := "/usr/bin/mcp-server"
			handler := &omniav1alpha1.HandlerDefinition{
				Name: "valid-mcp-stdio",
				Type: omniav1alpha1.HandlerTypeMCP,
				MCPConfig: &omniav1alpha1.MCPConfig{
					Transport: omniav1alpha1.MCPTransportStdio,
					Command:   &command,
				},
			}
			err := reconciler.validateHandler(handler)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept valid OpenAPI handler", func() {
			handler := &omniav1alpha1.HandlerDefinition{
				Name: "valid-openapi",
				Type: omniav1alpha1.HandlerTypeOpenAPI,
				OpenAPIConfig: &omniav1alpha1.OpenAPIConfig{
					SpecURL: "http://api/openapi.json",
				},
			}
			err := reconciler.validateHandler(handler)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When resolving endpoints for different handler types", func() {
		var (
			reconciler   *ToolRegistryReconciler
			toolRegistry *omniav1alpha1.ToolRegistry
		)

		BeforeEach(func() {
			reconciler = &ToolRegistryReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			toolRegistry = &omniav1alpha1.ToolRegistry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "endpoint-test",
					Namespace: registryNamespace,
				},
			}
		})

		It("should resolve gRPC endpoint", func() {
			handler := &omniav1alpha1.HandlerDefinition{
				Type: omniav1alpha1.HandlerTypeGRPC,
				GRPCConfig: &omniav1alpha1.GRPCConfig{
					Endpoint: "grpc-service:50051",
				},
			}
			endpoint, err := reconciler.resolveEndpoint(ctx, toolRegistry, handler)
			Expect(err).NotTo(HaveOccurred())
			Expect(endpoint).To(Equal("grpc-service:50051"))
		})

		It("should resolve MCP SSE endpoint", func() {
			sseEndpoint := "http://mcp-server/sse"
			handler := &omniav1alpha1.HandlerDefinition{
				Type: omniav1alpha1.HandlerTypeMCP,
				MCPConfig: &omniav1alpha1.MCPConfig{
					Transport: omniav1alpha1.MCPTransportSSE,
					Endpoint:  &sseEndpoint,
				},
			}
			endpoint, err := reconciler.resolveEndpoint(ctx, toolRegistry, handler)
			Expect(err).NotTo(HaveOccurred())
			Expect(endpoint).To(Equal("http://mcp-server/sse"))
		})

		It("should resolve MCP stdio as command path", func() {
			command := "/usr/bin/mcp-server"
			handler := &omniav1alpha1.HandlerDefinition{
				Type: omniav1alpha1.HandlerTypeMCP,
				MCPConfig: &omniav1alpha1.MCPConfig{
					Transport: omniav1alpha1.MCPTransportStdio,
					Command:   &command,
				},
			}
			endpoint, err := reconciler.resolveEndpoint(ctx, toolRegistry, handler)
			Expect(err).NotTo(HaveOccurred())
			Expect(endpoint).To(Equal("stdio:///usr/bin/mcp-server"))
		})

		It("should fail for MCP without endpoint or command", func() {
			handler := &omniav1alpha1.HandlerDefinition{
				Type: omniav1alpha1.HandlerTypeMCP,
				MCPConfig: &omniav1alpha1.MCPConfig{
					Transport: omniav1alpha1.MCPTransportSSE,
				},
			}
			_, err := reconciler.resolveEndpoint(ctx, toolRegistry, handler)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no endpoint configured"))
		})

		It("should resolve OpenAPI spec URL", func() {
			handler := &omniav1alpha1.HandlerDefinition{
				Type: omniav1alpha1.HandlerTypeOpenAPI,
				OpenAPIConfig: &omniav1alpha1.OpenAPIConfig{
					SpecURL: "https://api.example.com/openapi.yaml",
				},
			}
			endpoint, err := reconciler.resolveEndpoint(ctx, toolRegistry, handler)
			Expect(err).NotTo(HaveOccurred())
			Expect(endpoint).To(Equal("https://api.example.com/openapi.yaml"))
		})
	})

	Context("When discovering tools from self-describing handlers", func() {
		var reconciler *ToolRegistryReconciler

		BeforeEach(func() {
			reconciler = &ToolRegistryReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
		})

		It("should create placeholder for MCP handler", func() {
			handler := &omniav1alpha1.HandlerDefinition{
				Name: "mcp-handler",
				Type: omniav1alpha1.HandlerTypeMCP,
			}
			tools := reconciler.discoverToolsFromHandler(handler, "http://mcp-server/sse")
			Expect(tools).To(HaveLen(1))
			Expect(tools[0].Name).To(Equal("mcp-handler"))
			Expect(tools[0].Description).To(ContainSubstring("Self-describing"))
			Expect(tools[0].Status).To(Equal(omniav1alpha1.ToolStatusAvailable))
		})

		It("should create placeholder for OpenAPI handler", func() {
			handler := &omniav1alpha1.HandlerDefinition{
				Name: "openapi-handler",
				Type: omniav1alpha1.HandlerTypeOpenAPI,
			}
			tools := reconciler.discoverToolsFromHandler(handler, "https://api.example.com/openapi.json")
			Expect(tools).To(HaveLen(1))
			Expect(tools[0].Name).To(Equal("openapi-handler"))
			Expect(tools[0].Description).To(ContainSubstring("Self-describing"))
			Expect(tools[0].Endpoint).To(Equal("https://api.example.com/openapi.json"))
		})

		It("should return nil for HTTP handler without tool definition", func() {
			handler := &omniav1alpha1.HandlerDefinition{
				Name: "http-no-tool",
				Type: omniav1alpha1.HandlerTypeHTTP,
			}
			tools := reconciler.discoverToolsFromHandler(handler, "http://example.com")
			Expect(tools).To(BeNil())
		})
	})
})

// Ensure unused import doesn't cause issues
var _ = errors.IsNotFound
