/*
Copyright 2026 Altaira Labs.

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

package promptkit

import (
	"fmt"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/altairalabs/omnia/internal/schema"
)

// okBody is the response body for a passing health/readiness probe.
const okBody = "ok"

// packReadyError returns nil when the pack at packPath is present and passes
// schema validation, or an error describing why the runtime cannot serve it. The
// readiness probe calls this on every check, so a pod whose mounted pack is
// invalid — including a broken pack rolled onto a live agent — drops out of the
// Service rather than accepting conversations that fail at open-time (#1299). The
// schema validator uses an embedded schema, so a Validate error is a definitive
// invalid-pack result, not a transient network failure.
func packReadyError(validator *schema.SchemaValidator, packPath string) error {
	data, err := os.ReadFile(packPath)
	if err != nil {
		return fmt.Errorf("pack file unreadable: %w", err)
	}
	if validator != nil {
		if err := validator.Validate(data); err != nil {
			return fmt.Errorf("pack schema invalid: %w", err)
		}
	}
	return nil
}

// healthMux builds the runtime's HTTP health/metrics handler: a liveness probe
// (/healthz), a readiness probe (/readyz) that re-validates the mounted pack on
// every call, and /metrics served from the merged default + collector gatherers.
func (r *Runtime) healthMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(okBody))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if err := packReadyError(r.readyValidator, r.cfg.PromptPackPath); err != nil {
			r.log.V(1).Info("readiness check failed", "error", err.Error())
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(okBody))
	})
	mux.Handle("/metrics", promhttp.HandlerFor(r.gatherers, promhttp.HandlerOpts{}))
	return mux
}

// mergedGatherers builds the Prometheus gatherer set exposed at /metrics: the
// process default registry plus the runtime's isolated collector registry.
func mergedGatherers(collectorRegistry *prometheus.Registry) prometheus.Gatherers {
	return prometheus.Gatherers{prometheus.DefaultGatherer, collectorRegistry}
}
