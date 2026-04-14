/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"context"
	"fmt"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const (
	testIstioAPIVersion = "networking.istio.io/v1"
	testVSKind          = "VirtualService"
	testDRKind          = "DestinationRule"
)

// istioEnvtestCounter gives each spec a unique resource suffix.
var istioEnvtestCounter uint64

var _ = Describe("AgentRuntime Rollout Istio Patching (envtest)", func() {
	var (
		ctx       context.Context
		namespace string
		nextName  = func(prefix string) string {
			n := atomic.AddUint64(&istioEnvtestCounter, 1)
			return fmt.Sprintf("%s-%d", prefix, n)
		}
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = nextName("istio-test")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		})).To(Succeed())
	})

	AfterEach(func() {
		ns := &corev1.Namespace{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: namespace}, ns); err == nil {
			_ = k8sClient.Delete(ctx, ns)
		}
	})

	// newVirtualService seeds a VS with two destinations (stable, canary)
	// inside a single http route named "primary".
	newVirtualService := func(name string) *unstructured.Unstructured {
		vs := &unstructured.Unstructured{}
		vs.SetAPIVersion(testIstioAPIVersion)
		vs.SetKind(testVSKind)
		vs.SetName(name)
		vs.SetNamespace(namespace)
		vs.Object["spec"] = map[string]interface{}{
			"hosts": []interface{}{"example.default.svc.cluster.local"},
			"http": []interface{}{
				map[string]interface{}{
					"name": "primary",
					"route": []interface{}{
						map[string]interface{}{
							"destination": map[string]interface{}{
								"host":   "example",
								"subset": "stable",
							},
							"weight": int64(100),
						},
						map[string]interface{}{
							"destination": map[string]interface{}{
								"host":   "example",
								"subset": "canary",
							},
							"weight": int64(0),
						},
					},
				},
			},
		}
		return vs
	}

	newDestinationRule := func(name string) *unstructured.Unstructured {
		dr := &unstructured.Unstructured{}
		dr.SetAPIVersion(testIstioAPIVersion)
		dr.SetKind(testDRKind)
		dr.SetName(name)
		dr.SetNamespace(namespace)
		dr.Object["spec"] = map[string]interface{}{
			"host": "example",
			"subsets": []interface{}{
				map[string]interface{}{"name": "stable", "labels": map[string]interface{}{"track": "stable"}},
				map[string]interface{}{"name": "canary", "labels": map[string]interface{}{"track": "canary"}},
			},
		}
		return dr
	}

	istioConfig := func(vsName, drName string) *omniav1alpha1.IstioTrafficRouting {
		return &omniav1alpha1.IstioTrafficRouting{
			VirtualService: omniav1alpha1.IstioVirtualServiceRef{
				Name:   vsName,
				Routes: []string{"primary"},
			},
			DestinationRule: omniav1alpha1.IstioDestinationRuleRef{
				Name:            drName,
				StableSubset:    "stable",
				CandidateSubset: "canary",
			},
		}
	}

	// getRouteWeights reads back the stable/canary weights from the VS's
	// primary route for assertions.
	getRouteWeights := func(vsName string) (stable, canary int64) {
		vs := &unstructured.Unstructured{}
		vs.SetAPIVersion(testIstioAPIVersion)
		vs.SetKind(testVSKind)
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: vsName, Namespace: namespace}, vs)).To(Succeed())

		routes, found, err := unstructured.NestedSlice(vs.Object, "spec", "http")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(routes).NotTo(BeEmpty())
		primary, ok := routes[0].(map[string]interface{})
		Expect(ok).To(BeTrue())
		dests, _ := primary["route"].([]interface{})
		for _, d := range dests {
			dest := d.(map[string]interface{})
			subset, _, _ := unstructured.NestedString(dest, "destination", "subset")
			w, _ := dest["weight"].(int64)
			switch subset {
			case "stable":
				stable = w
			case "canary":
				canary = w
			}
		}
		return stable, canary
	}

	It("patchVirtualServiceWeights splits traffic according to candidateWeight", func() {
		vsName := nextName("vs")
		drName := nextName("dr")
		Expect(k8sClient.Create(ctx, newVirtualService(vsName))).To(Succeed())
		Expect(k8sClient.Create(ctx, newDestinationRule(drName))).To(Succeed())

		r := &AgentRuntimeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		Expect(r.patchVirtualServiceWeights(ctx, namespace, istioConfig(vsName, drName), 30)).To(Succeed())

		stable, canary := getRouteWeights(vsName)
		Expect(stable).To(Equal(int64(70)))
		Expect(canary).To(Equal(int64(30)))
	})

	It("resetTrafficRouting restores 100%% stable", func() {
		vsName := nextName("vs")
		drName := nextName("dr")
		Expect(k8sClient.Create(ctx, newVirtualService(vsName))).To(Succeed())
		Expect(k8sClient.Create(ctx, newDestinationRule(drName))).To(Succeed())

		r := &AgentRuntimeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		// Move traffic to 60/40, then reset.
		Expect(r.patchVirtualServiceWeights(ctx, namespace, istioConfig(vsName, drName), 40)).To(Succeed())
		Expect(r.resetTrafficRouting(ctx, namespace, istioConfig(vsName, drName))).To(Succeed())

		stable, canary := getRouteWeights(vsName)
		Expect(stable).To(Equal(int64(100)))
		Expect(canary).To(Equal(int64(0)))
	})

	It("patchVirtualServiceWeights skips routes not listed in VirtualService.Routes", func() {
		vsName := nextName("vs")
		drName := nextName("dr")

		// Add a second http route that is NOT in the target list — it should
		// be left alone.
		vs := newVirtualService(vsName)
		routes, _, _ := unstructured.NestedSlice(vs.Object, "spec", "http")
		routes = append(routes, map[string]interface{}{
			"name": "secondary",
			"route": []interface{}{
				map[string]interface{}{
					"destination": map[string]interface{}{"host": "example", "subset": "stable"},
					"weight":      int64(100),
				},
				map[string]interface{}{
					"destination": map[string]interface{}{"host": "example", "subset": "canary"},
					"weight":      int64(0),
				},
			},
		})
		Expect(unstructured.SetNestedSlice(vs.Object, routes, "spec", "http")).To(Succeed())

		Expect(k8sClient.Create(ctx, vs)).To(Succeed())
		Expect(k8sClient.Create(ctx, newDestinationRule(drName))).To(Succeed())

		r := &AgentRuntimeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		Expect(r.patchVirtualServiceWeights(ctx, namespace, istioConfig(vsName, drName), 50)).To(Succeed())

		fresh := &unstructured.Unstructured{}
		fresh.SetAPIVersion(testIstioAPIVersion)
		fresh.SetKind(testVSKind)
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: vsName, Namespace: namespace}, fresh)).To(Succeed())
		httpRoutes, _, _ := unstructured.NestedSlice(fresh.Object, "spec", "http")

		for _, r := range httpRoutes {
			route := r.(map[string]interface{})
			name, _ := route["name"].(string)
			dests := route["route"].([]interface{})
			var canaryW int64
			for _, d := range dests {
				dest := d.(map[string]interface{})
				subset, _, _ := unstructured.NestedString(dest, "destination", "subset")
				if subset == "canary" {
					w, _ := dest["weight"].(int64)
					canaryW = w
				}
			}
			if name == "primary" {
				Expect(canaryW).To(Equal(int64(50)), "primary should be patched to 50")
			} else {
				Expect(canaryW).To(Equal(int64(0)), "secondary should be untouched")
			}
		}
	})

	It("patchDestinationRuleConsistentHash sets and then clears the httpHeaderName", func() {
		vsName := nextName("vs")
		drName := nextName("dr")
		Expect(k8sClient.Create(ctx, newVirtualService(vsName))).To(Succeed())
		Expect(k8sClient.Create(ctx, newDestinationRule(drName))).To(Succeed())

		r := &AgentRuntimeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		Expect(r.patchDestinationRuleConsistentHash(ctx, namespace, istioConfig(vsName, drName), "x-user-id")).To(Succeed())

		dr := &unstructured.Unstructured{}
		dr.SetAPIVersion(testIstioAPIVersion)
		dr.SetKind(testDRKind)
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: drName, Namespace: namespace}, dr)).To(Succeed())

		header, found, err := unstructured.NestedString(dr.Object,
			"spec", "trafficPolicy", "loadBalancer", "consistentHash", "httpHeaderName")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(header).To(Equal("x-user-id"))

		// Clearing with an empty header removes the consistentHash block.
		Expect(r.patchDestinationRuleConsistentHash(ctx, namespace, istioConfig(vsName, drName), "")).To(Succeed())

		cleared := &unstructured.Unstructured{}
		cleared.SetAPIVersion(testIstioAPIVersion)
		cleared.SetKind(testDRKind)
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: drName, Namespace: namespace}, cleared)).To(Succeed())
		_, stillThere, _ := unstructured.NestedMap(cleared.Object,
			"spec", "trafficPolicy", "loadBalancer", "consistentHash")
		Expect(stillThere).To(BeFalse(),
			"consistentHash block should have been removed when header is empty")
	})

	It("patchVirtualServiceWeights returns a clear error when the VirtualService is missing", func() {
		r := &AgentRuntimeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		err := r.patchVirtualServiceWeights(ctx, namespace,
			istioConfig("does-not-exist-vs", "does-not-exist-dr"), 30)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("get VirtualService"))
	})
})
