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

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// peerAuthenticationGVK is the Istio PeerAuthentication GVK. We use an
// unstructured object so the operator does not take a build dependency on the
// Istio API module just for an optional defence-in-depth resource.
var peerAuthenticationGVK = schema.GroupVersionKind{
	Group:   "security.istio.io",
	Version: "v1",
	Kind:    "PeerAuthentication",
}

// reconcileServiceAuthNetworkHardening provisions a default-deny-ingress +
// allow-from-known-callers NetworkPolicy for the operator-managed session-api /
// memory-api pods in a workspace namespace, and (when Istio mTLS is requested)
// a STRICT PeerAuthentication. Both are gated by internal service auth being
// enabled; they are no-ops otherwise. This satisfies the review's "at minimum
// ship a default-deny NetworkPolicy" and the "additional layer for Istio" mTLS
// requirement, layered on top of the SA-token auth.
func (r *WorkspaceReconciler) reconcileServiceAuthNetworkHardening(
	ctx context.Context,
	workspace *omniav1alpha1.Workspace,
	namespace string,
) error {
	if r.ServiceBuilder == nil || !r.ServiceBuilder.ServiceAuth.Enabled {
		return nil
	}
	if err := r.reconcileServiceAuthNetworkPolicy(ctx, workspace, namespace); err != nil {
		return err
	}
	if r.ServiceBuilder.ServiceAuth.IstioMTLS {
		return r.reconcileServiceAuthPeerAuthentication(ctx, workspace, namespace)
	}
	return nil
}

// serviceAuthManagedPodSelector matches the operator-managed service pods
// (session-api + memory-api) for this workspace.
func serviceAuthManagedPodSelector(workspaceName string) metav1.LabelSelector {
	return metav1.LabelSelector{
		MatchLabels: map[string]string{
			labelAppManagedBy: labelValueOmniaOperator,
			labelWorkspace:    workspaceName,
		},
	}
}

// reconcileServiceAuthNetworkPolicy creates/updates the default-deny + allow
// NetworkPolicy. Ingress is allowed from pods in the same (workspace) namespace
// — the facade, memory-api and eval-worker callers live there — and from the
// operator namespace (dashboard / operator). Egress is left unrestricted; this
// policy is about closing inbound access to session-api/memory-api.
func (r *WorkspaceReconciler) reconcileServiceAuthNetworkPolicy(
	ctx context.Context,
	workspace *omniav1alpha1.Workspace,
	namespace string,
) error {
	name := fmt.Sprintf("service-auth-%s", workspace.Name)
	desired := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: serviceAuthManagedPodSelector(workspace.Name),
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress:     []networkingv1.NetworkPolicyIngressRule{r.serviceAuthIngressRule(namespace)},
		},
	}

	existing := &networkingv1.NetworkPolicy{}
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), existing)
	if apierrors.IsNotFound(err) {
		if err := controllerutil.SetControllerReference(workspace, desired, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	existing.Spec = desired.Spec
	return r.Update(ctx, existing)
}

// serviceAuthIngressRule builds the single allow-from-known-callers ingress
// rule: same-namespace pods plus the operator namespace (when known).
func (r *WorkspaceReconciler) serviceAuthIngressRule(namespace string) networkingv1.NetworkPolicyIngressRule {
	peers := []networkingv1.NetworkPolicyPeer{
		// Same workspace namespace: facade, memory-api, eval-worker callers.
		{
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{labelK8sMetadataName: namespace},
			},
		},
	}
	if r.OperatorNamespace != "" {
		// Operator / dashboard namespace.
		peers = append(peers, networkingv1.NetworkPolicyPeer{
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{labelK8sMetadataName: r.OperatorNamespace},
			},
		})
	}
	port := intstr.FromInt32(servicePort)
	proto := corev1.ProtocolTCP
	return networkingv1.NetworkPolicyIngressRule{
		From:  peers,
		Ports: []networkingv1.NetworkPolicyPort{{Protocol: &proto, Port: &port}},
	}
}

// reconcileServiceAuthPeerAuthentication creates/updates a STRICT mTLS Istio
// PeerAuthentication targeting the operator-managed service pods. Rendered as
// unstructured to avoid an Istio API build dependency.
func (r *WorkspaceReconciler) reconcileServiceAuthPeerAuthentication(
	ctx context.Context,
	workspace *omniav1alpha1.Workspace,
	namespace string,
) error {
	name := fmt.Sprintf("service-auth-%s", workspace.Name)
	desired := &unstructured.Unstructured{}
	desired.SetGroupVersionKind(peerAuthenticationGVK)
	desired.SetName(name)
	desired.SetNamespace(namespace)
	spec := map[string]interface{}{
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				labelAppManagedBy: labelValueOmniaOperator,
				labelWorkspace:    workspace.Name,
			},
		},
		"mtls": map[string]interface{}{
			"mode": "STRICT",
		},
	}
	if err := unstructured.SetNestedMap(desired.Object, spec, "spec"); err != nil {
		return err
	}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(peerAuthenticationGVK)
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), existing)
	if apierrors.IsNotFound(err) {
		if err := controllerutil.SetControllerReference(workspace, desired, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	if err := unstructured.SetNestedMap(existing.Object, spec, "spec"); err != nil {
		return err
	}
	return r.Update(ctx, existing)
}
