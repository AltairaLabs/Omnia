/*
Copyright 2026.

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

package controller

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRolloutMetrics_AllNonNil(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewRolloutMetrics(reg)

	require.NotNil(t, m)
	assert.NotNil(t, m.Active)
	assert.NotNil(t, m.StepTransitions)
	assert.NotNil(t, m.Promotions)
	assert.NotNil(t, m.Rollbacks)
	assert.NotNil(t, m.StepDuration)
	assert.NotNil(t, m.TrafficWeight)
}

func TestRolloutMetrics_Recording(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewRolloutMetrics(reg)

	// Record some metrics — should not panic
	m.Active.WithLabelValues("default", "customer-support").Set(1)
	m.StepTransitions.WithLabelValues("default", "customer-support", "setWeight").Inc()
	m.Promotions.WithLabelValues("default", "customer-support").Inc()
	m.Rollbacks.WithLabelValues("default", "customer-support", "analysis-failed").Inc()
	m.StepDuration.WithLabelValues("default", "customer-support", "pause").Observe(60.0)

	m.TrafficWeight.WithLabelValues("default", "customer-support", "stable").Set(80)
	m.TrafficWeight.WithLabelValues("default", "customer-support", "canary").Set(20)

	// Gather and verify counts
	families, err := reg.Gather()
	require.NoError(t, err)

	metricNames := make(map[string]bool)
	for _, f := range families {
		metricNames[f.GetName()] = true
	}

	assert.True(t, metricNames[metricRolloutActive])
	assert.True(t, metricNames[metricRolloutStepTransitions])
	assert.True(t, metricNames[metricRolloutPromotions])
	assert.True(t, metricNames[metricRolloutRollbacks])
	assert.True(t, metricNames[metricRolloutStepDuration])
	assert.True(t, metricNames[metricRolloutTrafficWeight])
}

func TestRolloutMetrics_DoubleRegisterPanics(t *testing.T) {
	reg := prometheus.NewRegistry()
	_ = NewRolloutMetrics(reg)

	assert.Panics(t, func() {
		_ = NewRolloutMetrics(reg)
	})
}
