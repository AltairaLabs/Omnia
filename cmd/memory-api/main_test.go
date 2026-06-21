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
	"testing"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"

	"github.com/altairalabs/omnia/internal/memory"
)

// TestConsolidationWorker_GatedByEnterprise asserts that
// buildConsolidationWorker returns nil when enterprise is off, even
// when a valid interval is supplied.
func TestConsolidationWorker_GatedByEnterprise(t *testing.T) {
	f := &flags{enterprise: false, consolidationInterval: "1m"}
	assert.Nil(t, buildConsolidationWorker(context.Background(), f, nil, nil, logr.Discard()),
		"consolidation worker must not be built when enterprise is off")
}

// TestProjectionWorker_GatedByEnterprise asserts that
// buildProjectionWorker returns nil when enterprise is off, even when
// a valid interval is supplied.
func TestProjectionWorker_GatedByEnterprise(t *testing.T) {
	f := &flags{enterprise: false, projectionInterval: "30s"}
	assert.Nil(t, buildProjectionWorker(f, nil, prometheus.DefaultRegisterer, logr.Discard()),
		"projection worker must not be built when enterprise is off")
}

// TestReembedWorker_NotGatedByEnterprise asserts that
// reembedWorkerOptions reports enabled=true when enterprise is off,
// proving that re-embed (the OSS floor that keeps embeddings fresh)
// is not an enterprise-only feature.
func TestReembedWorker_NotGatedByEnterprise(t *testing.T) {
	f := &flags{enterprise: false, reembedInterval: "60m"}
	// memory.NewEmbeddingService accepts nil store/provider in unit
	// tests — only ModelName() is called inside reembedWorkerOptions,
	// which returns an empty string (no panic).
	svc := memory.NewEmbeddingService(nil, nil, logr.Discard())
	_, enabled := f.reembedWorkerOptions(svc)
	assert.True(t, enabled, "reembed must remain enabled (OSS floor) when enterprise is off")
}

// TestEmbeddingMetricsCollector_NotGatedByEnterprise proves the embedding
// coverage/backlog collector builds even with enterprise off — coverage and
// re-embed backlog are OSS operational signals (#1442).
func TestEmbeddingMetricsCollector_NotGatedByEnterprise(t *testing.T) {
	f := &flags{enterprise: false, metricsCollectInterval: "60s"}
	mc := buildEmbeddingMetricsCollector(f, nil, nil, prometheus.NewRegistry(), logr.Discard())
	assert.NotNil(t, mc, "embedding metrics collector is OSS and must build when enterprise is off")
}

// TestEmbeddingMetricsCollector_UsesEmbeddingModelName covers the path where an
// embedding service is present (its ModelName is threaded into the collector).
func TestEmbeddingMetricsCollector_UsesEmbeddingModelName(t *testing.T) {
	f := &flags{metricsCollectInterval: "30s"}
	svc := memory.NewEmbeddingService(nil, nil, logr.Discard())
	mc := buildEmbeddingMetricsCollector(f, nil, svc, prometheus.NewRegistry(), logr.Discard())
	assert.NotNil(t, mc)
}

// TestEmbeddingMetricsCollector_Disabled asserts the collector is off for an
// empty interval and for an unparseable one (logged, not fatal).
func TestEmbeddingMetricsCollector_Disabled(t *testing.T) {
	empty := &flags{metricsCollectInterval: ""}
	assert.Nil(t, buildEmbeddingMetricsCollector(empty, nil, nil, prometheus.NewRegistry(), logr.Discard()),
		"empty interval disables the collector")

	bad := &flags{metricsCollectInterval: "not-a-duration"}
	assert.Nil(t, buildEmbeddingMetricsCollector(bad, nil, nil, prometheus.NewRegistry(), logr.Discard()),
		"unparseable interval disables the collector")
}
