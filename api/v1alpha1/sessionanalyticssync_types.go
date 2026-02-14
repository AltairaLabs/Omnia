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

// AnalyticsProvider defines the type of analytics backend.
// +kubebuilder:validation:Enum=snowflake;bigquery;clickhouse
type AnalyticsProvider string

const (
	// AnalyticsProviderSnowflake indicates a Snowflake analytics backend.
	AnalyticsProviderSnowflake AnalyticsProvider = "snowflake"
	// AnalyticsProviderBigQuery indicates a BigQuery analytics backend.
	AnalyticsProviderBigQuery AnalyticsProvider = "bigquery"
	// AnalyticsProviderClickHouse indicates a ClickHouse analytics backend.
	AnalyticsProviderClickHouse AnalyticsProvider = "clickhouse"
)

// SyncMode defines how data is synced to the analytics backend.
// +kubebuilder:validation:Enum=full;incremental
type SyncMode string

const (
	// SyncModeFull syncs all data on every run.
	SyncModeFull SyncMode = "full"
	// SyncModeIncremental syncs only new or changed data since the last sync.
	SyncModeIncremental SyncMode = "incremental"
)

// SourceType defines the source tier to sync from.
// +kubebuilder:validation:Enum=cold_archive;warm_store
type SourceType string

const (
	// SourceTypeColdArchive syncs from the cold archive tier.
	SourceTypeColdArchive SourceType = "cold_archive"
	// SourceTypeWarmStore syncs from the warm store tier.
	SourceTypeWarmStore SourceType = "warm_store"
)

// SessionAnalyticsSyncPhase represents the current phase of the sync configuration.
// +kubebuilder:validation:Enum=Active;Error;Syncing
type SessionAnalyticsSyncPhase string

const (
	// SessionAnalyticsSyncPhaseActive indicates the sync configuration is valid and active.
	SessionAnalyticsSyncPhaseActive SessionAnalyticsSyncPhase = "Active"
	// SessionAnalyticsSyncPhaseError indicates the sync configuration has an error.
	SessionAnalyticsSyncPhaseError SessionAnalyticsSyncPhase = "Error"
	// SessionAnalyticsSyncPhaseSyncing indicates data is currently being synced.
	SessionAnalyticsSyncPhaseSyncing SessionAnalyticsSyncPhase = "Syncing"
)

// SyncStatusType represents the result of a sync operation.
// +kubebuilder:validation:Enum=Success;Failed;Running
type SyncStatusType string

const (
	// SyncStatusSuccess indicates the last sync completed successfully.
	SyncStatusSuccess SyncStatusType = "Success"
	// SyncStatusFailed indicates the last sync failed.
	SyncStatusFailed SyncStatusType = "Failed"
	// SyncStatusRunning indicates a sync is currently in progress.
	SyncStatusRunning SyncStatusType = "Running"
)

// SnowflakeConfig defines the configuration for a Snowflake analytics backend.
type SnowflakeConfig struct {
	// account is the Snowflake account identifier (e.g., "xy12345.us-east-1").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Account string `json:"account"`

	// database is the target Snowflake database.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Database string `json:"database"`

	// schema is the target Snowflake schema.
	// +kubebuilder:default="PUBLIC"
	// +optional
	Schema string `json:"schema,omitempty"`

	// warehouse is the Snowflake warehouse to use for loading.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Warehouse string `json:"warehouse"`

	// role is the Snowflake role to use for loading.
	// +optional
	Role string `json:"role,omitempty"`

	// secretRef references a Secret containing Snowflake credentials.
	// +kubebuilder:validation:Required
	SecretRef corev1.LocalObjectReference `json:"secretRef"`
}

// BigQueryConfig defines the configuration for a BigQuery analytics backend.
type BigQueryConfig struct {
	// projectID is the GCP project ID.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ProjectID string `json:"projectID"`

	// dataset is the BigQuery dataset name.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Dataset string `json:"dataset"`

	// location is the BigQuery dataset location (e.g., "US", "EU").
	// +kubebuilder:default="US"
	// +optional
	Location string `json:"location,omitempty"`

	// secretRef references a Secret containing GCP credentials.
	// +kubebuilder:validation:Required
	SecretRef corev1.LocalObjectReference `json:"secretRef"`
}

// ClickHouseAuth defines authentication for a ClickHouse backend.
type ClickHouseAuth struct {
	// secretRef references a Secret containing ClickHouse credentials.
	// +kubebuilder:validation:Required
	SecretRef corev1.LocalObjectReference `json:"secretRef"`
}

// ClickHouseConfig defines the configuration for a ClickHouse analytics backend.
type ClickHouseConfig struct {
	// hosts is the list of ClickHouse host:port addresses.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Hosts []string `json:"hosts"`

	// database is the target ClickHouse database.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Database string `json:"database"`

	// auth defines the authentication configuration for ClickHouse.
	// +kubebuilder:validation:Required
	Auth ClickHouseAuth `json:"auth"`
}

