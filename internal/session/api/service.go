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

// Package api provides the HTTP API layer for session history with
// tiered hot→warm→cold storage fallback.
package api

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
)

// Sentinel errors returned by the session service.
var (
	ErrWarmStoreRequired = errors.New("warm store is required for this operation")
	ErrMissingWorkspace  = errors.New("workspace parameter is required")
	ErrMissingQuery      = errors.New("search query parameter is required")
	ErrMissingSessionID  = errors.New("session ID is required")
	ErrMissingBody       = errors.New("request body is required")
)

// DefaultCacheTTL is the default TTL for hot cache entries populated from warm/cold.
const DefaultCacheTTL = 15 * time.Minute

// ServiceConfig configures the SessionService.
type ServiceConfig struct {
	// CacheTTL is the TTL for hot cache entries populated from lower tiers.
	// Defaults to DefaultCacheTTL (15 minutes) if zero.
	CacheTTL time.Duration

	// AuditLogger is an optional audit logger for session operations.
	// When non-nil, session access events are logged asynchronously.
	AuditLogger AuditLogger
}

// SessionService provides tiered session retrieval with hot→warm→cold fallback.
type SessionService struct {
	registry    *providers.Registry
	cacheTTL    time.Duration
	auditLogger AuditLogger
	log         logr.Logger
}

// NewSessionService creates a new SessionService with the given registry and config.
func NewSessionService(registry *providers.Registry, cfg ServiceConfig, log logr.Logger) *SessionService {
	ttl := cfg.CacheTTL
	if ttl == 0 {
		ttl = DefaultCacheTTL
	}
	return &SessionService{
		registry:    registry,
		cacheTTL:    ttl,
		auditLogger: cfg.AuditLogger,
		log:         log.WithName("session-service"),
	}
}

// GetSession retrieves a session by ID using tiered fallback: hot → warm → cold.
func (s *SessionService) GetSession(ctx context.Context, sessionID string) (*session.Session, error) {
	if sessionID == "" {
		return nil, ErrMissingSessionID
	}

	// Try hot cache first.
	sess, err := s.getFromHot(ctx, sessionID)
	if err == nil {
		s.auditSessionAccess(ctx, sess)
		return sess, nil
	}

	// Try warm store.
	sess, err = s.getFromWarm(ctx, sessionID)
	if err == nil {
		s.populateHotCache(ctx, sess)
		s.auditSessionAccess(ctx, sess)
		return sess, nil
	}

	// Try cold archive.
	sess, err = s.getFromCold(ctx, sessionID)
	if err == nil {
		s.populateHotCache(ctx, sess)
		s.auditSessionAccess(ctx, sess)
		return sess, nil
	}

	return nil, session.ErrSessionNotFound
}

// GetMessages retrieves messages for a session with tiered fallback.
// Hot-eligible queries (no BeforeSeq/AfterSeq/Roles filter, ascending sort, no offset)
// are served from the hot cache when available.
func (s *SessionService) GetMessages(ctx context.Context, sessionID string, opts providers.MessageQueryOpts) ([]*session.Message, error) {
	if sessionID == "" {
		return nil, ErrMissingSessionID
	}

	// Try hot cache for simple queries.
	if isHotEligible(opts) {
		if hot, err := s.registry.HotCache(); err == nil {
			msgs, err := hot.GetRecentMessages(ctx, sessionID, opts.Limit)
			if err == nil {
				s.auditMessagesAccess(ctx, sessionID, len(msgs))
				return msgs, nil
			}
			if !errors.Is(err, session.ErrSessionNotFound) {
				s.log.Error(err, "hot cache GetRecentMessages failed", "sessionID", sessionID)
			}
		}
	}

	// Try warm store.
	if warm, err := s.registry.WarmStore(); err == nil {
		msgs, err := warm.GetMessages(ctx, sessionID, opts)
		if err == nil {
			s.auditMessagesAccess(ctx, sessionID, len(msgs))
			return msgs, nil
		}
		if !errors.Is(err, session.ErrSessionNotFound) {
			s.log.Error(err, "warm store GetMessages failed", "sessionID", sessionID)
		}
	}

	// Fall back to cold archive — retrieve full session and filter in memory.
	if cold, err := s.registry.ColdArchive(); err == nil {
		sess, err := cold.GetSession(ctx, sessionID)
		if err == nil {
			msgs := filterMessages(sess.Messages, opts)
			s.auditMessagesAccess(ctx, sessionID, len(msgs))
			return msgs, nil
		}
		if !errors.Is(err, session.ErrSessionNotFound) {
			s.log.Error(err, "cold archive GetSession failed", "sessionID", sessionID)
		}
	}

	return nil, session.ErrSessionNotFound
}

// ListSessions returns a paginated list of sessions. Requires a warm store.
func (s *SessionService) ListSessions(ctx context.Context, opts providers.SessionListOpts) (*providers.SessionPage, error) {
	warm, err := s.registry.WarmStore()
	if err != nil {
		return nil, ErrWarmStoreRequired
	}
	page, err := warm.ListSessions(ctx, opts)
	if err != nil {
		return nil, err
	}
	s.auditSearch(ctx, "", opts.WorkspaceName, len(page.Sessions))
	return page, nil
}

