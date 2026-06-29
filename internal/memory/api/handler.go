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

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/httputil"
	"github.com/altairalabs/omnia/internal/memory"
)

// Handler constants.
const (
	defaultListLimit  = 20
	maxListLimit      = 100
	maxStringParamLen = 253

	// DefaultMaxBodySize is the maximum allowed request body size (16 MB).
	DefaultMaxBodySize int64 = 16 << 20

	// defaultBatchDeleteLimit is the default number of rows deleted per batch delete request.
	defaultBatchDeleteLimit = 500
	// maxBatchDeleteLimit is the maximum number of rows allowed per batch delete request.
	maxBatchDeleteLimit = 10000
)

// MemoryWithTier wraps memory.Memory with a derived "tier" string for
// dashboard rendering. Tier is computed from the scope map; no schema change.
//
// Related is the per-memory neighbour list. The recall handler
// populates it from the memory_relations graph so the agent can see
// which memories share an entity (a user identity, a project, a
// workspace doc) and decide which ones a fresh observation should
// supersede or update. Empty / nil for list / institutional / agent-
// scoped responses where graph navigation isn't meaningful.
//
// Title, Summary, ContentPreview, BodySizeBytes, HasFullBody power
// the large-memory inline-vs-open contract. When the active
// observation's body is larger than InlineBodyThresholdBytes, the
// recall handler swaps Content for ContentPreview (the first
// previewBytes characters) and sets HasFullBody=true so the agent
// knows to call memory__open(id) when it needs the full text. For
// small memories they're omitted and Content carries the whole body
// inline. List / open responses always carry full content.
type MemoryWithTier struct {
	*memory.Memory
	Tier           string                  `json:"tier"`
	Title          string                  `json:"title,omitempty"`
	Summary        string                  `json:"summary,omitempty"`
	ContentPreview string                  `json:"content_preview,omitempty"`
	BodySizeBytes  int                     `json:"body_size_bytes,omitempty"`
	HasFullBody    bool                    `json:"has_full_body,omitempty"`
	Related        []memory.EntityRelation `json:"related,omitempty"`
}

// InlineBodyThresholdBytes is the cutoff above which recall returns
// only a content preview rather than the full body. 2 KiB keeps
// short observations (preferences, names, single-line notes) inline
// while still summarising workspace docs and session compactions.
// The agent fetches the full body via GET /api/v1/memories/{id}.
const InlineBodyThresholdBytes = 2048

// previewRunes is the size of the inline ContentPreview that recall
// returns for memories above the threshold. 240 runes is enough for
// the agent to recognise the memory and decide whether to open it,
// without paying the full body in context every time recall runs.
// Counted in runes (not bytes) so multi-byte UTF-8 content can't
// produce an invalid string mid-truncation.
const previewRunes = 240

// deriveTier returns "user_for_agent" / "user" / "agent" / "institutional"
// based on which scope keys are populated. Mirrors the SQL CASE expression
// used by Aggregate's groupBy=tier branch in internal/memory/stats.go and
// the TierUserForAgent / TierUser / TierAgent / TierInstitutional
// constants in retrieve_multi_tier.go.
func deriveTier(scope map[string]string) string {
	hasUser := scope[memory.ScopeUserID] != ""
	hasAgent := scope[memory.ScopeAgentID] != ""
	switch {
	case hasUser && hasAgent:
		return string(memory.TierUserForAgent)
	case hasUser:
		return string(memory.TierUser)
	case hasAgent:
		return string(memory.TierAgent)
	default:
		return string(memory.TierInstitutional)
	}
}

// wrapMemoriesWithTier maps a slice of *memory.Memory into the tier-tagged DTO.
// Title / Summary fields populate from Metadata in every code path; the
// body-size driven preview behaviour is opt-in via wrapMemoriesWithPreview
// for the recall handler only — list/open paths return full content.
func wrapMemoriesWithTier(rows []*memory.Memory) []*MemoryWithTier {
	out := make([]*MemoryWithTier, len(rows))
	for i, m := range rows {
		out[i] = newMemoryWithTier(m)
	}
	return out
}

