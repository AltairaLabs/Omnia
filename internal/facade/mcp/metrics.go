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

package mcp

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics owns the Prometheus collectors for the MCP facade. Constructed
// once per agent pod and shared across the transport handler and tool
// adapter.
//
// All metrics carry constant labels {agent, namespace} so a single
// Prometheus scrape distinguishes per-agent traffic without grafana
// needing to re-derive the labels.
type Metrics struct {
	requestsTotal        *prometheus.CounterVec
	requestDuration      *prometheus.HistogramVec
	toolInvocationsTotal *prometheus.CounterVec
}

// NewMetrics registers MCP collectors on prometheus.DefaultRegisterer.
// Use NewMetricsWithRegisterer in tests to avoid contention.
func NewMetrics(agent, namespace string) *Metrics {
	return NewMetricsWithRegisterer(agent, namespace, prometheus.DefaultRegisterer)
}

// NewMetricsWithRegisterer registers MCP collectors on the given
// Registerer. Tests pass a fresh prometheus.NewRegistry() so each test
// gets its own counter set.
func NewMetricsWithRegisterer(agent, namespace string, reg prometheus.Registerer) *Metrics {
	labels := prometheus.Labels{"agent": agent, "namespace": namespace}
	factory := promauto.With(reg)
	return &Metrics{
		requestsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name:        "omnia_mcp_requests_total",
			Help:        "Total MCP JSON-RPC requests handled, by method and status.",
			ConstLabels: labels,
		}, []string{"method", "status"}),
		requestDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "omnia_mcp_request_duration_seconds",
			Help:        "MCP request duration in seconds, by method.",
			ConstLabels: labels,
			Buckets:     prometheus.DefBuckets,
		}, []string{"method"}),
		toolInvocationsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name:        "omnia_mcp_tool_invocations_total",
			Help:        "Total tools/call invocations, by function and outcome.",
			ConstLabels: labels,
		}, []string{"function", "outcome"}),
	}
}

// RecordRequest counts one JSON-RPC method handled and records its
// duration. status is one of "ok" / "protocol_error" / "auth_error";
// tool-level outcomes are recorded separately via RecordToolInvocation.
func (m *Metrics) RecordRequest(method, status string, durationSeconds float64) {
	if m == nil {
		return
	}
	m.requestsTotal.WithLabelValues(method, status).Inc()
	m.requestDuration.WithLabelValues(method).Observe(durationSeconds)
}

// RecordToolInvocation counts one tools/call invocation by outcome.
// outcome matches facade.InvocationOutcome values ("ok",
// "input_invalid", "runtime_error", "output_invalid", etc.).
func (m *Metrics) RecordToolInvocation(function, outcome string) {
	if m == nil {
		return
	}
	m.toolInvocationsTotal.WithLabelValues(function, outcome).Inc()
}
