/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package classify

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/altairalabs/omnia/ee/pkg/privacy"
)

func TestMetrics_RegisterAndRecord(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics()
	if err := m.Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Override case
	m.RecordResult("memory:preferences", Result{
		Category:   privacy.ConsentMemoryHealth,
		Overridden: true,
		From:       privacy.ConsentMemoryPreferences,
		Source:     SourceEmbedding,
	})
	got := testutil.ToFloat64(
		m.Overrides.WithLabelValues("memory:preferences", "memory:health", "embedding"),
	)
	if got != 1 {
		t.Errorf("overrides = %v, want 1", got)
	}

	// Fill case
	m.RecordResult("", Result{
		Category: privacy.ConsentMemoryIdentity,
		Source:   SourceRegex,
	})
	if got := testutil.ToFloat64(m.Filled.WithLabelValues("memory:identity", "regex")); got != 1 {
		t.Errorf("filled = %v, want 1", got)
	}

	// Caller-claim-stands case
	m.RecordResult("memory:preferences", Result{
		Category: privacy.ConsentMemoryPreferences,
	})
	if got := testutil.ToFloat64(m.CategoryTotal.WithLabelValues("memory:preferences", "caller")); got != 1 {
		t.Errorf("category total caller = %v, want 1", got)
	}

	// Null case
	m.RecordResult("", Result{})
	if got := testutil.ToFloat64(m.CategoryTotal.WithLabelValues("null", "caller")); got != 1 {
		t.Errorf("category total null = %v, want 1", got)
	}
}
