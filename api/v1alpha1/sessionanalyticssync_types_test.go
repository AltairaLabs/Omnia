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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestAnalyticsProviderConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant AnalyticsProvider
		expected string
	}{
		{
			name:     "Snowflake provider",
			constant: AnalyticsProviderSnowflake,
			expected: "snowflake",
		},
		{
			name:     "BigQuery provider",
			constant: AnalyticsProviderBigQuery,
			expected: "bigquery",
		},
		{
			name:     "ClickHouse provider",
			constant: AnalyticsProviderClickHouse,
			expected: "clickhouse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("AnalyticsProvider constant = %v, want %v", tt.constant, tt.expected)
			}
		})
	}
}

func TestSyncModeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant SyncMode
		expected string
	}{
		{
			name:     "Full sync mode",
			constant: SyncModeFull,
			expected: "full",
		},
		{
			name:     "Incremental sync mode",
			constant: SyncModeIncremental,
			expected: "incremental",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("SyncMode constant = %v, want %v", tt.constant, tt.expected)
			}
		})
	}
}

func TestSourceTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant SourceType
		expected string
	}{
		{
			name:     "Cold archive source",
			constant: SourceTypeColdArchive,
			expected: "cold_archive",
		},
		{
			name:     "Warm store source",
			constant: SourceTypeWarmStore,
			expected: "warm_store",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("SourceType constant = %v, want %v", tt.constant, tt.expected)
			}
		})
	}
}

func TestSessionAnalyticsSyncPhaseConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant SessionAnalyticsSyncPhase
		expected string
	}{
		{
			name:     "Active phase",
			constant: SessionAnalyticsSyncPhaseActive,
			expected: "Active",
		},
		{
			name:     "Error phase",
			constant: SessionAnalyticsSyncPhaseError,
			expected: "Error",
		},
		{
			name:     "Syncing phase",
			constant: SessionAnalyticsSyncPhaseSyncing,
			expected: "Syncing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("SessionAnalyticsSyncPhase constant = %v, want %v", tt.constant, tt.expected)
			}
		})
	}
}

func TestSyncStatusTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant SyncStatusType
		expected string
	}{
		{
			name:     "Success status",
			constant: SyncStatusSuccess,
			expected: "Success",
		},
		{
			name:     "Failed status",
			constant: SyncStatusFailed,
			expected: "Failed",
		},
		{
			name:     "Running status",
			constant: SyncStatusRunning,
			expected: "Running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.constant) != tt.expected {
				t.Errorf("SyncStatusType constant = %v, want %v", tt.constant, tt.expected)
			}
		})
	}
}

func TestSessionAnalyticsSyncCreationSnowflake(t *testing.T) {
	enabled := true
	sync := &SessionAnalyticsSync{
		ObjectMeta: metav1.ObjectMeta{
			Name: "snowflake-sync",
		},
		Spec: SessionAnalyticsSyncSpec{
			Enabled:  &enabled,
			Provider: AnalyticsProviderSnowflake,
			Snowflake: &SnowflakeConfig{
				Account:   "xy12345.us-east-1",
				Database:  "OMNIA_DB",
				Schema:    "PUBLIC",
				Warehouse: "COMPUTE_WH",
				Role:      "OMNIA_LOADER",
				SecretRef: corev1.LocalObjectReference{
					Name: "snowflake-credentials",
				},
			},
			Sync: SyncConfig{
				Schedule:    "0 3 * * *",
				Mode:        SyncModeIncremental,
				BatchSize:   10000,
				Parallelism: 4,
			},
			Source: &SourceConfig{
				Type: SourceTypeColdArchive,
			},
			Tables: &TablesConfig{
				Sessions: &TableMapping{
					Target:      "SESSIONS",
					PartitionBy: "created_at",
				},
				Messages: &TableMapping{
					Target:      "MESSAGES",
					PartitionBy: "created_at",
				},
				Artifacts: &TableMapping{
					Target: "ARTIFACTS",
				},
			},
		},
	}

	if sync.Name != "snowflake-sync" {
		t.Errorf("Name = %v, want snowflake-sync", sync.Name)
	}

	if *sync.Spec.Enabled != true {
		t.Errorf("Enabled = %v, want true", *sync.Spec.Enabled)
	}

	if sync.Spec.Provider != AnalyticsProviderSnowflake {
		t.Errorf("Provider = %v, want snowflake", sync.Spec.Provider)
	}

	if sync.Spec.Snowflake == nil {
		t.Fatal("Snowflake config should not be nil")
	}

	if sync.Spec.Snowflake.Account != "xy12345.us-east-1" {
		t.Errorf("Snowflake.Account = %v, want xy12345.us-east-1", sync.Spec.Snowflake.Account)
	}

	if sync.Spec.Snowflake.Database != "OMNIA_DB" {
		t.Errorf("Snowflake.Database = %v, want OMNIA_DB", sync.Spec.Snowflake.Database)
	}

	if sync.Spec.Snowflake.Schema != "PUBLIC" {
		t.Errorf("Snowflake.Schema = %v, want PUBLIC", sync.Spec.Snowflake.Schema)
	}

	if sync.Spec.Snowflake.Warehouse != "COMPUTE_WH" {
		t.Errorf("Snowflake.Warehouse = %v, want COMPUTE_WH", sync.Spec.Snowflake.Warehouse)
	}

	if sync.Spec.Snowflake.Role != "OMNIA_LOADER" {
		t.Errorf("Snowflake.Role = %v, want OMNIA_LOADER", sync.Spec.Snowflake.Role)
	}

	if sync.Spec.Snowflake.SecretRef.Name != "snowflake-credentials" {
		t.Errorf("Snowflake.SecretRef.Name = %v, want snowflake-credentials", sync.Spec.Snowflake.SecretRef.Name)
	}

	if sync.Spec.Sync.Schedule != "0 3 * * *" {
		t.Errorf("Sync.Schedule = %v, want '0 3 * * *'", sync.Spec.Sync.Schedule)
	}

	if sync.Spec.Sync.Mode != SyncModeIncremental {
		t.Errorf("Sync.Mode = %v, want incremental", sync.Spec.Sync.Mode)
	}

	if sync.Spec.Sync.BatchSize != 10000 {
		t.Errorf("Sync.BatchSize = %v, want 10000", sync.Spec.Sync.BatchSize)
	}

	if sync.Spec.Sync.Parallelism != 4 {
		t.Errorf("Sync.Parallelism = %v, want 4", sync.Spec.Sync.Parallelism)
	}
}

