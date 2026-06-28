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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkspaceEnvironment defines the environment tier for a workspace.
// +kubebuilder:validation:Enum=development;staging;production
type WorkspaceEnvironment string

const (
	// WorkspaceEnvironmentDevelopment is for development workspaces.
	WorkspaceEnvironmentDevelopment WorkspaceEnvironment = "development"
	// WorkspaceEnvironmentStaging is for staging workspaces.
	WorkspaceEnvironmentStaging WorkspaceEnvironment = "staging"
	// WorkspaceEnvironmentProduction is for production workspaces.
	WorkspaceEnvironmentProduction WorkspaceEnvironment = "production"
)

// WorkspaceRole defines the role level for workspace access.
// +kubebuilder:validation:Enum=owner;editor;viewer
type WorkspaceRole string

const (
	// WorkspaceRoleOwner has full control within workspace including member management.
	WorkspaceRoleOwner WorkspaceRole = "owner"
	// WorkspaceRoleEditor can create/modify resources but not manage members.
	WorkspaceRoleEditor WorkspaceRole = "editor"
	// WorkspaceRoleViewer has read-only access to resources.
	WorkspaceRoleViewer WorkspaceRole = "viewer"
)

// NamespaceConfig defines the namespace configuration for a workspace.
type NamespaceConfig struct {
	// name is the name of the namespace for this workspace.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`

	// create specifies whether to auto-create the namespace if it doesn't exist.
	// Defaults to false for safety - users must explicitly enable namespace creation.
	// +optional
	Create bool `json:"create,omitempty"`

	// labels are additional labels to apply to the namespace.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// annotations are additional annotations to apply to the namespace.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ServiceAccountRef references a ServiceAccount.
type ServiceAccountRef struct {
	// name is the name of the ServiceAccount.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// namespace is the namespace of the ServiceAccount.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`
}

// RoleBinding defines a mapping from groups or ServiceAccounts to a workspace role.
type RoleBinding struct {
	// groups is a list of IdP group names that should be granted this role.
	// Group names must exactly match IdP group claim values.
	// +optional
	Groups []string `json:"groups,omitempty"`

	// serviceAccounts is a list of ServiceAccounts that should be granted this role.
	// +optional
	ServiceAccounts []ServiceAccountRef `json:"serviceAccounts,omitempty"`

	// role is the workspace role to grant.
	// +kubebuilder:validation:Required
	Role WorkspaceRole `json:"role"`
}

// DirectGrant defines a direct user grant for workspace access.
type DirectGrant struct {
	// user is the email or username of the user.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	User string `json:"user"`

	// role is the workspace role to grant.
	// +kubebuilder:validation:Required
	Role WorkspaceRole `json:"role"`

	// expires is an optional expiration time for this grant.
	// +optional
	Expires *metav1.Time `json:"expires,omitempty"`
}

// AnonymousAccess configures access for unauthenticated users.
// WARNING: Granting editor or owner access allows anonymous users to modify resources.
// Only use in isolated development environments.
type AnonymousAccess struct {
	// enabled specifies whether anonymous users can access this workspace.
	// If false or omitted, anonymous users have no access.
	// +kubebuilder:validation:Required
	Enabled bool `json:"enabled"`

	// role is the workspace role granted to anonymous users.
	// Defaults to viewer if enabled is true but role is not specified.
	// WARNING: editor and owner grant write access to anonymous users.
	// +optional
	Role WorkspaceRole `json:"role,omitempty"`
}

// BudgetExceededAction defines what action to take when budget is exceeded.
// +kubebuilder:validation:Enum=warn;pauseJobs;block
type BudgetExceededAction string

const (
	// BudgetExceededActionWarn only logs warnings when budget is exceeded.
	BudgetExceededActionWarn BudgetExceededAction = "warn"
	// BudgetExceededActionPauseJobs pauses Arena jobs when budget is exceeded.
	BudgetExceededActionPauseJobs BudgetExceededAction = "pauseJobs"
	// BudgetExceededActionBlock blocks new API requests when budget is exceeded.
	BudgetExceededActionBlock BudgetExceededAction = "block"
)

// CostAlertThreshold defines a threshold for cost alerts.
type CostAlertThreshold struct {
	// percent is the percentage of budget at which to trigger the alert.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	Percent int32 `json:"percent"`

	// notify is a list of email addresses to notify when threshold is reached.
	// +optional
	Notify []string `json:"notify,omitempty"`
}

// CostControls defines budget and cost control settings for a workspace.
type CostControls struct {
	// dailyBudget is the maximum daily spending limit in USD (e.g., "100.00").
	// +optional
	DailyBudget string `json:"dailyBudget,omitempty"`

	// monthlyBudget is the maximum monthly spending limit in USD (e.g., "2000.00").
	// +optional
	MonthlyBudget string `json:"monthlyBudget,omitempty"`

	// budgetExceededAction defines what action to take when budget is exceeded.
	// +kubebuilder:default=warn
	// +optional
	BudgetExceededAction BudgetExceededAction `json:"budgetExceededAction,omitempty"`

	// alertThresholds defines thresholds for cost alerts.
	// +optional
	AlertThresholds []CostAlertThreshold `json:"alertThresholds,omitempty"`
}

// IPBlock describes a CIDR block with optional exceptions.
type IPBlock struct {
	// cidr is a string representing the IP block (e.g., "192.168.1.0/24" or "0.0.0.0/0").
	// +kubebuilder:validation:Required
	CIDR string `json:"cidr"`

	// except is a list of CIDRs that should not be included within the IP block.
	// +optional
	Except []string `json:"except,omitempty"`
}

// LabelSelector represents a label selector for namespace or pod selection.
type LabelSelector struct {
	// matchLabels is a map of key-value pairs for label matching.
	// +optional
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
}

// NetworkPolicyPeer describes a peer to allow traffic to/from.
type NetworkPolicyPeer struct {
	// namespaceSelector selects namespaces by label.
	// +optional
	NamespaceSelector *LabelSelector `json:"namespaceSelector,omitempty"`

	// podSelector selects pods by label within the selected namespaces.
	// +optional
	PodSelector *LabelSelector `json:"podSelector,omitempty"`

	// ipBlock defines CIDR ranges to allow traffic to/from.
	// +optional
	IPBlock *IPBlock `json:"ipBlock,omitempty"`
}

// NetworkPolicyPort describes a port to allow traffic on.
type NetworkPolicyPort struct {
	// protocol is the protocol (TCP, UDP, or SCTP).
	// +kubebuilder:validation:Enum=TCP;UDP;SCTP
	// +kubebuilder:default=TCP
	// +optional
	Protocol string `json:"protocol,omitempty"`

	// port is the port number or name.
	// +kubebuilder:validation:Required
	Port int32 `json:"port"`
}

// NetworkPolicyRule defines a single ingress or egress rule.
type NetworkPolicyRule struct {
	// peers is a list of sources (for ingress) or destinations (for egress).
	// +optional
	Peers []NetworkPolicyPeer `json:"peers,omitempty"`

	// ports is a list of ports to allow.
	// +optional
	Ports []NetworkPolicyPort `json:"ports,omitempty"`
}

// WorkspaceStorageConfig defines shared storage configuration for a workspace.
type WorkspaceStorageConfig struct {
	// enabled specifies whether to create a shared PVC for this workspace.
	// When enabled, a PVC is created and mounted by dashboard and job runners.
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// storageClass is the Kubernetes storage class to use for the PVC.
	// If not specified, the cluster default storage class is used.
	// For production, use a ReadWriteMany-capable storage class (e.g., EFS, NFS).
	// +optional
	StorageClass string `json:"storageClass,omitempty"`

	// size is the requested storage size (e.g., "10Gi").
	// +kubebuilder:default="10Gi"
	// +optional
	Size string `json:"size,omitempty"`

	// accessModes specifies the PVC access modes.
	// Defaults to ReadWriteMany for shared access by dashboard and job runners.
	// +kubebuilder:default={"ReadWriteMany"}
	// +optional
	AccessModes []string `json:"accessModes,omitempty"`

	// retentionPolicy specifies what happens to the PVC when the workspace is deleted.
	// +kubebuilder:validation:Enum=Delete;Retain
	// +kubebuilder:default=Delete
	// +optional
	RetentionPolicy string `json:"retentionPolicy,omitempty"`
}

// WorkspaceNetworkPolicy defines network isolation settings for a workspace.
type WorkspaceNetworkPolicy struct {
	// isolate enables network isolation for the workspace namespace.
	// When true, a NetworkPolicy is created to restrict traffic.
	// +optional
	Isolate bool `json:"isolate,omitempty"`

	// allowFrom defines additional ingress rules.
	// +optional
	AllowFrom []NetworkPolicyRule `json:"allowFrom,omitempty"`

	// allowTo defines additional egress rules.
	// +optional
	AllowTo []NetworkPolicyRule `json:"allowTo,omitempty"`

	// allowExternalAPIs enables egress to external IPs (0.0.0.0/0 except private ranges).
	// Defaults to true when isolate is enabled.
	// +optional
	AllowExternalAPIs *bool `json:"allowExternalAPIs,omitempty"`

	// allowSharedNamespaces enables traffic to/from namespaces labeled omnia.altairalabs.ai/shared: true.
	// Defaults to true when isolate is enabled.
	// +optional
	AllowSharedNamespaces *bool `json:"allowSharedNamespaces,omitempty"`

	// allowPrivateNetworks removes the RFC 1918 private IP exclusions (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)
	// from the external APIs rule. Use this for local development or when agents need to access
	// services on private networks. Defaults to false for security.
	// +optional
	AllowPrivateNetworks *bool `json:"allowPrivateNetworks,omitempty"`
}

// NetworkPolicyStatus tracks the status of the workspace NetworkPolicy.
type NetworkPolicyStatus struct {
	// name is the name of the generated NetworkPolicy.
	// +optional
	Name string `json:"name,omitempty"`

	// enabled indicates whether network isolation is active.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// rulesCount is the total number of ingress and egress rules.
	// +optional
	RulesCount int32 `json:"rulesCount,omitempty"`
}

// WorkspaceStorageStatus tracks the status of workspace shared storage.
type WorkspaceStorageStatus struct {
	// pvcName is the name of the PVC created for this workspace.
	// +optional
	PVCName string `json:"pvcName,omitempty"`

	// phase is the current phase of the PVC (Pending, Bound, Lost).
	// +optional
	Phase string `json:"phase,omitempty"`

	// capacity is the actual provisioned capacity of the PVC.
	// +optional
	Capacity string `json:"capacity,omitempty"`

	// mountPath is the path where the PVC is mounted (e.g., /workspace-content/{workspace}/default).
	// +optional
	MountPath string `json:"mountPath,omitempty"`
}

// RuntimeDefaults are workspace-wide pod defaults applied to every AgentRuntime
// in the workspace. Its purpose is hyperscaler-agnostic cloud identity: the SA
// and pod labels needed to bind a runtime pod to a cloud workload identity
// (Azure Workload Identity, AWS IRSA, GKE Workload Identity) so keyless
// providers authenticate without a secret.
//
// Omnia treats these as opaque passthrough — it never interprets the values or
// branches on cloud provider. The annotated ServiceAccount and the cloud-side
// federated trust are provisioned out of band (IaC); the Workspace only
// references the SA by name and lists the pod labels the cloud's webhook needs.
//
// Precedence: an AgentRuntime that sets its own spec.podOverrides.serviceAccountName
// is bringing its own identity (its own annotated SA), so it opts OUT of these
// defaults as a unit — neither the workspace SA nor the workspace pod labels are
// applied to it. Agents that set no SA inherit these defaults.
type RuntimeDefaults struct {
	// serviceAccountName is the ServiceAccount every agent runtime pod in this
	// workspace runs as. Provisioned out of band (IaC) with the cloud identity
	// annotations (e.g. azure.workload.identity/client-id,
	// eks.amazonaws.com/role-arn, iam.gke.io/gcp-service-account). Empty = no
	// default; agents fall back to the operator-created per-agent SA.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// podLabels are added to every agent runtime pod, e.g.
	// {azure.workload.identity/use: "true"} to opt into the Azure webhook.
	// AWS IRSA and GKE WLI need none. Opaque to Omnia.
	// +optional
	PodLabels map[string]string `json:"podLabels,omitempty"`

	// podAnnotations are added to every agent runtime pod. Reserved for parity
	// with podLabels; rarely needed. Opaque to Omnia.
	// +optional
	PodAnnotations map[string]string `json:"podAnnotations,omitempty"`
}

// WorkspaceSpec defines the desired state of Workspace.
type WorkspaceSpec struct {
	// displayName is the human-readable name for this workspace.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	DisplayName string `json:"displayName"`

	// description is an optional description of the workspace.
	// +optional
	Description string `json:"description,omitempty"`

	// environment is the tier for this workspace (development, staging, production).
	// +kubebuilder:default=development
	// +optional
	Environment WorkspaceEnvironment `json:"environment,omitempty"`

	// defaultTags are labels applied to all resources created in this workspace.
	// Used for cost attribution and resource organization.
	// +optional
	DefaultTags map[string]string `json:"defaultTags,omitempty"`

	// namespace configures the Kubernetes namespace for this workspace.
	// +kubebuilder:validation:Required
	Namespace NamespaceConfig `json:"namespace"`

	// runtime are workspace-wide pod defaults applied to every AgentRuntime in
	// this workspace — primarily the cloud workload-identity SA + pod labels so
	// agents (including those provisioned via the deploy API, which can't carry
	// cloud-specific SA names) authenticate to keyless providers. Per-agent
	// spec.podOverrides.serviceAccountName overrides this as a unit. See
	// RuntimeDefaults.
	// +optional
	Runtime *RuntimeDefaults `json:"runtime,omitempty"`

	// roleBindings maps IdP groups and ServiceAccounts to workspace roles.
	// +optional
	RoleBindings []RoleBinding `json:"roleBindings,omitempty"`

	// directGrants are direct user grants for exceptions (use sparingly).
	// +optional
	DirectGrants []DirectGrant `json:"directGrants,omitempty"`

	// anonymousAccess configures access for unauthenticated users.
	// If omitted, anonymous users have no access to this workspace.
	// WARNING: Granting editor or owner allows anonymous users to modify resources.
	// +optional
	AnonymousAccess *AnonymousAccess `json:"anonymousAccess,omitempty"`

	// costControls defines budget and cost control settings.
	// +optional
	CostControls *CostControls `json:"costControls,omitempty"`

	// networkPolicy defines network isolation settings for this workspace.
	// +optional
	NetworkPolicy *WorkspaceNetworkPolicy `json:"networkPolicy,omitempty"`

	// storage configures shared storage for this workspace.
	// When enabled, a PVC is created for storing Arena content, PromptPacks, and datasets.
	// Job runners and dashboard mount this PVC directly for efficient content access.
	// +optional
	Storage *WorkspaceStorageConfig `json:"storage,omitempty"`

	// services defines per-workspace service groups for session-api and memory-api.
	// Each group can be managed (operator-provisioned) or external (user-supplied URLs).
	// Agents reference a group by name via spec.serviceGroup.
	// The maxItems cap bounds the per-item CEL validation cost (the array
	// multiplies each item rule's estimated cost; without a bound the
	// group-level redis exists_one rule exceeds the API server budget).
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=64
	Services []WorkspaceServiceGroup `json:"services,omitempty"`

	// mgmtPlaneMintServiceAccounts lists ServiceAccount names in this
	// workspace's namespace that may mint mgmt-plane JWTs via the dashboard's
	// service-token endpoint (used by in-cluster tooling such as the Arena
	// loadtest worker to authenticate to agent facades). The dashboard gates
	// minting on a Workspace existing for the caller's namespace AND the
	// caller's SA being listed here. When omitted, consumers default to
	// ["arena-worker"], so the operator-created worker SA works out of the box;
	// set this to override or extend the allowed SAs.
	// +optional
	// +kubebuilder:validation:MaxItems=16
	MgmtPlaneMintServiceAccounts []string `json:"mgmtPlaneMintServiceAccounts,omitempty"`

	// privacy optionally configures the per-workspace privacy-api service,
	// which owns consent grants and opt-out preferences for this workspace.
	// When omitted, no privacy-api is provisioned and the workspace's
	// session/memory services run without centralized preference enforcement.
	// +optional
	Privacy *PrivacyServiceConfig `json:"privacy,omitempty"`
}

// WorkspacePhase represents the current phase of a Workspace.
// +kubebuilder:validation:Enum=Pending;Ready;Suspended;Error
type WorkspacePhase string

const (
	// WorkspacePhasePending indicates the workspace is being set up.
	WorkspacePhasePending WorkspacePhase = "Pending"
	// WorkspacePhaseReady indicates the workspace is ready for use.
	WorkspacePhaseReady WorkspacePhase = "Ready"
	// WorkspacePhaseSuspended indicates the workspace is suspended.
	WorkspacePhaseSuspended WorkspacePhase = "Suspended"
	// WorkspacePhaseError indicates the workspace has an error.
	WorkspacePhaseError WorkspacePhase = "Error"
)

// NamespaceStatus tracks the status of the workspace namespace.
type NamespaceStatus struct {
	// name is the name of the namespace.
	Name string `json:"name,omitempty"`

	// created indicates whether the namespace was created by the controller.
	Created bool `json:"created,omitempty"`
}

// ServiceAccountStatus tracks the status of workspace ServiceAccounts.
type ServiceAccountStatus struct {
	// owner is the name of the owner ServiceAccount.
	Owner string `json:"owner,omitempty"`

	// editor is the name of the editor ServiceAccount.
	Editor string `json:"editor,omitempty"`

	// viewer is the name of the viewer ServiceAccount.
	Viewer string `json:"viewer,omitempty"`
}

// MemberCount tracks the number of members by role.
type MemberCount struct {
	// owners is the count of owner members.
	Owners int32 `json:"owners,omitempty"`

	// editors is the count of editor members.
	Editors int32 `json:"editors,omitempty"`

	// viewers is the count of viewer members.
	Viewers int32 `json:"viewers,omitempty"`
}

// CostUsage tracks the current cost usage for a workspace.
type CostUsage struct {
	// dailySpend is the current day's spending in USD.
	// +optional
	DailySpend string `json:"dailySpend,omitempty"`

	// dailyBudget is the configured daily budget in USD.
	// +optional
	DailyBudget string `json:"dailyBudget,omitempty"`

	// monthlySpend is the current month's spending in USD.
	// +optional
	MonthlySpend string `json:"monthlySpend,omitempty"`

	// monthlyBudget is the configured monthly budget in USD.
	// +optional
	MonthlyBudget string `json:"monthlyBudget,omitempty"`

	// lastUpdated is the timestamp of the last cost calculation.
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`
}

// WorkspaceStatus defines the observed state of Workspace.
type WorkspaceStatus struct {
	// phase represents the current lifecycle phase of the Workspace.
	// +optional
	Phase WorkspacePhase `json:"phase,omitempty"`

	// observedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// namespace tracks the status of the workspace namespace.
	// +optional
	Namespace *NamespaceStatus `json:"namespace,omitempty"`

	// serviceAccounts tracks the workspace ServiceAccounts.
	// +optional
	ServiceAccounts *ServiceAccountStatus `json:"serviceAccounts,omitempty"`

	// members tracks the count of members by role.
	// +optional
	Members *MemberCount `json:"members,omitempty"`

	// costUsage tracks the current cost usage for this workspace.
	// +optional
	CostUsage *CostUsage `json:"costUsage,omitempty"`

	// networkPolicy tracks the status of the workspace NetworkPolicy.
	// +optional
	NetworkPolicy *NetworkPolicyStatus `json:"networkPolicy,omitempty"`

	// storage tracks the status of workspace shared storage.
	// +optional
	Storage *WorkspaceStorageStatus `json:"storage,omitempty"`

	// services tracks the status of each service group defined in spec.services.
	// +optional
	Services []ServiceGroupStatus `json:"services,omitempty"`

	// privacyURL is the resolved URL of the per-workspace privacy-api.
	// +optional
	PrivacyURL string `json:"privacyURL,omitempty"`

	// conditions represent the current state of the Workspace resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Display Name",type=string,JSONPath=`.spec.displayName`
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.spec.environment`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Namespace",type=string,JSONPath=`.status.namespace.name`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Workspace is the Schema for the workspaces API.
// It defines a multi-tenant workspace with isolated namespace, RBAC, and resource quotas.
type Workspace struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Workspace
	// +required
	Spec WorkspaceSpec `json:"spec"`

	// status defines the observed state of Workspace
	// +optional
	Status WorkspaceStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// WorkspaceList contains a list of Workspace.
type WorkspaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Workspace `json:"items"`
}

// ServiceMode defines how a service group's endpoints are provisioned.
// +kubebuilder:validation:Enum=managed;external
type ServiceMode string

const (
	// ServiceModeManaged indicates the operator provisions and manages the service endpoints.
	ServiceModeManaged ServiceMode = "managed"
	// ServiceModeExternal indicates the user supplies pre-existing endpoint URLs.
	ServiceModeExternal ServiceMode = "external"
)

// WorkspaceServiceGroup defines a named group of session-api and memory-api endpoints
// for a workspace. Agents reference a group by name via spec.serviceGroup.
// +kubebuilder:validation:XValidation:rule="self.mode != 'managed' || (has(self.memory) && has(self.session))",message="managed mode requires both memory and session configuration"
// +kubebuilder:validation:XValidation:rule="self.mode != 'external' || has(self.external)",message="external mode requires external endpoints"
// +kubebuilder:validation:XValidation:rule="!has(self.redis) || [has(self.redis.existingSecret), has(self.redis.url) && size(self.redis.url) > 0, has(self.redis.host) && size(self.redis.host) > 0, has(self.redis.serviceRef)].exists_one(b, b)",message="services[].redis must use exactly one of existingSecret, url, host, or serviceRef"
type WorkspaceServiceGroup struct {
	// name is the unique identifier for this service group within the workspace.
	// Referenced by AgentRuntime spec.serviceGroup.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`

	// mode specifies whether the operator manages these endpoints or they are user-supplied.
	// +kubebuilder:default="managed"
	// +optional
	Mode ServiceMode `json:"mode,omitempty"`

	// redis declares a single Redis target for every managed service in
	// this group (session-api, memory-api). The operator resolves it to a
	// URL and injects it as --redis-url. This is a REFERENCE to an
	// existing Redis — the operator does not provision one. Per-component
	// session.redis / memory.redis override this for that component;
	// unset components fall back to the operator-wide default.
	// +optional
	Redis *RedisConfig `json:"redis,omitempty"`

	// memory configures the memory-api for this service group.
	// Required when mode is "managed".
	// +optional
	Memory *MemoryServiceConfig `json:"memory,omitempty"`

	// session configures the session-api for this service group.
	// Required when mode is "managed".
	// +optional
	Session *SessionServiceConfig `json:"session,omitempty"`

	// external specifies pre-existing endpoint URLs when mode is "external".
	// Required when mode is "external".
	// +optional
	External *ExternalEndpoints `json:"external,omitempty"`

	// privacyPolicyRef references a SessionPrivacyPolicy in this workspace's namespace
	// that applies to all agents in this service group. Agents may override this
	// with their own spec.privacyPolicyRef.
	// +optional
	PrivacyPolicyRef *corev1.LocalObjectReference `json:"privacyPolicyRef,omitempty"`

	// evalWorker opts this service group into the out-of-band eval-worker.
	// +optional
	EvalWorker *ServiceGroupEvalWorker `json:"evalWorker,omitempty"`

	// autoscaling is the default autoscaling policy for every AgentRuntime in
	// this service group. An agent that omits spec.runtime.autoscaling inherits
	// this policy whole; an agent that sets its own block fully owns autoscaling
	// and this default is ignored (explicit agent spec wins as a unit). When the
	// resolved policy requests type "keda" but KEDA is not installed in the
	// cluster, the agent surfaces an AutoscalingReady=False condition and stays
	// at static replicas rather than failing.
	// +optional
	Autoscaling *AutoscalingConfig `json:"autoscaling,omitempty"`
}

// ServiceGroupEvalWorker configures the per-service-group eval-worker.
//
// The operator runs at most one eval-worker per service group and creates it
// automatically when the group has a non-PromptKit, eval-enabled AgentRuntime.
// PromptKit agents self-evaluate lightweight evals inline and are excluded by
// default, so their llm_judge (long-running / external) evals — the only path
// that tags omnia_eval_* with the `variant` label rollout-analysis gates key
// on — never run out-of-band.
type ServiceGroupEvalWorker struct {
	// enabled forces creation of the eval-worker for this service group even
	// when all of its eval-enabled agents use the PromptKit framework. It has
	// no effect when the group has no eval-enabled agent (there is nothing to
	// evaluate). Non-PromptKit agents always get a worker when evals are
	// enabled, regardless of this flag.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// podOverrides customizes the per-service-group eval-worker Pod — most
	// commonly a ServiceAccount plus an `azure.workload.identity/use: "true"`
	// label so the worker's llm_judge can mint a federated token for a keyless
	// cloud judge provider (auth.type: workloadIdentity). Mirrors the
	// memory/session podOverrides on this service group. When set it takes
	// precedence over the legacy per-agent spec.evals.podOverrides for this
	// group's worker.
	// +optional
	PodOverrides *PodOverrides `json:"podOverrides,omitempty"`
}

// MemoryServiceConfig defines the configuration for a managed memory-api instance.
//
// +kubebuilder:validation:XValidation:rule="!has(self.redis) || [has(self.redis.existingSecret), has(self.redis.url) && size(self.redis.url) > 0, has(self.redis.host) && size(self.redis.host) > 0, has(self.redis.serviceRef)].exists_one(b, b)",message="memory.redis must use exactly one of existingSecret, url, host, or serviceRef"
type MemoryServiceConfig struct {
	// database configures the PostgreSQL database for this memory service.
	// +kubebuilder:validation:Required
	Database DatabaseConfig `json:"database"`

	// providerRef optionally references a Provider CRD used for embedding generation.
	// +optional
	ProviderRef *corev1.LocalObjectReference `json:"providerRef,omitempty"`

	// policyRef optionally references a MemoryPolicy that applies to
	// this service group. The same MemoryPolicy may be referenced by
	// many workspaces. When unset the memory-api falls back to the
	// baked-in legacy interval policy.
	// +optional
	PolicyRef *corev1.LocalObjectReference `json:"policyRef,omitempty"`

	// redis pins this workspace's memory-api cache + event publisher
	// to a specific Redis instance, overriding the operator-wide
	// default. Use for SaaS multi-tenancy where each customer's data
	// must live on physically separate Redis (data residency, blast-
	// radius isolation), or to point a single workspace at a more
	// durable Redis tier than the cluster default. Unset = inherit
	// the operator-wide --memory-redis-url passed by the chart.
	// +optional
	Redis *RedisConfig `json:"redis,omitempty"`

	// podOverrides customizes the managed memory-api Pod (SA, scheduling,
	// CSI secret-stores, etc.).
	// +optional
	PodOverrides *PodOverrides `json:"podOverrides,omitempty"`
}

// SessionServiceConfig defines the configuration for a managed session-api instance.
//
// +kubebuilder:validation:XValidation:rule="!has(self.redis) || [has(self.redis.existingSecret), has(self.redis.url) && size(self.redis.url) > 0, has(self.redis.host) && size(self.redis.host) > 0, has(self.redis.serviceRef)].exists_one(b, b)",message="session.redis must use exactly one of existingSecret, url, host, or serviceRef"
type SessionServiceConfig struct {
	// database configures the PostgreSQL database for this session service.
	// +kubebuilder:validation:Required
	Database DatabaseConfig `json:"database"`

	// policyRef optionally references a SessionRetentionPolicy that
	// applies to this service group. The same SessionRetentionPolicy
	// may be referenced by many workspaces. When unset the session-api
	// falls back to its baked-in defaults.
	// +optional
	PolicyRef *corev1.LocalObjectReference `json:"policyRef,omitempty"`

	// redis pins this workspace's session-api hot cache to a specific
	// Redis instance, overriding the operator-wide default. Use for
	// SaaS multi-tenancy where each customer's session data must live
	// on physically separate Redis (data residency, blast-radius
	// isolation), or per-workspace durability tier choices. Unset =
	// inherit the operator-wide --session-redis-url passed by the chart.
	// +optional
	Redis *RedisConfig `json:"redis,omitempty"`

	// podOverrides customizes the managed session-api Pod (SA, scheduling,
	// CSI secret-stores, etc.).
	// +optional
	PodOverrides *PodOverrides `json:"podOverrides,omitempty"`
}

// PrivacyServiceConfig configures the per-workspace managed privacy-api service.
type PrivacyServiceConfig struct {
	// database configures the PostgreSQL consent database for this workspace.
	// The Secret must have a key "POSTGRES_CONN".
	// +kubebuilder:validation:Required
	Database DatabaseConfig `json:"database"`

	// NOTE: privacy-api intentionally has no podOverrides field. Embedding the
	// full PodOverrides pod-spec schema a third time (session + memory already
	// carry it) pushed the bundled Workspace CRD past the 1 MiB Helm release
	// Secret limit, breaking `helm install` (caught by E2E, not by template/lint).
	// privacy-api is a simple new service; podOverrides can be added later only
	// alongside a chart change that stops bundling CRDs into the release Secret.
}

// DatabaseConfig holds the reference to a Secret containing database connection details.
type DatabaseConfig struct {
	// secretRef references a Secret containing the database connection string.
	// The Secret must have a key "POSTGRES_CONN" with a valid PostgreSQL connection string.
	// +kubebuilder:validation:Required
	SecretRef corev1.LocalObjectReference `json:"secretRef"`
}

// ExternalEndpoints holds user-supplied URLs for a service group using external mode.
type ExternalEndpoints struct {
	// sessionURL is the base URL of the external session-api (e.g., "https://session.example.com").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https?://`
	SessionURL string `json:"sessionURL"`

	// memoryURL is the base URL of the external memory-api (e.g., "https://memory.example.com").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https?://`
	MemoryURL string `json:"memoryURL"`
}

// ServiceGroupStatus tracks the observed state of a single service group.
type ServiceGroupStatus struct {
	// name is the name of the service group (matches spec.services[].name).
	Name string `json:"name,omitempty"`

	// sessionURL is the resolved URL of the session-api for this group.
	// +optional
	SessionURL string `json:"sessionURL,omitempty"`

	// memoryURL is the resolved URL of the memory-api for this group.
	// +optional
	MemoryURL string `json:"memoryURL,omitempty"`

	// ready indicates whether all components of this service group are operational.
	// +optional
	Ready bool `json:"ready,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Workspace{}, &WorkspaceList{})
}
