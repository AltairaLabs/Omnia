# Per-Workspace Session-API and Memory-API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace singleton Helm-managed session-api and memory-api with per-workspace, operator-managed instances configured from the Workspace CRD.

**Architecture:** The Workspace CRD gains a `services` block with named, paired service groups. The Workspace controller reconciles Deployments and Services for managed instances. A shared service-discovery client resolves URLs from the Workspace CRD status (in-cluster) or env vars (local dev). All consumers (facade, runtime, dashboard, Arena) switch from hardcoded singleton URLs to workspace-aware resolution.

**Tech Stack:** Go, Kubernetes controller-runtime, kubebuilder CRD markers, CEL validation, PostgreSQL migrations, Helm, Next.js (dashboard proxy updates)

**Spec:** `docs/superpowers/specs/2026-04-04-per-workspace-services-design.md`

---

## File Structure

### New Files

| File | Purpose |
|------|---------|
| `pkg/servicediscovery/resolver.go` | Service URL resolution: env var fallback → Workspace CRD status |
| `pkg/servicediscovery/resolver_test.go` | Unit tests for resolver |
| `pkg/servicediscovery/config.go` | Service config resolution for session-api/memory-api self-configuration |
| `pkg/servicediscovery/config_test.go` | Unit tests for config resolution |
| `internal/controller/service_builder.go` | Deployment/Service builder for workspace service instances |
| `internal/controller/service_builder_test.go` | Unit tests for service builder |
| `internal/controller/workspace_services.go` | `reconcileServices` method for workspace controller |
| `internal/controller/workspace_services_test.go` | Integration tests (envtest) for service reconciliation |
| `internal/memory/postgres/migrations/000001_initial_schema.up.sql` | Fresh memory-only schema |
| `internal/memory/postgres/migrations/000001_initial_schema.down.sql` | Drop memory schema |
| `internal/memory/postgres/migrator.go` | Memory-specific migrator (embed migrations) |
| `internal/memory/postgres/migrator_test.go` | Migrator tests |
| `cmd/session-api/SERVICE.md` | Session-api architectural docs |
| `cmd/session-api/CLAUDE.md` | Session-api dev instructions |
| `cmd/memory-api/SERVICE.md` | Memory-api architectural docs |
| `cmd/memory-api/CLAUDE.md` | Memory-api dev instructions |

### Modified Files

| File | Change |
|------|--------|
| `api/v1alpha1/workspace_types.go` | Add `Services` field to spec, `Services` to status, new types |
| `api/v1alpha1/agentruntime_types.go` | Add `ServiceGroup` field to spec |
| `internal/controller/workspace_controller.go` | Call `reconcileServices`, add RBAC markers, add K8s Service to watches |
| `internal/controller/deployment_builder.go` | Remove `SESSION_API_URL` env var injection from agent pods |
| `internal/controller/agentruntime_controller.go` | Remove `SessionAPIURL` field |
| `internal/controller/eval_worker.go` | Resolve session-api URL from workspace instead of reconciler field |
| `cmd/main.go` | Remove `sessionAPIURL` from AgentRuntimeReconciler init; add service images to WorkspaceReconciler |
| `cmd/agent/main.go` | Replace env var with service discovery client |
| `internal/runtime/config_crd.go` | Replace env var reads with Workspace CRD status lookup |
| `internal/runtime/config.go` | Remove `envSessionAPIURL` / `envMemoryAPIURL` constants |
| `cmd/session-api/main.go` | Add `--workspace`/`--service-group` flags, K8s client config resolution |
| `cmd/memory-api/main.go` | Add `--workspace`/`--service-group` flags, K8s client config resolution, separate memory migrator |
| `internal/session/postgres/migrations/` | Remove memory tables (000025-000027), keep session-only schema |
| `internal/doctor/checks/sessions.go` | Make workspace-aware |
| `internal/doctor/checks/memory.go` | Make workspace-aware |
| `cmd/doctor/main.go` | Resolve service URLs per workspace |
| `ee/internal/controller/arenajob_controller.go` | Resolve session-api URL from workspace |
| `ee/internal/controller/arenadevsession_controller.go` | Resolve session-api URL from workspace |
| `ee/cmd/omnia-arena-controller/main.go` | Remove `sessionAPIURL` field from controller init |
| `charts/omnia/values.yaml` | Remove sessionApi/memoryApi sections |
| `charts/omnia/templates/clusterrole.yaml` | Add K8s Service RBAC if not present |

### Deleted Files

| File | Reason |
|------|--------|
| `charts/omnia/templates/session-api/` (8 files) | Singleton replaced by operator-managed instances |
| `charts/omnia/templates/memory-api/` (7 files) | Singleton replaced by operator-managed instances |

---

## Task 1: CRD Type Definitions

**Files:**
- Modify: `api/v1alpha1/workspace_types.go:412-621`
- Modify: `api/v1alpha1/agentruntime_types.go` (AgentRuntimeSpec struct)

### Steps

- [ ] **Step 1: Write test for new Workspace CRD types**

Create a test file that validates the new types compile and serialize correctly:

```go
// api/v1alpha1/workspace_types_test.go
package v1alpha1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestWorkspaceServiceGroupJSON(t *testing.T) {
	sg := WorkspaceServiceGroup{
		Name: "default",
		Mode: ServiceModeManaged,
		Memory: &MemoryServiceConfig{
			Database: DatabaseConfig{
				SecretRef: corev1.LocalObjectReference{Name: "mem-db"},
			},
			ProviderRef: &corev1.LocalObjectReference{Name: "ollama-embed"},
			Retention:   &MemoryRetentionConfig{DefaultTTL: "720h"},
		},
		Session: &SessionServiceConfig{
			Database: DatabaseConfig{
				SecretRef: corev1.LocalObjectReference{Name: "sess-db"},
			},
			Retention: &SessionRetentionConfig{WarmDays: ptr(int32(30))},
		},
	}

	data, err := json.Marshal(sg)
	require.NoError(t, err)

	var decoded WorkspaceServiceGroup
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "default", decoded.Name)
	assert.Equal(t, ServiceModeManaged, decoded.Mode)
	assert.Equal(t, "mem-db", decoded.Memory.Database.SecretRef.Name)
	assert.Equal(t, "ollama-embed", decoded.Memory.ProviderRef.Name)
	assert.Equal(t, "720h", decoded.Memory.Retention.DefaultTTL)
	assert.Equal(t, "sess-db", decoded.Session.Database.SecretRef.Name)
	assert.Equal(t, int32(30), *decoded.Session.Retention.WarmDays)
}

func TestWorkspaceServiceGroupExternalJSON(t *testing.T) {
	sg := WorkspaceServiceGroup{
		Name: "legacy",
		Mode: ServiceModeExternal,
		External: &ExternalEndpoints{
			SessionURL: "http://custom-session:8080",
			MemoryURL:  "http://custom-memory:8080",
		},
	}

	data, err := json.Marshal(sg)
	require.NoError(t, err)

	var decoded WorkspaceServiceGroup
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, ServiceModeExternal, decoded.Mode)
	assert.Equal(t, "http://custom-session:8080", decoded.External.SessionURL)
	assert.Equal(t, "http://custom-memory:8080", decoded.External.MemoryURL)
}

func TestServiceGroupStatusJSON(t *testing.T) {
	s := ServiceGroupStatus{
		Name:       "default",
		SessionURL: "http://session-myws-default.ws-myws:8080",
		MemoryURL:  "http://memory-myws-default.ws-myws:8080",
		Ready:      true,
	}

	data, err := json.Marshal(s)
	require.NoError(t, err)

	var decoded ServiceGroupStatus
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.True(t, decoded.Ready)
	assert.Equal(t, "http://session-myws-default.ws-myws:8080", decoded.SessionURL)
}

func TestWorkspaceServiceGroupDefaultMode(t *testing.T) {
	sg := WorkspaceServiceGroup{Name: "default"}
	assert.Equal(t, ServiceMode(""), sg.Mode) // empty means managed
}

func ptr[T any](v T) *T { return &v }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/v1alpha1/ -run TestWorkspaceServiceGroup -count=1 -v`
Expected: FAIL — types don't exist yet

- [ ] **Step 3: Add new types to workspace_types.go**

In `api/v1alpha1/workspace_types.go`, add the `Services` field to `WorkspaceSpec` (after the `Storage` field, before the closing brace at line 469):

```go
	// services defines per-workspace service instances (session-api + memory-api pairs).
	// Each entry is a named service group. AgentRuntimes reference a group by name.
	// +optional
	Services []WorkspaceServiceGroup `json:"services,omitempty"`
```

Add the `Services` field to `WorkspaceStatus` (after `Storage` field, before `Conditions`):

```go
	// services tracks the status of per-workspace service instances.
	// +optional
	Services []ServiceGroupStatus `json:"services,omitempty"`
```

Add new types before the `func init()` at line 619:

```go
// ServiceMode defines whether a service group is operator-managed or externally provided.
// +kubebuilder:validation:Enum=managed;external
type ServiceMode string

const (
	// ServiceModeManaged means the operator creates and manages Deployments/Services.
	ServiceModeManaged ServiceMode = "managed"
	// ServiceModeExternal means the user provides pre-existing service URLs.
	ServiceModeExternal ServiceMode = "external"
)

// WorkspaceServiceGroup defines a named pair of session-api and memory-api instances.
type WorkspaceServiceGroup struct {
	// name is the identifier for this service group. AgentRuntimes reference this name.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`
	Name string `json:"name"`

	// mode specifies whether the operator manages this service group or if it's external.
	// Defaults to "managed" if omitted.
	// +kubebuilder:default=managed
	// +optional
	Mode ServiceMode `json:"mode,omitempty"`

	// memory configures the memory-api instance for this service group.
	// Required when mode is "managed".
	// +optional
	Memory *MemoryServiceConfig `json:"memory,omitempty"`

	// session configures the session-api instance for this service group.
	// Required when mode is "managed".
	// +optional
	Session *SessionServiceConfig `json:"session,omitempty"`

	// external provides URLs for pre-existing service instances.
	// Required when mode is "external".
	// +optional
	External *ExternalEndpoints `json:"external,omitempty"`
}

// MemoryServiceConfig configures a managed memory-api instance.
type MemoryServiceConfig struct {
	// database configures the PostgreSQL connection for this memory-api instance.
	// +kubebuilder:validation:Required
	Database DatabaseConfig `json:"database"`

	// providerRef references a Provider CRD for the embedding model.
	// If omitted, memory-api starts without embeddings (semantic search disabled).
	// +optional
	ProviderRef *corev1.LocalObjectReference `json:"providerRef,omitempty"`

	// retention configures memory retention policies.
	// +optional
	Retention *MemoryRetentionConfig `json:"retention,omitempty"`
}

// SessionServiceConfig configures a managed session-api instance.
type SessionServiceConfig struct {
	// database configures the PostgreSQL connection for this session-api instance.
	// +kubebuilder:validation:Required
	Database DatabaseConfig `json:"database"`

	// retention configures session retention policies.
	// +optional
	Retention *SessionRetentionConfig `json:"retention,omitempty"`
}

