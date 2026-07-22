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
	envPrivacyAPIURL  = "PRIVACY_API_URL"
	envOmniaNamespace = "OMNIA_NAMESPACE"
)

// namespaceFilePath is the standard in-cluster service account namespace file.
// It is a variable so tests can override it.
var namespaceFilePath = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

// ServiceURLs holds the resolved URLs for session-api, memory-api, and privacy-api.
type ServiceURLs struct {
	// SessionURL is the base URL of the session-api.
	SessionURL string
	// MemoryURL is the base URL of the memory-api.
	MemoryURL string
	// PrivacyURL is the base URL of the per-workspace privacy-api.
	// Empty string means no privacy-api is configured for this workspace.
	PrivacyURL string
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

// ResolveServiceURLs returns the session-api and memory-api URLs for a service
// group in the named workspace. It first checks environment variable overrides;
// if both are set, they are returned immediately without any Kubernetes lookups.
// Otherwise it requires a non-nil Kubernetes client and does a Get on the named
// Workspace.
//
// The lookup is by name rather than a cluster-wide list filtered on
// spec.namespace.name so that a caller's RBAC can be narrowed with
// resourceNames — an agent pod has no business enumerating other workspaces
// (#1875).
//
// workspaceName is the Workspace CR's metadata.name (e.g. "demo"), NOT the
// namespace that workspace owns (e.g. "omnia-demo").
func (r *Resolver) ResolveServiceURLs(
	ctx context.Context, workspaceName, serviceGroup string,
) (*ServiceURLs, error) {
	if urls := resolveFromEnv(); urls != nil {
		return urls, nil
	}
	if r.client == nil {
		return nil, fmt.Errorf("no env var overrides set and no Kubernetes client available")
	}
	if workspaceName == "" {
		return nil, fmt.Errorf("workspace name is required to resolve service URLs")
	}

	ws, err := r.GetWorkspace(ctx, workspaceName)
	if err != nil {
		return nil, err
	}
	return urlsForGroup(ws, serviceGroup)
}

// GetWorkspace fetches a Workspace by name. Exported so callers needing more
// than URLs off the same object — the runtime wants metadata.uid to scope
// memory — reuse this single read instead of issuing their own lookup.
func (r *Resolver) GetWorkspace(
	ctx context.Context, workspaceName string,
) (*omniav1alpha1.Workspace, error) {
	var ws omniav1alpha1.Workspace
	if err := r.client.Get(ctx, client.ObjectKey{Name: workspaceName}, &ws); err != nil {
		return nil, fmt.Errorf("get workspace %q: %w", workspaceName, err)
	}
	return &ws, nil
}

// urlsForGroup picks a service group's URLs out of a Workspace's status.
func urlsForGroup(ws *omniav1alpha1.Workspace, serviceGroup string) (*ServiceURLs, error) {
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
			PrivacyURL: ws.Status.PrivacyURL,
		}, nil
	}
	return nil, fmt.Errorf("service group %q not found in workspace %q", serviceGroup, ws.Name)
}

// resolveFromEnv returns ServiceURLs from environment variables if both session and
// memory URLs are set, otherwise returns nil. PrivacyURL is additive — it is
// populated if set, but its absence does not block the env-var path.
func resolveFromEnv() *ServiceURLs {
	sessionURL := os.Getenv(envSessionAPIURL)
	memoryURL := os.Getenv(envMemoryAPIURL)
	if sessionURL != "" && memoryURL != "" {
		return &ServiceURLs{
			SessionURL: sessionURL,
			MemoryURL:  memoryURL,
			PrivacyURL: os.Getenv(envPrivacyAPIURL),
		}
	}
	return nil
}

// ResolveByWorkspaceName is retained for cross-namespace consumers like the
// doctor, which run in omnia-system and discover services in some other
// workspace's namespace. It is now exactly ResolveServiceURLs — the two used to
// differ only in how they located the Workspace, and both now do a Get.
func (r *Resolver) ResolveByWorkspaceName(
	ctx context.Context, workspaceName, serviceGroup string,
) (*ServiceURLs, error) {
	return r.ResolveServiceURLs(ctx, workspaceName, serviceGroup)
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

// DetectNamespace returns the Kubernetes namespace this process is running in.
// It checks the OMNIA_NAMESPACE env var first, then the in-cluster service
// account namespace file, and falls back to "default" if neither is available.
// Use this in cmd binaries that need the current namespace but do not need the
// full Resolver; it avoids duplicating the detection logic in each binary.
func DetectNamespace() string {
	ns, err := currentNamespace()
	if err != nil {
		return defaultNamespaceFallback
	}
	return ns
}

// defaultNamespaceFallback is used when the running namespace can't be detected.
const defaultNamespaceFallback = "default"
