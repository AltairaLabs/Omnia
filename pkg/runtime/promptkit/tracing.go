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
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"

	pkruntime "github.com/altairalabs/omnia/internal/runtime"
	"github.com/altairalabs/omnia/internal/tracing"
)

// newTracingProvider constructs the OpenTelemetry tracing provider when
// cfg.TracingEnabled, sets it as the global provider so the PromptKit SDK emits
// spans through it, and returns it. Tracing is optional: a construction failure
// logs and returns nil so the runtime still serves. A nil return means "no
// tracing", which every downstream caller treats as a no-op.
func newTracingProvider(cfg *pkruntime.Config, log logr.Logger) *tracing.Provider {
	if !cfg.TracingEnabled {
		return nil
	}
	tracingCfg := tracing.Config{
		Enabled:        true,
		Endpoint:       cfg.TracingEndpoint,
		ServiceName:    fmt.Sprintf("omnia-runtime-%s", cfg.AgentName),
		ServiceVersion: "1.0.0",
		Environment:    cfg.Namespace,
		SampleRate:     cfg.TracingSampleRate,
		Insecure:       cfg.TracingInsecure,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	provider, err := tracing.NewProvider(ctx, tracingCfg)
	if err != nil {
		log.Error(err, "failed to initialize tracing")
		return nil
	}
	provider = provider.WithLogger(log)
	// Set as global provider so the PromptKit SDK can use it. Safe because the
	// runtime is isolated in its own container.
	otel.SetTracerProvider(provider.TracerProvider())
	log.Info("tracing initialized",
		"endpoint", cfg.TracingEndpoint,
		"sampleRate", cfg.TracingSampleRate)
	return provider
}
