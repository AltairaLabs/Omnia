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

func TestNewAuditMetrics(t *testing.T) {
	m := NewAuditMetrics()
	require.NotNil(t, m)
	require.NotNil(t, m.EventsTotal)
	require.NotNil(t, m.WriteErrors)
	require.NotNil(t, m.WriteDuration)
	require.NotNil(t, m.BufferDrops)
	require.NotNil(t, m.QueriesTotal)
	require.NotNil(t, m.QueryDuration)
}

func TestNewAuditMetricsWithRegistry(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewAuditMetricsWithRegistry(reg)
	require.NotNil(t, m)
	require.NotNil(t, m.EventsTotal)
	require.NotNil(t, m.WriteErrors)
	require.NotNil(t, m.WriteDuration)
	require.NotNil(t, m.BufferDrops)
	require.NotNil(t, m.QueriesTotal)
	require.NotNil(t, m.QueryDuration)
}

func TestAuditMetrics_EventsTotal(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewAuditMetricsWithRegistry(reg)

	m.EventsTotal.WithLabelValues("session_accessed").Inc()
	m.EventsTotal.WithLabelValues("session_accessed").Inc()
	m.EventsTotal.WithLabelValues("session_searched").Inc()

	counter, err := m.EventsTotal.GetMetricWithLabelValues("session_accessed")
	require.NoError(t, err)
	metric := &dto.Metric{}
	require.NoError(t, counter.Write(metric))
	assert.Equal(t, float64(2), metric.GetCounter().GetValue())
}

func TestAuditMetrics_WriteErrors(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewAuditMetricsWithRegistry(reg)

	m.WriteErrors.WithLabelValues("session_accessed").Inc()

	counter, err := m.WriteErrors.GetMetricWithLabelValues("session_accessed")
	require.NoError(t, err)
	metric := &dto.Metric{}
	require.NoError(t, counter.Write(metric))
	assert.Equal(t, float64(1), metric.GetCounter().GetValue())
}

func TestAuditMetrics_BufferDrops(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewAuditMetricsWithRegistry(reg)

	m.BufferDrops.WithLabelValues("session_accessed").Inc()

	counter, err := m.BufferDrops.GetMetricWithLabelValues("session_accessed")
	require.NoError(t, err)
	metric := &dto.Metric{}
	require.NoError(t, counter.Write(metric))
	assert.Equal(t, float64(1), metric.GetCounter().GetValue())
}

func TestAuditMetrics_QueriesTotal(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewAuditMetricsWithRegistry(reg)

	m.QueriesTotal.Inc()
	m.QueriesTotal.Inc()

	metric := &dto.Metric{}
	require.NoError(t, m.QueriesTotal.Write(metric))
	assert.Equal(t, float64(2), metric.GetCounter().GetValue())
}

func TestAuditMetrics_WriteDuration(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewAuditMetricsWithRegistry(reg)

	m.WriteDuration.WithLabelValues("session_accessed").Observe(0.5)

	observer, err := m.WriteDuration.GetMetricWithLabelValues("session_accessed")
	require.NoError(t, err)
	metric := &dto.Metric{}
	require.NoError(t, observer.(prometheus.Metric).Write(metric))
	assert.Equal(t, uint64(1), metric.GetHistogram().GetSampleCount())
}

func TestAuditMetrics_QueryDuration(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewAuditMetricsWithRegistry(reg)

	m.QueryDuration.Observe(0.1)

	metric := &dto.Metric{}
	require.NoError(t, m.QueryDuration.Write(metric))
	assert.Equal(t, uint64(1), metric.GetHistogram().GetSampleCount())
}
