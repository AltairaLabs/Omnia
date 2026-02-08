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

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestNewRetentionMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newRetentionMetricsWithRegistry(reg)
	if m == nil {
		t.Fatal("NewRetentionMetrics returned nil")
	}
	if m.ReconcileErrorsTotal == nil {
		t.Error("ReconcileErrorsTotal is nil")
	}
	if m.ActivePolicies == nil {
		t.Error("ActivePolicies is nil")
	}
	if m.WorkspaceOverrides == nil {
		t.Error("WorkspaceOverrides is nil")
	}
	if m.ConfigMapSyncErrors == nil {
		t.Error("ConfigMapSyncErrors is nil")
	}
}

func TestNewRetentionMetrics_Promauto(t *testing.T) {
	m := NewRetentionMetrics()
	if m == nil {
		t.Fatal("NewRetentionMetrics returned nil")
	}
	if m.ReconcileErrorsTotal == nil {
		t.Error("ReconcileErrorsTotal is nil")
	}
	if m.ActivePolicies == nil {
		t.Error("ActivePolicies is nil")
	}
	if m.WorkspaceOverrides == nil {
		t.Error("WorkspaceOverrides is nil")
	}
	if m.ConfigMapSyncErrors == nil {
		t.Error("ConfigMapSyncErrors is nil")
	}
}

func TestRetentionMetrics_Initialize(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newRetentionMetricsWithRegistry(reg)
	m.Initialize()

	// Verify ActivePolicies gauge is 0 after initialize
	var metric dto.Metric
	if err := m.ActivePolicies.Write(&metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}
	if metric.GetGauge().GetValue() != 0 {
		t.Errorf("Expected ActivePolicies to be 0, got %v", metric.GetGauge().GetValue())
	}
}

func TestRetentionMetrics_RecordReconcileError(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newRetentionMetricsWithRegistry(reg)

	m.RecordReconcileError("test-policy", "validation")
	m.RecordReconcileError("test-policy", "validation")

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range metrics {
		if mf.GetName() == "omnia_retention_reconcile_errors_total" {
			found = true
			for _, m := range mf.GetMetric() {
				if m.GetCounter().GetValue() != 2 {
					t.Errorf("Expected counter value 2, got %v", m.GetCounter().GetValue())
				}
			}
		}
	}
	if !found {
		t.Error("omnia_retention_reconcile_errors_total metric not found")
	}
}

func TestRetentionMetrics_RecordConfigMapSyncError(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newRetentionMetricsWithRegistry(reg)

	m.RecordConfigMapSyncError("test-policy")

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range metrics {
		if mf.GetName() == "omnia_retention_configmap_sync_errors_total" {
			found = true
			for _, m := range mf.GetMetric() {
				if m.GetCounter().GetValue() != 1 {
					t.Errorf("Expected counter value 1, got %v", m.GetCounter().GetValue())
				}
			}
		}
	}
	if !found {
		t.Error("omnia_retention_configmap_sync_errors_total metric not found")
	}
}

func TestRetentionMetrics_SetWorkspaceOverrides(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newRetentionMetricsWithRegistry(reg)

	m.SetWorkspaceOverrides("test-policy", 5)

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range metrics {
		if mf.GetName() == "omnia_retention_workspace_overrides" {
			found = true
			for _, m := range mf.GetMetric() {
				if m.GetGauge().GetValue() != 5 {
					t.Errorf("Expected gauge value 5, got %v", m.GetGauge().GetValue())
				}
			}
		}
	}
	if !found {
		t.Error("omnia_retention_workspace_overrides metric not found")
	}
}
