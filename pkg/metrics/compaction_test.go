/*
Copyright 2025.

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

package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestNewCompactionMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewCompactionMetricsWithRegistry(reg)
	if m == nil {
		t.Fatal("NewCompactionMetricsWithRegistry returned nil")
	}
	if m.RunDurationSeconds == nil {
		t.Error("RunDurationSeconds is nil")
	}
	if m.SessionsCompactedTotal == nil {
		t.Error("SessionsCompactedTotal is nil")
	}
	if m.BatchesProcessedTotal == nil {
		t.Error("BatchesProcessedTotal is nil")
	}
	if m.ErrorsTotal == nil {
		t.Error("ErrorsTotal is nil")
	}
	if m.LastRunTimestamp == nil {
		t.Error("LastRunTimestamp is nil")
	}
}

func TestNewCompactionMetrics_Promauto(t *testing.T) {
	m := NewCompactionMetrics()
	if m == nil {
		t.Fatal("NewCompactionMetrics returned nil")
	}
	if m.RunDurationSeconds == nil {
		t.Error("RunDurationSeconds is nil")
	}
	if m.SessionsCompactedTotal == nil {
		t.Error("SessionsCompactedTotal is nil")
	}
	if m.BatchesProcessedTotal == nil {
		t.Error("BatchesProcessedTotal is nil")
	}
	if m.ErrorsTotal == nil {
		t.Error("ErrorsTotal is nil")
	}
	if m.LastRunTimestamp == nil {
		t.Error("LastRunTimestamp is nil")
	}
}

func TestCompactionMetrics_RecordDuration(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewCompactionMetricsWithRegistry(reg)

	m.RecordDuration(5 * time.Second)

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range metrics {
		if mf.GetName() == "omnia_compaction_run_duration_seconds" {
			found = true
			hist := mf.GetMetric()[0].GetHistogram()
			if hist.GetSampleCount() != 1 {
				t.Errorf("Expected sample count 1, got %d", hist.GetSampleCount())
			}
		}
	}
	if !found {
		t.Error("omnia_compaction_run_duration_seconds metric not found")
	}
}

func TestCompactionMetrics_RecordSessionsCompacted(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewCompactionMetricsWithRegistry(reg)

	m.RecordSessionsCompacted(42)

	var metric dto.Metric
	if err := m.SessionsCompactedTotal.Write(&metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}
	if metric.GetCounter().GetValue() != 42 {
		t.Errorf("Expected 42, got %v", metric.GetCounter().GetValue())
	}
}

func TestCompactionMetrics_RecordBatchProcessed(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewCompactionMetricsWithRegistry(reg)

	m.RecordBatchProcessed()
	m.RecordBatchProcessed()

	var metric dto.Metric
	if err := m.BatchesProcessedTotal.Write(&metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}
	if metric.GetCounter().GetValue() != 2 {
		t.Errorf("Expected 2, got %v", metric.GetCounter().GetValue())
	}
}

func TestCompactionMetrics_RecordError(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewCompactionMetricsWithRegistry(reg)

	m.RecordError("write_parquet")
	m.RecordError("write_parquet")
	m.RecordError("delete_warm")

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range metrics {
		if mf.GetName() == "omnia_compaction_errors_total" {
			found = true
			if len(mf.GetMetric()) != 2 {
				t.Errorf("Expected 2 label sets, got %d", len(mf.GetMetric()))
			}
		}
	}
	if !found {
		t.Error("omnia_compaction_errors_total metric not found")
	}
}

func TestCompactionMetrics_RecordLastRun(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewCompactionMetricsWithRegistry(reg)

	before := float64(time.Now().Unix())
	m.RecordLastRun()

	var metric dto.Metric
	if err := m.LastRunTimestamp.Write(&metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}
	val := metric.GetGauge().GetValue()
	if val < before {
		t.Errorf("Expected timestamp >= %v, got %v", before, val)
	}
}
