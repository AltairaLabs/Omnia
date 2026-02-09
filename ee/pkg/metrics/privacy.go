/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// PrivacyPolicyMetrics holds Prometheus metrics for SessionPrivacyPolicy reconciliation.
type PrivacyPolicyMetrics struct {
	// ReconcileErrorsTotal counts reconcile errors by policy name and error type.
	ReconcileErrorsTotal *prometheus.CounterVec
	// ActivePolicies tracks the current count of active privacy policies by level.
	ActivePolicies *prometheus.GaugeVec
	// EffectivePolicyComputations counts effective policy computations by policy name.
	EffectivePolicyComputations *prometheus.CounterVec
	// ConfigMapSyncErrors counts ConfigMap sync failures by policy name.
	ConfigMapSyncErrors *prometheus.CounterVec
	// InheritanceDepth tracks the inheritance chain depth by policy name.
	InheritanceDepth *prometheus.GaugeVec
}

// NewPrivacyPolicyMetrics creates and registers all Prometheus metrics for privacy policy reconciliation.
func NewPrivacyPolicyMetrics() *PrivacyPolicyMetrics {
	return &PrivacyPolicyMetrics{
		ReconcileErrorsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_privacy_policy_reconcile_errors_total",
			Help: "Total number of privacy policy reconcile errors",
		}, []string{"policy_name", "error_type"}),

		ActivePolicies: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "omnia_privacy_policy_active_policies",
			Help: "Current number of active privacy policies by level",
		}, []string{"level"}),

		EffectivePolicyComputations: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_privacy_policy_effective_computations_total",
			Help: "Total number of effective policy computations",
		}, []string{"policy_name"}),

		ConfigMapSyncErrors: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_privacy_policy_configmap_sync_errors_total",
			Help: "Total number of ConfigMap sync errors for privacy policies",
		}, []string{"policy_name"}),

		InheritanceDepth: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "omnia_privacy_policy_inheritance_depth",
			Help: "Inheritance chain depth for privacy policies",
		}, []string{"policy_name"}),
	}
}

// Initialize pre-registers privacy policy metrics so they appear in /metrics output at startup.
func (m *PrivacyPolicyMetrics) Initialize() {
	m.ActivePolicies.WithLabelValues("global").Set(0)
	m.ActivePolicies.WithLabelValues("workspace").Set(0)
	m.ActivePolicies.WithLabelValues("agent").Set(0)
}

// RecordReconcileError increments the reconcile error counter.
func (m *PrivacyPolicyMetrics) RecordReconcileError(policyName, errorType string) {
	m.ReconcileErrorsTotal.WithLabelValues(policyName, errorType).Inc()
}

// RecordEffectivePolicyComputation increments the effective policy computation counter.
func (m *PrivacyPolicyMetrics) RecordEffectivePolicyComputation(policyName string) {
	m.EffectivePolicyComputations.WithLabelValues(policyName).Inc()
}

// RecordConfigMapSyncError increments the ConfigMap sync error counter.
func (m *PrivacyPolicyMetrics) RecordConfigMapSyncError(policyName string) {
	m.ConfigMapSyncErrors.WithLabelValues(policyName).Inc()
}

// SetActivePolicies sets the active policy count for a level.
func (m *PrivacyPolicyMetrics) SetActivePolicies(level string, count int) {
	m.ActivePolicies.WithLabelValues(level).Set(float64(count))
}

// SetInheritanceDepth sets the inheritance chain depth for a policy.
func (m *PrivacyPolicyMetrics) SetInheritanceDepth(policyName string, depth int) {
	m.InheritanceDepth.WithLabelValues(policyName).Set(float64(depth))
}

// NewPrivacyPolicyMetricsWithRegistry creates privacy policy metrics with a custom registry for testing.
func NewPrivacyPolicyMetricsWithRegistry(reg *prometheus.Registry) *PrivacyPolicyMetrics {
	reconcileErrorsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "omnia_privacy_policy_reconcile_errors_total",
		Help: "Total number of privacy policy reconcile errors",
	}, []string{"policy_name", "error_type"})

	activePolicies := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "omnia_privacy_policy_active_policies",
		Help: "Current number of active privacy policies by level",
	}, []string{"level"})

	effectivePolicyComputations := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "omnia_privacy_policy_effective_computations_total",
		Help: "Total number of effective policy computations",
	}, []string{"policy_name"})

	configMapSyncErrors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "omnia_privacy_policy_configmap_sync_errors_total",
		Help: "Total number of ConfigMap sync errors for privacy policies",
	}, []string{"policy_name"})

	inheritanceDepth := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "omnia_privacy_policy_inheritance_depth",
		Help: "Inheritance chain depth for privacy policies",
	}, []string{"policy_name"})

	reg.MustRegister(
		reconcileErrorsTotal, activePolicies,
		effectivePolicyComputations, configMapSyncErrors, inheritanceDepth,
	)

	return &PrivacyPolicyMetrics{
		ReconcileErrorsTotal:        reconcileErrorsTotal,
		ActivePolicies:              activePolicies,
		EffectivePolicyComputations: effectivePolicyComputations,
		ConfigMapSyncErrors:         configMapSyncErrors,
		InheritanceDepth:            inheritanceDepth,
	}
}
