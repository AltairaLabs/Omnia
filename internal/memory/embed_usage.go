/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

package memory

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// EmbeddingUsageSource is the provider_usage.source value for embedding spend.
const EmbeddingUsageSource = "embedding"

// embedTokensTotal counts embedding tokens consumed, the spend that was
// previously invisible in session totals: memory-api embeds queries and
// documents with no session context, so the tokens never reached a
// provider_calls row. This counter (and the session-api provider_usage emit)
// make that spend observable. Embeddings are input-only, so this is effectively
// prompt-token volume.
var embedTokensTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "omnia_embedding_tokens_total",
	Help: "Embedding tokens consumed by the memory subsystem, labelled by workspace/model/provider/source. Previously invisible — embeddings carry no session context so they never hit session token totals.",
}, []string{"workspace", "model", "provider", "source"})

// ProviderUsageRecord is one workspace-scoped, session-less provider-spend
// record forwarded to session-api's provider_usage table.
type ProviderUsageRecord struct {
	Namespace     string
	WorkspaceName string
	Provider      string // provider type (e.g. "openai", "azure")
	ProviderName  string // Provider CRD name
	Model         string
	Source        string
	InputTokens   int64
	CallCount     int32
}

// ProviderUsageEmitter forwards usage records to a sink (session-api). It is
// best-effort: implementations must not block or fail the calling path.
type ProviderUsageEmitter interface {
	EmitProviderUsage(ctx context.Context, rec ProviderUsageRecord)
}

// EmbeddingUsageRecorder records embedding token spend: it increments the
// Prometheus counter and, when an emitter is configured, forwards the spend to
// session-api's provider_usage table. The workspace/namespace/provider context
// is process-level (memory-api is deployed per workspace service group), so the
// recorder holds it as static fields.
type EmbeddingUsageRecorder struct {
	namespace     string // attribution key for session-api (the workspace namespace)
	workspaceName string // human-facing workspace name (Prometheus label + provider_usage)
	providerType  string // Provider CRD spec.type
	providerName  string // Provider CRD name
	emitter       ProviderUsageEmitter
	log           logr.Logger
}

// NewEmbeddingUsageRecorder builds a recorder. emitter may be nil, in which
// case only the Prometheus counter is updated.
func NewEmbeddingUsageRecorder(namespace, workspaceName, providerType, providerName string, emitter ProviderUsageEmitter, log logr.Logger) *EmbeddingUsageRecorder {
	return &EmbeddingUsageRecorder{
		namespace:     namespace,
		workspaceName: workspaceName,
		providerType:  providerType,
		providerName:  providerName,
		emitter:       emitter,
		log:           log.WithName("embed-usage"),
	}
}

// RecordEmbeddingUsage increments the token counter and emits a provider_usage
// record. model falls back to the provider CRD name when the provider did not
// report one. A nil recorder is a no-op so call sites need no guard.
func (r *EmbeddingUsageRecorder) RecordEmbeddingUsage(ctx context.Context, model string, tokens int) {
	if r == nil || tokens <= 0 {
		return
	}
	if model == "" {
		model = r.providerName
	}
	embedTokensTotal.WithLabelValues(r.workspaceName, model, r.providerType, EmbeddingUsageSource).Add(float64(tokens))

	if r.emitter == nil {
		return
	}
	r.emitter.EmitProviderUsage(ctx, ProviderUsageRecord{
		Namespace:     r.namespace,
		WorkspaceName: r.workspaceName,
		Provider:      r.providerType,
		ProviderName:  r.providerName,
		Model:         model,
		Source:        EmbeddingUsageSource,
		InputTokens:   int64(tokens),
		CallCount:     1,
	})
}
