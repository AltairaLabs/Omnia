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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// DefaultExposureConfig is the chart-configured Gateway + base domain used to
// provision per-agent HTTPRoutes (#1553). The feature is OFF unless both a
// gateway name and a base domain are set (`configured`); even then an agent is
// exposed only when it opts in via the primary facade's expose.enabled.
type DefaultExposureConfig struct {
	BaseDomain       string
	GatewayName      string
	GatewayNamespace string
	GatewaySection   string
}

// configured reports whether the platform has a default-exposure Gateway wired.
func (c DefaultExposureConfig) configured() bool {
	return c.GatewayName != "" && c.BaseDomain != ""
}

// facadeRouteName is the name of the operator-managed HTTPRoute for an agent.
func facadeRouteName(agent *omniav1alpha1.AgentRuntime) string {
	return agent.Name + "-facade"
}

// reconcileFacadeRoute creates/updates the agent's external HTTPRoute when the
// agent opted into exposure and a default-exposure Gateway is configured, and
// removes it otherwise. It never touches an HTTPRoute of the same name that the
// operator does not own (e.g. a hand-written route), and is a no-op when the
// Gateway API CRDs are not installed.
func (r *AgentRuntimeReconciler) reconcileFacadeRoute(
	ctx context.Context,
	agent *omniav1alpha1.AgentRuntime,
) error {
	log := logf.FromContext(ctx)
	key := types.NamespacedName{Namespace: agent.Namespace, Name: facadeRouteName(agent)}

	existing := &gatewayv1.HTTPRoute{}
	err := r.Get(ctx, key, existing)
	switch {
	case apierrors.IsNotFound(err):
		existing = nil
	case meta.IsNoMatchError(err):
		// Gateway API CRDs absent — nothing to provision or clean up.
		return nil
	case err != nil:
		return err
	}

	// Never adopt or delete a route the operator does not own.
	if existing != nil && !metav1.IsControlledBy(existing, agent) {
		log.V(1).Info("facade route exists but is not operator-owned; leaving it",
			"route", key.Name)
		return nil
	}

	want, host, port := r.exposeDecision(agent)
	if !want {
		if existing == nil {
			return nil
		}
		log.V(1).Info("removing facade route (exposure disabled)", "route", key.Name)
		return client.IgnoreNotFound(r.Delete(ctx, existing))
	}

	route := &gatewayv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace}}
	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, route, func() error {
		if err := controllerutil.SetControllerReference(agent, route, r.Scheme); err != nil {
			return err
		}
		route.Labels = map[string]string{
			labelAppName:      labelValueOmniaAgent,
			labelAppInstance:  agent.Name,
			labelAppManagedBy: labelValueOmniaOperator,
		}
		route.Spec = r.buildHTTPRouteSpec(agent.Name, host, port)
		return nil
	})
	if err != nil {
		return err
	}
	log.V(1).Info("facade route reconciled", "route", key.Name, "host", host, "result", result)
	return nil
}

// exposeDecision returns whether the agent should have an external route, and
// the hostname + backend Service port to use. host-based only: a root-path
// HTTPRoute that the #1559 discovery marks valid:true with no URL rewrite.
//
// Exposure follows the primary facade's expose config and routes to the primary
// facade port. Independently exposing a secondary facade (e.g. an external a2a
// route alongside a websocket primary) is a known gap — a dual agent's a2a card
// then falls back to the in-cluster URL (correct, just not externally routed).
func (r *AgentRuntimeReconciler) exposeDecision(agent *omniav1alpha1.AgentRuntime) (bool, string, int32) {
	if !r.DefaultExposure.configured() {
		return false, "", 0
	}
	f := primaryFacade(agent)
	if f == nil || f.Expose == nil || !f.Expose.Enabled {
		return false, "", 0
	}
	host := f.Expose.Host
	if host == "" {
		host = fmt.Sprintf("%s.%s.%s", agent.Name, agent.Namespace, r.DefaultExposure.BaseDomain)
	}
	return true, host, primaryFacadePort(agent)
}

// buildHTTPRouteSpec builds a host-based HTTPRoute spec pointing the configured
// Gateway at the agent's facade Service (named after the agent) on the facade
// port, matching all paths at root (no rewrite needed).
func (r *AgentRuntimeReconciler) buildHTTPRouteSpec(serviceName, host string, port int32) gatewayv1.HTTPRouteSpec {
	parent := gatewayv1.ParentReference{Name: gatewayv1.ObjectName(r.DefaultExposure.GatewayName)}
	if r.DefaultExposure.GatewayNamespace != "" {
		ns := gatewayv1.Namespace(r.DefaultExposure.GatewayNamespace)
		parent.Namespace = &ns
	}
	if r.DefaultExposure.GatewaySection != "" {
		sn := gatewayv1.SectionName(r.DefaultExposure.GatewaySection)
		parent.SectionName = &sn
	}

	pathType := gatewayv1.PathMatchPathPrefix
	rootPath := "/"

	return gatewayv1.HTTPRouteSpec{
		CommonRouteSpec: gatewayv1.CommonRouteSpec{ParentRefs: []gatewayv1.ParentReference{parent}},
		Hostnames:       []gatewayv1.Hostname{gatewayv1.Hostname(host)},
		Rules: []gatewayv1.HTTPRouteRule{{
			Matches: []gatewayv1.HTTPRouteMatch{{
				Path: &gatewayv1.HTTPPathMatch{Type: &pathType, Value: &rootPath},
			}},
			BackendRefs: []gatewayv1.HTTPBackendRef{{
				BackendRef: gatewayv1.BackendRef{
					BackendObjectReference: gatewayv1.BackendObjectReference{
						Name: gatewayv1.ObjectName(serviceName),
						Port: &port, // gatewayv1.PortNumber is an alias for int32
					},
				},
			}},
		}},
	}
}
