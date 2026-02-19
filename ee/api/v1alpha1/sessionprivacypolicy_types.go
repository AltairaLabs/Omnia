/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// PolicyLevel represents the scope at which a privacy policy applies.
// +kubebuilder:validation:Enum=global;workspace;agent
type PolicyLevel string

const (
	// PolicyLevelGlobal applies the policy across the entire cluster.
	PolicyLevelGlobal PolicyLevel = "global"
	// PolicyLevelWorkspace applies the policy to a specific workspace.
	PolicyLevelWorkspace PolicyLevel = "workspace"
	// PolicyLevelAgent applies the policy to a specific agent.
	PolicyLevelAgent PolicyLevel = "agent"
)

// SessionPrivacyPolicyPhase represents the current phase of the policy.
// +kubebuilder:validation:Enum=Active;Error
type SessionPrivacyPolicyPhase string

const (
	// SessionPrivacyPolicyPhaseActive indicates the policy is valid and active.
	SessionPrivacyPolicyPhaseActive SessionPrivacyPolicyPhase = "Active"
	// SessionPrivacyPolicyPhaseError indicates the policy has a configuration error.
	SessionPrivacyPolicyPhaseError SessionPrivacyPolicyPhase = "Error"
)

// RedactionStrategy represents the method used to redact PII data.
// +kubebuilder:validation:Enum=replace;hash;mask
type RedactionStrategy string

const (
	// RedactionStrategyReplace swaps PII with a token like [REDACTED_SSN].
	RedactionStrategyReplace RedactionStrategy = "replace"
	// RedactionStrategyHash replaces PII with a deterministic SHA-256 truncated hash.
	RedactionStrategyHash RedactionStrategy = "hash"
	// RedactionStrategyMask preserves the last 4 characters, masking the rest with *.
	RedactionStrategyMask RedactionStrategy = "mask"
)

// KMSProvider represents a supported key management service provider.
// +kubebuilder:validation:Enum="aws-kms";"azure-keyvault";"gcp-kms";"vault"
type KMSProvider string

const (
	// KMSProviderAWSKMS uses AWS Key Management Service.
	KMSProviderAWSKMS KMSProvider = "aws-kms"
	// KMSProviderAzureKeyVault uses Azure Key Vault.
	KMSProviderAzureKeyVault KMSProvider = "azure-keyvault"
	// KMSProviderGCPKMS uses Google Cloud KMS.
	KMSProviderGCPKMS KMSProvider = "gcp-kms"
	// KMSProviderVault uses HashiCorp Vault.
	KMSProviderVault KMSProvider = "vault"
)

// PIIConfig configures PII (Personally Identifiable Information) handling.
type PIIConfig struct {
	// redact enables automatic PII redaction in session data.
	// +optional
	Redact bool `json:"redact,omitempty"`

	// encrypt enables PII encryption instead of or in addition to redaction.
	// +optional
	Encrypt bool `json:"encrypt,omitempty"`

	// patterns specifies which PII patterns to detect.
	// Built-in patterns: ssn, credit_card, phone_number, email, ip_address.
	// Custom regex patterns can be specified with the "custom:" prefix (e.g., "custom:^[A-Z]{2}\\d{6}$").
	// +optional
	Patterns []string `json:"patterns,omitempty"`

	// strategy specifies how PII is redacted. Options: replace (default), hash, mask.
	// +optional
	Strategy RedactionStrategy `json:"strategy,omitempty"`
}

// RecordingConfig configures what session data is recorded.
type RecordingConfig struct {
	// enabled specifies whether session recording is active.
	Enabled bool `json:"enabled"`

	// facadeData enables recording of facade-layer session data (summaries, metadata).
	// +optional
	FacadeData bool `json:"facadeData,omitempty"`

	// richData enables recording of full session content (messages, tool calls, artifacts).
	// +optional
	RichData bool `json:"richData,omitempty"`

	// pii configures PII handling within recorded session data.
	// +optional
	PII *PIIConfig `json:"pii,omitempty"`
}