func TestSessionAnalyticsSyncCreationBigQuery(t *testing.T) {
	enabled := true
	sync := &SessionAnalyticsSync{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bigquery-sync",
		},
		Spec: SessionAnalyticsSyncSpec{
			Enabled:  &enabled,
			Provider: AnalyticsProviderBigQuery,
			BigQuery: &BigQueryConfig{
				ProjectID: "my-gcp-project",
				Dataset:   "omnia_sessions",
				Location:  "US",
				SecretRef: corev1.LocalObjectReference{
					Name: "gcp-credentials",
				},
			},
			Sync: SyncConfig{
				Schedule: "0 3 * * *",
			},
		},
	}

	if sync.Spec.Provider != AnalyticsProviderBigQuery {
		t.Errorf("Provider = %v, want bigquery", sync.Spec.Provider)
	}

	if sync.Spec.BigQuery == nil {
		t.Fatal("BigQuery config should not be nil")
	}

	if sync.Spec.BigQuery.ProjectID != "my-gcp-project" {
		t.Errorf("BigQuery.ProjectID = %v, want my-gcp-project", sync.Spec.BigQuery.ProjectID)
	}

	if sync.Spec.BigQuery.Dataset != "omnia_sessions" {
		t.Errorf("BigQuery.Dataset = %v, want omnia_sessions", sync.Spec.BigQuery.Dataset)
	}

	if sync.Spec.BigQuery.Location != "US" {
		t.Errorf("BigQuery.Location = %v, want US", sync.Spec.BigQuery.Location)
	}

	if sync.Spec.BigQuery.SecretRef.Name != "gcp-credentials" {
		t.Errorf("BigQuery.SecretRef.Name = %v, want gcp-credentials", sync.Spec.BigQuery.SecretRef.Name)
	}
}