// SearchSessions performs full-text search across sessions. Requires a warm store.
func (s *SessionService) SearchSessions(ctx context.Context, query string, opts providers.SessionListOpts) (*providers.SessionPage, error) {
	warm, err := s.registry.WarmStore()
	if err != nil {
		return nil, ErrWarmStoreRequired
	}
	page, err := warm.SearchSessions(ctx, query, opts)
	if err != nil {
		return nil, err
	}
	s.auditSearch(ctx, query, opts.WorkspaceName, len(page.Sessions))
	return page, nil
}

// CreateSession persists a new session via the warm store.
func (s *SessionService) CreateSession(ctx context.Context, sess *session.Session) error {
	warm, err := s.registry.WarmStore()
	if err != nil {
		return ErrWarmStoreRequired
	}
	if err := warm.CreateSession(ctx, sess); err != nil {
		return err
	}
	s.auditSessionCreated(ctx, sess)
	return nil
}

// DeleteSession removes a session from the warm store.
func (s *SessionService) DeleteSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return ErrMissingSessionID
	}
	warm, err := s.registry.WarmStore()
	if err != nil {
		return ErrWarmStoreRequired
	}
	// Fetch session metadata before deletion for the audit entry.
	sess, getErr := warm.GetSession(ctx, sessionID)
	if err := warm.DeleteSession(ctx, sessionID); err != nil {
		return err
	}
	s.auditSessionDeleted(ctx, sessionID, sess, getErr)
	return nil
}

// AppendMessage adds a message to a session via the warm store.
func (s *SessionService) AppendMessage(ctx context.Context, sessionID string, msg *session.Message) error {
	if sessionID == "" {
		return ErrMissingSessionID
	}
	warm, err := s.registry.WarmStore()
	if err != nil {
		return ErrWarmStoreRequired
	}
	return warm.AppendMessage(ctx, sessionID, msg)
}

// UpdateSessionStats applies incremental counter updates to a session.
func (s *SessionService) UpdateSessionStats(ctx context.Context, sessionID string, update session.SessionStatsUpdate) error {
	if sessionID == "" {
		return ErrMissingSessionID
	}
	warm, err := s.registry.WarmStore()
	if err != nil {
		return ErrWarmStoreRequired
	}

	// Fetch the existing session to apply the incremental updates.
	sess, err := warm.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	sess.TotalInputTokens += int64(update.AddInputTokens)
	sess.TotalOutputTokens += int64(update.AddOutputTokens)
	sess.EstimatedCostUSD += update.AddCostUSD
	sess.ToolCallCount += update.AddToolCalls
	sess.MessageCount += update.AddMessages
	if update.SetStatus != "" {
		sess.Status = update.SetStatus
	}
	sess.UpdatedAt = time.Now()

	return warm.UpdateSession(ctx, sess)
}

// RefreshTTL extends the expiry of a session.
func (s *SessionService) RefreshTTL(ctx context.Context, sessionID string, ttl time.Duration) error {
	if sessionID == "" {
		return ErrMissingSessionID
	}
	warm, err := s.registry.WarmStore()
	if err != nil {
		return ErrWarmStoreRequired
	}

	sess, err := warm.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	sess.ExpiresAt = time.Now().Add(ttl)
	sess.UpdatedAt = time.Now()

	return warm.UpdateSession(ctx, sess)
}

// getFromHot attempts to retrieve a session from the hot cache.
func (s *SessionService) getFromHot(ctx context.Context, sessionID string) (*session.Session, error) {
	hot, err := s.registry.HotCache()
	if err != nil {
		return nil, err
	}
	return hot.GetSession(ctx, sessionID)
}

// getFromWarm attempts to retrieve a session from the warm store.
func (s *SessionService) getFromWarm(ctx context.Context, sessionID string) (*session.Session, error) {
	warm, err := s.registry.WarmStore()
	if err != nil {
		return nil, err
	}
	return warm.GetSession(ctx, sessionID)
}

// getFromCold attempts to retrieve a session from the cold archive.
func (s *SessionService) getFromCold(ctx context.Context, sessionID string) (*session.Session, error) {
	cold, err := s.registry.ColdArchive()
	if err != nil {
		return nil, err
	}
	return cold.GetSession(ctx, sessionID)
}

// populateHotCache stores a session in the hot cache on a best-effort basis.
func (s *SessionService) populateHotCache(ctx context.Context, sess *session.Session) {
	hot, err := s.registry.HotCache()
	if err != nil {
		return
	}
	if err := hot.SetSession(ctx, sess, s.cacheTTL); err != nil {
		s.log.Error(err, "failed to populate hot cache", "sessionID", sess.ID)
	}
}

