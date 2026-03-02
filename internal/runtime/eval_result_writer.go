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

package runtime

import (
	"context"

	"github.com/AltairaLabs/PromptKit/runtime/evals"
	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/pkg/metrics"
)

// PrometheusResultWriter implements evals.ResultWriter and records eval results
// to proper Prometheus CounterVec/GaugeVec/HistogramVec metrics with dimensional labels.
type PrometheusResultWriter struct {
	metrics metrics.EvalMetricsRecorder
	defs    map[string]*evals.EvalDef
	log     logr.Logger
}

// NewPrometheusResultWriter creates a new PrometheusResultWriter.
func NewPrometheusResultWriter(
	m metrics.EvalMetricsRecorder,
	defs []evals.EvalDef,
	log logr.Logger,
) *PrometheusResultWriter {
	defMap := make(map[string]*evals.EvalDef, len(defs))
	for i := range defs {
		defMap[defs[i].ID] = &defs[i]
	}
	return &PrometheusResultWriter{
		metrics: m,
		defs:    defMap,
		log:     log,
	}
}

// WriteResults records each eval result to Prometheus metrics.
func (w *PrometheusResultWriter) WriteResults(_ context.Context, results []evals.EvalResult) error {
	w.log.V(1).Info("writing eval results to prometheus", "resultCount", len(results))
	for i := range results {
		r := &results[i]

		trigger := "unknown"
		if def, ok := w.defs[r.EvalID]; ok {
			trigger = string(def.Trigger)
		} else {
			w.log.V(1).Info("eval def not found for result",
				"evalID", r.EvalID)
		}

		w.metrics.RecordEval(metrics.EvalRecordMetrics{
			EvalID:      r.EvalID,
			EvalType:    r.Type,
			Trigger:     trigger,
			Passed:      r.Passed,
			Score:       r.Score,
			DurationSec: float64(r.DurationMs) / 1000.0,
			Skipped:     r.Skipped,
			HasError:    r.Error != "",
		})
	}
	return nil
}
