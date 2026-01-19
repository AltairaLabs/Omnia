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

// ComputeQuotas defines compute resource quotas.
type ComputeQuotas struct {
	// requestsCPU is the total CPU requests allowed (e.g., "50").
	// +optional
	RequestsCPU string `json:"requests.cpu,omitempty"`

	// requestsMemory is the total memory requests allowed (e.g., "100Gi").
	// +optional
	RequestsMemory string `json:"requests.memory,omitempty"`

	// limitsCPU is the total CPU limits allowed (e.g., "100").
	// +optional
	LimitsCPU string `json:"limits.cpu,omitempty"`

	// limitsMemory is the total memory limits allowed (e.g., "200Gi").
	// +optional
	LimitsMemory string `json:"limits.memory,omitempty"`
}

// ObjectQuotas defines object count quotas.
type ObjectQuotas struct {
	// configmaps is the maximum number of ConfigMaps allowed.
	// +optional
	ConfigMaps *int32 `json:"configmaps,omitempty"`

	// secrets is the maximum number of Secrets allowed.
	// +optional
	Secrets *int32 `json:"secrets,omitempty"`

	// persistentvolumeclaims is the maximum number of PVCs allowed.
	// +optional
	PersistentVolumeClaims *int32 `json:"persistentvolumeclaims,omitempty"`
}

// ArenaQuotas defines Arena-specific quotas.
type ArenaQuotas struct {
	// maxConcurrentJobs is the maximum number of concurrent Arena jobs.
	// +optional
	MaxConcurrentJobs *int32 `json:"maxConcurrentJobs,omitempty"`

	// maxJobsPerDay is the maximum number of Arena jobs per day.
	// +optional
	MaxJobsPerDay *int32 `json:"maxJobsPerDay,omitempty"`

	// maxWorkersPerJob is the maximum number of workers per Arena job.
	// +optional
	MaxWorkersPerJob *int32 `json:"maxWorkersPerJob,omitempty"`
}

// AgentQuotas defines AgentRuntime-specific quotas.
type AgentQuotas struct {
	// maxAgentRuntimes is the maximum number of AgentRuntimes allowed.
	// +optional
	MaxAgentRuntimes *int32 `json:"maxAgentRuntimes,omitempty"`

	// maxReplicasPerAgent is the maximum replicas per AgentRuntime.
	// +optional
	MaxReplicasPerAgent *int32 `json:"maxReplicasPerAgent,omitempty"`
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

// WorkspaceQuotas defines resource quotas for a workspace.
type WorkspaceQuotas struct {
	// compute defines compute resource quotas.
	// +optional
	Compute *ComputeQuotas `json:"compute,omitempty"`

	// objects defines object count quotas.
	// +optional
	Objects *ObjectQuotas `json:"objects,omitempty"`

	// arena defines Arena-specific quotas.
	// +optional
	Arena *ArenaQuotas `json:"arena,omitempty"`

	// agents defines AgentRuntime-specific quotas.
	// +optional
	Agents *AgentQuotas `json:"agents,omitempty"`
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

	// quotas defines resource quotas for this workspace.
	// +optional
	Quotas *WorkspaceQuotas `json:"quotas,omitempty"`

	// costControls defines budget and cost control settings.
	// +optional
	CostControls *CostControls `json:"costControls,omitempty"`

	// networkPolicy defines network isolation settings for this workspace.
	// +optional
	NetworkPolicy *WorkspaceNetworkPolicy `json:"networkPolicy,omitempty"`
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

func init() {
	SchemeBuilder.Register(&Workspace{}, &WorkspaceList{})
}
