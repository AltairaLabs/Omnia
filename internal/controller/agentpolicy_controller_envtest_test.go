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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// agentPolicyEnvtestCounter gives each spec a unique resource suffix.
var agentPolicyEnvtestCounter uint64

var _ = Describe("AgentPolicy Controller (envtest)", func() {
	var (
		ctx       context.Context
		namespace string
		nextName  = func(prefix string) string {
			n := atomic.AddUint64(&agentPolicyEnvtestCounter, 1)
			return fmt.Sprintf("%s-%d", prefix, n)
		}
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = nextName("ap-test")
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

	Context("field validation (API server enforcement)", func() {
		It("rejects a claim mapping header that doesn't start with X-Omnia-Claim-", func() {
			p := &omniav1alpha1.AgentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: nextName("ap"), Namespace: namespace},
				Spec: omniav1alpha1.AgentPolicySpec{
					ClaimMapping: &omniav1alpha1.ClaimMapping{
						ForwardClaims: []omniav1alpha1.ClaimMappingEntry{
							{Claim: "team", Header: "X-Custom-Team"},
						},
					},
				},
			}
			err := k8sClient.Create(ctx, p)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue(),
				"expected 400 Invalid, got: %v", err)
		})

		It("rejects an empty claim name", func() {
			p := &omniav1alpha1.AgentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: nextName("ap"), Namespace: namespace},
				Spec: omniav1alpha1.AgentPolicySpec{
					ClaimMapping: &omniav1alpha1.ClaimMapping{
						ForwardClaims: []omniav1alpha1.ClaimMappingEntry{
							{Claim: "", Header: "X-Omnia-Claim-Team"},
						},
					},
				},
			}
			err := k8sClient.Create(ctx, p)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("rejects an invalid mode enum value", func() {
			p := &omniav1alpha1.AgentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: nextName("ap"), Namespace: namespace},
				Spec: omniav1alpha1.AgentPolicySpec{
					Mode: omniav1alpha1.AgentPolicyMode("bogus"),
				},
			}
			err := k8sClient.Create(ctx, p)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("rejects toolAccess with an empty Rules list", func() {
			p := &omniav1alpha1.AgentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: nextName("ap"), Namespace: namespace},
				Spec: omniav1alpha1.AgentPolicySpec{
					ToolAccess: &omniav1alpha1.ToolAccessConfig{
						Mode:  omniav1alpha1.ToolAccessModeAllowlist,
						Rules: []omniav1alpha1.ToolAccessRule{},
					},
				},
			}
			err := k8sClient.Create(ctx, p)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("rejects a toolAccess rule with an empty Tools list", func() {
			p := &omniav1alpha1.AgentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: nextName("ap"), Namespace: namespace},
				Spec: omniav1alpha1.AgentPolicySpec{
					ToolAccess: &omniav1alpha1.ToolAccessConfig{
						Mode: omniav1alpha1.ToolAccessModeAllowlist,
						Rules: []omniav1alpha1.ToolAccessRule{
							{Registry: "r1", Tools: []string{}},
						},
					},
				},
			}
			err := k8sClient.Create(ctx, p)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("accepts a well-formed claim-mapping policy", func() {
			p := &omniav1alpha1.AgentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: nextName("ap"), Namespace: namespace},
				Spec: omniav1alpha1.AgentPolicySpec{
					ClaimMapping: &omniav1alpha1.ClaimMapping{
						ForwardClaims: []omniav1alpha1.ClaimMappingEntry{
							{Claim: "team", Header: "X-Omnia-Claim-Team"},
						},
					},
					Mode: omniav1alpha1.AgentPolicyModeEnforce,
				},
			}
			Expect(k8sClient.Create(ctx, p)).To(Succeed())
		})
	})

	Context("reconcile against real API server", func() {
		It("reaches Active phase, sets observedGeneration, and writes Valid+Applied conditions", func() {
			name := nextName("ap")
			p := &omniav1alpha1.AgentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Spec: omniav1alpha1.AgentPolicySpec{
					ClaimMapping: &omniav1alpha1.ClaimMapping{
						ForwardClaims: []omniav1alpha1.ClaimMappingEntry{
							{Claim: "team", Header: "X-Omnia-Claim-Team"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, p)).To(Succeed())

			reconciler := &AgentPolicyReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			var updated omniav1alpha1.AgentPolicy
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: name, Namespace: namespace,
			}, &updated)).To(Succeed())

			Expect(updated.Status.Phase).To(Equal(omniav1alpha1.AgentPolicyPhaseActive))
			Expect(updated.Status.ObservedGeneration).To(Equal(updated.Generation))
			expectAgentPolicyCondition(&updated, AgentPolicyConditionTypeValid, metav1.ConditionTrue)
			expectAgentPolicyCondition(&updated, AgentPolicyConditionTypeApplied, metav1.ConditionTrue)
		})

		It("catches observedGeneration up after a spec change", func() {
			name := nextName("ap")
			p := &omniav1alpha1.AgentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Spec: omniav1alpha1.AgentPolicySpec{
					ClaimMapping: &omniav1alpha1.ClaimMapping{
						ForwardClaims: []omniav1alpha1.ClaimMappingEntry{
							{Claim: "team", Header: "X-Omnia-Claim-Team"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, p)).To(Succeed())

			reconciler := &AgentPolicyReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name, Namespace: namespace},
			}
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			var first omniav1alpha1.AgentPolicy
			Expect(k8sClient.Get(ctx, req.NamespacedName, &first)).To(Succeed())
			gen1 := first.Generation

			first.Spec.ClaimMapping.ForwardClaims = append(first.Spec.ClaimMapping.ForwardClaims,
				omniav1alpha1.ClaimMappingEntry{Claim: "region", Header: "X-Omnia-Claim-Region"})
			Expect(k8sClient.Update(ctx, &first)).To(Succeed())

			var afterUpdate omniav1alpha1.AgentPolicy
			Expect(k8sClient.Get(ctx, req.NamespacedName, &afterUpdate)).To(Succeed())
			Expect(afterUpdate.Generation).To(BeNumerically(">", gen1))

			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			var final omniav1alpha1.AgentPolicy
			Expect(k8sClient.Get(ctx, req.NamespacedName, &final)).To(Succeed())
			Expect(final.Status.ObservedGeneration).To(Equal(final.Generation))
		})

		It("counts matched agents against real AgentRuntime objects in the namespace", func() {
			// Two matching AgentRuntimes, one not in the selector.
			port := int32(8080)
			for _, agentName := range []string{"agent-a", "agent-b", "agent-c"} {
				ar := &omniav1alpha1.AgentRuntime{
					ObjectMeta: metav1.ObjectMeta{Name: agentName, Namespace: namespace},
					Spec: omniav1alpha1.AgentRuntimeSpec{
						PromptPackRef: omniav1alpha1.PromptPackRef{Name: "dummy"},
						Facade: omniav1alpha1.FacadeConfig{
							Type: omniav1alpha1.FacadeType("websocket"),
							Port: &port,
						},
					},
				}
				Expect(k8sClient.Create(ctx, ar)).To(Succeed())
			}

			name := nextName("ap")
			p := &omniav1alpha1.AgentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
				Spec: omniav1alpha1.AgentPolicySpec{
					Selector: &omniav1alpha1.AgentPolicySelector{Agents: []string{"agent-a", "agent-b"}},
					ClaimMapping: &omniav1alpha1.ClaimMapping{
						ForwardClaims: []omniav1alpha1.ClaimMappingEntry{
							{Claim: "team", Header: "X-Omnia-Claim-Team"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, p)).To(Succeed())

			reconciler := &AgentPolicyReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name, Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			var updated omniav1alpha1.AgentPolicy
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: name, Namespace: namespace,
			}, &updated)).To(Succeed())
			Expect(updated.Status.MatchedAgents).To(Equal(int32(2)))
		})
	})
})

func expectAgentPolicyCondition(p *omniav1alpha1.AgentPolicy, condType string, want metav1.ConditionStatus) {
	GinkgoHelper()
	for _, c := range p.Status.Conditions {
		if c.Type == condType {
			Expect(c.Status).To(Equal(want),
				"condition %q status mismatch (reason=%s message=%s)",
				condType, c.Reason, c.Message)
			return
		}
	}
	Fail(fmt.Sprintf("condition %q not present", condType))
}