func TestSessionAnalyticsSyncCreationClickHouse(t *testing.T) {
	sync := &SessionAnalyticsSync{
		ObjectMeta: metav1.ObjectMeta{
			Name: "clickhouse-sync",
		},
		Spec: SessionAnalyticsSyncSpec{
			Provider: AnalyticsProviderClickHouse,
			ClickHouse: &ClickHouseConfig{
				Hosts:    []string{"clickhouse-0:9000", "clickhouse-1:9000"},
				Database: "omnia",
				Auth: ClickHouseAuth{
					SecretRef: corev1.LocalObjectReference{
						Name: "clickhouse-credentials",
					},
				},
			},
			Sync: SyncConfig{
				Schedule: "0 3 * * *",
			},
		},
	}

	if sync.Spec.Provider != AnalyticsProviderClickHouse {
		t.Errorf("Provider = %v, want clickhouse", sync.Spec.Provider)
	}

	if sync.Spec.ClickHouse == nil {
		t.Fatal("ClickHouse config should not be nil")
	}

	if len(sync.Spec.ClickHouse.Hosts) != 2 {
		t.Fatalf("ClickHouse.Hosts length = %d, want 2", len(sync.Spec.ClickHouse.Hosts))
	}

	if sync.Spec.ClickHouse.Hosts[0] != "clickhouse-0:9000" {
		t.Errorf("ClickHouse.Hosts[0] = %v, want clickhouse-0:9000", sync.Spec.ClickHouse.Hosts[0])
	}

	if sync.Spec.ClickHouse.Database != "omnia" {
		t.Errorf("ClickHouse.Database = %v, want omnia", sync.Spec.ClickHouse.Database)
	}

	if sync.Spec.ClickHouse.Auth.SecretRef.Name != "clickhouse-credentials" {
		t.Errorf("ClickHouse.Auth.SecretRef.Name = %v, want clickhouse-credentials", sync.Spec.ClickHouse.Auth.SecretRef.Name)
	}
}

func TestSessionAnalyticsSyncSourceConfig(t *testing.T) {
	tests := []struct {
		name       string
		sourceType SourceType
	}{
		{
			name:       "Cold archive source",
			sourceType: SourceTypeColdArchive,
		},
		{
			name:       "Warm store source",
			sourceType: SourceTypeWarmStore,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sync := &SessionAnalyticsSync{
				Spec: SessionAnalyticsSyncSpec{
					Provider: AnalyticsProviderSnowflake,
					Sync: SyncConfig{
						Schedule: "0 3 * * *",
					},
					Source: &SourceConfig{
						Type: tt.sourceType,
					},
				},
			}

			if sync.Spec.Source.Type != tt.sourceType {
				t.Errorf("Source.Type = %v, want %v", sync.Spec.Source.Type, tt.sourceType)
			}
		})
	}
}

func TestSessionAnalyticsSyncTableMappings(t *testing.T) {
	sync := &SessionAnalyticsSync{
		Spec: SessionAnalyticsSyncSpec{
			Provider: AnalyticsProviderSnowflake,
			Sync: SyncConfig{
				Schedule: "0 3 * * *",
			},
			Tables: &TablesConfig{
				Sessions: &TableMapping{
					Target:      "SESSIONS",
					PartitionBy: "created_at",
				},
				Messages: &TableMapping{
					Target:      "MESSAGES",
					PartitionBy: "created_at",
				},
				Artifacts: &TableMapping{
					Target: "ARTIFACTS",
				},
			},
		},
	}

	if sync.Spec.Tables.Sessions.Target != "SESSIONS" {
		t.Errorf("Tables.Sessions.Target = %v, want SESSIONS", sync.Spec.Tables.Sessions.Target)
	}

	if sync.Spec.Tables.Sessions.PartitionBy != "created_at" {
		t.Errorf("Tables.Sessions.PartitionBy = %v, want created_at", sync.Spec.Tables.Sessions.PartitionBy)
	}

	if sync.Spec.Tables.Messages.Target != "MESSAGES" {
		t.Errorf("Tables.Messages.Target = %v, want MESSAGES", sync.Spec.Tables.Messages.Target)
	}

	if sync.Spec.Tables.Artifacts.Target != "ARTIFACTS" {
		t.Errorf("Tables.Artifacts.Target = %v, want ARTIFACTS", sync.Spec.Tables.Artifacts.Target)
	}

	if sync.Spec.Tables.Artifacts.PartitionBy != "" {
		t.Errorf("Tables.Artifacts.PartitionBy = %v, want empty string", sync.Spec.Tables.Artifacts.PartitionBy)
	}
}

