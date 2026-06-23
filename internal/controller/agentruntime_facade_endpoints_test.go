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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// This is a wiring test (see CLAUDE.md "Wiring tests"): it verifies that the
// AgentRuntime reconciler's facade-endpoint reconcile path actually reads real
// HTTPRoute/Gateway objects from the API server and populates status.facade.
// The matcher's logic is unit-tested in facade_endpoints_test.go; here we prove
// the wiring (reconcileFacadeEndpoints reads the cluster and the degradation
// gate works).
var _ = Describe("AgentRuntime facade endpoints", func() {
	const (
		ns              = "default"
		testMappedAgent = "mapped-agent"
	)

	Context("when the Gateway API is present", func() {
		BeforeEach(func() {
			if !gatewayCRDsInstalled {
				Skip("gateway-api CRDs not available in module cache; matcher unit tests cover the logic")
			}
		})

		It("populates status.facade from an HTTPRoute + HTTPS Gateway", func() {
			gw := &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "edge-gw", Namespace: ns},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "external",
					Listeners: []gatewayv1.Listener{{
						Name:     "https",
						Port:     443,
						Protocol: gatewayv1.HTTPSProtocolType,
					}},
				},
			}
			Expect(k8sClient.Create(ctx, gw)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, gw) })

			pt := gatewayv1.PathMatchPathPrefix
			prefix := "/"
			pn := gatewayv1.PortNumber(8080)
			route := &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "my-agent-route", Namespace: ns},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{Name: "edge-gw"}},
					},
					Hostnames: []gatewayv1.Hostname{"agents.example.com"},
					Rules: []gatewayv1.HTTPRouteRule{{
						Matches: []gatewayv1.HTTPRouteMatch{{
							Path: &gatewayv1.HTTPPathMatch{Type: &pt, Value: &prefix},
						}},
						BackendRefs: []gatewayv1.HTTPBackendRef{{BackendRef: gatewayv1.BackendRef{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name: testFacadeAgentName,
								Port: &pn,
							},
						}}},
					}},
				},
			}
			Expect(k8sClient.Create(ctx, route)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, route) })

			r := &AgentRuntimeReconciler{Client: k8sClient, gatewayAPIPresent: true}
			agent := wsAgent(testFacadeAgentName)

			Eventually(func(g Gomega) {
				agent.Status.Facade = nil
				r.reconcileFacadeEndpoints(ctx, agent)
				g.Expect(agent.Status.Facade).NotTo(BeNil())
				g.Expect(agent.Status.Facade.Endpoints).To(HaveLen(1))
				ep := agent.Status.Facade.Endpoints[0]
				g.Expect(ep.URL).To(Equal("wss://agents.example.com/ws"))
				g.Expect(ep.Valid).To(BeTrue())
			}).Should(Succeed())
		})

		It("resolves a parentRef Gateway, honouring an explicit namespace", func() {
			gwNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gw-ns"}}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, gwNS))).To(Succeed())
			gw := &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "shared-gw", Namespace: gwNS.Name},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "external",
					Listeners: []gatewayv1.Listener{{
						Name: appProtocolHTTP, Port: 80, Protocol: gatewayv1.HTTPProtocolType,
					}},
				},
			}
			Expect(k8sClient.Create(ctx, gw)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, gw) })

			r := &AgentRuntimeReconciler{Client: k8sClient, gatewayAPIPresent: true}
			resolve := r.gatewayResolver(ctx, ns) // route lives in default ns
			explicitNS := gatewayv1.Namespace(gwNS.Name)

			// Explicit parent.Namespace overrides the route namespace.
			got, ok := resolve(gatewayv1.ParentReference{Name: "shared-gw", Namespace: &explicitNS}, "")
			Expect(ok).To(BeTrue())
			Expect(got.Name).To(Equal("shared-gw"))

			// An unresolvable Gateway returns (nil,false) so the endpoint is skipped.
			_, ok = resolve(gatewayv1.ParentReference{Name: "missing-gw"}, "")
			Expect(ok).To(BeFalse())
		})

		It("clears status.facade when no route matches the agent", func() {
			r := &AgentRuntimeReconciler{Client: k8sClient, gatewayAPIPresent: true}
			agent := wsAgent("unrouted-agent")
			agent.Status.Facade = &omniav1alpha1.FacadeStatus{
				Endpoints: []omniav1alpha1.FacadeEndpoint{{URL: "stale"}},
			}
			r.reconcileFacadeEndpoints(ctx, agent)
			Expect(agent.Status.Facade).To(BeNil())
		})

		It("detects the Gateway API via the cluster RESTMapper", func() {
			httpClient, err := rest.HTTPClientFor(cfg)
			Expect(err).NotTo(HaveOccurred())
			mapper, err := apiutil.NewDynamicRESTMapper(cfg, httpClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(gatewayAPIAvailable(mapper)).To(BeTrue())
		})

		It("maps an HTTPRoute change to the AgentRuntimes its backendRefs name", func() {
			mapNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "facade-map-ns"}}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, mapNS))).To(Succeed())

			port := int32(8080)
			agent := &omniav1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{Name: testMappedAgent, Namespace: mapNS.Name},
				Spec: omniav1alpha1.AgentRuntimeSpec{
					PromptPackRef: omniav1alpha1.PromptPackRef{Name: "dummy"},
					Facade:        omniav1alpha1.FacadeConfig{Type: omniav1alpha1.FacadeType(omniav1alpha1.FacadeProtocolWebSocket), Port: &port},
				},
			}
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, agent) })

			pn := gatewayv1.PortNumber(8080)
			route := &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "map-route", Namespace: mapNS.Name},
				Spec: gatewayv1.HTTPRouteSpec{
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{BackendRef: gatewayv1.BackendRef{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name: testMappedAgent, Port: &pn,
							},
						}}},
					}},
				},
			}
			r := &AgentRuntimeReconciler{Client: k8sClient, gatewayAPIPresent: true}

			reqs := r.findAgentRuntimesForHTTPRoute(ctx, route)
			Expect(reqs).To(ConsistOf(reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testMappedAgent, Namespace: mapNS.Name},
			}))

			// A non-HTTPRoute object yields no requests (type-guard branch).
			Expect(r.findAgentRuntimesForHTTPRoute(ctx, &gatewayv1.Gateway{})).To(BeNil())

			// A Gateway change enqueues all AgentRuntimes; the mapped agent is
			// included.
			Expect(r.findAgentRuntimesForGateway(ctx, &gatewayv1.Gateway{})).To(ContainElement(reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testMappedAgent, Namespace: mapNS.Name},
			}))
		})
	})

	Context("when the Gateway API is absent (graceful degradation)", func() {
		It("clears status.facade and returns no error", func() {
			r := &AgentRuntimeReconciler{Client: k8sClient, gatewayAPIPresent: false}
			agent := wsAgent("any-agent")
			agent.Status.Facade = &omniav1alpha1.FacadeStatus{
				Endpoints: []omniav1alpha1.FacadeEndpoint{{URL: "wss://stale.example.com/ws"}},
			}
			r.reconcileFacadeEndpoints(ctx, agent)
			Expect(agent.Status.Facade).To(BeNil())
		})
	})
})
