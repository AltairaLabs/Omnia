/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package api

import "github.com/prometheus/client_golang/prometheus"

// SuppressionMetrics groups the memory-write suppression Prometheus collectors.
// Operators alert on sudden spikes ("did a deploy break consent capture?").
type SuppressionMetrics struct {
	WritesSuppressed *prometheus.CounterVec
}

// NewSuppressionMetrics builds the collector set. Caller is responsible for
// registering them on a registry.
func NewSuppressionMetrics() *SuppressionMetrics {
	return &SuppressionMetrics{
		WritesSuppressed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_memory_writes_suppressed_total",
			Help: "Memory writes dropped before storage. Labels: layer (per-message|session|persistent|unknown), category, reason (no-grant|opt-out).",
		}, []string{"layer", "category", "reason"}),
	}
}

// Register registers the collectors on the given registry.
func (m *SuppressionMetrics) Register(reg prometheus.Registerer) error {
	return reg.Register(m.WritesSuppressed)
}

// RecordSuppression increments the suppression counter for the given dimensions.
// Sanitises empty values to "unknown" so dashboards don't end up with empty-
// label time series.
func (m *SuppressionMetrics) RecordSuppression(layer, category, reason string) {
	if layer == "" {
		layer = "unknown"
	}
	if category == "" {
		category = "unknown"
	}
	if reason == "" {
		reason = "unknown"
	}
	m.WritesSuppressed.WithLabelValues(layer, category, reason).Inc()
}
