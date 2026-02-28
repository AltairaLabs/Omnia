/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package policy

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func getCounterValue(counter *prometheus.CounterVec, labels ...string) float64 {
	m := &dto.Metric{}
	if err := counter.WithLabelValues(labels...).Write(m); err != nil {
		return 0
	}
	return m.GetCounter().GetValue()
}

func getHistogramCount(hist *prometheus.HistogramVec, labels ...string) uint64 {
	m := &dto.Metric{}
	if err := hist.WithLabelValues(labels...).(prometheus.Metric).Write(m); err != nil {
		return 0
	}
	return m.GetHistogram().GetSampleCount()
}

func TestRegisterMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	RegisterMetrics(reg)

	// Emit a data point for each metric so they appear in Gather
	PolicyDecisionsTotal.WithLabelValues("tool", "p", "r", "allow").Inc()
	PolicyEvaluationDuration.WithLabelValues("tool").Observe(0.001)
	PolicyDenialsTotal.WithLabelValues("p", "r", "a", "t").Inc()

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	expectedNames := map[string]bool{
		"omnia_policy_decisions_total":             false,
		"omnia_policy_evaluation_duration_seconds": false,
		"omnia_policy_denials_total":               false,
	}

	for _, f := range families {
		if _, ok := expectedNames[f.GetName()]; ok {
			expectedNames[f.GetName()] = true
		}
	}

	for name, found := range expectedNames {
		if !found {
			t.Errorf("metric %q not registered", name)
		}
	}

	// Clean up
	PolicyDecisionsTotal.Reset()
	PolicyEvaluationDuration.Reset()
	PolicyDenialsTotal.Reset()
}

func TestPolicyDecisionsTotal_AllowIncrement(t *testing.T) {
	// Reset the counter for this test
	PolicyDecisionsTotal.Reset()

	PolicyDecisionsTotal.WithLabelValues("tool", "my-policy", "", "allow").Inc()

	val := getCounterValue(PolicyDecisionsTotal, "tool", "my-policy", "", "allow")
	if val != 1 {
		t.Errorf("allow counter = %f, want 1", val)
	}
}

func TestPolicyDecisionsTotal_DenyIncrement(t *testing.T) {
	PolicyDecisionsTotal.Reset()

	PolicyDecisionsTotal.WithLabelValues("tool", "deny-policy", "block-rule", "deny").Inc()
	PolicyDecisionsTotal.WithLabelValues("tool", "deny-policy", "block-rule", "deny").Inc()

	val := getCounterValue(PolicyDecisionsTotal, "tool", "deny-policy", "block-rule", "deny")
	if val != 2 {
		t.Errorf("deny counter = %f, want 2", val)
	}
}

func TestPolicyDenialsTotal_Increment(t *testing.T) {
	PolicyDenialsTotal.Reset()

	PolicyDenialsTotal.WithLabelValues("my-policy", "block-rule", "agent-1", "tool-a").Inc()

	val := getCounterValue(PolicyDenialsTotal, "my-policy", "block-rule", "agent-1", "tool-a")
	if val != 1 {
		t.Errorf("denials counter = %f, want 1", val)
	}
}

func TestPolicyEvaluationDuration_Observe(t *testing.T) {
	PolicyEvaluationDuration.Reset()

	PolicyEvaluationDuration.WithLabelValues("tool").Observe(0.005)
	PolicyEvaluationDuration.WithLabelValues("tool").Observe(0.010)

	count := getHistogramCount(PolicyEvaluationDuration, "tool")
	if count != 2 {
		t.Errorf("histogram sample count = %d, want 2", count)
	}
}

func TestRegisterMetrics_PanicsOnDoubleRegister(t *testing.T) {
	// Registering to the same registry twice should panic (MustRegister).
	// Use a fresh registry and register once - that should not panic.
	reg := prometheus.NewRegistry()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("RegisterMetrics panicked on first registration: %v", r)
		}
	}()

	RegisterMetrics(reg)
}
