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

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *WorkspaceReconciler) reconcileNetworkPolicy(ctx context.Context, workspace *omniav1alpha1.Workspace) error {
	log := logf.FromContext(ctx)
	namespaceName := workspace.Spec.Namespace.Name
	npName := fmt.Sprintf("workspace-%s-isolation", workspace.Name)

	// If network policy is not configured or isolation is disabled, delete existing NetworkPolicy
	if workspace.Spec.NetworkPolicy == nil || !workspace.Spec.NetworkPolicy.Isolate {
		np := &networkingv1.NetworkPolicy{}
		err := r.Get(ctx, client.ObjectKey{Name: npName, Namespace: namespaceName}, np)
		if err == nil {
			// NetworkPolicy exists, delete it
			if err := r.Delete(ctx, np); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to delete NetworkPolicy %s: %w", npName, err)
			}
			log.Info("Deleted NetworkPolicy (isolation disabled)", "name", npName)
		}
		// Clear status
		workspace.Status.NetworkPolicy = nil
		return nil
	}

	// Build the NetworkPolicy
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      npName,
			Namespace: namespaceName,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, np, func() error {
		np.Labels = map[string]string{
			labelWorkspace:        workspace.Name,
			labelWorkspaceManaged: labelValueTrue,
		}

		// Build ingress rules
		ingressRules := r.buildIngressRules(workspace)

		// Build egress rules
		egressRules := r.buildEgressRules(workspace)

		np.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{}, // Select all pods in namespace
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: ingressRules,
			Egress:  egressRules,
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to create/update NetworkPolicy %s: %w", npName, err)
	}

	if result != controllerutil.OperationResultNone {
		log.Info("NetworkPolicy reconciled", "name", npName, "result", result)
	}

	// Update status
	rulesCount := int32(len(np.Spec.Ingress) + len(np.Spec.Egress))
	workspace.Status.NetworkPolicy = &omniav1alpha1.NetworkPolicyStatus{
		Name:       npName,
		Enabled:    true,
		RulesCount: rulesCount,
	}

	return nil
}

// buildIngressRules builds the ingress rules for the NetworkPolicy
func (r *WorkspaceReconciler) buildIngressRules(workspace *omniav1alpha1.Workspace) []networkingv1.NetworkPolicyIngressRule {
	policy := workspace.Spec.NetworkPolicy
	// Pre-allocate: 1 for same namespace + 1 for shared (if enabled) +
	// 1 for operator namespace (if known) + custom rules.
	capacity := 1 + len(policy.AllowFrom)
	if policy.AllowSharedNamespaces == nil || *policy.AllowSharedNamespaces {
		capacity++
	}
	if r.OperatorNamespace != "" {
		capacity++
	}
	rules := make([]networkingv1.NetworkPolicyIngressRule, 0, capacity)

	// Allow from the operator namespace (dashboard, operator, Prometheus).
	// Matches by the kube-controller-injected namespace label so users
	// don't have to apply `omnia.altairalabs.ai/shared: true` by hand.
	if r.OperatorNamespace != "" {
		rules = append(rules, networkingv1.NetworkPolicyIngressRule{
			From: []networkingv1.NetworkPolicyPeer{
				{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							labelK8sMetadataName: r.OperatorNamespace,
						},
					},
				},
			},
		})
	}

	// Allow from shared namespaces (default true)
	if policy.AllowSharedNamespaces == nil || *policy.AllowSharedNamespaces {
		rules = append(rules, networkingv1.NetworkPolicyIngressRule{
			From: []networkingv1.NetworkPolicyPeer{
				{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							labelSharedNamespace: labelValueTrue,
						},
					},
				},
			},
		})
	}

	// Allow from same namespace
	rules = append(rules, networkingv1.NetworkPolicyIngressRule{
		From: []networkingv1.NetworkPolicyPeer{
			{
				PodSelector: &metav1.LabelSelector{}, // All pods in same namespace
			},
		},
	})

	// Add custom allowFrom rules
	for _, rule := range policy.AllowFrom {
		ingressRule := networkingv1.NetworkPolicyIngressRule{
			From:  convertPeers(rule.Peers),
			Ports: convertPorts(rule.Ports),
		}
		rules = append(rules, ingressRule)
	}

	return rules
}