// DatabaseConfig configures a PostgreSQL connection via a Secret reference.
type DatabaseConfig struct {
	// secretRef references a Secret containing a "POSTGRES_CONN" key with the full connection string.
	// +kubebuilder:validation:Required
	SecretRef corev1.LocalObjectReference `json:"secretRef"`
}

// MemoryRetentionConfig configures memory retention policies.
type MemoryRetentionConfig struct {
	// defaultTTL is the default time-to-live for memory entries (e.g. "720h").
	// +optional
	DefaultTTL string `json:"defaultTTL,omitempty"`
}

// SessionRetentionConfig configures session retention policies.
type SessionRetentionConfig struct {
	// warmDays is the number of days to keep sessions in the warm (fast-access) tier.
	// +optional
	WarmDays *int32 `json:"warmDays,omitempty"`
}

// ExternalEndpoints provides URLs for externally managed service instances.
type ExternalEndpoints struct {
	// sessionURL is the HTTP URL of the external session-api instance.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https?://`
	SessionURL string `json:"sessionURL"`

	// memoryURL is the HTTP URL of the external memory-api instance.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https?://`
	MemoryURL string `json:"memoryURL"`
}

// ServiceGroupStatus reports the resolved URLs and readiness of a service group.
type ServiceGroupStatus struct {
	// name matches the service group name from spec.services[].name.
	Name string `json:"name"`

	// sessionURL is the resolved URL for the session-api instance.
	SessionURL string `json:"sessionURL"`

	// memoryURL is the resolved URL for the memory-api instance.
	MemoryURL string `json:"memoryURL"`

	// ready indicates whether both services are available and healthy.
	Ready bool `json:"ready"`
}
```

Also add the `corev1` import if not already present (it likely is, check before adding).

- [ ] **Step 4: Add ServiceGroup field to AgentRuntimeSpec**

In `api/v1alpha1/agentruntime_types.go`, find `type AgentRuntimeSpec struct` and add after the last field before the closing brace:

```go
	// serviceGroup is the name of the service group (session-api + memory-api pair)
	// from the Workspace's spec.services[]. Defaults to "default".
	// +kubebuilder:default="default"
	// +optional
	ServiceGroup string `json:"serviceGroup,omitempty"`
