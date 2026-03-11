/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// ResolvedA2AClient is the JSON-serializable form injected into the container
// via the OMNIA_A2A_CLIENTS env var.
type ResolvedA2AClient struct {
	Name          string `json:"name"`
	URL           string `json:"url"`
	ExposeAsTools bool   `json:"exposeAsTools,omitempty"`
	// AuthTokenEnv is the env var name holding the bearer token, if configured.
	AuthTokenEnv string `json:"authTokenEnv,omitempty"`
}

// resolveA2AClients resolves all A2A client specs to concrete URLs and reports
// per-client status. In-cluster refs are resolved via the target AgentRuntime's
// service endpoint; external URLs are passed through directly.
func (r *AgentRuntimeReconciler) resolveA2AClients(
	ctx context.Context,
	log logr.Logger,
	agentRuntime *omniav1alpha1.AgentRuntime,
) ([]ResolvedA2AClient, []omniav1alpha1.A2AClientStatus) {
	if agentRuntime.Spec.A2A == nil || len(agentRuntime.Spec.A2A.Clients) == 0 {
		return nil, nil
	}

	resolved := make([]ResolvedA2AClient, 0, len(agentRuntime.Spec.A2A.Clients))
	statuses := make([]omniav1alpha1.A2AClientStatus, 0, len(agentRuntime.Spec.A2A.Clients))

	for _, client := range agentRuntime.Spec.A2A.Clients {
		rc, status := r.resolveOneClient(ctx, log, agentRuntime, client)
		statuses = append(statuses, status)
		if rc != nil {
			resolved = append(resolved, *rc)
		}
	}

	return resolved, statuses
}

// resolveOneClient resolves a single A2AClientSpec.
func (r *AgentRuntimeReconciler) resolveOneClient(
	ctx context.Context,
	log logr.Logger,
	agentRuntime *omniav1alpha1.AgentRuntime,
	client omniav1alpha1.A2AClientSpec,
) (*ResolvedA2AClient, omniav1alpha1.A2AClientStatus) {
	status := omniav1alpha1.A2AClientStatus{Name: client.Name}

	var url string
	if client.AgentRuntimeRef != nil {
		resolved, err := r.resolveAgentRuntimeRef(ctx, agentRuntime, client)
		if err != nil {
			log.V(1).Info("client resolution failed",
				"client", client.Name,
				"error", err)
			status.Error = err.Error()
			return nil, status
		}
		url = resolved
	} else if client.URL != "" {
		url = client.URL
	} else {
		status.Error = "either agentRuntimeRef or url must be specified"
		return nil, status
	}

	status.ResolvedURL = url
	status.Ready = true

	rc := &ResolvedA2AClient{
		Name:          client.Name,
		URL:           url,
		ExposeAsTools: client.ExposeAsTools,
	}

	// If auth is configured, the token will be injected as an env var.
	if client.Authentication != nil && client.Authentication.SecretRef != nil {
		rc.AuthTokenEnv = a2aClientTokenEnvName(client.Name)
	}

	return rc, status
}

// resolveAgentRuntimeRef looks up the target AgentRuntime and extracts its
// A2A endpoint from status, falling back to the service endpoint.
func (r *AgentRuntimeReconciler) resolveAgentRuntimeRef(
	ctx context.Context,
	agentRuntime *omniav1alpha1.AgentRuntime,
	client omniav1alpha1.A2AClientSpec,
) (string, error) {
	ref := client.AgentRuntimeRef
	namespace := agentRuntime.Namespace
	if ref.Namespace != nil {
		namespace = *ref.Namespace
	}

	target := &omniav1alpha1.AgentRuntime{}
	key := types.NamespacedName{Name: ref.Name, Namespace: namespace}
	if err := r.Get(ctx, key, target); err != nil {
		return "", fmt.Errorf("AgentRuntime %s not found: %w", key, err)
	}

	// Prefer the A2A-specific endpoint from status.
	if target.Status.A2A != nil && target.Status.A2A.Endpoint != "" {
		return target.Status.A2A.Endpoint, nil
	}

	// Fall back to the service endpoint.
	if target.Status.ServiceEndpoint != "" {
		return "http://" + target.Status.ServiceEndpoint, nil
	}

	return "", fmt.Errorf("AgentRuntime %s has no A2A or service endpoint", key)
}

// a2aClientTokenEnvName returns the env var name for a client's auth token.
func a2aClientTokenEnvName(clientName string) string {
	return "OMNIA_A2A_CLIENT_TOKEN_" + sanitizeEnvName(clientName)
}

// sanitizeEnvName converts a name to a valid env var suffix (uppercase, underscores).
func sanitizeEnvName(name string) string {
	result := make([]byte, 0, len(name))
	for _, c := range []byte(name) {
		switch {
		case c >= 'a' && c <= 'z':
			result = append(result, c-32) // to uppercase
		case c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			result = append(result, c)
		default:
			result = append(result, '_')
		}
	}
	return string(result)
}

// marshalA2AClients serializes the resolved client list to JSON for the env var.
func marshalA2AClients(clients []ResolvedA2AClient) (string, error) {
	data, err := json.Marshal(clients)
	if err != nil {
		return "", fmt.Errorf("marshal A2A clients: %w", err)
	}
	return string(data), nil
}