// buildEgressRules builds the egress rules for the NetworkPolicy
func (r *WorkspaceReconciler) buildEgressRules(workspace *omniav1alpha1.Workspace) []networkingv1.NetworkPolicyEgressRule {
	policy := workspace.Spec.NetworkPolicy
	// Pre-allocate: 1 for DNS + 1 for same namespace + 1 for shared (if
	// enabled) + 1 for external (if enabled) + 1 for operator namespace
	// (if known) + custom rules.
	capacity := 2 + len(policy.AllowTo)
	if policy.AllowSharedNamespaces == nil || *policy.AllowSharedNamespaces {
		capacity++
	}
	if policy.AllowExternalAPIs == nil || *policy.AllowExternalAPIs {
		capacity++
	}
	if r.OperatorNamespace != "" {
		capacity++
	}
	rules := make([]networkingv1.NetworkPolicyEgressRule, 0, capacity)

	// Always allow DNS to kube-system
	dnsPort53 := intstr.FromInt32(53)
	protocolUDP := corev1.ProtocolUDP
	protocolTCP := corev1.ProtocolTCP
	rules = append(rules, networkingv1.NetworkPolicyEgressRule{
		To: []networkingv1.NetworkPolicyPeer{
			{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						labelK8sMetadataName: "kube-system",
					},
				},
			},
		},
		Ports: []networkingv1.NetworkPolicyPort{
			{Protocol: &protocolUDP, Port: &dnsPort53},
			{Protocol: &protocolTCP, Port: &dnsPort53},
		},
	})

	// Allow egress to the operator namespace — the session-api and
	// memory-api pods in a workspace need to reach Postgres / Redis /
	// tracing collectors / other chart-managed services running alongside
	// the operator and dashboard. Chart installs may co-locate Postgres
	// in `omnia-system` as a StatefulSet; operator managed-DB clusters
	// will still route through here for the tracing collector and any
	// enterprise Redis sidecar.
	if r.OperatorNamespace != "" {
		rules = append(rules, networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{
				{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							labelK8sMetadataName: r.OperatorNamespace,
						},
					},
				},
			},
		})
	}

	// Allow to shared namespaces (default true)
	if policy.AllowSharedNamespaces == nil || *policy.AllowSharedNamespaces {
		rules = append(rules, networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{
				{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							labelSharedNamespace: labelValueTrue,
						},
					},
				},
			},
		})
	}

	// Allow to same namespace
	rules = append(rules, networkingv1.NetworkPolicyEgressRule{
		To: []networkingv1.NetworkPolicyPeer{
			{
				PodSelector: &metav1.LabelSelector{}, // All pods in same namespace
			},
		},
	})

	// Allow external APIs (default true) - 0.0.0.0/0 excluding private IP ranges
	if policy.AllowExternalAPIs == nil || *policy.AllowExternalAPIs {
		ipBlock := &networkingv1.IPBlock{
			CIDR: "0.0.0.0/0",
		}
		// Only exclude private networks if allowPrivateNetworks is not explicitly true
		if policy.AllowPrivateNetworks == nil || !*policy.AllowPrivateNetworks {
			ipBlock.Except = []string{
				"10.0.0.0/8",
				"172.16.0.0/12",
				"192.168.0.0/16",
			}
		}
		rules = append(rules, networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{
				{
					IPBlock: ipBlock,
				},
			},
		})
	}

	// Add custom allowTo rules
	for _, rule := range policy.AllowTo {
		egressRule := networkingv1.NetworkPolicyEgressRule{
			To:    convertPeers(rule.Peers),
			Ports: convertPorts(rule.Ports),
		}
		rules = append(rules, egressRule)
	}

	return rules
}

// convertPeers converts API peers to networking v1 peers
func convertPeers(peers []omniav1alpha1.NetworkPolicyPeer) []networkingv1.NetworkPolicyPeer {
	result := make([]networkingv1.NetworkPolicyPeer, 0, len(peers))
	for _, peer := range peers {
		npPeer := networkingv1.NetworkPolicyPeer{}

		if peer.NamespaceSelector != nil {
			npPeer.NamespaceSelector = &metav1.LabelSelector{
				MatchLabels: peer.NamespaceSelector.MatchLabels,
			}
		}

		if peer.PodSelector != nil {
			npPeer.PodSelector = &metav1.LabelSelector{
				MatchLabels: peer.PodSelector.MatchLabels,
			}
		}

		if peer.IPBlock != nil {
			npPeer.IPBlock = &networkingv1.IPBlock{
				CIDR:   peer.IPBlock.CIDR,
				Except: peer.IPBlock.Except,
			}
		}

		result = append(result, npPeer)
	}
	return result
}

// convertPorts converts API ports to networking v1 ports
func convertPorts(ports []omniav1alpha1.NetworkPolicyPort) []networkingv1.NetworkPolicyPort {
	result := make([]networkingv1.NetworkPolicyPort, 0, len(ports))
	for _, port := range ports {
		npPort := networkingv1.NetworkPolicyPort{}

		if port.Protocol != "" {
			protocol := corev1.Protocol(port.Protocol)
			npPort.Protocol = &protocol
		}

		if port.Port != 0 {
			portVal := intstr.FromInt32(port.Port)
			npPort.Port = &portVal
		}

		result = append(result, npPort)
	}
	return result
}