func TestSessionAnalyticsSyncStatus(t *testing.T) {
	now := metav1.Now()
	nextSync := metav1.NewTime(now.Add(24 * 60 * 60 * 1e9)) // +24h

	sync := &SessionAnalyticsSync{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sync",
		},
		Status: SessionAnalyticsSyncStatus{
			Phase:              SessionAnalyticsSyncPhaseActive,
			ObservedGeneration: 3,
			LastSyncAt:         &now,
			LastSyncStatus:     SyncStatusSuccess,
			RowsSynced:         50000,
			NextSyncAt:         &nextSync,
			Errors:             []string{},
			Conditions: []metav1.Condition{
				{
					Type:               "SyncReady",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "ConfigValid",
					Message:            "Sync configuration is valid",
				},
			},
		},
	}

	if sync.Status.Phase != SessionAnalyticsSyncPhaseActive {
		t.Errorf("Status.Phase = %v, want Active", sync.Status.Phase)
	}

	if sync.Status.ObservedGeneration != 3 {
		t.Errorf("Status.ObservedGeneration = %v, want 3", sync.Status.ObservedGeneration)
	}

	if sync.Status.LastSyncAt == nil {
		t.Fatal("Status.LastSyncAt should not be nil")
	}

	if sync.Status.LastSyncStatus != SyncStatusSuccess {
		t.Errorf("Status.LastSyncStatus = %v, want Success", sync.Status.LastSyncStatus)
	}

	if sync.Status.RowsSynced != 50000 {
		t.Errorf("Status.RowsSynced = %v, want 50000", sync.Status.RowsSynced)
	}

	if sync.Status.NextSyncAt == nil {
		t.Fatal("Status.NextSyncAt should not be nil")
	}

	if len(sync.Status.Errors) != 0 {
		t.Errorf("Status.Errors length = %v, want 0", len(sync.Status.Errors))
	}

	if len(sync.Status.Conditions) != 1 {
		t.Fatalf("len(Status.Conditions) = %v, want 1", len(sync.Status.Conditions))
	}

	if sync.Status.Conditions[0].Type != "SyncReady" {
		t.Errorf("Condition type = %v, want SyncReady", sync.Status.Conditions[0].Type)
	}
}

func TestSessionAnalyticsSyncStatusWithErrors(t *testing.T) {
	sync := &SessionAnalyticsSync{
		ObjectMeta: metav1.ObjectMeta{
			Name: "failed-sync",
		},
		Status: SessionAnalyticsSyncStatus{
			Phase:          SessionAnalyticsSyncPhaseError,
			LastSyncStatus: SyncStatusFailed,
			Errors: []string{
				"connection timeout to Snowflake",
				"authentication failed",
			},
		},
	}

	if sync.Status.Phase != SessionAnalyticsSyncPhaseError {
		t.Errorf("Status.Phase = %v, want Error", sync.Status.Phase)
	}

	if sync.Status.LastSyncStatus != SyncStatusFailed {
		t.Errorf("Status.LastSyncStatus = %v, want Failed", sync.Status.LastSyncStatus)
	}

	if len(sync.Status.Errors) != 2 {
		t.Fatalf("Status.Errors length = %v, want 2", len(sync.Status.Errors))
	}

	if sync.Status.Errors[0] != "connection timeout to Snowflake" {
		t.Errorf("Status.Errors[0] = %v, want 'connection timeout to Snowflake'", sync.Status.Errors[0])
	}
}

const testModifiedSyncName = "modified"

func TestSessionAnalyticsSyncDeepCopy(t *testing.T) {
	enabled := true
	now := metav1.Now()

	original := &SessionAnalyticsSync{
		ObjectMeta: metav1.ObjectMeta{
			Name: "original",
		},
		Spec: SessionAnalyticsSyncSpec{
			Enabled:  &enabled,
			Provider: AnalyticsProviderSnowflake,
			Snowflake: &SnowflakeConfig{
				Account:   "xy12345.us-east-1",
				Database:  "OMNIA_DB",
				Warehouse: "COMPUTE_WH",
				SecretRef: corev1.LocalObjectReference{
					Name: "snowflake-credentials",
				},
			},
			Sync: SyncConfig{
				Schedule:    "0 3 * * *",
				Mode:        SyncModeIncremental,
				BatchSize:   10000,
				Parallelism: 4,
			},
			Source: &SourceConfig{
				Type: SourceTypeColdArchive,
			},
			Tables: &TablesConfig{
				Sessions: &TableMapping{
					Target:      "SESSIONS",
					PartitionBy: "created_at",
				},
			},
		},
		Status: SessionAnalyticsSyncStatus{
			Phase:              SessionAnalyticsSyncPhaseActive,
			ObservedGeneration: 1,
			LastSyncAt:         &now,
			LastSyncStatus:     SyncStatusSuccess,
			RowsSynced:         50000,
			Errors:             []string{},
			Conditions: []metav1.Condition{
				{
					Type:   "SyncReady",
					Status: metav1.ConditionTrue,
				},
			},
		},
	}

	copied := original.DeepCopy()

	if copied == original {
		t.Error("DeepCopy should return a new object, not the same pointer")
	}

	if copied.Name != original.Name {
		t.Errorf("DeepCopy().Name = %v, want %v", copied.Name, original.Name)
	}

	if copied.Status.Phase != original.Status.Phase {
		t.Errorf("DeepCopy().Status.Phase = %v, want %v", copied.Status.Phase, original.Status.Phase)
	}

	// Verify nested pointer fields are deep copied
	if copied.Spec.Snowflake == original.Spec.Snowflake {
		t.Error("DeepCopy should create new Snowflake pointer")
	}

	if copied.Spec.Enabled == original.Spec.Enabled {
		t.Error("DeepCopy should create new Enabled pointer")
	}

	if copied.Spec.Source == original.Spec.Source {
		t.Error("DeepCopy should create new Source pointer")
	}

	if copied.Spec.Tables == original.Spec.Tables {
		t.Error("DeepCopy should create new Tables pointer")
	}

	if copied.Spec.Tables.Sessions == original.Spec.Tables.Sessions {
		t.Error("DeepCopy should create new Sessions TableMapping pointer")
	}

	// Modify the copy and verify original is unchanged
	copied.Name = testModifiedSyncName
	if original.Name == testModifiedSyncName {
		t.Error("Modifying copy should not affect original")
	}
}

