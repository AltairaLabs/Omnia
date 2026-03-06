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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// RetentionMetrics holds Prometheus metrics for retention policy reconciliation.
type RetentionMetrics struct {
	// ReconcileErrorsTotal counts reconcile errors by error type.
	ReconcileErrorsTotal *prometheus.CounterVec
	// ActivePolicies is the current count of Active retention policies.
	ActivePolicies prometheus.Gauge
	// WorkspaceOverrides tracks the total number of workspace overrides.
	WorkspaceOverrides prometheus.Gauge
	// ConfigMapSyncErrors counts ConfigMap sync failures.
	ConfigMapSyncErrors prometheus.Counter
}

// NewRetentionMetrics creates and registers all Prometheus metrics for retention policy reconciliation.
func NewRetentionMetrics() *RetentionMetrics {
	return &RetentionMetrics{
		ReconcileErrorsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_retention_reconcile_errors_total",
			Help: "Total number of retention policy reconcile errors",
		}, []string{"error_type"}),

		ActivePolicies: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "omnia_retention_active_policies",
			Help: "Current number of active retention policies",
		}),

		WorkspaceOverrides: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "omnia_retention_workspace_overrides",
			Help: "Total number of workspace overrides for retention policies",
		}),

		ConfigMapSyncErrors: promauto.NewCounter(prometheus.CounterOpts{
			Name: "omnia_retention_configmap_sync_errors_total",
			Help: "Total number of ConfigMap sync errors for retention policies",
		}),
	}
}

// Initialize pre-registers retention metrics so they appear in /metrics output at startup.
func (m *RetentionMetrics) Initialize() {
	m.ActivePolicies.Set(0)
}

// RecordReconcileError increments the reconcile error counter.
func (m *RetentionMetrics) RecordReconcileError(_, errorType string) {
	m.ReconcileErrorsTotal.WithLabelValues(errorType).Inc()
}

// RecordConfigMapSyncError increments the ConfigMap sync error counter.
func (m *RetentionMetrics) RecordConfigMapSyncError(_ string) {
	m.ConfigMapSyncErrors.Inc()
}

// SetWorkspaceOverrides sets the workspace override count.
func (m *RetentionMetrics) SetWorkspaceOverrides(_ string, count int) {
	m.WorkspaceOverrides.Set(float64(count))
}

// newRetentionMetricsWithRegistry creates retention metrics with a custom registry for testing.
func newRetentionMetricsWithRegistry(reg *prometheus.Registry) *RetentionMetrics {
	reconcileErrorsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "omnia_retention_reconcile_errors_total",
		Help: "Total number of retention policy reconcile errors",
	}, []string{"error_type"})

	activePolicies := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "omnia_retention_active_policies",
		Help: "Current number of active retention policies",
	})

	workspaceOverrides := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "omnia_retention_workspace_overrides",
		Help: "Total number of workspace overrides for retention policies",
	})

	configMapSyncErrors := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "omnia_retention_configmap_sync_errors_total",
		Help: "Total number of ConfigMap sync errors for retention policies",
	})

	reg.MustRegister(reconcileErrorsTotal, activePolicies, workspaceOverrides, configMapSyncErrors)

	return &RetentionMetrics{
		ReconcileErrorsTotal: reconcileErrorsTotal,
		ActivePolicies:       activePolicies,
		WorkspaceOverrides:   workspaceOverrides,
		ConfigMapSyncErrors:  configMapSyncErrors,
	}
}
