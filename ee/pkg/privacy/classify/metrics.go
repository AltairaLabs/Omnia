/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package classify

import "github.com/prometheus/client_golang/prometheus"

// Metrics groups the consent-classifier Prometheus collectors.
// Construct via NewMetrics + register on the Prometheus registry once.
type Metrics struct {
	Overrides     *prometheus.CounterVec
	Filled        *prometheus.CounterVec
	CategoryTotal *prometheus.CounterVec
	Errors        *prometheus.CounterVec
}

// NewMetrics builds the collector set. Caller is responsible for
// registering them on a registry.
func NewMetrics() *Metrics {
	return &Metrics{
		Overrides: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_memory_classify_overrides_total",
			Help: "Memory consent categories upgraded by the validator. Labels: from, to, source.",
		}, []string{"from", "to", "source"}),
		Filled: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_memory_classify_filled_total",
			Help: "Memory consent categories filled in by the validator (caller had none). Labels: category, source.",
		}, []string{"category", "source"}),
		CategoryTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_memory_classify_category_total",
			Help: "Distribution of consent categories at write time. Labels: category (\"null\" for unset), source.",
		}, []string{"category", "source"}),
		Errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "omnia_memory_classify_errors_total",
			Help: "Errors observed by the consent classifier. Labels: reason.",
		}, []string{"reason"}),
	}
}

// Register registers all collectors on the given registry. Returns the
// first registration error encountered.
func (m *Metrics) Register(reg prometheus.Registerer) error {
	collectors := []prometheus.Collector{m.Overrides, m.Filled, m.CategoryTotal, m.Errors}
	for _, c := range collectors {
		if err := reg.Register(c); err != nil {
			return err
		}
	}
	return nil
}

// RecordResult updates the metrics for a single Validator.Apply outcome.
// claimed is the caller's category (empty when none was supplied).
func (m *Metrics) RecordResult(claimed string, res Result) {
	switch {
	case res.Overridden:
		m.Overrides.WithLabelValues(string(res.From), string(res.Category), res.Source).Inc()
	case claimed == "" && res.Category != "":
		m.Filled.WithLabelValues(string(res.Category), res.Source).Inc()
	}
	cat := string(res.Category)
	if cat == "" {
		cat = "null"
	}
	source := res.Source
	if source == "" {
		source = "caller"
	}
	m.CategoryTotal.WithLabelValues(cat, source).Inc()
}