func TestSessionAnalyticsSyncListDeepCopy(t *testing.T) {
	original := &SessionAnalyticsSyncList{
		Items: []SessionAnalyticsSync{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sync1",
				},
				Spec: SessionAnalyticsSyncSpec{
					Provider: AnalyticsProviderSnowflake,
					Sync: SyncConfig{
						Schedule: "0 3 * * *",
					},
				},
			},
		},
	}

	copied := original.DeepCopy()

	if copied == original {
		t.Error("DeepCopy should return a new object")
	}

	if len(copied.Items) != len(original.Items) {
		t.Errorf("DeepCopy().Items length = %v, want %v", len(copied.Items), len(original.Items))
	}

	copied.Items[0].Name = testModifiedSyncName
	if original.Items[0].Name == testModifiedSyncName {
		t.Error("Modifying copy should not affect original")
	}
}

func TestSessionAnalyticsSyncTypeRegistration(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	gvk := GroupVersion.WithKind("SessionAnalyticsSync")
	if !scheme.Recognizes(gvk) {
		t.Errorf("scheme does not recognize %v", gvk)
	}

	gvkList := GroupVersion.WithKind("SessionAnalyticsSyncList")
	if !scheme.Recognizes(gvkList) {
		t.Errorf("scheme does not recognize %v", gvkList)
	}
}

func TestSessionAnalyticsSyncDeepCopyObject(t *testing.T) {
	sync := &SessionAnalyticsSync{}
	syncList := &SessionAnalyticsSyncList{}

	// These should not panic if types are registered correctly
	_ = sync.DeepCopyObject()
	_ = syncList.DeepCopyObject()
}

func TestSessionAnalyticsSyncNilOptionalFields(t *testing.T) {
	sync := &SessionAnalyticsSync{
		ObjectMeta: metav1.ObjectMeta{
			Name: "minimal-sync",
		},
		Spec: SessionAnalyticsSyncSpec{
			Provider: AnalyticsProviderSnowflake,
			Sync: SyncConfig{
				Schedule: "0 3 * * *",
			},
		},
	}

	if sync.Spec.Enabled != nil {
		t.Errorf("Enabled should be nil when not set, got %v", sync.Spec.Enabled)
	}

	if sync.Spec.BigQuery != nil {
		t.Error("BigQuery should be nil when not set")
	}

	if sync.Spec.ClickHouse != nil {
		t.Error("ClickHouse should be nil when not set")
	}

	if sync.Spec.Source != nil {
		t.Error("Source should be nil when not set")
	}

	if sync.Spec.Tables != nil {
		t.Error("Tables should be nil when not set")
	}

	if sync.Status.LastSyncAt != nil {
		t.Error("Status.LastSyncAt should be nil when not set")
	}

	if sync.Status.NextSyncAt != nil {
		t.Error("Status.NextSyncAt should be nil when not set")
	}

	if sync.Status.Errors != nil {
		t.Error("Status.Errors should be nil when not set")
	}
}

func TestSessionAnalyticsSyncSyncingPhase(t *testing.T) {
	sync := &SessionAnalyticsSync{
		ObjectMeta: metav1.ObjectMeta{
			Name: "syncing",
		},
		Status: SessionAnalyticsSyncStatus{
			Phase:          SessionAnalyticsSyncPhaseSyncing,
			LastSyncStatus: SyncStatusRunning,
		},
	}

	if sync.Status.Phase != SessionAnalyticsSyncPhaseSyncing {
		t.Errorf("Status.Phase = %v, want Syncing", sync.Status.Phase)
	}

	if sync.Status.LastSyncStatus != SyncStatusRunning {
		t.Errorf("Status.LastSyncStatus = %v, want Running", sync.Status.LastSyncStatus)
	}
}
