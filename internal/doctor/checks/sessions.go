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
)

const (
	sessionTimeout = 10 * time.Second

	sessionAPIPathSessions  = "/api/v1/sessions"
	sessionAPIPathDocs      = "/docs"
	sessionAPIPathMessages  = "/messages"
	sessionAPIPathProviders = "/provider-calls"

	msgNoSessionAvailable = "no agent chat session available"
)

// SessionChecker runs checks against the session-api HTTP service.
type SessionChecker struct {
	sessionAPIURL string
	namespace     string
	// GetSessionID returns the session ID from the most recent agent chat check.
	GetSessionID func() string
}

// NewSessionChecker creates a new SessionChecker.
func NewSessionChecker(sessionAPIURL, namespace string, getSessionID func() string) *SessionChecker {
	return &SessionChecker{
		sessionAPIURL: sessionAPIURL,
		namespace:     namespace,
		GetSessionID:  getSessionID,
	}
}

// Checks returns the list of session-api checks.
func (s *SessionChecker) Checks() []doctor.Check {
	return []doctor.Check{
		{Name: "SessionAPIDocsServed", Category: "Sessions", Run: s.checkDocs},
		{Name: "SessionCreated", Category: "Sessions", Run: s.checkSessionExists},
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

// checkSessionExists verifies at least one session exists for the configured namespace.
func (s *SessionChecker) checkSessionExists(ctx context.Context) doctor.TestResult {
	path := fmt.Sprintf("%s?namespace=%s&limit=1", sessionAPIPathSessions, s.namespace)
	resp, err := s.sessionHTTPGet(ctx, path)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "GET sessions failed"}
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: fmt.Sprintf("GET sessions returned HTTP %d", resp.StatusCode),
		}
	}

	var result struct {
		Sessions []json.RawMessage `json:"sessions"`
		Total    int64             `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "decoding sessions response"}
	}

	if len(result.Sessions) == 0 {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: "no sessions found in namespace",
		}
	}

	count := len(result.Sessions)
	if result.Total > 0 {
		count = int(result.Total)
	}
	return doctor.TestResult{
		Status: doctor.StatusPass,
		Detail: fmt.Sprintf("%d session(s) found", count),
	}
}

// checkMessages verifies that the session has both user and assistant messages recorded.
func (s *SessionChecker) checkMessages(ctx context.Context) doctor.TestResult {
	sessionID := s.GetSessionID()
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
	sessionID := s.GetSessionID()
	if sessionID == "" {
		return doctor.TestResult{Status: doctor.StatusSkip, Detail: msgNoSessionAvailable}
	}

	path := fmt.Sprintf("%s/%s%s", sessionAPIPathSessions, sessionID, sessionAPIPathProviders)
	resp, err := s.sessionHTTPGet(ctx, path)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "GET provider-calls failed"}
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: fmt.Sprintf("GET provider-calls returned HTTP %d", resp.StatusCode),
		}
	}

	var calls []struct {
		InputTokens int64 `json:"inputTokens"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&calls); err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "decoding provider-calls response"}
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
func anyProviderCallHasTokens(calls []struct {
	InputTokens int64 `json:"inputTokens"`
}) bool {
	for _, c := range calls {
		if c.InputTokens > 0 {
			return true
		}
	}
	return false
}
