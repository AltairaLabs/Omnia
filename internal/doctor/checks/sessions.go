package checks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/altairalabs/omnia/internal/doctor"
	"github.com/altairalabs/omnia/internal/session"
)

const (
	sessionTimeout = 10 * time.Second

	sessionAPIPathSessions = "/api/v1/sessions"
	sessionAPIPathDocs     = "/docs"
	sessionAPIPathMessages = "/messages"

	msgNoSessionAvailable = "no agent chat session available"
)

// SessionChecker runs checks against the session-api HTTP service.
type SessionChecker struct {
	sessionAPIURL string
	namespace     string
	store         session.Store // session store for provider-call queries
	// GetSessionID returns the session ID from the most recent agent chat check.
	GetSessionID  func() string
	lastSessionID string // populated by checkSessionExists as fallback
}

// NewSessionChecker creates a new SessionChecker.
func NewSessionChecker(sessionAPIURL, namespace string, store session.Store, getSessionID func() string) *SessionChecker {
	return &SessionChecker{
		sessionAPIURL: sessionAPIURL,
		namespace:     namespace,
		store:         store,
		GetSessionID:  getSessionID,
	}
}

// Checks returns the list of session-api checks.
func (s *SessionChecker) Checks() []doctor.Check {
	return []doctor.Check{
		{Name: "SessionAPIDocsServed", Category: "Sessions", Run: s.checkDocs},
		{Name: "SessionCreated", Category: "Sessions", Run: s.checkSessionExists},
		{Name: "SessionSearch", Category: "Sessions", Run: s.checkSessionSearch},
		{Name: "MessagesRecorded", Category: "Sessions", Run: s.checkMessages},
		{Name: "ProviderCallsTracked", Category: "Sessions", Run: s.checkProviderCalls},
	}
}

// sessionHTTPGet performs a GET against the session-api and returns the response body.
// The caller is responsible for closing the body.
func (s *SessionChecker) sessionHTTPGet(ctx context.Context, path string) (*http.Response, error) {
	client := &http.Client{Timeout: sessionTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.sessionAPIURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	return client.Do(req)
}

// checkDocs verifies GET /docs returns 200 and identifies itself as the Session API.
func (s *SessionChecker) checkDocs(ctx context.Context) doctor.TestResult {
	resp, err := s.sessionHTTPGet(ctx, sessionAPIPathDocs)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "GET /docs failed"}
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: fmt.Sprintf("GET /docs returned HTTP %d", resp.StatusCode),
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "reading /docs body"}
	}

	if !strings.Contains(string(body), "Session API") {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: "GET /docs body does not contain \"Session API\"",
		}
	}

	return doctor.TestResult{Status: doctor.StatusPass, Detail: "docs endpoint responding"}
}

// sessionListResult holds the common response shape for session list/search endpoints.
type sessionListResult struct {
	Sessions []struct {
		ID string `json:"id"`
	} `json:"sessions"`
	Total int64 `json:"total"`
}

// fetchSessionList performs a GET against a session list/search path and returns parsed results.
func (s *SessionChecker) fetchSessionList(ctx context.Context, path, label string) (*sessionListResult, *doctor.TestResult) {
	resp, err := s.sessionHTTPGet(ctx, path)
	if err != nil {
		r := doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: fmt.Sprintf("GET %s failed", label)}
		return nil, &r
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		r := doctor.TestResult{Status: doctor.StatusFail, Detail: fmt.Sprintf("GET %s returned HTTP %d", label, resp.StatusCode)}
		return nil, &r
	}

	var result sessionListResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		r := doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: fmt.Sprintf("decoding %s response", label)}
		return nil, &r
	}
	return &result, nil
}

// sessionCount returns the best available count from a session list result.
func (r *sessionListResult) sessionCount() int {
	if r.Total > 0 {
		return int(r.Total)
	}
	return len(r.Sessions)
}

// checkSessionExists verifies at least one session exists for the configured namespace.
func (s *SessionChecker) checkSessionExists(ctx context.Context) doctor.TestResult {
	path := fmt.Sprintf("%s?namespace=%s&limit=1", sessionAPIPathSessions, s.namespace)
	result, fail := s.fetchSessionList(ctx, path, "sessions")
	if fail != nil {
		return *fail
	}

	if len(result.Sessions) == 0 {
		return doctor.TestResult{Status: doctor.StatusFail, Detail: "no sessions found in namespace"}
	}

	if result.Sessions[0].ID != "" {
		s.lastSessionID = result.Sessions[0].ID
	}

	return doctor.TestResult{
		Status: doctor.StatusPass,
		Detail: fmt.Sprintf("%d session(s) found", result.sessionCount()),
	}
}