```

- [ ] **Step 5: Run tests to verify types compile and serialize**

Run: `go test ./api/v1alpha1/ -run TestWorkspaceServiceGroup -count=1 -v`
Expected: PASS

Run: `go test ./api/v1alpha1/ -run TestServiceGroupStatus -count=1 -v`
Expected: PASS

- [ ] **Step 6: Run deepcopy generation and CRD manifests**

Run: `make generate && make manifests`

This regenerates `zz_generated.deepcopy.go` for the new types and updates CRD YAML manifests with the new fields.

- [ ] **Step 7: Run goimports on changed files**

Run: `goimports -w api/v1alpha1/workspace_types.go api/v1alpha1/workspace_types_test.go api/v1alpha1/agentruntime_types.go`

- [ ] **Step 8: Commit**

```
feat: add service group types to Workspace and AgentRuntime CRDs (#715)

Workspace CRD gains spec.services[] for named session-api/memory-api
pairs (managed or external) and status.services[] for resolved URLs.
AgentRuntime CRD gains spec.serviceGroup (defaults to "default").
```

---

## Task 2: Service Discovery Client

**Files:**
- Create: `pkg/servicediscovery/resolver.go`
- Create: `pkg/servicediscovery/resolver_test.go`
- Create: `pkg/servicediscovery/config.go`
- Create: `pkg/servicediscovery/config_test.go`

### Steps

- [ ] **Step 1: Write test for URL resolver (env var path)**

```go
// pkg/servicediscovery/resolver_test.go
package servicediscovery

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveServiceURLs_EnvVarOverride(t *testing.T) {
	t.Setenv("SESSION_API_URL", "http://localhost:8080")
	t.Setenv("MEMORY_API_URL", "http://localhost:8081")

	r := NewResolver(nil) // nil client = no K8s
	urls, err := r.ResolveServiceURLs(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:8080", urls.SessionURL)
	assert.Equal(t, "http://localhost:8081", urls.MemoryURL)
}

func TestResolveServiceURLs_NoEnvNoClient(t *testing.T) {
	r := NewResolver(nil)
	_, err := r.ResolveServiceURLs(context.Background(), "default")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no service URLs available")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/servicediscovery/ -run TestResolveServiceURLs -count=1 -v`
Expected: FAIL — package doesn't exist

- [ ] **Step 3: Implement URL resolver**

```go
// pkg/servicediscovery/resolver.go
package servicediscovery

import (
	"context"
	"fmt"
	"os"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	envSessionAPIURL = "SESSION_API_URL"
	envMemoryAPIURL  = "MEMORY_API_URL"
)

// ServiceURLs holds the resolved session-api and memory-api URLs.
type ServiceURLs struct {
	SessionURL string
	MemoryURL  string
}

// Resolver resolves service URLs from env vars (local dev) or Workspace CRD status (in-cluster).
type Resolver struct {
	client client.Client
}

// NewResolver creates a service URL resolver. Pass nil client for local-dev-only mode.
func NewResolver(c client.Client) *Resolver {
	return &Resolver{client: c}
}

// ResolveServiceURLs returns service URLs for the given service group.
// Priority: env var override → Workspace CRD status lookup.
func (r *Resolver) ResolveServiceURLs(ctx context.Context, serviceGroup string) (*ServiceURLs, error) {
	// 1. Check env var overrides (local dev, Docker, testing)
	sessionURL := os.Getenv(envSessionAPIURL)
	memoryURL := os.Getenv(envMemoryAPIURL)
	if sessionURL != "" && memoryURL != "" {
		return &ServiceURLs{SessionURL: sessionURL, MemoryURL: memoryURL}, nil
	}

	// 2. Fall back to K8s client
	if r.client == nil {
		return nil, fmt.Errorf("no service URLs available: env vars %s/%s not set and no K8s client configured", envSessionAPIURL, envMemoryAPIURL)
	}

	return r.resolveFromWorkspace(ctx, serviceGroup)
}

// resolveFromWorkspace looks up the Workspace CRD by namespace and finds the service group URLs.
func (r *Resolver) resolveFromWorkspace(ctx context.Context, serviceGroup string) (*ServiceURLs, error) {
	ws, err := r.findWorkspaceByNamespace(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolving workspace: %w", err)
	}

	for _, sg := range ws.Status.Services {
		if sg.Name == serviceGroup {
			if !sg.Ready {
				return nil, fmt.Errorf("service group %q in workspace %q is not ready", serviceGroup, ws.Name)
			}
			return &ServiceURLs{SessionURL: sg.SessionURL, MemoryURL: sg.MemoryURL}, nil
		}
	}

	return nil, fmt.Errorf("service group %q not found in workspace %q status", serviceGroup, ws.Name)
}

// findWorkspaceByNamespace lists all Workspaces and finds the one whose spec.namespace.name
// matches the current pod's namespace (read from the downward API env var or /var/run/secrets).
func (r *Resolver) findWorkspaceByNamespace(ctx context.Context) (*omniav1alpha1.Workspace, error) {
	namespace := os.Getenv("OMNIA_NAMESPACE")
	if namespace == "" {
		// Fall back to the in-cluster namespace from the service account mount
		data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		if err != nil {
			return nil, fmt.Errorf("cannot determine namespace: OMNIA_NAMESPACE not set and serviceaccount mount not found")
		}
		namespace = string(data)
	}

	var list omniav1alpha1.WorkspaceList
	if err := r.client.List(ctx, &list); err != nil {
		return nil, fmt.Errorf("listing workspaces: %w", err)
	}
	for i := range list.Items {
		if list.Items[i].Spec.Namespace.Name == namespace {
			return &list.Items[i], nil
		}
	}
	return nil, fmt.Errorf("no workspace found for namespace %q", namespace)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/servicediscovery/ -run TestResolveServiceURLs -count=1 -v`
Expected: PASS

- [ ] **Step 5: Write test for K8s-backed resolution**

Add to `resolver_test.go`:

```go
func TestResolveServiceURLs_FromWorkspaceStatus(t *testing.T) {
	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "production"},
		Spec: omniav1alpha1.WorkspaceSpec{
			DisplayName: "Production",
			Namespace:   omniav1alpha1.NamespaceConfig{Name: "ws-production"},
		},
		Status: omniav1alpha1.WorkspaceStatus{
			Services: []omniav1alpha1.ServiceGroupStatus{
				{
					Name:       "default",
					SessionURL: "http://session-production-default.ws-production:8080",
					MemoryURL:  "http://memory-production-default.ws-production:8080",
					Ready:      true,
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()

	t.Setenv("OMNIA_NAMESPACE", "ws-production")
	// Clear any URL overrides
	t.Setenv("SESSION_API_URL", "")
	t.Setenv("MEMORY_API_URL", "")

	r := NewResolver(fakeClient)
	urls, err := r.ResolveServiceURLs(context.Background(), "default")
	require.NoError(t, err)
	assert.Equal(t, "http://session-production-default.ws-production:8080", urls.SessionURL)
	assert.Equal(t, "http://memory-production-default.ws-production:8080", urls.MemoryURL)
}

func TestResolveServiceURLs_ServiceGroupNotReady(t *testing.T) {
	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "staging"},
		Spec: omniav1alpha1.WorkspaceSpec{
			DisplayName: "Staging",
			Namespace:   omniav1alpha1.NamespaceConfig{Name: "ws-staging"},
		},
		Status: omniav1alpha1.WorkspaceStatus{
			Services: []omniav1alpha1.ServiceGroupStatus{
				{Name: "default", Ready: false},
			},
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()

	t.Setenv("OMNIA_NAMESPACE", "ws-staging")
	t.Setenv("SESSION_API_URL", "")
	t.Setenv("MEMORY_API_URL", "")

	r := NewResolver(fakeClient)
	_, err := r.ResolveServiceURLs(context.Background(), "default")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not ready")
}

func TestResolveServiceURLs_ServiceGroupNotFound(t *testing.T) {
	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "dev"},
		Spec: omniav1alpha1.WorkspaceSpec{
			DisplayName: "Dev",
			Namespace:   omniav1alpha1.NamespaceConfig{Name: "ws-dev"},
		},
		Status: omniav1alpha1.WorkspaceStatus{
			Services: []omniav1alpha1.ServiceGroupStatus{
				{Name: "default", SessionURL: "http://s:8080", MemoryURL: "http://m:8080", Ready: true},
			},
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()

	t.Setenv("OMNIA_NAMESPACE", "ws-dev")
	t.Setenv("SESSION_API_URL", "")
	t.Setenv("MEMORY_API_URL", "")

	r := NewResolver(fakeClient)
	_, err := r.ResolveServiceURLs(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
```

Add imports for `metav1`, `runtime`, `fake`, and `omniav1alpha1` at the top.

- [ ] **Step 6: Run K8s-backed resolver tests**

Run: `go test ./pkg/servicediscovery/ -run TestResolveServiceURLs -count=1 -v`
Expected: PASS

- [ ] **Step 7: Write test for service config resolution**

```go
// pkg/servicediscovery/config_test.go
package servicediscovery

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestResolveSessionConfig(t *testing.T) {
	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "production"},
		Spec: omniav1alpha1.WorkspaceSpec{
			DisplayName: "Production",
			Namespace:   omniav1alpha1.NamespaceConfig{Name: "ws-production"},
			Services: []omniav1alpha1.WorkspaceServiceGroup{
				{
					Name: "default",
					Mode: omniav1alpha1.ServiceModeManaged,
					Session: &omniav1alpha1.SessionServiceConfig{
						Database:  omniav1alpha1.DatabaseConfig{SecretRef: corev1.LocalObjectReference{Name: "sess-db"}},
						Retention: &omniav1alpha1.SessionRetentionConfig{WarmDays: ptr(int32(30))},
					},
					Memory: &omniav1alpha1.MemoryServiceConfig{
						Database:    omniav1alpha1.DatabaseConfig{SecretRef: corev1.LocalObjectReference{Name: "mem-db"}},
						ProviderRef: &corev1.LocalObjectReference{Name: "ollama-embed"},
						Retention:   &omniav1alpha1.MemoryRetentionConfig{DefaultTTL: "720h"},
					},
				},
			},
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "sess-db", Namespace: "ws-production"},
		Data:       map[string][]byte{"POSTGRES_CONN": []byte("postgres://sess:pass@db:5432/sessions")},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws, secret).Build()

	cr := NewConfigResolver(fakeClient)
	cfg, err := cr.ResolveSessionConfig(context.Background(), "production", "default", "ws-production")
	require.NoError(t, err)
	assert.Equal(t, "postgres://sess:pass@db:5432/sessions", cfg.PostgresConn)
	assert.Equal(t, int32(30), *cfg.WarmDays)
}

func TestResolveMemoryConfig(t *testing.T) {
	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "production"},
		Spec: omniav1alpha1.WorkspaceSpec{
			DisplayName: "Production",
			Namespace:   omniav1alpha1.NamespaceConfig{Name: "ws-production"},
			Services: []omniav1alpha1.WorkspaceServiceGroup{
				{
					Name: "default",
					Mode: omniav1alpha1.ServiceModeManaged,
					Session: &omniav1alpha1.SessionServiceConfig{
						Database: omniav1alpha1.DatabaseConfig{SecretRef: corev1.LocalObjectReference{Name: "sess-db"}},
					},
					Memory: &omniav1alpha1.MemoryServiceConfig{
						Database:    omniav1alpha1.DatabaseConfig{SecretRef: corev1.LocalObjectReference{Name: "mem-db"}},
						ProviderRef: &corev1.LocalObjectReference{Name: "ollama-embed"},
						Retention:   &omniav1alpha1.MemoryRetentionConfig{DefaultTTL: "720h"},
					},
				},
			},
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "mem-db", Namespace: "ws-production"},
		Data:       map[string][]byte{"POSTGRES_CONN": []byte("postgres://mem:pass@db:5432/memories")},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws, secret).Build()

	cr := NewConfigResolver(fakeClient)
	cfg, err := cr.ResolveMemoryConfig(context.Background(), "production", "default", "ws-production")
	require.NoError(t, err)
	assert.Equal(t, "postgres://mem:pass@db:5432/memories", cfg.PostgresConn)
	assert.Equal(t, "ollama-embed", cfg.EmbeddingProviderName)
	assert.Equal(t, "720h", cfg.DefaultTTL)
}

func TestResolveMemoryConfig_NoProvider(t *testing.T) {
	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "dev"},
		Spec: omniav1alpha1.WorkspaceSpec{
			DisplayName: "Dev",
			Namespace:   omniav1alpha1.NamespaceConfig{Name: "ws-dev"},
			Services: []omniav1alpha1.WorkspaceServiceGroup{
				{
					Name: "default",
					Mode: omniav1alpha1.ServiceModeManaged,
					Session: &omniav1alpha1.SessionServiceConfig{
						Database: omniav1alpha1.DatabaseConfig{SecretRef: corev1.LocalObjectReference{Name: "sess-db"}},
					},
					Memory: &omniav1alpha1.MemoryServiceConfig{
						Database: omniav1alpha1.DatabaseConfig{SecretRef: corev1.LocalObjectReference{Name: "mem-db"}},
						// No ProviderRef — embeddings disabled
					},
				},
			},
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "mem-db", Namespace: "ws-dev"},
		Data:       map[string][]byte{"POSTGRES_CONN": []byte("postgres://mem:pass@db:5432/dev")},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws, secret).Build()

	cr := NewConfigResolver(fakeClient)
	cfg, err := cr.ResolveMemoryConfig(context.Background(), "dev", "default", "ws-dev")
	require.NoError(t, err)
	assert.Equal(t, "", cfg.EmbeddingProviderName) // empty, no provider
}

func ptr[T any](v T) *T { return &v }
```

- [ ] **Step 8: Implement config resolver**

```go
// pkg/servicediscovery/config.go
package servicediscovery

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// SessionConfig holds resolved configuration for a session-api instance.
type SessionConfig struct {
	PostgresConn string
	WarmDays     *int32
}

// MemoryConfig holds resolved configuration for a memory-api instance.
type MemoryConfig struct {
	PostgresConn          string
	EmbeddingProviderName string
	DefaultTTL            string
}

// ConfigResolver resolves service configuration from the Workspace CRD and associated Secrets.
type ConfigResolver struct {
	client client.Client
}

// NewConfigResolver creates a config resolver backed by a K8s client.
func NewConfigResolver(c client.Client) *ConfigResolver {
	return &ConfigResolver{client: c}
}

// ResolveSessionConfig reads the Workspace CRD and referenced Secret to build session-api config.
func (cr *ConfigResolver) ResolveSessionConfig(ctx context.Context, workspace, serviceGroup, namespace string) (*SessionConfig, error) {
	sg, err := cr.findServiceGroup(ctx, workspace, serviceGroup)
	if err != nil {
		return nil, err
	}
	if sg.Session == nil {
		return nil, fmt.Errorf("service group %q has no session config", serviceGroup)
	}

	connStr, err := cr.readPostgresConn(ctx, sg.Session.Database.SecretRef.Name, namespace)
	if err != nil {
		return nil, fmt.Errorf("reading session database secret: %w", err)
	}

	cfg := &SessionConfig{PostgresConn: connStr}
	if sg.Session.Retention != nil {
		cfg.WarmDays = sg.Session.Retention.WarmDays
	}
	return cfg, nil
}

// ResolveMemoryConfig reads the Workspace CRD and referenced Secret to build memory-api config.
func (cr *ConfigResolver) ResolveMemoryConfig(ctx context.Context, workspace, serviceGroup, namespace string) (*MemoryConfig, error) {
	sg, err := cr.findServiceGroup(ctx, workspace, serviceGroup)
	if err != nil {
		return nil, err
	}
	if sg.Memory == nil {
		return nil, fmt.Errorf("service group %q has no memory config", serviceGroup)
	}

	connStr, err := cr.readPostgresConn(ctx, sg.Memory.Database.SecretRef.Name, namespace)
	if err != nil {
		return nil, fmt.Errorf("reading memory database secret: %w", err)
	}

	cfg := &MemoryConfig{PostgresConn: connStr}
	if sg.Memory.ProviderRef != nil {
		cfg.EmbeddingProviderName = sg.Memory.ProviderRef.Name
	}
	if sg.Memory.Retention != nil {
		cfg.DefaultTTL = sg.Memory.Retention.DefaultTTL
	}
	return cfg, nil
}

// findServiceGroup looks up the Workspace by name and finds the matching service group in spec.
func (cr *ConfigResolver) findServiceGroup(ctx context.Context, workspace, serviceGroup string) (*omniav1alpha1.WorkspaceServiceGroup, error) {
	ws := &omniav1alpha1.Workspace{}
	if err := cr.client.Get(ctx, types.NamespacedName{Name: workspace}, ws); err != nil {
		return nil, fmt.Errorf("getting workspace %q: %w", workspace, err)
	}
	for i := range ws.Spec.Services {
		if ws.Spec.Services[i].Name == serviceGroup {
			return &ws.Spec.Services[i], nil
		}
	}
	return nil, fmt.Errorf("service group %q not found in workspace %q", serviceGroup, workspace)
}

// readPostgresConn reads the POSTGRES_CONN key from a named Secret.
func (cr *ConfigResolver) readPostgresConn(ctx context.Context, secretName, namespace string) (string, error) {
	secret := &corev1.Secret{}
	if err := cr.client.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err != nil {
		return "", fmt.Errorf("getting secret %q in namespace %q: %w", secretName, namespace, err)
	}
	conn, ok := secret.Data["POSTGRES_CONN"]
	if !ok {
		return "", fmt.Errorf("secret %q missing required key POSTGRES_CONN", secretName)
	}
	return string(conn), nil
}
```

- [ ] **Step 9: Run all service discovery tests**

Run: `go test ./pkg/servicediscovery/... -count=1 -v`
Expected: PASS

- [ ] **Step 10: Run goimports**

Run: `goimports -w pkg/servicediscovery/`

- [ ] **Step 11: Commit**

```
feat: add service discovery client for per-workspace URL resolution (#715)

Resolver checks env vars (local dev) before falling back to Workspace
CRD status lookup. ConfigResolver reads Workspace spec + Secrets for
session-api/memory-api self-configuration.
```

---

## Task 3: Service Deployment Builder

**Files:**
- Create: `internal/controller/service_builder.go`
- Create: `internal/controller/service_builder_test.go`

### Steps

- [ ] **Step 1: Write test for session-api Deployment builder**

```go
// internal/controller/service_builder_test.go
package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestBuildSessionDeployment(t *testing.T) {
	sb := &ServiceBuilder{
		SessionImage: "omnia-session-api:latest",
		MemoryImage:  "omnia-memory-api:latest",
	}

	sg := omniav1alpha1.WorkspaceServiceGroup{
		Name: "default",
		Mode: omniav1alpha1.ServiceModeManaged,
		Session: &omniav1alpha1.SessionServiceConfig{
			Database: omniav1alpha1.DatabaseConfig{
				SecretRef: corev1.LocalObjectReference{Name: "sess-db"},
			},
		},
	}

	dep := sb.BuildSessionDeployment("production", "ws-production", sg)
	require.NotNil(t, dep)
	assert.Equal(t, "session-production-default", dep.Name)
	assert.Equal(t, "ws-production", dep.Namespace)

	// Verify container args
	container := dep.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "omnia-session-api:latest", container.Image)
	assert.Contains(t, container.Args, "--workspace=production")
	assert.Contains(t, container.Args, "--service-group=default")

	// Verify labels
	assert.Equal(t, "session-api", dep.Spec.Template.Labels["app.kubernetes.io/component"])
	assert.Equal(t, "production", dep.Spec.Template.Labels["omnia.altairalabs.ai/workspace"])
	assert.Equal(t, "default", dep.Spec.Template.Labels["omnia.altairalabs.ai/service-group"])
}

func TestBuildMemoryDeployment(t *testing.T) {
	sb := &ServiceBuilder{
		SessionImage: "omnia-session-api:latest",
		MemoryImage:  "omnia-memory-api:latest",
	}

	sg := omniav1alpha1.WorkspaceServiceGroup{
		Name: "default",
		Mode: omniav1alpha1.ServiceModeManaged,
		Memory: &omniav1alpha1.MemoryServiceConfig{
			Database: omniav1alpha1.DatabaseConfig{
				SecretRef: corev1.LocalObjectReference{Name: "mem-db"},
			},
			ProviderRef: &corev1.LocalObjectReference{Name: "ollama-embed"},
		},
	}

	dep := sb.BuildMemoryDeployment("production", "ws-production", sg)
	require.NotNil(t, dep)
	assert.Equal(t, "memory-production-default", dep.Name)
	assert.Equal(t, "ws-production", dep.Namespace)

	container := dep.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "omnia-memory-api:latest", container.Image)
	assert.Contains(t, container.Args, "--workspace=production")
	assert.Contains(t, container.Args, "--service-group=default")
	assert.Equal(t, "memory-api", dep.Spec.Template.Labels["app.kubernetes.io/component"])
}

func TestBuildService(t *testing.T) {
	sb := &ServiceBuilder{}

	svc := sb.BuildService("session-production-default", "ws-production", "session-api", "production", "default")
	require.NotNil(t, svc)
	assert.Equal(t, "session-production-default", svc.Name)
	assert.Equal(t, "ws-production", svc.Namespace)
	assert.Equal(t, int32(8080), svc.Spec.Ports[0].Port)
	assert.Equal(t, "session-api", svc.Spec.Selector["app.kubernetes.io/component"])
	assert.Equal(t, "production", svc.Spec.Selector["omnia.altairalabs.ai/workspace"])
	assert.Equal(t, "default", svc.Spec.Selector["omnia.altairalabs.ai/service-group"])
}

func TestServiceURL(t *testing.T) {
	assert.Equal(t,
		"http://session-production-default.ws-production:8080",
		ServiceURL("session-production-default", "ws-production"),
	)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/controller/ -run TestBuildSession -count=1 -v`
Expected: FAIL — types don't exist

- [ ] **Step 3: Implement service builder**

```go
// internal/controller/service_builder.go
package controller

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const (
	servicePort = 8080
	healthPort  = 8081
)

// ServiceBuilder constructs Kubernetes Deployments and Services for workspace service instances.
type ServiceBuilder struct {
	SessionImage           string
	SessionImagePullPolicy corev1.PullPolicy
	MemoryImage            string
	MemoryImagePullPolicy  corev1.PullPolicy
}

// BuildSessionDeployment creates a Deployment for a session-api instance.
func (sb *ServiceBuilder) BuildSessionDeployment(workspaceName, namespace string, sg omniav1alpha1.WorkspaceServiceGroup) *appsv1.Deployment {
	name := fmt.Sprintf("session-%s-%s", workspaceName, sg.Name)
	labels := serviceLabels("session-api", workspaceName, sg.Name)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "session-api",
							Image:           sb.SessionImage,
							ImagePullPolicy: sb.SessionImagePullPolicy,
							Args: []string{
								fmt.Sprintf("--workspace=%s", workspaceName),
								fmt.Sprintf("--service-group=%s", sg.Name),
							},
							Ports: []corev1.ContainerPort{
								{Name: "http", ContainerPort: servicePort, Protocol: corev1.ProtocolTCP},
								{Name: "health", ContainerPort: healthPort, Protocol: corev1.ProtocolTCP},
							},
							LivenessProbe:  httpProbe("/healthz", healthPort),
							ReadinessProbe: httpProbe("/readyz", healthPort),
							SecurityContext: restrictedSecurityContext(),
						},
					},
				},
			},
		},
	}
}

// BuildMemoryDeployment creates a Deployment for a memory-api instance.
func (sb *ServiceBuilder) BuildMemoryDeployment(workspaceName, namespace string, sg omniav1alpha1.WorkspaceServiceGroup) *appsv1.Deployment {
	name := fmt.Sprintf("memory-%s-%s", workspaceName, sg.Name)
	labels := serviceLabels("memory-api", workspaceName, sg.Name)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "memory-api",
							Image:           sb.MemoryImage,
							ImagePullPolicy: sb.MemoryImagePullPolicy,
							Args: []string{
								fmt.Sprintf("--workspace=%s", workspaceName),
								fmt.Sprintf("--service-group=%s", sg.Name),
							},
							Ports: []corev1.ContainerPort{
								{Name: "http", ContainerPort: servicePort, Protocol: corev1.ProtocolTCP},
								{Name: "health", ContainerPort: healthPort, Protocol: corev1.ProtocolTCP},
							},
							LivenessProbe:  httpProbe("/healthz", healthPort),
							ReadinessProbe: httpProbe("/readyz", healthPort),
							SecurityContext: restrictedSecurityContext(),
						},
					},
				},
			},
		},
	}
}

// BuildService creates a ClusterIP Service for a workspace service instance.
func (sb *ServiceBuilder) BuildService(name, namespace, component, workspaceName, groupName string) *corev1.Service {
	labels := serviceLabels(component, workspaceName, groupName)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       servicePort,
					TargetPort: intstr.FromString("http"),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}

// ServiceURL returns the in-cluster DNS URL for a workspace service.
func ServiceURL(serviceName, namespace string) string {
	return fmt.Sprintf("http://%s.%s:%d", serviceName, namespace, servicePort)
}

func serviceLabels(component, workspaceName, groupName string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/component":        component,
		"app.kubernetes.io/managed-by":       "omnia-operator",
		"omnia.altairalabs.ai/workspace":     workspaceName,
		"omnia.altairalabs.ai/service-group": groupName,
	}
}

func httpProbe(path string, port int) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path:   path,
				Port:   intstr.FromInt32(int32(port)),
				Scheme: corev1.URISchemeHTTP,
			},
		},
		InitialDelaySeconds: 5,
		PeriodSeconds:       10,
	}
}

func restrictedSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		RunAsNonRoot:             ptr.To(true),
		AllowPrivilegeEscalation: ptr.To(false),
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/controller/ -run "TestBuildSession|TestBuildMemory|TestBuildService|TestServiceURL" -count=1 -v`
Expected: PASS

- [ ] **Step 5: Run goimports**

Run: `goimports -w internal/controller/service_builder.go internal/controller/service_builder_test.go`

- [ ] **Step 6: Commit**

```
feat: add service deployment builder for workspace service instances (#715)

Constructs Deployments and Services for per-workspace session-api and
memory-api instances with --workspace and --service-group startup args.
```

---

## Task 4: Workspace Controller — Service Reconciliation

**Files:**
- Create: `internal/controller/workspace_services.go`
- Create: `internal/controller/workspace_services_test.go`
- Modify: `internal/controller/workspace_controller.go:87-250`

### Steps

- [ ] **Step 1: Write integration test for service reconciliation**

```go
// internal/controller/workspace_services_test.go
package controller

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestReconcileServices_ManagedCreatesDeploymentsAndServices(t *testing.T) {
	g := NewWithT(t)

	workspace := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "production"},
		Spec: omniav1alpha1.WorkspaceSpec{
			DisplayName: "Production",
			Namespace:   omniav1alpha1.NamespaceConfig{Name: "ws-production"},
			Services: []omniav1alpha1.WorkspaceServiceGroup{
				{
					Name: "default",
					Mode: omniav1alpha1.ServiceModeManaged,
					Session: &omniav1alpha1.SessionServiceConfig{
						Database: omniav1alpha1.DatabaseConfig{SecretRef: corev1.LocalObjectReference{Name: "sess-db"}},
					},
					Memory: &omniav1alpha1.MemoryServiceConfig{
						Database: omniav1alpha1.DatabaseConfig{SecretRef: corev1.LocalObjectReference{Name: "mem-db"}},
					},
				},
			},
		},
		Status: omniav1alpha1.WorkspaceStatus{
			Namespace: &omniav1alpha1.NamespaceStatus{Name: "ws-production", Created: true},
		},
	}

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ws-production"}}

	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(workspace, ns).
		WithStatusSubresource(workspace).
		Build()

	r := &WorkspaceReconciler{
		Client: fakeClient,
		Scheme: scheme,
		ServiceBuilder: &ServiceBuilder{
			SessionImage: "session-api:test",
			MemoryImage:  "memory-api:test",
		},
	}

	err := r.reconcileServices(context.Background(), workspace)
	g.Expect(err).NotTo(HaveOccurred())

	// Verify session-api Deployment created
	sessionDep := &appsv1.Deployment{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "session-production-default", Namespace: "ws-production",
	}, sessionDep)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(sessionDep.Spec.Template.Spec.Containers[0].Image).To(Equal("session-api:test"))

	// Verify memory-api Deployment created
	memoryDep := &appsv1.Deployment{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "memory-production-default", Namespace: "ws-production",
	}, memoryDep)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(memoryDep.Spec.Template.Spec.Containers[0].Image).To(Equal("memory-api:test"))

	// Verify Services created
	sessionSvc := &corev1.Service{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "session-production-default", Namespace: "ws-production",
	}, sessionSvc)
	g.Expect(err).NotTo(HaveOccurred())

	memorySvc := &corev1.Service{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name: "memory-production-default", Namespace: "ws-production",
	}, memorySvc)
	g.Expect(err).NotTo(HaveOccurred())

	// Verify status updated
	g.Expect(workspace.Status.Services).To(HaveLen(1))
	g.Expect(workspace.Status.Services[0].Name).To(Equal("default"))
	g.Expect(workspace.Status.Services[0].SessionURL).To(Equal("http://session-production-default.ws-production:8080"))
	g.Expect(workspace.Status.Services[0].MemoryURL).To(Equal("http://memory-production-default.ws-production:8080"))
}

func TestReconcileServices_ExternalWritesURLsToStatus(t *testing.T) {
	g := NewWithT(t)

	workspace := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "legacy"},
		Spec: omniav1alpha1.WorkspaceSpec{
			DisplayName: "Legacy",
			Namespace:   omniav1alpha1.NamespaceConfig{Name: "ws-legacy"},
			Services: []omniav1alpha1.WorkspaceServiceGroup{
				{
					Name: "legacy",
					Mode: omniav1alpha1.ServiceModeExternal,
					External: &omniav1alpha1.ExternalEndpoints{
						SessionURL: "http://custom-session:8080",
						MemoryURL:  "http://custom-memory:8080",
					},
				},
			},
		},
		Status: omniav1alpha1.WorkspaceStatus{
			Namespace: &omniav1alpha1.NamespaceStatus{Name: "ws-legacy", Created: true},
		},
	}

	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(workspace).
		WithStatusSubresource(workspace).
		Build()

	r := &WorkspaceReconciler{
		Client: fakeClient,
		Scheme: scheme,
		ServiceBuilder: &ServiceBuilder{},
	}

	err := r.reconcileServices(context.Background(), workspace)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(workspace.Status.Services).To(HaveLen(1))
	g.Expect(workspace.Status.Services[0].SessionURL).To(Equal("http://custom-session:8080"))
	g.Expect(workspace.Status.Services[0].MemoryURL).To(Equal("http://custom-memory:8080"))
	g.Expect(workspace.Status.Services[0].Ready).To(BeTrue())

	// Verify no Deployments created for external mode
	depList := &appsv1.DeploymentList{}
	err = fakeClient.List(context.Background(), depList, client.InNamespace("ws-legacy"))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(depList.Items).To(BeEmpty())
}

func TestReconcileServices_NoServicesBlock(t *testing.T) {
	g := NewWithT(t)

	workspace := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "empty"},
		Spec: omniav1alpha1.WorkspaceSpec{
			DisplayName: "Empty",
			Namespace:   omniav1alpha1.NamespaceConfig{Name: "ws-empty"},
		},
		Status: omniav1alpha1.WorkspaceStatus{},
	}

	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(workspace).
		WithStatusSubresource(workspace).
		Build()

	r := &WorkspaceReconciler{
		Client:         fakeClient,
		Scheme:         scheme,
		ServiceBuilder: &ServiceBuilder{},
	}

	err := r.reconcileServices(context.Background(), workspace)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(workspace.Status.Services).To(BeEmpty())
}
```

Add a helper at the bottom:

```go
func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)
	return s
}
```

(Add the `"k8s.io/apimachinery/pkg/runtime"` import.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/controller/ -run TestReconcileServices -count=1 -v`
Expected: FAIL — `reconcileServices` doesn't exist

- [ ] **Step 3: Implement reconcileServices**

```go
// internal/controller/workspace_services.go
package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// reconcileServices creates, updates, or deletes Deployments and Services
// for each service group defined in the workspace spec.
func (r *WorkspaceReconciler) reconcileServices(ctx context.Context, workspace *omniav1alpha1.Workspace) error {
	log := logf.FromContext(ctx)
	namespace := workspace.Spec.Namespace.Name

	var statuses []omniav1alpha1.ServiceGroupStatus

	for _, sg := range workspace.Spec.Services {
		if sg.Mode == omniav1alpha1.ServiceModeExternal {
			statuses = append(statuses, omniav1alpha1.ServiceGroupStatus{
				Name:       sg.Name,
				SessionURL: sg.External.SessionURL,
				MemoryURL:  sg.External.MemoryURL,
				Ready:      true,
			})
			log.V(1).Info("external service group configured",
				"serviceGroup", sg.Name,
				"sessionURL", sg.External.SessionURL,
				"memoryURL", sg.External.MemoryURL)
			continue
		}

		// Managed mode: create Deployments and Services
		sessionDepName := fmt.Sprintf("session-%s-%s", workspace.Name, sg.Name)
		memoryDepName := fmt.Sprintf("memory-%s-%s", workspace.Name, sg.Name)

		// Session-api Deployment
		if err := r.reconcileManagedDeployment(ctx, workspace, namespace,
			r.ServiceBuilder.BuildSessionDeployment(workspace.Name, namespace, sg)); err != nil {
			return fmt.Errorf("reconciling session deployment for group %q: %w", sg.Name, err)
		}

		// Memory-api Deployment
		if err := r.reconcileManagedDeployment(ctx, workspace, namespace,
			r.ServiceBuilder.BuildMemoryDeployment(workspace.Name, namespace, sg)); err != nil {
			return fmt.Errorf("reconciling memory deployment for group %q: %w", sg.Name, err)
		}

		// Session-api Service
		if err := r.reconcileManagedService(ctx, workspace, namespace,
			r.ServiceBuilder.BuildService(sessionDepName, namespace, "session-api", workspace.Name, sg.Name)); err != nil {
			return fmt.Errorf("reconciling session service for group %q: %w", sg.Name, err)
		}

		// Memory-api Service
		if err := r.reconcileManagedService(ctx, workspace, namespace,
			r.ServiceBuilder.BuildService(memoryDepName, namespace, "memory-api", workspace.Name, sg.Name)); err != nil {
			return fmt.Errorf("reconciling memory service for group %q: %w", sg.Name, err)
		}

		// Check readiness
		ready := r.isDeploymentReady(ctx, sessionDepName, namespace) &&
			r.isDeploymentReady(ctx, memoryDepName, namespace)

		statuses = append(statuses, omniav1alpha1.ServiceGroupStatus{
			Name:       sg.Name,
			SessionURL: ServiceURL(sessionDepName, namespace),
			MemoryURL:  ServiceURL(memoryDepName, namespace),
			Ready:      ready,
		})

		log.V(1).Info("managed service group reconciled",
			"serviceGroup", sg.Name,
			"ready", ready)
	}

	workspace.Status.Services = statuses

	// Clean up Deployments/Services for removed service groups
	return r.cleanupRemovedServiceGroups(ctx, workspace, namespace)
}

// reconcileManagedDeployment creates or updates a Deployment with owner reference to the workspace.
func (r *WorkspaceReconciler) reconcileManagedDeployment(ctx context.Context, workspace *omniav1alpha1.Workspace, namespace string, desired *appsv1.Deployment) error {
	existing := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: namespace}, existing)
	if apierrors.IsNotFound(err) {
		if err := controllerutil.SetControllerReference(workspace, desired, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	// Update existing
	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	return r.Update(ctx, existing)
}

// reconcileManagedService creates or updates a Service with owner reference to the workspace.
func (r *WorkspaceReconciler) reconcileManagedService(ctx context.Context, workspace *omniav1alpha1.Workspace, namespace string, desired *corev1.Service) error {
	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: namespace}, existing)
	if apierrors.IsNotFound(err) {
		if err := controllerutil.SetControllerReference(workspace, desired, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	existing.Spec.Selector = desired.Spec.Selector
	existing.Spec.Ports = desired.Spec.Ports
	existing.Labels = desired.Labels
	return r.Update(ctx, existing)
}

// isDeploymentReady checks if a Deployment has at least one ready replica.
func (r *WorkspaceReconciler) isDeploymentReady(ctx context.Context, name, namespace string) bool {
	dep := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, dep); err != nil {
		return false
	}
	return dep.Status.ReadyReplicas > 0
}

// cleanupRemovedServiceGroups deletes Deployments and Services for service groups
// that are no longer in the workspace spec. Owner references handle cascading for
// Deployments, but we also clean up Services explicitly.
func (r *WorkspaceReconciler) cleanupRemovedServiceGroups(ctx context.Context, workspace *omniav1alpha1.Workspace, namespace string) error {
	// Build set of expected service group names
	expected := make(map[string]bool, len(workspace.Spec.Services))
	for _, sg := range workspace.Spec.Services {
		if sg.Mode != omniav1alpha1.ServiceModeExternal {
			expected[sg.Name] = true
		}
	}

	// List all Deployments owned by this workspace
	depList := &appsv1.DeploymentList{}
	if err := r.List(ctx, depList,
		client.InNamespace(namespace),
		client.MatchingLabels{"app.kubernetes.io/managed-by": "omnia-operator"},
	); err != nil {
		return err
	}

	for i := range depList.Items {
		dep := &depList.Items[i]
		groupName := dep.Labels["omnia.altairalabs.ai/service-group"]
		wsName := dep.Labels["omnia.altairalabs.ai/workspace"]
		if wsName == workspace.Name && !expected[groupName] {
			if err := r.Delete(ctx, dep); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}

	// Clean up Services too
	svcList := &corev1.ServiceList{}
	if err := r.List(ctx, svcList,
		client.InNamespace(namespace),
		client.MatchingLabels{"app.kubernetes.io/managed-by": "omnia-operator"},
	); err != nil {
		return err
	}

	for i := range svcList.Items {
		svc := &svcList.Items[i]
		groupName := svc.Labels["omnia.altairalabs.ai/service-group"]
		wsName := svc.Labels["omnia.altairalabs.ai/workspace"]
		if wsName == workspace.Name && !expected[groupName] {
			if err := r.Delete(ctx, svc); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}
```

- [ ] **Step 4: Add ServiceBuilder field to WorkspaceReconciler**

In `internal/controller/workspace_controller.go`, add to the `WorkspaceReconciler` struct:

```go
	// ServiceBuilder constructs Deployments/Services for workspace service instances
	ServiceBuilder *ServiceBuilder
```

- [ ] **Step 5: Add reconcileServices call to Reconcile method**

In `internal/controller/workspace_controller.go`, after the storage reconciliation block (after line 223) and before `r.updateMemberCount(workspace)` (line 226), add:

```go
	// Reconcile service instances (session-api, memory-api)
	if err := r.reconcileServices(ctx, workspace); err != nil {
		SetCondition(&workspace.Status.Conditions, workspace.Generation, ConditionTypeServicesReady, metav1.ConditionFalse,
			"ServicesFailed", err.Error())
		workspace.Status.Phase = omniav1alpha1.WorkspacePhaseError
		if statusErr := r.Status().Update(ctx, workspace); statusErr != nil {
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		return ctrl.Result{}, err
	}
	servicesReady := true
	for _, sg := range workspace.Status.Services {
		if !sg.Ready {
			servicesReady = false
			break
		}
	}
	if len(workspace.Spec.Services) > 0 {
		if servicesReady {
			SetCondition(&workspace.Status.Conditions, workspace.Generation, ConditionTypeServicesReady, metav1.ConditionTrue,
				"ServicesReady", "All service instances are ready")
		} else {
			SetCondition(&workspace.Status.Conditions, workspace.Generation, ConditionTypeServicesReady, metav1.ConditionFalse,
				"ServicesNotReady", "One or more service instances are not ready")
		}
	}
```

Add the condition type constant (likely in the same file or in a shared constants file):

```go
const ConditionTypeServicesReady = "ServicesReady"
```

- [ ] **Step 6: Add RBAC markers and K8s Service watch**

In `workspace_controller.go`, add the `client` import for the `client.InNamespace` / `client.MatchingLabels` usage in `workspace_services.go`. Also verify the RBAC markers at lines 87-102 already include `apps/deployments` and `core/services` — they do (line 100), so no change needed.

- [ ] **Step 7: Run tests**

Run: `go test ./internal/controller/ -run TestReconcileServices -count=1 -v`
Expected: PASS

- [ ] **Step 8: Run goimports and lint**

Run: `goimports -w internal/controller/workspace_services.go internal/controller/workspace_services_test.go internal/controller/workspace_controller.go`

Run: `golangci-lint run ./internal/controller/...`

- [ ] **Step 9: Commit**

```
feat: workspace controller reconciles per-workspace service instances (#715)

Managed service groups get Deployments and Services with owner references.
External groups copy URLs to status. Cleanup removes resources for
deleted service groups.
```

---

## Task 5: Remove Singleton Session/Memory ENV Var Injection from Agent Pods

**Files:**
- Modify: `internal/controller/deployment_builder.go:475-478,731-734,820-823`
- Modify: `internal/controller/agentruntime_controller.go:59-60`
- Modify: `internal/controller/eval_worker.go:187-190`
- Modify: `cmd/main.go:218`
- Modify: `internal/controller/deployment_builder_test.go:101`
- Modify: `internal/controller/eval_worker_test.go:49,159`

### Steps

- [ ] **Step 1: Update deployment_builder_test to expect no SESSION_API_URL env var**

In `internal/controller/deployment_builder_test.go`, find any test assertions that check for `SESSION_API_URL` being injected into agent pods. Update them to assert it's NOT present.

Find line 101 (`r := &AgentRuntimeReconciler{SessionAPIURL: "http://session-api:8080"}`) and remove the `SessionAPIURL` field.

- [ ] **Step 2: Remove SessionAPIURL field from AgentRuntimeReconciler**

In `internal/controller/agentruntime_controller.go`, remove lines 59-60:
```go
	// SessionAPIURL is the internal URL of the session-api service for session recording
	SessionAPIURL string
```

- [ ] **Step 3: Remove SESSION_API_URL env var injection from deployment_builder.go**

Remove the three blocks that inject `SESSION_API_URL`:

At line ~475-478:
```go
	if r.SessionAPIURL != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "SESSION_API_URL",
			Value: r.SessionAPIURL,
		})
	}
```

At line ~731-734: same pattern, remove.

At line ~820-823: same pattern, remove.

- [ ] **Step 4: Update eval_worker.go to resolve session-api URL from workspace**

In `internal/controller/eval_worker.go`, the eval worker still needs a session-api URL. Change lines 187-190 to resolve from the Workspace CRD status instead of `r.SessionAPIURL`. The eval worker runs in a workspace namespace, so it can look up the workspace and service group.

Read the AgentRuntime's `ServiceGroup` field (defaults to "default") and find the matching Workspace status entry:

```go
	// Resolve session-api URL from workspace service group
	sessionURL := r.resolveSessionURLForWorkspace(ctx, agentRuntime.Namespace, agentRuntime.Spec.ServiceGroup)
	if sessionURL != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  envSessionAPIURL,
			Value: sessionURL,
		})
	}
```

Add a helper method on the reconciler:

```go
func (r *AgentRuntimeReconciler) resolveSessionURLForWorkspace(ctx context.Context, namespace, serviceGroup string) string {
	var list omniav1alpha1.WorkspaceList
	if err := r.List(ctx, &list); err != nil {
		return ""
	}
	for _, ws := range list.Items {
		if ws.Spec.Namespace.Name == namespace {
			for _, sg := range ws.Status.Services {
				if sg.Name == serviceGroup && sg.Ready {
					return sg.SessionURL
				}
			}
		}
	}
	return ""
}
```

- [ ] **Step 5: Remove sessionAPIURL from operator main.go**

In `cmd/main.go`, find line 218 (`SessionAPIURL: sessionAPIURL,`) and remove it. Also remove the `sessionAPIURL` variable declaration and flag registration earlier in the file.

- [ ] **Step 6: Add ServiceBuilder to WorkspaceReconciler init in main.go**

In `cmd/main.go`, find where the WorkspaceReconciler is created and add the `ServiceBuilder` field:

```go
	ServiceBuilder: &controller.ServiceBuilder{
		SessionImage:           sessionAPIImage,
		SessionImagePullPolicy: corev1.PullPolicy(sessionAPIImagePullPolicy),
		MemoryImage:            memoryAPIImage,
		MemoryImagePullPolicy:  corev1.PullPolicy(memoryAPIImagePullPolicy),
	},
```

Add the corresponding flag variables and flag registrations for `session-api-image`, `memory-api-image`, and their pull policies.

- [ ] **Step 7: Update eval_worker_test.go**

In `internal/controller/eval_worker_test.go`:
- Line 49: remove `SessionAPIURL: "http://session-api:8080"`
- Line 159: update or remove the assertion for `envSessionAPIURL`
- Add a test that verifies the workspace-based URL resolution for eval workers

- [ ] **Step 8: Run tests**

Run: `go test ./internal/controller/... -count=1 -v`
Expected: PASS (may need to fix compilation errors from removing the field)

- [ ] **Step 9: Run goimports and lint**

Run: `goimports -w internal/controller/deployment_builder.go internal/controller/agentruntime_controller.go internal/controller/eval_worker.go cmd/main.go`
Run: `golangci-lint run ./internal/controller/... ./cmd/...`

- [ ] **Step 10: Commit**

```
refactor: remove singleton SessionAPIURL from agent runtime reconciler (#715)

Agent pods no longer receive SESSION_API_URL via env var injection.
Eval worker resolves session-api URL from Workspace CRD status.
Operator main.go configures ServiceBuilder on WorkspaceReconciler.
```

---

## Task 6: Update Facade and Runtime to Use Service Discovery

**Files:**
- Modify: `cmd/agent/main.go:136-144`
- Modify: `internal/runtime/config_crd.go:105-127`
- Modify: `internal/runtime/config.go:75,105`

### Steps

- [ ] **Step 1: Update facade to use service discovery resolver**

In `cmd/agent/main.go`, replace `initSessionStore` function (lines 136-144):

```go
func initSessionStore(log logr.Logger) (session.Store, error) {
	// Service discovery: env var override for local dev, Workspace CRD for in-cluster
	resolver := servicediscovery.NewResolver(buildK8sClient())
	urls, err := resolver.ResolveServiceURLs(context.Background(), resolveServiceGroup())
	if err != nil {
		log.Info("service discovery unavailable, using in-memory session store", "reason", err.Error())
		return session.NewMemoryStore(), nil
	}
	log.Info("using session-api HTTP store", "url", urls.SessionURL)
	return httpclient.NewStore(urls.SessionURL, log), nil
}
```

Add helpers:

```go
func buildK8sClient() client.Client {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil // Not in cluster, will fall back to env vars
	}
	scheme := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil
	}
	return c
}

func resolveServiceGroup() string {
	sg := os.Getenv("OMNIA_SERVICE_GROUP")
	if sg != "" {
		return sg
	}
	return "default"
}
```

Add the `servicediscovery` import.

- [ ] **Step 2: Update runtime config_crd.go**

In `internal/runtime/config_crd.go`, replace lines 105-127 (the session URL and memory URL resolution block):

```go
	// Service URLs from Workspace CRD status (in-cluster) or env vars (local dev)
	resolver := servicediscovery.NewResolver(c)
	serviceGroup := ar.Spec.ServiceGroup
	if serviceGroup == "" {
		serviceGroup = "default"
	}
	urls, err := resolver.ResolveServiceURLs(ctx, serviceGroup)
	if err == nil {
		cfg.SessionAPIURL = urls.SessionURL
		cfg.MemoryAPIURL = urls.MemoryURL
	}

	// Memory config from CRD
	if ar.Spec.Memory != nil && ar.Spec.Memory.Enabled {
		cfg.MemoryEnabled = true
		// Workspace UID for memory scope
		cfg.WorkspaceUID = resolveWorkspaceUID(ctx, c, namespace)
	}
```

Remove the old `envSessionAPIURL` / `envMemoryAPIURL` env var reads and the string replacement hack.

- [ ] **Step 3: Clean up config.go constants**

In `internal/runtime/config.go`, remove or mark as deprecated:
- Line 105: `envSessionAPIURL = "SESSION_API_URL"`
- Any `envMemoryAPIURL` constant

Keep the `SessionAPIURL` and `MemoryAPIURL` fields on the config struct — they're still used, just populated differently.

- [ ] **Step 4: Run tests**

Run: `go test ./cmd/agent/... -count=1 -v` (if agent has tests)
Run: `go test ./internal/runtime/... -count=1 -v` (runtime tests — these are excluded from linting but should still compile)

- [ ] **Step 5: Run goimports**

Run: `goimports -w cmd/agent/main.go internal/runtime/config_crd.go internal/runtime/config.go`

- [ ] **Step 6: Commit**

```
feat: facade and runtime use service discovery for URL resolution (#715)

Replaces hardcoded SESSION_API_URL / MEMORY_API_URL env var reads with
Workspace CRD status lookup via pkg/servicediscovery. Falls back to
env vars for local development.
```

---

## Task 7: Session-API and Memory-API Self-Configuration

**Files:**
- Modify: `cmd/session-api/main.go:75-114,168-278`
- Modify: `cmd/memory-api/main.go:91-128`

### Steps

- [ ] **Step 1: Add workspace/service-group flags to session-api**

In `cmd/session-api/main.go`, add to the `flags` struct:

```go
	workspace    string
	serviceGroup string
```

In `parseFlags()`, add:

```go
	flag.StringVar(&f.workspace, "workspace", "", "Workspace name (K8s CRD resolution mode)")
	flag.StringVar(&f.serviceGroup, "service-group", "", "Service group name within workspace")
```

- [ ] **Step 2: Add K8s config resolution to session-api run()**

In the `run()` function, after `parseFlags()` but before the existing postgres validation, add a K8s config resolution path:

```go
	// If workspace flags are set, resolve config from Workspace CRD
	if f.workspace != "" && f.serviceGroup != "" {
		cfg, err := ctrl.GetConfig()
		if err != nil {
			return fmt.Errorf("building K8s client config: %w", err)
		}
		scheme := runtime.NewScheme()
		_ = omniav1alpha1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)
		c, err := client.New(cfg, client.Options{Scheme: scheme})
		if err != nil {
			return fmt.Errorf("creating K8s client: %w", err)
		}

		cr := servicediscovery.NewConfigResolver(c)
		namespace := detectNamespace()
		sessCfg, err := cr.ResolveSessionConfig(ctx, f.workspace, f.serviceGroup, namespace)
		if err != nil {
			return fmt.Errorf("resolving session config from workspace: %w", err)
		}
		f.postgresConn = sessCfg.PostgresConn
		// Apply retention config if present
		// WarmDays would be applied to the retention worker config
		log.Info("config resolved from workspace CRD",
			"workspace", f.workspace,
			"serviceGroup", f.serviceGroup)
	}
```

Add a `detectNamespace()` helper:

```go
func detectNamespace() string {
	if ns := os.Getenv("OMNIA_NAMESPACE"); ns != "" {
		return ns
	}
	data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "default"
	}
	return string(data)
}
```

- [ ] **Step 3: Add same flags and resolution to memory-api**

In `cmd/memory-api/main.go`, add the same `workspace` and `serviceGroup` fields to the `flags` struct and `parseFlags()`. Add K8s config resolution in the `run()` function:

```go
	if f.workspace != "" && f.serviceGroup != "" {
		cfg, err := ctrl.GetConfig()
		if err != nil {
			return fmt.Errorf("building K8s client config: %w", err)
		}
		scheme := runtime.NewScheme()
		_ = omniav1alpha1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)
		c, err := client.New(cfg, client.Options{Scheme: scheme})
		if err != nil {
			return fmt.Errorf("creating K8s client: %w", err)
		}

		cr := servicediscovery.NewConfigResolver(c)
		namespace := detectNamespace()
		memCfg, err := cr.ResolveMemoryConfig(ctx, f.workspace, f.serviceGroup, namespace)
		if err != nil {
			return fmt.Errorf("resolving memory config from workspace: %w", err)
		}
		f.postgresConn = memCfg.PostgresConn
		f.embeddingProviderName = memCfg.EmbeddingProviderName
		if memCfg.DefaultTTL != "" {
			f.defaultTTL = memCfg.DefaultTTL
		}
		log.Info("config resolved from workspace CRD",
			"workspace", f.workspace,
			"serviceGroup", f.serviceGroup,
			"hasEmbeddingProvider", memCfg.EmbeddingProviderName != "")
	}
