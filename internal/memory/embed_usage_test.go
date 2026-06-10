/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeEmitter captures emitted records for assertions.
type fakeEmitter struct {
	records []ProviderUsageRecord
}

func (f *fakeEmitter) EmitProviderUsage(_ context.Context, rec ProviderUsageRecord) {
	f.records = append(f.records, rec)
}

func TestEmbeddingUsageRecorder_RecordsCounterAndEmits(t *testing.T) {
	emitter := &fakeEmitter{}
	rec := NewEmbeddingUsageRecorder("omnia-demo", "demo", "azure", "azure-embed", emitter, logr.Discard())

	before := testutil.ToFloat64(embedTokensTotal.WithLabelValues("demo", "text-embedding-3-small", "azure", EmbeddingUsageSource))
	rec.RecordEmbeddingUsage(context.Background(), "text-embedding-3-small", 512)
	after := testutil.ToFloat64(embedTokensTotal.WithLabelValues("demo", "text-embedding-3-small", "azure", EmbeddingUsageSource))

	assert.InDelta(t, 512, after-before, 0.001)

	require.Len(t, emitter.records, 1)
	got := emitter.records[0]
	assert.Equal(t, "omnia-demo", got.Namespace)
	assert.Equal(t, "demo", got.WorkspaceName)
	assert.Equal(t, "azure", got.Provider)
	assert.Equal(t, "azure-embed", got.ProviderName)
	assert.Equal(t, "text-embedding-3-small", got.Model)
	assert.Equal(t, EmbeddingUsageSource, got.Source)
	assert.Equal(t, int64(512), got.InputTokens)
	assert.Equal(t, int32(1), got.CallCount)
}

func TestEmbeddingUsageRecorder_ModelFallsBackToProviderName(t *testing.T) {
	emitter := &fakeEmitter{}
	rec := NewEmbeddingUsageRecorder("ns", "ws", "openai", "openai-embed", emitter, logr.Discard())

	rec.RecordEmbeddingUsage(context.Background(), "", 10)
	require.Len(t, emitter.records, 1)
	assert.Equal(t, "openai-embed", emitter.records[0].Model)
}

func TestEmbeddingUsageRecorder_ZeroTokensNoop(t *testing.T) {
	emitter := &fakeEmitter{}
	rec := NewEmbeddingUsageRecorder("ns", "ws", "openai", "p", emitter, logr.Discard())

	rec.RecordEmbeddingUsage(context.Background(), "m", 0)
	rec.RecordEmbeddingUsage(context.Background(), "m", -5)
	assert.Empty(t, emitter.records)
}

func TestEmbeddingUsageRecorder_NilEmitterStillCountsTokens(t *testing.T) {
	rec := NewEmbeddingUsageRecorder("ns", "ws-nil-emitter", "openai", "p", nil, logr.Discard())

	before := testutil.ToFloat64(embedTokensTotal.WithLabelValues("ws-nil-emitter", "m", "openai", EmbeddingUsageSource))
	rec.RecordEmbeddingUsage(context.Background(), "m", 7)
	after := testutil.ToFloat64(embedTokensTotal.WithLabelValues("ws-nil-emitter", "m", "openai", EmbeddingUsageSource))
	assert.InDelta(t, 7, after-before, 0.001)
}

func TestEmbeddingUsageRecorder_NilRecorderNoop(t *testing.T) {
	var rec *EmbeddingUsageRecorder
	// Must not panic.
	rec.RecordEmbeddingUsage(context.Background(), "m", 100)
}