// wrapMemoriesWithPreview is the recall variant: large bodies are
// replaced with a preview + has_full_body=true so the agent can
// decide whether to fetch the full content via memory__open. The
// inline cutoff comes from MemoryPolicy.recall.inlineThresholdBytes
// when configured; falls back to InlineBodyThresholdBytes.
func wrapMemoriesWithPreview(rows []*memory.Memory, inlineThreshold int) []*MemoryWithTier {
	if inlineThreshold <= 0 {
		inlineThreshold = InlineBodyThresholdBytes
	}
	out := make([]*MemoryWithTier, len(rows))
	for i, m := range rows {
		mw := newMemoryWithTier(m)
		applyInlinePreview(mw, inlineThreshold)
		out[i] = mw
	}
	return out
}

// newMemoryWithTier builds the base DTO and pulls Title / Summary /
// BodySizeBytes out of Metadata so the JSON shape advertises them as
// first-class fields rather than buried under Metadata.
func newMemoryWithTier(m *memory.Memory) *MemoryWithTier {
	if m == nil {
		return &MemoryWithTier{}
	}
	mw := &MemoryWithTier{Memory: m, Tier: deriveTier(m.Scope)}
	if m.Metadata == nil {
		return mw
	}
	if title, ok := m.Metadata[memory.MetaKeyTitle].(string); ok {
		mw.Title = title
	}
	if summary, ok := m.Metadata[memory.MetaKeySummary].(string); ok {
		mw.Summary = summary
	}
	if size, ok := readBodySize(m.Metadata[memory.MetaKeyBodySize]); ok {
		mw.BodySizeBytes = size
	}
	return mw
}

// applyInlinePreview swaps full content for a preview when the body
// exceeds the supplied inline threshold. Mutates in place.
//
// Counts runes (not bytes) so the preview never splits a multi-byte
// UTF-8 sequence mid-character — an agent reading a corrupted
// preview would see a U+FFFD replacement glyph and have to fetch
// the full body anyway.
//
// Uses utf8.DecodeRuneInString so we walk only previewRunes ahead
// of the cutoff (or detect "fits inline" early), avoiding a full
// rune-slice allocation for every recall result.
func applyInlinePreview(mw *MemoryWithTier, inlineThreshold int) {
	if mw.Memory == nil || mw.BodySizeBytes <= inlineThreshold {
		return
	}
	cutByte, fits := previewByteOffset(mw.Content, previewRunes)
	if fits {
		// Content fits inside the preview window even though the
		// byte count crossed the inline threshold (multi-byte UTF-8).
		// Leave it inline.
		mw.HasFullBody = false
		return
	}
	mw.ContentPreview = mw.Content[:cutByte]
	mw.Content = ""
	mw.HasFullBody = true
}

// previewByteOffset returns the byte index after the maxRunes-th
// rune of s, plus a "fits" flag set when s is shorter than that.
// Walks at most maxRunes+1 runes from the start — O(maxRunes), not
// O(len(s)).
func previewByteOffset(s string, maxRunes int) (int, bool) {
	i, runes := 0, 0
	for i < len(s) && runes < maxRunes {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
		runes++
	}
	return i, i >= len(s)
}

// readBodySize tolerates both int and float64 — the latter is what
// Go's json package decodes integer fields to when round-tripping
// metadata through JSON.
func readBodySize(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}

// MemoryListResponse is the JSON response for memory list/search endpoints.
type MemoryListResponse struct {
	Memories []*MemoryWithTier `json:"memories"`
	Total    int               `json:"total"`
}

// MemoryResponse is the JSON response for a single memory creation.
type MemoryResponse struct {
	Memory *memory.Memory `json:"memory"`
}

// ErrorResponse is the JSON response for errors.
type ErrorResponse struct {
	Error string `json:"error"`
}

// BatchDeleteResponse is the JSON response for DELETE /api/v1/memories/batch.
type BatchDeleteResponse struct {
	Deleted int `json:"deleted"`
}