// checkSessionSearch verifies the session search endpoint returns results for a known greeting.
func (s *SessionChecker) checkSessionSearch(ctx context.Context) doctor.TestResult {
	path := fmt.Sprintf("/api/v1/sessions/search?namespace=%s&q=Hello", s.namespace)
	result, fail := s.fetchSessionList(ctx, path, "search")
	if fail != nil {
		return *fail
	}

	if len(result.Sessions) == 0 {
		return doctor.TestResult{Status: doctor.StatusFail, Detail: "search returned no results for 'Hello'"}
	}

	return doctor.TestResult{
		Status: doctor.StatusPass,
		Detail: fmt.Sprintf("search found %d session(s) matching 'Hello'", result.sessionCount()),
	}
}

// resolveSessionID returns the best available session ID.
// Prefers the session from the list endpoint (known to exist in session-api)
// over the WS session ID (which may not be persisted yet).
func (s *SessionChecker) resolveSessionID() string {
	if s.lastSessionID != "" {
		return s.lastSessionID
	}
	return s.GetSessionID()
}

// checkMessages verifies that the session has both user and assistant messages recorded.
func (s *SessionChecker) checkMessages(ctx context.Context) doctor.TestResult {
	sessionID := s.resolveSessionID()
	if sessionID == "" {
		return doctor.TestResult{Status: doctor.StatusSkip, Detail: msgNoSessionAvailable}
	}

	path := fmt.Sprintf("%s/%s%s", sessionAPIPathSessions, sessionID, sessionAPIPathMessages)
	resp, err := s.sessionHTTPGet(ctx, path)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "GET messages failed"}
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: fmt.Sprintf("GET messages returned HTTP %d", resp.StatusCode),
		}
	}

	var result struct {
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "decoding messages response"}
	}

	hasUser, hasAssistant := classifyMessages(result.Messages)

	if !hasUser || !hasAssistant {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: fmt.Sprintf("missing roles: hasUser=%v hasAssistant=%v", hasUser, hasAssistant),
		}
	}

	return doctor.TestResult{
		Status: doctor.StatusPass,
		Detail: fmt.Sprintf("%d messages recorded with user and assistant roles", len(result.Messages)),
	}
}

// classifyMessages returns whether user and assistant roles are present.
func classifyMessages(messages []struct {
	Role string `json:"role"`
}) (hasUser, hasAssistant bool) {
	for _, m := range messages {
		switch m.Role {
		case "user":
			hasUser = true
		case "assistant":
			hasAssistant = true
		}
	}
	return
}

// checkProviderCalls verifies the session has at least one provider call with non-zero input tokens.
func (s *SessionChecker) checkProviderCalls(ctx context.Context) doctor.TestResult {
	sessionID := s.resolveSessionID()
	if sessionID == "" {
		return doctor.TestResult{Status: doctor.StatusSkip, Detail: msgNoSessionAvailable}
	}

	if s.store == nil {
		return doctor.TestResult{Status: doctor.StatusSkip, Detail: "no session store available"}
	}

	calls, err := s.store.GetProviderCalls(ctx, sessionID, 0, 0)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "get provider calls failed"}
	}

	if len(calls) == 0 {
		return doctor.TestResult{Status: doctor.StatusFail, Detail: "no provider calls recorded"}
	}

	if !anyProviderCallHasTokens(calls) {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: fmt.Sprintf("%d provider call(s) found but none have inputTokens > 0", len(calls)),
		}
	}

	return doctor.TestResult{
		Status: doctor.StatusPass,
		Detail: fmt.Sprintf("%d provider call(s) with token usage recorded", len(calls)),
	}
}

// anyProviderCallHasTokens returns true if at least one call has InputTokens > 0.
func anyProviderCallHasTokens(calls []session.ProviderCall) bool {
	for _, c := range calls {
		if c.InputTokens > 0 {
			return true
		}
	}
	return false
}
