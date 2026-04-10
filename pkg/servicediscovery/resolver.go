/*
Copyright 2026.

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

// Package servicediscovery provides URL and config resolution for per-workspace
// session-api and memory-api endpoints. Services resolve their target URLs via
// environment variable overrides (for singletons) or by reading the Workspace CRD
// status from the Kubernetes API (for per-workspace deployments).
package servicediscovery

import (
	"context"
	"fmt"
	"os"

	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// Environment variable names used for service URL overrides and namespace detection.
const (
	envSessionAPIURL  = "SESSION_API_URL"
	envMemoryAPIURL   = "MEMORY_API_URL"
	envOmniaNamespace = "OMNIA_NAMESPACE"
)

// namespaceFilePath is the standard in-cluster service account namespace file.
// It is a variable so tests can override it.
var namespaceFilePath = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

// ServiceURLs holds the resolved URLs for session-api and memory-api.
type ServiceURLs struct {
	// SessionURL is the base URL of the session-api.
	SessionURL string
	// MemoryURL is the base URL of the memory-api.
	MemoryURL string
}

// Resolver resolves service URLs for a given workspace service group.
// It checks environment variable overrides first, then falls back to
// reading Workspace CRD status via the Kubernetes API.
type Resolver struct {
	client client.Client
}

// NewResolver creates a new Resolver. Pass nil for c to use env-var-only mode
// (no Kubernetes lookups will be attempted).
func NewResolver(c client.Client) *Resolver {
	return &Resolver{client: c}
}

// ResolveServiceURLs returns the session-api and memory-api URLs for the given
// service group. It first checks environment variable overrides; if both are set,
// they are returned immediately without any Kubernetes lookups. Otherwise it
// requires a non-nil Kubernetes client to look up the Workspace CRD.
func (r *Resolver) ResolveServiceURLs(ctx context.Context, serviceGroup string) (*ServiceURLs, error) {
	if urls := resolveFromEnv(); urls != nil {
		return urls, nil
	}
	if r.client == nil {
		return nil, fmt.Errorf("no env var overrides set and no Kubernetes client available")
	}
	return r.resolveFromWorkspace(ctx, serviceGroup)
}

// resolveFromEnv returns ServiceURLs from environment variables if both are set,
// otherwise returns nil.
func resolveFromEnv() *ServiceURLs {
	sessionURL := os.Getenv(envSessionAPIURL)
	memoryURL := os.Getenv(envMemoryAPIURL)
	if sessionURL != "" && memoryURL != "" {
		return &ServiceURLs{
			SessionURL: sessionURL,
			MemoryURL:  memoryURL,
		}
	}
	return nil
}

// resolveFromWorkspace looks up the Workspace CRD and returns URLs from its status.
func (r *Resolver) resolveFromWorkspace(ctx context.Context, serviceGroup string) (*ServiceURLs, error) {
	ws, err := r.findWorkspaceByNamespace(ctx)
	if err != nil {
		return nil, fmt.Errorf("find workspace: %w", err)
	}

	for _, svc := range ws.Status.Services {
		if svc.Name != serviceGroup {
			continue
		}
		// Return URLs as soon as they're populated, even if the group isn't
		// fully Ready. Ready requires ALL services (session + memory) to be
		// available, but callers like the eval-worker only need session-api.
		// Blocking on Ready causes unnecessary CrashLoopBackOff when one
		// service has an independent failure (e.g. memory-api migration).
		if svc.SessionURL == "" {
			return nil, fmt.Errorf("service group %q is not ready", serviceGroup)
		}
		return &ServiceURLs{
			SessionURL: svc.SessionURL,
			MemoryURL:  svc.MemoryURL,
		}, nil
	}
	return nil, fmt.Errorf("service group %q not found in workspace %q", serviceGroup, ws.Name)
}

// ResolveByWorkspaceName returns the session-api and memory-api URLs for a
// workspace looked up by its metadata.name rather than by the caller's namespace.
// This is used by cross-namespace consumers like the doctor that run in omnia-system
// but need to discover services in a workspace's namespace.
func (r *Resolver) ResolveByWorkspaceName(
	ctx context.Context, workspaceName, serviceGroup string,
) (*ServiceURLs, error) {
	if urls := resolveFromEnv(); urls != nil {
		return urls, nil
	}
	if r.client == nil {
		return nil, fmt.Errorf("no env var overrides set and no Kubernetes client available")
	}

	var ws omniav1alpha1.Workspace
	if err := r.client.Get(ctx, client.ObjectKey{Name: workspaceName}, &ws); err != nil {
		return nil, fmt.Errorf("get workspace %q: %w", workspaceName, err)
	}

	for _, svc := range ws.Status.Services {
		if svc.Name != serviceGroup {
			continue
		}
		if svc.SessionURL == "" {
			return nil, fmt.Errorf("service group %q is not ready in workspace %q", serviceGroup, workspaceName)
		}
		return &ServiceURLs{
			SessionURL: svc.SessionURL,
			MemoryURL:  svc.MemoryURL,
		}, nil
	}
	return nil, fmt.Errorf("service group %q not found in workspace %q", serviceGroup, workspaceName)
}

// findWorkspaceByNamespace detects the current namespace and finds the Workspace
// whose spec.namespace.name matches it. The namespace is read from the OMNIA_NAMESPACE
// env var, falling back to the in-cluster service account token file.
func (r *Resolver) findWorkspaceByNamespace(ctx context.Context) (*omniav1alpha1.Workspace, error) {
	ns, err := currentNamespace()
	if err != nil {
		return nil, fmt.Errorf("detect namespace: %w", err)
	}

	var list omniav1alpha1.WorkspaceList
	if err := r.client.List(ctx, &list); err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}

	for i := range list.Items {
		if list.Items[i].Spec.Namespace.Name == ns {
			return &list.Items[i], nil
		}
	}
	return nil, fmt.Errorf("no workspace found for namespace %q", ns)
}

// currentNamespace returns the namespace this process is running in.
// It checks the OMNIA_NAMESPACE env var first, then falls back to the
// in-cluster service account namespace file.
func currentNamespace() (string, error) {
	if ns := os.Getenv(envOmniaNamespace); ns != "" {
		return ns, nil
	}
	data, err := os.ReadFile(namespaceFilePath)
	if err != nil {
		return "", fmt.Errorf("read namespace file: %w", err)
	}
	return string(data), nil
}