// SaveMemoryRequest is the JSON body for POST /api/v1/memories.
//
// About is the structured-dedup hint. When set, the server uses
// (workspace, user, agent, About.Kind, About.Key) as a soft-unique
// key — a second write with the same About atomically supersedes
// the first under the same entity. Use for identity-class facts
// where the user has one current value (name, location, single-
// valued preference). Omit for free-form notes; the embedding-
// similarity path catches near-duplicates without a key.
//
// Title and Summary apply to large memories (workspace docs,
// session summaries). Recall returns the title + summary for
// memories above the inline threshold, full content otherwise.
type SaveMemoryRequest struct {
	Type       string            `json:"type"`
	Content    string            `json:"content"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
	Confidence float64           `json:"confidence"`
	Scope      map[string]string `json:"scope"`
	SessionID  string            `json:"session_id,omitempty"`
	TurnRange  [2]int            `json:"turn_range,omitempty"`
	Category   string            `json:"category,omitempty"`
	About      *AboutKey         `json:"about,omitempty"`
	Title      string            `json:"title,omitempty"`
	Summary    string            `json:"summary,omitempty"`
}

// AboutKey is the structured-dedup hint surface. Both Kind and Key
// are required for the server to engage the structured-key dedup
// path; an empty value on either side falls through to the
// embedding-similarity path.
type AboutKey struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
}

// SaveMemoryResponse is the JSON body returned from POST
// /api/v1/memories. The Memory field carries the same shape as the
// existing MemoryResponse for backwards compatibility; the
// `action`/`supersedes`/`potential_duplicates` fields are new and
// surface what the dedup pipeline did, so the agent's reply can be
// honest ("Got it" vs "Updated your name from Slim Shard to Phil").
type SaveMemoryResponse struct {
	Memory                   MemoryWithTier              `json:"memory"`
	Action                   memory.SaveAction           `json:"action"`
	SupersededObservationIDs []string                    `json:"supersedes,omitempty"`
	SupersedeReason          memory.SaveSupersedeReason  `json:"supersede_reason,omitempty"`
	PotentialDuplicates      []memory.DuplicateCandidate `json:"potential_duplicates,omitempty"`
}

// UpdateMemoryRequest is the JSON body for PATCH /api/v1/memories/{id}.
// The path parameter identifies the entity to attach the new
// observation to; the body carries the new content. The server
// supersedes the entity's prior active observation in the same
// transaction so recall sees the new value immediately.
type UpdateMemoryRequest struct {
	Content    string            `json:"content"`
	Type       string            `json:"type,omitempty"`
	Confidence float64           `json:"confidence,omitempty"`
	Scope      map[string]string `json:"scope"`
}

// SupersedeRequest is the JSON body for POST /api/v1/memories/supersede.
// SourceIDs lists the entity IDs whose active observations should
// be marked inactive; the new content lands as a fresh active
// observation under SourceIDs[0]. The agent uses this to collapse
// N stale memories about the same fact (typically duplicates the
// agent created before it learned to set `about`) into one canonical
// truth in a single round trip.
type SupersedeRequest struct {
	SourceIDs  []string          `json:"source_ids"`
	Content    string            `json:"content"`
	Type       string            `json:"type,omitempty"`
	Confidence float64           `json:"confidence,omitempty"`
	Scope      map[string]string `json:"scope"`
}

// LinkRequest is the JSON body for POST /api/v1/relations. Connects
// source_id to target_id with the given relation_type so derived
// facts (preferences, notes) survive renames of the anchor entity.
type LinkRequest struct {
	SourceID     string            `json:"source_id"`
	TargetID     string            `json:"target_id"`
	RelationType string            `json:"relation_type"`
	Weight       float64           `json:"weight,omitempty"`
	Scope        map[string]string `json:"scope"`
}

// LinkResponse carries the newly-created relation ID so callers can
// reference it from later updates / inspections.
type LinkResponse struct {
	ID string `json:"id"`
}

// Handler provides HTTP endpoints for the memory API.
type Handler struct {
	service          *MemoryService
	log              logr.Logger
	maxBodySize      int64
	recordDimConsent DimensionConsentRecorder
	enterprise       bool
}

// NewHandler creates a new memory API handler.
func NewHandler(service *MemoryService, log logr.Logger) *Handler {
	return &Handler{
		service:     service,
		log:         log.WithName("memory-handler"),
		maxBodySize: DefaultMaxBodySize,
	}
}

// WithDimensionConsentRecorder wires the recorder used by the embedding
// dimension-change admin endpoint. Without it that endpoint responds 503.
func (h *Handler) WithDimensionConsentRecorder(r DimensionConsentRecorder) *Handler {
	h.recordDimConsent = r
	return h
}

// WithEnterprise gates enterprise-only routes (institutional, projection,
// analytics). When false they return 403 enterprise_required.
func (h *Handler) WithEnterprise(enterprise bool) *Handler {
	h.enterprise = enterprise
	return h
}

// requireEnterprise wraps a handler so it returns 403 enterprise_required when
// the binary is not in enterprise mode. Applied to the institutional tier,
// projection, and memory-analytics routes; agent + user routes stay open.
func (h *Handler) requireEnterprise(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.enterprise {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"enterprise_required"}`))
			return
		}
		next(w, r)
	}
}

