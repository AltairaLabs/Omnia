/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package api

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestSuppressionMetrics_RegisterAndRecord(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewSuppressionMetrics()
	if err := m.Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	m.RecordSuppression("session", "memory:health", "no-grant")
	got := testutil.ToFloat64(
		m.WritesSuppressed.WithLabelValues("session", "memory:health", "no-grant"),
	)
	if got != 1 {
		t.Errorf("counter = %v, want 1", got)
	}
}

func TestSuppressionMetrics_BlankFieldsBecomeUnknown(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewSuppressionMetrics()
	if err := m.Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	m.RecordSuppression("", "", "")
	got := testutil.ToFloat64(
		m.WritesSuppressed.WithLabelValues("unknown", "unknown", "unknown"),
	)
	if got != 1 {
		t.Errorf("counter = %v, want 1", got)
	}
}
