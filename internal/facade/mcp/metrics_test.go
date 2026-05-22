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
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetrics_RecordRequest(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetricsWithRegisterer("agent", "ns", reg)

	m.RecordRequest("tools/call", "ok", 0.012)
	m.RecordRequest("tools/call", "ok", 0.025)
	m.RecordRequest("tools/call", "protocol_error", 0.001)

	if got := promtest.ToFloat64(m.requestsTotal.WithLabelValues("tools/call", "ok")); got != 2 {
		t.Errorf("ok counter: got %v want 2", got)
	}
	if got := promtest.ToFloat64(m.requestsTotal.WithLabelValues("tools/call", "protocol_error")); got != 1 {
		t.Errorf("protocol_error counter: got %v want 1", got)
	}
}

func TestMetrics_RecordToolInvocation(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetricsWithRegisterer("agent", "ns", reg)

	m.RecordToolInvocation("echo", "ok")
	m.RecordToolInvocation("echo", "input_invalid")
	m.RecordToolInvocation("echo", "ok")

	if got := promtest.ToFloat64(m.toolInvocationsTotal.WithLabelValues("echo", "ok")); got != 2 {
		t.Errorf("ok counter: got %v want 2", got)
	}
	if got := promtest.ToFloat64(m.toolInvocationsTotal.WithLabelValues("echo", "input_invalid")); got != 1 {
		t.Errorf("input_invalid counter: got %v want 1", got)
	}
}

func TestMetrics_NilSafeRecord(t *testing.T) {
	// Both Record methods must accept a nil *Metrics so callers don't
	// need to nil-check at every call site.
	var m *Metrics
	m.RecordRequest("tools/list", "ok", 0.001)
	m.RecordToolInvocation("echo", "ok")
}