// RegisterRoutes registers all memory-api HTTP routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /api/v1/memories", h.handleListMemories)
	mux.HandleFunc("GET /api/v1/memories/search", h.handleSearchMemories)
	mux.HandleFunc("GET /api/v1/memories/export", h.handleExportMemories)
	mux.HandleFunc("POST /api/v1/memories", h.handleSaveMemory)
	mux.HandleFunc("GET /api/v1/memories/aggregate", h.requireEnterprise(h.handleMemoryAggregate))
	mux.HandleFunc("GET /api/v1/memories/projection", h.requireEnterprise(h.handleProjection))
	mux.HandleFunc("GET /api/v1/memories/{id}", h.handleOpenMemory)
	mux.HandleFunc("PATCH /api/v1/memories/{id}", h.handleUpdateMemory)
	mux.HandleFunc("POST /api/v1/memories/supersede", h.handleSupersedeMemories)
	mux.HandleFunc("GET /api/v1/memories/conflicts", h.handleListConflicts)
	mux.HandleFunc("POST /api/v1/relations", h.handleLinkMemories)
	mux.HandleFunc("DELETE /api/v1/memories/{id}", h.handleDeleteMemory)
	mux.HandleFunc("DELETE /api/v1/memories/batch", h.handleBatchDeleteMemories)
	mux.HandleFunc("DELETE /api/v1/memories", h.handleDeleteAllMemories)
	mux.HandleFunc("POST /api/v1/memories/retrieve", h.handleRetrieveMultiTier)
	mux.HandleFunc("POST /api/v1/memories/retrieve/semantic", h.handleSemanticRetrieve)

	mux.HandleFunc("POST /api/v1/institutional/memories", h.requireEnterprise(h.handleSaveInstitutional))
	mux.HandleFunc("GET /api/v1/institutional/memories", h.requireEnterprise(h.handleListInstitutional))
	mux.HandleFunc("DELETE /api/v1/institutional/memories/{id}", h.requireEnterprise(h.handleDeleteInstitutional))
	mux.HandleFunc("POST /api/v1/institutional/ingest", h.requireEnterprise(h.handleIngest))
	mux.HandleFunc("GET /api/v1/ingest/summary-candidates", h.handleListSummaryCandidates)
	mux.HandleFunc("POST /api/v1/ingest/summaries", h.handleSaveDocumentSummary)

	mux.HandleFunc("POST /api/v1/agent-memories", h.handleSaveAgentScoped)
	mux.HandleFunc("GET /api/v1/agent-memories", h.handleListAgentScoped)
	mux.HandleFunc("DELETE /api/v1/agent-memories/{id}", h.handleDeleteAgentScoped)

	mux.HandleFunc("GET /api/v1/compaction/candidates", h.handleListCompactionCandidates)
	mux.HandleFunc("POST /api/v1/compaction/summaries", h.handleSaveCompactionSummary)

	// Consent events: per-user/category prune on consent revocation (CE1).
	// Enterprise-gated: privacy-api calls this after removing a consent grant.
	mux.HandleFunc("POST /api/v1/memories/consent-events", h.requireEnterprise(h.handleConsentEvent))

	// Admin: record one-shot consent for a destructive embedding-dimension
	// change (#1309). Consumed by the startup reconciler on the next reshape.
	mux.HandleFunc("POST /admin/embedding-dimension-change", h.handleEmbeddingDimensionChange)

	h.registerDocsRoutes(mux)
}

