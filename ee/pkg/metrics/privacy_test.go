/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPrivacyPolicyMetrics(t *testing.T) {
	m := NewPrivacyPolicyMetrics()

	require.NotNil(t, m)
	require.NotNil(t, m.ReconcileErrorsTotal)
	require.NotNil(t, m.ActivePolicies)
	require.NotNil(t, m.EffectivePolicyComputations)
	require.NotNil(t, m.ConfigMapSyncErrors)
	require.NotNil(t, m.InheritanceDepth)

	// Exercise Initialize on the production constructor
	m.Initialize()
}

func TestNewPrivacyPolicyMetricsWithRegistry(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewPrivacyPolicyMetricsWithRegistry(reg)

	require.NotNil(t, m)
	require.NotNil(t, m.ReconcileErrorsTotal)
	require.NotNil(t, m.ActivePolicies)
	require.NotNil(t, m.EffectivePolicyComputations)
	require.NotNil(t, m.ConfigMapSyncErrors)
	require.NotNil(t, m.InheritanceDepth)
}

func TestInitialize(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewPrivacyPolicyMetricsWithRegistry(reg)

	m.Initialize()

	// Verify all three levels are initialized to 0
	for _, level := range []string{"global", "workspace", "agent"} {
		gauge, err := m.ActivePolicies.GetMetricWithLabelValues(level)
		require.NoError(t, err)
		metric := &dto.Metric{}
		err = gauge.Write(metric)
		require.NoError(t, err)
		assert.Equal(t, float64(0), metric.GetGauge().GetValue(),
			"expected %s level to be initialized to 0", level)
	}
}

func TestRecordReconcileError(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewPrivacyPolicyMetricsWithRegistry(reg)

	m.RecordReconcileError("test-policy", "parent_lookup")
	m.RecordReconcileError("test-policy", "parent_lookup")

	counter, err := m.ReconcileErrorsTotal.GetMetricWithLabelValues(
		"test-policy", "parent_lookup")
	require.NoError(t, err)
	metric := &dto.Metric{}
	err = counter.Write(metric)
	require.NoError(t, err)
	assert.Equal(t, float64(2), metric.GetCounter().GetValue())
}

func TestRecordEffectivePolicyComputation(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewPrivacyPolicyMetricsWithRegistry(reg)

	m.RecordEffectivePolicyComputation("my-policy")

	counter, err := m.EffectivePolicyComputations.GetMetricWithLabelValues("my-policy")
	require.NoError(t, err)
	metric := &dto.Metric{}
	err = counter.Write(metric)
	require.NoError(t, err)
	assert.Equal(t, float64(1), metric.GetCounter().GetValue())
}

func TestRecordConfigMapSyncError(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewPrivacyPolicyMetricsWithRegistry(reg)

	m.RecordConfigMapSyncError("sync-policy")

	counter, err := m.ConfigMapSyncErrors.GetMetricWithLabelValues("sync-policy")
	require.NoError(t, err)
	metric := &dto.Metric{}
	err = counter.Write(metric)
	require.NoError(t, err)
	assert.Equal(t, float64(1), metric.GetCounter().GetValue())
}

func TestSetActivePolicies(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewPrivacyPolicyMetricsWithRegistry(reg)

	m.SetActivePolicies("global", 3)
	m.SetActivePolicies("workspace", 5)

	gauge, err := m.ActivePolicies.GetMetricWithLabelValues("global")
	require.NoError(t, err)
	metric := &dto.Metric{}
	err = gauge.Write(metric)
	require.NoError(t, err)
	assert.Equal(t, float64(3), metric.GetGauge().GetValue())

	gauge, err = m.ActivePolicies.GetMetricWithLabelValues("workspace")
	require.NoError(t, err)
	metric = &dto.Metric{}
	err = gauge.Write(metric)
	require.NoError(t, err)
	assert.Equal(t, float64(5), metric.GetGauge().GetValue())
}

func TestSetInheritanceDepth(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewPrivacyPolicyMetricsWithRegistry(reg)

	m.SetInheritanceDepth("deep-policy", 3)

	gauge, err := m.InheritanceDepth.GetMetricWithLabelValues("deep-policy")
	require.NoError(t, err)
	metric := &dto.Metric{}
	err = gauge.Write(metric)
	require.NoError(t, err)
	assert.Equal(t, float64(3), metric.GetGauge().GetValue())
}