// isHotEligible returns true if the query can be served from the hot cache.
// Hot cache only supports simple "recent messages" queries: no sequence filters,
// no role filters, ascending sort, and no offset.
func isHotEligible(opts providers.MessageQueryOpts) bool {
	if opts.BeforeSeq != 0 || opts.AfterSeq != 0 {
		return false
	}
	if len(opts.Roles) > 0 {
		return false
	}
	if opts.Offset != 0 {
		return false
	}
	if opts.SortOrder == providers.SortDesc {
		return false
	}
	return true
}

// --- audit helpers ----------------------------------------------------------

// auditSessionAccess logs a session_accessed event if an audit logger is configured.
func (s *SessionService) auditSessionAccess(ctx context.Context, sess *session.Session) {
	if s.auditLogger == nil {
		return
	}
	rc, _ := requestContextFromCtx(ctx)
	s.auditLogger.LogEvent(ctx, &AuditEntry{
		EventType: "session_accessed",
		SessionID: sess.ID,
		Workspace: sess.WorkspaceName,
		AgentName: sess.AgentName,
		Namespace: sess.Namespace,
		IPAddress: rc.IPAddress,
		UserAgent: rc.UserAgent,
	})
}

// auditMessagesAccess logs a session_accessed event for message retrieval.
func (s *SessionService) auditMessagesAccess(ctx context.Context, sessionID string, count int) {
	if s.auditLogger == nil {
		return
	}
	rc, _ := requestContextFromCtx(ctx)
	s.auditLogger.LogEvent(ctx, &AuditEntry{
		EventType:   "session_accessed",
		SessionID:   sessionID,
		ResultCount: count,
		IPAddress:   rc.IPAddress,
		UserAgent:   rc.UserAgent,
	})
}

// auditSessionCreated logs a session_created event if an audit logger is configured.
func (s *SessionService) auditSessionCreated(ctx context.Context, sess *session.Session) {
	if s.auditLogger == nil {
		return
	}
	rc, _ := requestContextFromCtx(ctx)
	s.auditLogger.LogEvent(ctx, &AuditEntry{
		EventType: "session_created",
		SessionID: sess.ID,
		Workspace: sess.WorkspaceName,
		AgentName: sess.AgentName,
		Namespace: sess.Namespace,
		IPAddress: rc.IPAddress,
		UserAgent: rc.UserAgent,
	})
}

// auditSessionDeleted logs a session_deleted event if an audit logger is configured.
func (s *SessionService) auditSessionDeleted(ctx context.Context, sessionID string, sess *session.Session, getErr error) {
	if s.auditLogger == nil {
		return
	}
	rc, _ := requestContextFromCtx(ctx)
	entry := &AuditEntry{
		EventType: "session_deleted",
		SessionID: sessionID,
		IPAddress: rc.IPAddress,
		UserAgent: rc.UserAgent,
	}
	if getErr == nil && sess != nil {
		entry.Workspace = sess.WorkspaceName
		entry.AgentName = sess.AgentName
		entry.Namespace = sess.Namespace
	}
	s.auditLogger.LogEvent(ctx, entry)
}

// auditSearch logs a session_searched event.
func (s *SessionService) auditSearch(ctx context.Context, query, workspace string, count int) {
	if s.auditLogger == nil {
		return
	}
	rc, _ := requestContextFromCtx(ctx)
	s.auditLogger.LogEvent(ctx, &AuditEntry{
		EventType:   "session_searched",
		Workspace:   workspace,
		Query:       query,
		ResultCount: count,
		IPAddress:   rc.IPAddress,
		UserAgent:   rc.UserAgent,
	})
}

// filterMessages applies MessageQueryOpts to a slice of messages in memory.
// Used as a fallback when reading from cold archive which returns full sessions.
func filterMessages(messages []session.Message, opts providers.MessageQueryOpts) []*session.Message {
	result := make([]*session.Message, 0, len(messages))

	// Build a role lookup set if filtering by roles.
	roleSet := make(map[session.MessageRole]struct{}, len(opts.Roles))
	for _, r := range opts.Roles {
		roleSet[r] = struct{}{}
	}

	for i := range messages {
		msg := &messages[i]

		// Apply sequence filters.
		if opts.BeforeSeq != 0 && msg.SequenceNum >= opts.BeforeSeq {
			continue
		}
		if opts.AfterSeq != 0 && msg.SequenceNum <= opts.AfterSeq {
			continue
		}

		// Apply role filter.
		if len(roleSet) > 0 {
			if _, ok := roleSet[msg.Role]; !ok {
				continue
			}
		}

		result = append(result, msg)
	}

	// Apply sort order.
	if opts.SortOrder == providers.SortDesc {
		sort.Slice(result, func(i, j int) bool {
			return result[i].SequenceNum > result[j].SequenceNum
		})
	} else {
		sort.Slice(result, func(i, j int) bool {
			return result[i].SequenceNum < result[j].SequenceNum
		})
	}

	// Apply offset.
	if opts.Offset > 0 && opts.Offset < len(result) {
		result = result[opts.Offset:]
	} else if opts.Offset >= len(result) {
		return nil
	}

	// Apply limit.
	if opts.Limit > 0 && opts.Limit < len(result) {
		result = result[:opts.Limit]
	}

	return result
}
