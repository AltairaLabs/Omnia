/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	facadea2a "github.com/altairalabs/omnia/internal/facade/a2a"
	"github.com/altairalabs/omnia/internal/tracing"
)

// TestBuildA2AHandler_WiresTracingProvider verifies the wiring contract for
// #728 items 3+5: when a non-nil tracing.Provider is passed to
// buildA2AHandler, the returned handler emits OpenTelemetry spans for
// incoming requests. Previously:
//
//   - cmd/agent/a2a.go (standalone mode) blank-identified the tracing
//     provider, so A2A standalone had no distributed tracing even when
//     OMNIA_TRACING_ENABLED=true.
//   - cmd/agent/websocket.go startA2AServer (dual-protocol mode) did not
//     take a tracing provider at all, so dual-protocol A2A was also
//     silently untraced.
//
// Both paths now go through buildA2AHandler. A regression that drops the
// otelhttp wrapper — or stops threading the provider from the run* entry
// points — is caught here.
func TestBuildA2AHandler_WiresTracingProvider(t *testing.T) {
	freshPromRegistry(t)

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	provider := tracing.NewTestProvider(tp)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	metrics := facadea2a.NewMetrics("probe", "ns")
	handler := buildA2AHandler(inner, metrics, provider)

	req := httptest.NewRequest(http.MethodGet, "/a2a/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("inner handler not invoked: got status %d", rr.Code)
	}

	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Error("no spans recorded — otelhttp middleware is not wired; " +
			"A2A requests will not be traced (items 3+5 of #728)")
	}
	// Server spans from otelhttp are named after the operation passed to
	// NewHandler. Allow some flexibility in matching.
	foundA2ASpan := false
	for _, span := range spans {
		if span.Name == "a2a-facade" || span.Name == "GET /a2a/test" || span.Name == "GET" {
			foundA2ASpan = true
			break
		}
	}
	if !foundA2ASpan {
		t.Errorf("expected a span named 'a2a-facade' (or the method span) from otelhttp; got %d spans with names: %v",
			len(spans), spanNames(spans))
	}
}

// TestBuildA2AHandler_NoTracingProviderLeavesHandlerClean verifies the
// inverse: when tracingProvider is nil, no OTel wrapper is applied and the
// handler chain is just metrics -> inner. A request still reaches the inner
// handler and no spans are recorded.
func TestBuildA2AHandler_NoTracingProviderLeavesHandlerClean(t *testing.T) {
	freshPromRegistry(t)

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	metrics := facadea2a.NewMetrics("probe", "ns")
	handler := buildA2AHandler(inner, metrics, nil)

	req := httptest.NewRequest(http.MethodGet, "/a2a/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("inner handler was not called")
	}
	if len(exporter.GetSpans()) != 0 {
		t.Errorf("expected no spans when tracing provider is nil, got %d", len(exporter.GetSpans()))
	}
}

func spanNames(spans tracetest.SpanStubs) []string {
	out := make([]string, 0, len(spans))
	for _, s := range spans {
		out = append(out, s.Name)
	}
	return out
}