// PrivacyRetentionTierConfig defines retention duration for a specific data tier.
type PrivacyRetentionTierConfig struct {
	// warmDays is the number of days to retain data in the warm store.
	// +kubebuilder:validation:Minimum=0
	// +optional
	WarmDays *int32 `json:"warmDays,omitempty"`

	// coldDays is the number of days to retain data in the cold archive.
	// +kubebuilder:validation:Minimum=0
	// +optional
	ColdDays *int32 `json:"coldDays,omitempty"`
}

// PrivacyRetentionConfig defines retention overrides specific to privacy policy.
type PrivacyRetentionConfig struct {
	// facade configures retention for facade-layer data.
	// +optional
	Facade *PrivacyRetentionTierConfig `json:"facade,omitempty"`

	// richData configures retention for full session content.
	// +optional
	RichData *PrivacyRetentionTierConfig `json:"richData,omitempty"`
}

// UserOptOutConfig configures user opt-out and data deletion capabilities.
type UserOptOutConfig struct {
	// enabled specifies whether users can opt out of session recording.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// honorDeleteRequests specifies whether user data deletion requests are honored.
	// +optional
	HonorDeleteRequests bool `json:"honorDeleteRequests,omitempty"`

	// deleteWithinDays is the maximum number of days to fulfill a deletion request.
	// +kubebuilder:validation:Minimum=1
	// +optional
	DeleteWithinDays *int32 `json:"deleteWithinDays,omitempty"`
}

// EncryptionConfig configures encryption for session data at rest.
// +kubebuilder:validation:XValidation:rule="!self.enabled || has(self.kmsProvider)",message="kmsProvider is required when encryption is enabled"
// +kubebuilder:validation:XValidation:rule="!self.enabled || has(self.keyID)",message="keyID is required when encryption is enabled"
type EncryptionConfig struct {
	// enabled specifies whether encryption is active.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// kmsProvider specifies the key management service to use.
	// Required when enabled is true.
	// +optional
	KMSProvider KMSProvider `json:"kmsProvider,omitempty"`

	// keyID is the identifier of the encryption key within the KMS provider.
	// Required when enabled is true.
	// +optional
	KeyID string `json:"keyID,omitempty"`

	// secretRef references a Secret containing encryption credentials.
	// +optional
	SecretRef *corev1alpha1.LocalObjectReference `json:"secretRef,omitempty"`

	// keyRotation configures automatic key rotation.
	// +optional
	KeyRotation *KeyRotationConfig `json:"keyRotation,omitempty"`
}

// KeyRotationConfig configures automatic key rotation.
type KeyRotationConfig struct {
	// enabled specifies whether automatic key rotation is active.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// schedule is a cron expression for automatic rotation (e.g. "0 0 1 * *" for monthly).
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// reEncryptExisting specifies whether existing data should be re-encrypted after rotation.
	// +optional
	ReEncryptExisting bool `json:"reEncryptExisting,omitempty"`

	// batchSize is the number of messages to re-encrypt per batch. Default 100, max 1000.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1000
	// +optional
	BatchSize *int32 `json:"batchSize,omitempty"`
}

// AuditLogConfig configures audit logging for privacy-related operations.
type AuditLogConfig struct {
	// enabled specifies whether audit logging is active.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// retentionDays is the number of days to retain audit log entries.
	// +kubebuilder:validation:Minimum=1
	// +optional
	RetentionDays *int32 `json:"retentionDays,omitempty"`
}