```

- [ ] **Step 4: Add embedding warning logs to memory-api**

Find where the memory-api sets up the embedding service (where it checks `embeddingProviderName != ""`). After the existing "no embedding provider" log, upgrade it to a warning that repeats on requests:

In the request handler or middleware, add:

```go
	if embeddingService == nil {
		log.Info("WARNING: memory-api running without embedding provider — semantic search disabled",
			"workspace", f.workspace,
			"serviceGroup", f.serviceGroup)
	}
```

For per-request warnings, add a middleware or check in the search handler that logs:

```go
	log.V(0).Info("semantic search unavailable",
		"reason", "no embedding provider configured")
```

- [ ] **Step 5: Run tests**

Run: `go test ./cmd/session-api/... -count=1 -v`
Run: `go test ./cmd/memory-api/... -count=1 -v`

- [ ] **Step 6: Run goimports**

Run: `goimports -w cmd/session-api/main.go cmd/memory-api/main.go`

- [ ] **Step 7: Commit**

```
feat: session-api and memory-api self-configure from Workspace CRD (#715)

Both services accept --workspace and --service-group flags. When set,
they read config from the Workspace CRD via K8s client. Falls back to
existing flag/env var config for local dev. Memory-api logs warnings
when running without an embedding provider.
```

---

## Task 8: Split Migrations — Separate Memory Schema from Session Schema

**Files:**
- Delete: `internal/session/postgres/migrations/000025_memory_tables.up.sql`
- Delete: `internal/session/postgres/migrations/000025_memory_tables.down.sql`
- Delete: `internal/session/postgres/migrations/000026_consent_grants.up.sql`
- Delete: `internal/session/postgres/migrations/000026_consent_grants.down.sql`
- Delete: `internal/session/postgres/migrations/000027_audit_memory_event_types.up.sql`
- Delete: `internal/session/postgres/migrations/000027_audit_memory_event_types.down.sql`
- Create: `internal/memory/postgres/migrations/000001_initial_schema.up.sql`
- Create: `internal/memory/postgres/migrations/000001_initial_schema.down.sql`
- Create: `internal/memory/postgres/migrator.go`
- Create: `internal/memory/postgres/migrator_test.go`
- Modify: `cmd/memory-api/main.go` (use memory migrator instead of session migrator)

### Steps

- [ ] **Step 1: Read existing memory migration files to understand the schema**

Read:
- `internal/session/postgres/migrations/000025_memory_tables.up.sql`
- `internal/session/postgres/migrations/000026_consent_grants.up.sql`
- `internal/session/postgres/migrations/000027_audit_memory_event_types.up.sql`

Combine their contents into a single fresh migration.

- [ ] **Step 2: Create fresh memory migrations directory and initial schema**

Create `internal/memory/postgres/migrations/000001_initial_schema.up.sql` with the combined memory table DDL from the three migration files above.

Create `internal/memory/postgres/migrations/000001_initial_schema.down.sql` with the corresponding DROP statements.

- [ ] **Step 3: Create memory migrator**

```go
// internal/memory/postgres/migrator.go
package postgres

import (
	"embed"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrator wraps golang-migrate for memory-api schema management.
type Migrator struct {
	m *migrate.Migrate
}

// NewMigrator creates a migrator for the memory database.
func NewMigrator(connStr string, log logr.Logger) (*Migrator, error) {
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("creating migration source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", source, connStr)
	if err != nil {
		return nil, fmt.Errorf("creating migrator: %w", err)
	}
	return &Migrator{m: m}, nil
}

// Up applies all pending migrations.
func (m *Migrator) Up() error {
	err := m.m.Up()
	if err == migrate.ErrNoChange {
		return nil
	}
	return err
}

// Close releases migrator resources.
func (m *Migrator) Close() error {
	srcErr, dbErr := m.m.Close()
	if srcErr != nil {
		return srcErr
	}
	return dbErr
}
```

- [ ] **Step 4: Delete memory migration files from session migrations**

Delete:
- `internal/session/postgres/migrations/000025_memory_tables.up.sql`
- `internal/session/postgres/migrations/000025_memory_tables.down.sql`
- `internal/session/postgres/migrations/000026_consent_grants.up.sql`
- `internal/session/postgres/migrations/000026_consent_grants.down.sql`
- `internal/session/postgres/migrations/000027_audit_memory_event_types.up.sql`
- `internal/session/postgres/migrations/000027_audit_memory_event_types.down.sql`

- [ ] **Step 5: Update memory-api main.go to use memory migrator**

In `cmd/memory-api/main.go`, change the `runMigrations` function to use the memory migrator instead of `sessionpg.NewMigrator`:

```go
import memorypg "github.com/altairalabs/omnia/internal/memory/postgres"

func runMigrations(connStr string, log logr.Logger) error {
	migrator, err := memorypg.NewMigrator(connStr, log)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}
	if err := migrator.Up(); err != nil {
		_ = migrator.Close()
		return fmt.Errorf("running migrations: %w", err)
	}
	_ = migrator.Close()
	return nil
}
```

- [ ] **Step 6: Run goimports**

Run: `goimports -w internal/memory/postgres/migrator.go cmd/memory-api/main.go`

- [ ] **Step 7: Run tests**

Run: `go test ./internal/memory/postgres/... -count=1 -v`
Run: `go test ./cmd/memory-api/... -count=1 -v`

- [ ] **Step 8: Commit**

```
refactor: separate memory migrations from session database (#715)

Memory tables (memory_entities, consent_grants, audit event types)
moved to internal/memory/postgres/migrations/ as a fresh 000001 schema.
Memory-api now uses its own migrator. Session-api keeps session-only
schema.
```

---

## Task 9: Remove Helm Singleton Templates

**Files:**
- Delete: `charts/omnia/templates/session-api/` (8 files)
- Delete: `charts/omnia/templates/memory-api/` (7 files)
- Modify: `charts/omnia/values.yaml`

### Steps

- [ ] **Step 1: Delete session-api Helm templates**

Delete the entire `charts/omnia/templates/session-api/` directory:
- `deployment.yaml`
- `service.yaml`
- `serviceaccount.yaml`
- `hpa.yaml`
- `pdb.yaml`
- `networkpolicy.yaml`
- `privacy-rbac.yaml`
- `dev-postgres.yaml`

- [ ] **Step 2: Delete memory-api Helm templates**

Delete the entire `charts/omnia/templates/memory-api/` directory:
- `deployment.yaml`
- `service.yaml`
- `serviceaccount.yaml`
- `hpa.yaml`
- `pdb.yaml`
- `networkpolicy.yaml`
- `privacy-rbac.yaml`

- [ ] **Step 3: Update values.yaml**

Remove `sessionApi` and `memoryApi` top-level sections from `charts/omnia/values.yaml`.

Add new values for the operator to know which images to use for workspace service instances:

```yaml
workspaceServices:
  sessionApi:
    image:
      repository: ghcr.io/altairalabs/omnia-session-api
      tag: ""  # defaults to chart appVersion
      pullPolicy: IfNotPresent
  memoryApi:
    image:
      repository: ghcr.io/altairalabs/omnia-memory-api
      tag: ""
      pullPolicy: IfNotPresent
```

- [ ] **Step 4: Verify Helm template renders**

Run: `helm template omnia charts/omnia/ --dry-run 2>&1 | head -50`
Expected: No errors about missing session-api/memory-api templates

- [ ] **Step 5: Commit**

```
refactor: remove singleton session-api and memory-api Helm templates (#715)

Per-workspace instances are now managed by the operator. Helm values
updated to provide service images to the operator instead of deploying
singletons.
```

---

## Task 10: Update Arena/EE Consumers

**Files:**
- Modify: `ee/internal/controller/arenajob_controller.go:130-132,672-675`
- Modify: `ee/internal/controller/arenadevsession_controller.go:62-64,538-541`
- Modify: `ee/cmd/omnia-arena-controller/main.go:288,299`
- Modify: `ee/cmd/arena-worker/worker.go:106,165,575-577`

### Steps

- [ ] **Step 1: Update ArenaJobReconciler to resolve from workspace**

In `ee/internal/controller/arenajob_controller.go`:
- Remove `SessionAPIURL string` field (line 132)
- At line 672-675 where it injects `SESSION_API_URL`, resolve from workspace status instead:

```go
	if arenaJob.Spec.SessionRecording {
		sessionURL := r.resolveSessionURLForWorkspace(ctx, arenaJob.Namespace)
		if sessionURL != "" {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "SESSION_API_URL",
				Value: sessionURL,
			})
		}
	}
```

Add a resolver helper method (same pattern as Task 5's eval worker helper):

```go
func (r *ArenaJobReconciler) resolveSessionURLForWorkspace(ctx context.Context, namespace string) string {
	var list omniav1alpha1.WorkspaceList
	if err := r.List(ctx, &list); err != nil {
		return ""
	}
	for _, ws := range list.Items {
		if ws.Spec.Namespace.Name == namespace {
			for _, sg := range ws.Status.Services {
				if sg.Name == "default" && sg.Ready {
					return sg.SessionURL
				}
			}
		}
	}
	return ""
}
```

- [ ] **Step 2: Update ArenaDevSessionReconciler**

In `ee/internal/controller/arenadevsession_controller.go`:
- Remove `SessionAPIURL string` field (line 64)
- At lines 538-541, use the same workspace resolution pattern

- [ ] **Step 3: Update omnia-arena-controller main.go**

In `ee/cmd/omnia-arena-controller/main.go`:
- Remove `SessionAPIURL` from both ArenaJobReconciler and ArenaDevSessionReconciler initialization (lines 288, 299)
- Remove the `sessionAPIURL` variable/flag

- [ ] **Step 4: Update arena-worker**

In `ee/cmd/arena-worker/worker.go`:
- The worker reads `SESSION_API_URL` from env (line 165) — this is injected by the ArenaJob controller, which we updated in Step 1. No change needed in the worker itself.

- [ ] **Step 5: Update arena test files**

Update `ee/internal/controller/arenajob_controller_test.go`:
- Line 383: remove `SessionAPIURL: "http://session-api:8080"` from reconciler setup
- Add test for workspace-based URL resolution

- [ ] **Step 6: Run tests**

Run: `go test ./ee/internal/controller/... -count=1 -v`

- [ ] **Step 7: Run goimports and lint**

Run: `goimports -w ee/internal/controller/arenajob_controller.go ee/internal/controller/arenadevsession_controller.go ee/cmd/omnia-arena-controller/main.go`

- [ ] **Step 8: Commit**

```
refactor: Arena controllers resolve session-api URL from workspace (#715)

ArenaJob and ArenaDevSession reconcilers no longer take a static
SessionAPIURL. They resolve it from the Workspace CRD status for the
namespace the job runs in.
```

---

## Task 11: Update Doctor Smoke Tests

**Files:**
- Modify: `internal/doctor/checks/sessions.go`
- Modify: `internal/doctor/checks/memory.go`
- Modify: `cmd/doctor/main.go`

### Steps

- [ ] **Step 1: Update doctor main.go to resolve URLs per workspace**

In `cmd/doctor/main.go`, replace the singleton session-api/memory-api URL configuration with workspace-aware resolution. The doctor should accept a `--workspace` flag and resolve service URLs from the Workspace CRD status.

Read `cmd/doctor/main.go` fully first, then:

- Add `--workspace` and `--service-group` flags (default "default" for service group)
- Use the service discovery resolver to find URLs
- Fall back to env vars for testing against local services

- [ ] **Step 2: Add workspace service health checks**

Add a new check to the workspace checker (`internal/doctor/checks/workspace.go`):

```go
// checkServiceGroupHealth verifies all managed service groups have running Deployments.
func (w *WorkspaceChecker) checkServiceGroupHealth(ctx context.Context) doctor.TestResult {
	// For each workspace with services, verify:
	// 1. Deployments exist and have ready replicas
	// 2. Status URLs are populated
	// 3. Health endpoints respond
}
```

- [ ] **Step 3: Update existing session/memory checks**

`SessionChecker` and `MemoryChecker` already take URLs as constructor params — they just need the doctor main to pass workspace-resolved URLs instead of singleton URLs. No changes needed to the checkers themselves, only to how they're constructed in `cmd/doctor/main.go`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/doctor/... -count=1 -v`
Run: `go test ./cmd/doctor/... -count=1 -v`

- [ ] **Step 5: Run goimports**

Run: `goimports -w cmd/doctor/main.go internal/doctor/checks/workspace.go`

- [ ] **Step 6: Commit**

```
feat: doctor smoke tests resolve service URLs per workspace (#715)

Doctor accepts --workspace/--service-group flags. Service URLs resolved
from Workspace CRD status. Falls back to env vars for local testing.
```

---

## Task 12: Service Documentation

**Files:**
- Create: `cmd/session-api/SERVICE.md`
- Create: `cmd/session-api/CLAUDE.md`
- Create: `cmd/memory-api/SERVICE.md`
- Create: `cmd/memory-api/CLAUDE.md`

### Steps

- [ ] **Step 1: Write session-api SERVICE.md**

```markdown
# Session API Service

## Overview

Per-workspace HTTP service for session CRUD operations. Manages chat sessions,
messages, tool calls, runtime events, eval results, and audit logs.

## Ownership

- Session lifecycle (create, read, update, delete)
- Message storage and retrieval
- Tool call tracking
- Runtime event recording
- Eval result storage
- Audit logging (enterprise)
- Privacy/deletion request processing

## Inputs

- HTTP REST API on port 8080 (session CRUD, message queries, tool call queries)
- Health/readiness probes on port 8081
- Metrics on port 9090

## Configuration

**In-cluster (managed by operator):**
Configured via Workspace CRD. Operator creates Deployment with:
- `--workspace=<name>` — Workspace CRD name
- `--service-group=<name>` — Service group within workspace

The service reads its config (database Secret, retention settings) from the
Workspace CRD via K8s client.

**Local dev:**
Configured via flags/env vars:
- `--postgres-conn` / `POSTGRES_CONN` — PostgreSQL connection string
- `--redis-addrs` / `REDIS_ADDRS` — Redis addresses (optional)
- `--enterprise` / `ENTERPRISE_ENABLED` — Enable enterprise features

## Data Flow

```
Agent Pod (facade) → HTTP → Session API → PostgreSQL
                                       → Redis (warm cache, optional)
                                       → S3/GCS/Azure (cold archive, optional)
```

## Dependencies

- PostgreSQL (required) — session storage
- Redis (optional) — warm cache
- S3/GCS/Azure (optional) — cold archive
```

- [ ] **Step 2: Write session-api CLAUDE.md**

```markdown
# Session API — Dev Instructions

## Running Locally

```bash
# Start with local Postgres
go run ./cmd/session-api/ --postgres-conn="postgres://user:pass@localhost:5432/sessions"

# With all options
go run ./cmd/session-api/ \
  --postgres-conn="postgres://user:pass@localhost:5432/sessions" \
  --redis-addrs="localhost:6379" \
  --enterprise=true
```

## Running in K8s Mode (simulated)

```bash
go run ./cmd/session-api/ --workspace=dev --service-group=default
```
Requires K8s access and a Workspace CRD with matching service group.

## Testing

```bash
go test ./cmd/session-api/... -count=1 -v
go test ./internal/session/... -count=1 -v
```

## Migrations

Session-api runs its own migrations at startup from `internal/session/postgres/migrations/`.
```

- [ ] **Step 3: Write memory-api SERVICE.md**

```markdown
# Memory API Service

## Overview

Per-workspace HTTP service for agentic memory operations. Stores, retrieves,
and searches memory entries with optional semantic search via embeddings.

## Ownership

- Memory entity lifecycle (save, retrieve, search, forget)
- Embedding generation (via Provider CRD reference)
- Consent grant management
- Memory retention/TTL enforcement
- Privacy/deletion processing

## Inputs

- HTTP REST API on port 8080 (memory CRUD, search, consent)
- Health/readiness probes on port 8081
- Metrics on port 9090

## Configuration

**In-cluster (managed by operator):**
Configured via Workspace CRD. Operator creates Deployment with:
- `--workspace=<name>` — Workspace CRD name
- `--service-group=<name>` — Service group within workspace

Reads database Secret, embedding Provider CRD ref, and retention config from
the Workspace CRD.

**Local dev:**
- `--postgres-conn` / `POSTGRES_CONN` — PostgreSQL connection string
- `--embedding-provider` / `EMBEDDING_PROVIDER` — Provider CRD name (optional)
- `--default-ttl` / `DEFAULT_TTL` — Default memory TTL (optional)

## Data Flow

```
Agent Pod (runtime) → HTTP → Memory API → PostgreSQL
                                        → Provider CRD → Embedding Model (Ollama, etc.)
```

## Dependencies

- PostgreSQL (required) — memory storage
- Provider CRD + embedding model (optional) — semantic search
- Redis (optional) — event publishing

## Warning: No Embedding Provider

If started without a `providerRef` in the Workspace CRD (or `--embedding-provider`),
semantic search is disabled. The service logs warnings on every search request
that would have used embeddings.
```

- [ ] **Step 4: Write memory-api CLAUDE.md**

```markdown
# Memory API — Dev Instructions

## Running Locally

```bash
# Minimal (no embeddings)
go run ./cmd/memory-api/ --postgres-conn="postgres://user:pass@localhost:5432/memories"

# With embeddings (requires Ollama or other provider running)
go run ./cmd/memory-api/ \
  --postgres-conn="postgres://user:pass@localhost:5432/memories" \
  --embedding-provider=ollama-local \
  --default-ttl=720h
```

## Running in K8s Mode (simulated)

```bash
go run ./cmd/memory-api/ --workspace=dev --service-group=default
```

## Testing

```bash
go test ./cmd/memory-api/... -count=1 -v
go test ./internal/memory/... -count=1 -v
```

## Migrations

Memory-api runs its own migrations at startup from `internal/memory/postgres/migrations/`.
Separate from session-api migrations — each service has its own database.
```

- [ ] **Step 5: Commit**

```
docs: add SERVICE.md and CLAUDE.md for session-api and memory-api (#715)
```

---

## Task 13: Regenerate CRDs and Final Integration

**Files:**
- Run: `make generate && make manifests`
- Modify: `charts/omnia/templates/clusterrole.yaml` (if needed)

### Steps

- [ ] **Step 1: Regenerate all generated code**

Run: `make generate && make manifests`

This updates:
- `zz_generated.deepcopy.go` for new types
- CRD YAML manifests in `config/crd/`
- RBAC manifests

- [ ] **Step 2: Sync chart CRDs**

Run: `make sync-chart-crds`

This copies updated CRD YAMLs into `charts/omnia/crds/`.

- [ ] **Step 3: Verify ClusterRole has required permissions**

Read `charts/omnia/templates/clusterrole.yaml` and verify it includes permissions for:
- `apps/deployments` — get, list, watch, create, update, patch, delete
- `core/services` — get, list, watch, create, update, patch, delete

The kubebuilder RBAC markers on the workspace controller should generate these. If not in the chart template, add them manually.

- [ ] **Step 4: Run full build**

Run: `go build ./...`
Expected: PASS — everything compiles

- [ ] **Step 5: Run full test suite**

Run: `go test ./... -count=1` (excluding runtime packages which need PromptKit)
Expected: PASS

- [ ] **Step 6: Run lint**

Run: `golangci-lint run ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```
chore: regenerate CRDs, manifests, and sync chart for #715
```

---

## Task 14: Dashboard Consumer Updates (if applicable)

**Files:**
- Modify: `dashboard/src/lib/data/session-api-service.test.ts` (update mock URLs)
- Modify: `dashboard/src/lib/data/live-service.ts` (if it references singleton URLs)
- Modify: `dashboard/src/lib/data/index.ts` (if it exports singleton URLs)

### Steps

- [ ] **Step 1: Investigate dashboard proxy pattern**

Read `dashboard/src/lib/data/index.ts` and related files to understand how the dashboard currently resolves session-api and memory-api URLs. The dashboard proxies through the operator, so the operator API handlers need to become workspace-aware.

- [ ] **Step 2: Update operator API handlers**

The operator's API handlers that proxy to session-api/memory-api need to accept a workspace context (from the request path or query parameter) and resolve the correct service URL from the Workspace CRD status.

Read the relevant handler files in the operator and update them to:
1. Extract workspace context from the request
2. Look up Workspace CRD → find service group → get URL from status
3. Proxy the request to the resolved URL

- [ ] **Step 3: Update dashboard data services**

Update the dashboard data services to pass workspace context in API requests. The dashboard already has workspace context (workspace selector), so this should be adding the workspace name to the proxy request path/params.

- [ ] **Step 4: Run dashboard tests**

Run: `cd dashboard && npx vitest run --coverage`
Expected: PASS

- [ ] **Step 5: Run dashboard lint and typecheck**

Run: `cd dashboard && npm run lint && npm run typecheck`
Expected: PASS

- [ ] **Step 6: Commit**

```
feat: dashboard proxies session/memory requests per workspace (#715)
```

---

## Summary

| Task | Component | Commits |
|------|-----------|---------|
| 1 | CRD type definitions | 1 |
| 2 | Service discovery client | 1 |
| 3 | Service deployment builder | 1 |
| 4 | Workspace controller service reconciliation | 1 |
| 5 | Remove singleton env var injection | 1 |
| 6 | Facade/runtime service discovery | 1 |
| 7 | Session-api/memory-api self-configuration | 1 |
| 8 | Split migrations | 1 |
| 9 | Remove Helm singleton templates | 1 |
| 10 | Arena/EE consumer updates | 1 |
| 11 | Doctor smoke tests | 1 |
| 12 | Service documentation | 1 |
| 13 | Regenerate CRDs and final integration | 1 |
| 14 | Dashboard consumer updates | 1 |

**Total: 14 tasks, ~14 commits**

Tasks 1-4 are sequential (each builds on the previous). Tasks 5-12 can be partially parallelized. Task 13 is the integration gate. Task 14 may be split into a follow-up PR if dashboard changes are complex.
