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

package servicediscovery

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// secretKeyPostgresConn is the key in the database Secret that holds the PostgreSQL connection string.
const secretKeyPostgresConn = "POSTGRES_CONN"

// SessionConfig holds the resolved configuration for a session-api instance.
type SessionConfig struct {
	// PostgresConn is the PostgreSQL connection string.
	PostgresConn string
}

// MemoryConfig holds the resolved configuration for a memory-api instance.
type MemoryConfig struct {
	// PostgresConn is the PostgreSQL connection string.
	PostgresConn string
	// EmbeddingProviderName is the name of the Provider CRD to use for embeddings.
	// Empty string means no embedding provider is configured.
	EmbeddingProviderName string
}

// ConfigResolver resolves runtime configuration for session-api and memory-api
// by reading Workspace CRDs and referenced Kubernetes Secrets.
type ConfigResolver struct {
	client client.Client
}

// NewConfigResolver creates a new ConfigResolver backed by the given Kubernetes client.
func NewConfigResolver(c client.Client) *ConfigResolver {
	return &ConfigResolver{client: c}
}

// ResolveSessionConfig reads the Workspace CRD and returns session-api configuration
// for the given service group. It reads the database Secret from the given namespace.
func (r *ConfigResolver) ResolveSessionConfig(
	ctx context.Context, workspace, serviceGroup, namespace string,
) (*SessionConfig, error) {
	group, err := r.findServiceGroup(ctx, workspace, serviceGroup)
	if err != nil {
		return nil, err
	}
	if group.Session == nil {
		return nil, fmt.Errorf("service group %q in workspace %q has no session configuration", serviceGroup, workspace)
	}
	conn, err := r.readPostgresConn(ctx, group.Session.Database.SecretRef.Name, namespace)
	if err != nil {
		return nil, err
	}

	return &SessionConfig{PostgresConn: conn}, nil
}

// ResolveMemoryConfig reads the Workspace CRD and returns memory-api configuration
// for the given service group. It reads the database Secret from the given namespace.
func (r *ConfigResolver) ResolveMemoryConfig(
	ctx context.Context, workspace, serviceGroup, namespace string,
) (*MemoryConfig, error) {
	group, err := r.findServiceGroup(ctx, workspace, serviceGroup)
	if err != nil {
		return nil, err
	}
	if group.Memory == nil {
		return nil, fmt.Errorf("service group %q in workspace %q has no memory configuration", serviceGroup, workspace)
	}
	conn, err := r.readPostgresConn(ctx, group.Memory.Database.SecretRef.Name, namespace)
	if err != nil {
		return nil, err
	}

	cfg := &MemoryConfig{PostgresConn: conn}
	if group.Memory.ProviderRef != nil {
		cfg.EmbeddingProviderName = group.Memory.ProviderRef.Name
	}
	return cfg, nil
}

// findServiceGroup retrieves a Workspace by name and returns the named service group.
func (r *ConfigResolver) findServiceGroup(
	ctx context.Context, workspace, serviceGroup string,
) (*omniav1alpha1.WorkspaceServiceGroup, error) {
	var ws omniav1alpha1.Workspace
	if err := r.client.Get(ctx, client.ObjectKey{Name: workspace}, &ws); err != nil {
		return nil, fmt.Errorf("get workspace %q: %w", workspace, err)
	}
	for i := range ws.Spec.Services {
		if ws.Spec.Services[i].Name == serviceGroup {
			return &ws.Spec.Services[i], nil
		}
	}
	return nil, fmt.Errorf("service group %q not found in workspace %q", serviceGroup, workspace)
}

// readPostgresConn reads the POSTGRES_CONN key from a Secret.
func (r *ConfigResolver) readPostgresConn(ctx context.Context, secretName, namespace string) (string, error) {
	var secret corev1.Secret
	key := client.ObjectKey{Name: secretName, Namespace: namespace}
	if err := r.client.Get(ctx, key, &secret); err != nil {
		return "", fmt.Errorf("get secret %q in namespace %q: %w", secretName, namespace, err)
	}
	conn := string(secret.Data[secretKeyPostgresConn])
	if conn == "" {
		return "", fmt.Errorf("secret %q in namespace %q has no %q key", secretName, namespace, secretKeyPostgresConn)
	}
	return conn, nil
}