// SessionPrivacyPolicySpec defines the desired state of SessionPrivacyPolicy.
// +kubebuilder:validation:XValidation:rule="self.level != 'workspace' || has(self.workspaceRef)",message="workspaceRef is required when level is 'workspace'"
// +kubebuilder:validation:XValidation:rule="self.level != 'agent' || has(self.agentRef)",message="agentRef is required when level is 'agent'"
// +kubebuilder:validation:XValidation:rule="self.level != 'global' || !has(self.workspaceRef)",message="workspaceRef must not be set when level is 'global'"
// +kubebuilder:validation:XValidation:rule="self.level != 'global' || !has(self.agentRef)",message="agentRef must not be set when level is 'global'"
type SessionPrivacyPolicySpec struct {
	// level defines the scope at which this policy applies.
	// +kubebuilder:validation:Required
	Level PolicyLevel `json:"level"`

	// workspaceRef references the Workspace this policy applies to.
	// Required when level is "workspace".
	// +optional
	WorkspaceRef *corev1alpha1.LocalObjectReference `json:"workspaceRef,omitempty"`

	// agentRef references the AgentRuntime this policy applies to.
	// Required when level is "agent".
	// +optional
	AgentRef *corev1alpha1.NamespacedObjectReference `json:"agentRef,omitempty"`

	// recording configures what session data is recorded.
	// +kubebuilder:validation:Required
	Recording RecordingConfig `json:"recording"`

	// retention configures privacy-specific retention overrides.
	// +optional
	Retention *PrivacyRetentionConfig `json:"retention,omitempty"`

	// userOptOut configures user opt-out and data deletion capabilities.
	// +optional
	UserOptOut *UserOptOutConfig `json:"userOptOut,omitempty"`

	// encryption configures encryption for session data at rest.
	// +optional
	Encryption *EncryptionConfig `json:"encryption,omitempty"`

	// auditLog configures audit logging for privacy-related operations.
	// +optional
	AuditLog *AuditLogConfig `json:"auditLog,omitempty"`
}

// KeyRotationStatus reports the current state of key rotation.
type KeyRotationStatus struct {
	// lastRotatedAt is the timestamp of the last successful key rotation.
	// +optional
	LastRotatedAt *metav1.Time `json:"lastRotatedAt,omitempty"`

	// currentKeyVersion is the version of the key currently in use for encryption.
	// +optional
	CurrentKeyVersion string `json:"currentKeyVersion,omitempty"`

	// reEncryptionProgress tracks the progress of re-encrypting existing data.
	// +optional
	ReEncryptionProgress *ReEncryptionProgress `json:"reEncryptionProgress,omitempty"`
}

// ReEncryptionProgress tracks the progress of a re-encryption operation.
type ReEncryptionProgress struct {
	// status is the current state of re-encryption: Pending, InProgress, Completed, or Failed.
	// +optional
	Status string `json:"status,omitempty"`

	// messagesProcessed is the total number of messages re-encrypted so far.
	// +optional
	MessagesProcessed int64 `json:"messagesProcessed,omitempty"`

	// startedAt is when the re-encryption operation began.
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// completedAt is when the re-encryption operation finished.
	// +optional
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`
}

// SessionPrivacyPolicyStatus defines the observed state of SessionPrivacyPolicy.
type SessionPrivacyPolicyStatus struct {
	// phase represents the current lifecycle phase of the policy.
	// +optional
	Phase SessionPrivacyPolicyPhase `json:"phase,omitempty"`

	// observedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// conditions represent the current state of the SessionPrivacyPolicy resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// keyRotation reports the current state of key rotation.
	// +optional
	KeyRotation *KeyRotationStatus `json:"keyRotation,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Level",type=string,JSONPath=`.spec.level`
// +kubebuilder:printcolumn:name="Recording",type=boolean,JSONPath=`.spec.recording.enabled`
// +kubebuilder:printcolumn:name="PII Redact",type=boolean,JSONPath=`.spec.recording.pii.redact`
// +kubebuilder:printcolumn:name="Encryption",type=boolean,JSONPath=`.spec.encryption.enabled`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SessionPrivacyPolicy is the Schema for the sessionprivacypolicies API.
// It defines privacy rules for session data including PII handling, encryption,
// retention overrides, user opt-out, and audit logging.
type SessionPrivacyPolicy struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired state of SessionPrivacyPolicy
	// +required
	Spec SessionPrivacyPolicySpec `json:"spec"`

	// status defines the observed state of SessionPrivacyPolicy
	// +optional
	Status SessionPrivacyPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SessionPrivacyPolicyList contains a list of SessionPrivacyPolicy.
type SessionPrivacyPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SessionPrivacyPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SessionPrivacyPolicy{}, &SessionPrivacyPolicyList{})
}