// SyncConfig defines the scheduling and behavior of sync operations.
type SyncConfig struct {
	// schedule is a cron expression for when to run the sync.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Schedule string `json:"schedule"`

	// mode defines whether to run a full or incremental sync.
	// +kubebuilder:default=incremental
	// +optional
	Mode SyncMode `json:"mode,omitempty"`

	// batchSize is the number of rows to process per batch.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=10000
	// +optional
	BatchSize int32 `json:"batchSize,omitempty"`

	// parallelism is the number of parallel sync workers.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=32
	// +kubebuilder:default=4
	// +optional
	Parallelism int32 `json:"parallelism,omitempty"`
}

// SourceConfig defines where to read session data from.
type SourceConfig struct {
	// type specifies which storage tier to sync from.
	// +kubebuilder:default=cold_archive
	// +optional
	Type SourceType `json:"type,omitempty"`
}

// TableMapping defines how a source table maps to a target table in the analytics backend.
type TableMapping struct {
	// target is the name of the destination table in the analytics backend.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Target string `json:"target"`

	// partitionBy is the column used for partitioning in the target table.
	// +optional
	PartitionBy string `json:"partitionBy,omitempty"`
}

// TablesConfig defines the table mappings for sync.
type TablesConfig struct {
	// sessions defines the mapping for the sessions table.
	// +optional
	Sessions *TableMapping `json:"sessions,omitempty"`

	// messages defines the mapping for the messages table.
	// +optional
	Messages *TableMapping `json:"messages,omitempty"`

	// artifacts defines the mapping for the artifacts table.
	// +optional
	Artifacts *TableMapping `json:"artifacts,omitempty"`
}

// SessionAnalyticsSyncSpec defines the desired state of SessionAnalyticsSync.
// +kubebuilder:validation:XValidation:rule="self.provider != 'snowflake' || has(self.snowflake)",message="snowflake configuration is required when provider is snowflake"
// +kubebuilder:validation:XValidation:rule="self.provider != 'bigquery' || has(self.bigquery)",message="bigquery configuration is required when provider is bigquery"
// +kubebuilder:validation:XValidation:rule="self.provider != 'clickhouse' || has(self.clickhouse)",message="clickhouse configuration is required when provider is clickhouse"
type SessionAnalyticsSyncSpec struct {
	// enabled specifies whether the analytics sync is active.
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// provider specifies the analytics backend to sync to.
	// +kubebuilder:validation:Required
	Provider AnalyticsProvider `json:"provider"`

	// snowflake defines the Snowflake backend configuration.
	// Required when provider is "snowflake".
	// +optional
	Snowflake *SnowflakeConfig `json:"snowflake,omitempty"`

	// bigquery defines the BigQuery backend configuration.
	// Required when provider is "bigquery".
	// +optional
	BigQuery *BigQueryConfig `json:"bigquery,omitempty"`

	// clickhouse defines the ClickHouse backend configuration.
	// Required when provider is "clickhouse".
	// +optional
	ClickHouse *ClickHouseConfig `json:"clickhouse,omitempty"`

	// sync defines the scheduling and behavior of sync operations.
	// +kubebuilder:validation:Required
	Sync SyncConfig `json:"sync"`

	// source defines where to read session data from.
	// +optional
	Source *SourceConfig `json:"source,omitempty"`

	// tables defines the table mappings for sync.
	// +optional
	Tables *TablesConfig `json:"tables,omitempty"`
}

// SessionAnalyticsSyncStatus defines the observed state of SessionAnalyticsSync.
type SessionAnalyticsSyncStatus struct {
	// phase represents the current lifecycle phase of the sync configuration.
	// +optional
	Phase SessionAnalyticsSyncPhase `json:"phase,omitempty"`

	// observedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// lastSyncAt is the timestamp of the last sync operation.
	// +optional
	LastSyncAt *metav1.Time `json:"lastSyncAt,omitempty"`

	// lastSyncStatus is the result of the last sync operation.
	// +optional
	LastSyncStatus SyncStatusType `json:"lastSyncStatus,omitempty"`

	// rowsSynced is the number of rows synced in the last operation.
	// +optional
	RowsSynced int64 `json:"rowsSynced,omitempty"`

	// nextSyncAt is the scheduled time for the next sync operation.
	// +optional
	NextSyncAt *metav1.Time `json:"nextSyncAt,omitempty"`

	// errors contains any errors from the last sync operation.
	// +optional
	Errors []string `json:"errors,omitempty"`

	// conditions represent the current state of the SessionAnalyticsSync resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Provider",type=string,JSONPath=`.spec.provider`
// +kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=`.spec.sync.schedule`
// +kubebuilder:printcolumn:name="Last Sync Status",type=string,JSONPath=`.status.lastSyncStatus`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SessionAnalyticsSync is the Schema for the sessionanalyticssyncs API.
// It defines configuration for automatic sync of session data to analytics backends.
type SessionAnalyticsSync struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of SessionAnalyticsSync
	// +required
	Spec SessionAnalyticsSyncSpec `json:"spec"`

	// status defines the observed state of SessionAnalyticsSync
	// +optional
	Status SessionAnalyticsSyncStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// SessionAnalyticsSyncList contains a list of SessionAnalyticsSync.
type SessionAnalyticsSyncList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []SessionAnalyticsSync `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SessionAnalyticsSync{}, &SessionAnalyticsSyncList{})
}