// exportFilename is the Content-Disposition filename for DSAR exports.
const exportFilename = `attachment; filename="memories-export.json"`

// ConflictsResponse is the JSON shape returned by GET
// /api/v1/memories/conflicts. Carries the list of triage rows
// plus a count for dashboard pagination.
type ConflictsResponse struct {
	Conflicts []memory.ConflictedEntity `json:"conflicts"`
	Total     int                       `json:"total"`
}

// parseWorkspaceScope extracts and validates the workspace parameter, then builds the full scope.
func parseWorkspaceScope(r *http.Request) (map[string]string, error) {
	q := r.URL.Query()
	workspace := truncateParam(q.Get("workspace"))
	if workspace == "" {
		return nil, ErrMissingWorkspace
	}
	return buildScope(q), nil
}

// buildScope constructs a scope map from query parameters.
func buildScope(q interface{ Get(string) string }) map[string]string {
	scope := map[string]string{
		memory.ScopeWorkspaceID: truncateParam(q.Get("workspace")),
	}
	if uid := q.Get("user_id"); uid != "" {
		scope[memory.ScopeUserID] = truncateParam(uid)
	}
	if agent := q.Get("agent"); agent != "" {
		scope[memory.ScopeAgentID] = truncateParam(agent)
	}
	return scope
}

// parseTypes splits a comma-separated type parameter into a slice.
func parseTypes(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// parseIntParam returns an integer query parameter or the default value.
func parseIntParam(r *http.Request, name string, defaultVal int) int {
	s := r.URL.Query().Get(name)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return defaultVal
	}
	return v
}

// parseMinConfidence extracts the min_confidence query parameter.
func parseMinConfidence(r *http.Request) float64 {
	s := r.URL.Query().Get("min_confidence")
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}

// truncateParam silently truncates s to maxStringParamLen if it exceeds the limit.
func truncateParam(s string) string {
	if len(s) > maxStringParamLen {
		return s[:maxStringParamLen]
	}
	return s
}

// writeJSON writes a JSON 200 OK response.
func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	_ = json.NewEncoder(w).Encode(data)
}

// writeError maps known errors to HTTP status codes and writes a JSON error response.
func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	msg := "internal server error"

	// Per-handler structured errors (httpError) carry their own status + msg.
	var he httpError
	if errors.As(err, &he) {
		w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
		w.WriteHeader(he.status)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: he.msg})
		return
	}

	switch {
	case errors.Is(err, ErrMissingWorkspace):
		status = http.StatusBadRequest
		msg = ErrMissingWorkspace.Error()
	case errors.Is(err, ErrMissingUserID):
		status = http.StatusBadRequest
		msg = ErrMissingUserID.Error()
	case errors.Is(err, ErrMissingQuery):
		status = http.StatusBadRequest
		msg = ErrMissingQuery.Error()
	case errors.Is(err, ErrMissingMemoryID):
		status = http.StatusBadRequest
		msg = ErrMissingMemoryID.Error()
	case errors.Is(err, ErrMissingBody):
		status = http.StatusBadRequest
		msg = ErrMissingBody.Error()
	case errors.Is(err, ErrBodyTooLarge) || isMaxBytesError(err):
		status = http.StatusRequestEntityTooLarge
		msg = ErrBodyTooLarge.Error()
	case errors.Is(err, ErrExpiresAtInPast):
		status = http.StatusBadRequest
		msg = ErrExpiresAtInPast.Error()
	case errors.Is(err, ErrMissingAgentID):
		status = http.StatusBadRequest
		msg = ErrMissingAgentID.Error()
	case errors.Is(err, ErrAboutRequired):
		status = http.StatusBadRequest
		msg = ErrAboutRequired.Error()
	case errors.Is(err, memory.ErrNotFound):
		status = http.StatusNotFound
		msg = "memory not found"
	}

	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: msg})
}

// isMaxBytesError checks if the error is an http.MaxBytesError from MaxBytesReader.
func isMaxBytesError(err error) bool {
	var maxBytesErr *http.MaxBytesError
	return errors.As(err, &maxBytesErr)
}
