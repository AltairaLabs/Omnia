package checks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/doctor"
)

const (
	testSessionAPIID    = "11111111-2222-3333-4444-555555555555"
	testSessionNS       = "test-namespace"
	testSessionDocsBody = `<html><title>Omnia Session API</title></html>`
)

// --- mock server helpers ---

// sessionAPIMux builds an http.ServeMux that mimics the session-api responses.
type sessionAPIMuxConfig struct {
	docsStatus     int
	docsBody       string
	sessionsStatus int
	sessionsBody   interface{}
	messagesStatus int
	messagesBody   interface{}
	providerStatus int
	providerBody   interface{}
}

func defaultSessionAPIMux(t *testing.T, cfg sessionAPIMuxConfig) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/docs", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(cfg.docsStatus)
		_, _ = w.Write([]byte(cfg.docsBody))
	})

	mux.HandleFunc("/api/v1/sessions", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(cfg.sessionsStatus)
		if cfg.sessionsBody != nil {
			require.NoError(t, json.NewEncoder(w).Encode(cfg.sessionsBody))
		}
	})

	mux.HandleFunc("/api/v1/sessions/"+testSessionAPIID+"/messages", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(cfg.messagesStatus)
		if cfg.messagesBody != nil {
			require.NoError(t, json.NewEncoder(w).Encode(cfg.messagesBody))
		}
	})

	mux.HandleFunc("/api/v1/sessions/"+testSessionAPIID+"/provider-calls", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(cfg.providerStatus)
		if cfg.providerBody != nil {
			require.NoError(t, json.NewEncoder(w).Encode(cfg.providerBody))
		}
	})

	return httptest.NewServer(mux)
}

func goodSessionID() string  { return testSessionAPIID }
func emptySessionID() string { return "" }

// sessionListResponse is the real SessionListResponse shape from session-api.
type sessionListResponse struct {
	Sessions []map[string]interface{} `json:"sessions"`
	Total    int64                    `json:"total"`
	HasMore  bool                     `json:"hasMore"`
}

// messagesResponse is the real MessagesResponse shape.
type messagesResponse struct {
	Messages []map[string]interface{} `json:"messages"`
	HasMore  bool                     `json:"hasMore"`
}

// --- TestSessionCheckerChecks ---

func TestSessionCheckerChecks_ReturnsFour(t *testing.T) {
	c := NewSessionChecker("http://localhost", testSessionNS, goodSessionID)
	checks := c.Checks()
	require.Len(t, checks, 4)
	assert.Equal(t, "SessionAPIDocsServed", checks[0].Name)
	assert.Equal(t, "SessionCreated", checks[1].Name)
	assert.Equal(t, "MessagesRecorded", checks[2].Name)
	assert.Equal(t, "ProviderCallsTracked", checks[3].Name)
	for _, ch := range checks {
		assert.Equal(t, "Sessions", ch.Category)
	}
}

// --- checkDocs ---

func TestSessionCheckDocs_Pass(t *testing.T) {
	srv := defaultSessionAPIMux(t, sessionAPIMuxConfig{
		docsStatus: http.StatusOK,
		docsBody:   testSessionDocsBody,
	})
	defer srv.Close()

	c := NewSessionChecker(srv.URL, testSessionNS, goodSessionID)
	result := c.checkDocs(context.Background())
	assert.Equal(t, doctor.StatusPass, result.Status)
}

func TestCheckDocs_Fail_NotFound(t *testing.T) {
	srv := defaultSessionAPIMux(t, sessionAPIMuxConfig{
		docsStatus: http.StatusNotFound,
		docsBody:   "not found",
	})
	defer srv.Close()

	c := NewSessionChecker(srv.URL, testSessionNS, goodSessionID)
	result := c.checkDocs(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "404")
}

func TestCheckDocs_Fail_MissingText(t *testing.T) {
	srv := defaultSessionAPIMux(t, sessionAPIMuxConfig{
		docsStatus: http.StatusOK,
		docsBody:   "<html><title>Something Else</title></html>",
	})
	defer srv.Close()

	c := NewSessionChecker(srv.URL, testSessionNS, goodSessionID)
	result := c.checkDocs(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "Session API")
}

func TestCheckDocs_Fail_ConnectionError(t *testing.T) {
	c := NewSessionChecker("http://127.0.0.1:1", testSessionNS, goodSessionID)
	result := c.checkDocs(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.NotEmpty(t, result.Error)
}

// --- checkSessionExists ---

func TestCheckSessionExists_Pass(t *testing.T) {
	body := sessionListResponse{
		Sessions: []map[string]interface{}{{"id": testSessionAPIID}},
		Total:    1,
	}
	srv := defaultSessionAPIMux(t, sessionAPIMuxConfig{
		sessionsStatus: http.StatusOK,
		sessionsBody:   body,
	})
	defer srv.Close()

	c := NewSessionChecker(srv.URL, testSessionNS, goodSessionID)
	result := c.checkSessionExists(context.Background())
	assert.Equal(t, doctor.StatusPass, result.Status)
	assert.Contains(t, result.Detail, "1")
}

func TestCheckSessionExists_Fail_EmptyList(t *testing.T) {
	body := sessionListResponse{Sessions: []map[string]interface{}{}, Total: 0}
	srv := defaultSessionAPIMux(t, sessionAPIMuxConfig{
		sessionsStatus: http.StatusOK,
		sessionsBody:   body,
	})
	defer srv.Close()

	c := NewSessionChecker(srv.URL, testSessionNS, goodSessionID)
	result := c.checkSessionExists(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "no sessions")
}

func TestCheckSessionExists_Fail_HTTPError(t *testing.T) {
	srv := defaultSessionAPIMux(t, sessionAPIMuxConfig{
		sessionsStatus: http.StatusInternalServerError,
	})
	defer srv.Close()

	c := NewSessionChecker(srv.URL, testSessionNS, goodSessionID)
	result := c.checkSessionExists(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
}

func TestCheckSessionExists_Fail_ConnectionError(t *testing.T) {
	c := NewSessionChecker("http://127.0.0.1:1", testSessionNS, goodSessionID)
	result := c.checkSessionExists(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
}

func TestCheckSessionExists_Fail_BadJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/sessions", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewSessionChecker(srv.URL, testSessionNS, goodSessionID)
	result := c.checkSessionExists(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
}

// --- checkMessages ---

func TestCheckMessages_Pass(t *testing.T) {
	body := messagesResponse{
		Messages: []map[string]interface{}{
			{"role": "user", "content": "hello"},
			{"role": "assistant", "content": "hi"},
		},
	}
	srv := defaultSessionAPIMux(t, sessionAPIMuxConfig{
		messagesStatus: http.StatusOK,
		messagesBody:   body,
	})
	defer srv.Close()

	c := NewSessionChecker(srv.URL, testSessionNS, goodSessionID)
	result := c.checkMessages(context.Background())
	assert.Equal(t, doctor.StatusPass, result.Status)
	assert.Contains(t, result.Detail, "2")
}

func TestCheckMessages_Skip_NoSession(t *testing.T) {
	c := NewSessionChecker("http://127.0.0.1:1", testSessionNS, emptySessionID)
	result := c.checkMessages(context.Background())
	assert.Equal(t, doctor.StatusSkip, result.Status)
	assert.Equal(t, msgNoSessionAvailable, result.Detail)
}

func TestCheckMessages_Fail_MissingUserRole(t *testing.T) {
	body := messagesResponse{
		Messages: []map[string]interface{}{
			{"role": "assistant", "content": "response"},
		},
	}
	srv := defaultSessionAPIMux(t, sessionAPIMuxConfig{
		messagesStatus: http.StatusOK,
		messagesBody:   body,
	})
	defer srv.Close()

	c := NewSessionChecker(srv.URL, testSessionNS, goodSessionID)
	result := c.checkMessages(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "hasUser=false")
}

func TestCheckMessages_Fail_MissingAssistantRole(t *testing.T) {
	body := messagesResponse{
		Messages: []map[string]interface{}{
			{"role": "user", "content": "hello"},
		},
	}
	srv := defaultSessionAPIMux(t, sessionAPIMuxConfig{
		messagesStatus: http.StatusOK,
		messagesBody:   body,
	})
	defer srv.Close()

	c := NewSessionChecker(srv.URL, testSessionNS, goodSessionID)
	result := c.checkMessages(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "hasAssistant=false")
}

func TestCheckMessages_Fail_HTTPError(t *testing.T) {
	srv := defaultSessionAPIMux(t, sessionAPIMuxConfig{
		messagesStatus: http.StatusNotFound,
	})
	defer srv.Close()

	c := NewSessionChecker(srv.URL, testSessionNS, goodSessionID)
	result := c.checkMessages(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
}

func TestCheckMessages_Fail_ConnectionError(t *testing.T) {
	c := NewSessionChecker("http://127.0.0.1:1", testSessionNS, goodSessionID)
	result := c.checkMessages(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
}

func TestCheckMessages_Fail_BadJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/sessions/"+testSessionAPIID+"/messages", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewSessionChecker(srv.URL, testSessionNS, goodSessionID)
	result := c.checkMessages(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
}

// --- checkProviderCalls ---

func TestCheckProviderCalls_Pass(t *testing.T) {
	// provider-calls returns a bare JSON array
	body := []map[string]interface{}{
		{"inputTokens": 150, "outputTokens": 75},
	}
	srv := defaultSessionAPIMux(t, sessionAPIMuxConfig{
		providerStatus: http.StatusOK,
		providerBody:   body,
	})
	defer srv.Close()

	c := NewSessionChecker(srv.URL, testSessionNS, goodSessionID)
	result := c.checkProviderCalls(context.Background())
	assert.Equal(t, doctor.StatusPass, result.Status)
	assert.Contains(t, result.Detail, "1 provider call")
}

func TestCheckProviderCalls_Skip_NoSession(t *testing.T) {
	c := NewSessionChecker("http://127.0.0.1:1", testSessionNS, emptySessionID)
	result := c.checkProviderCalls(context.Background())
	assert.Equal(t, doctor.StatusSkip, result.Status)
	assert.Equal(t, msgNoSessionAvailable, result.Detail)
}

func TestCheckProviderCalls_Fail_Empty(t *testing.T) {
	body := []map[string]interface{}{}
	srv := defaultSessionAPIMux(t, sessionAPIMuxConfig{
		providerStatus: http.StatusOK,
		providerBody:   body,
	})
	defer srv.Close()

	c := NewSessionChecker(srv.URL, testSessionNS, goodSessionID)
	result := c.checkProviderCalls(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "no provider calls")
}

func TestCheckProviderCalls_Fail_ZeroTokens(t *testing.T) {
	body := []map[string]interface{}{
		{"inputTokens": 0, "outputTokens": 0},
	}
	srv := defaultSessionAPIMux(t, sessionAPIMuxConfig{
		providerStatus: http.StatusOK,
		providerBody:   body,
	})
	defer srv.Close()

	c := NewSessionChecker(srv.URL, testSessionNS, goodSessionID)
	result := c.checkProviderCalls(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "inputTokens > 0")
}

func TestCheckProviderCalls_Fail_HTTPError(t *testing.T) {
	srv := defaultSessionAPIMux(t, sessionAPIMuxConfig{
		providerStatus: http.StatusInternalServerError,
	})
	defer srv.Close()

	c := NewSessionChecker(srv.URL, testSessionNS, goodSessionID)
	result := c.checkProviderCalls(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
}

func TestCheckProviderCalls_Fail_ConnectionError(t *testing.T) {
	c := NewSessionChecker("http://127.0.0.1:1", testSessionNS, goodSessionID)
	result := c.checkProviderCalls(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
}

func TestCheckProviderCalls_Fail_BadJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/sessions/"+testSessionAPIID+"/provider-calls", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewSessionChecker(srv.URL, testSessionNS, goodSessionID)
	result := c.checkProviderCalls(context.Background())
	assert.Equal(t, doctor.StatusFail, result.Status)
}

// --- classifyMessages unit test ---

func TestClassifyMessages(t *testing.T) {
	type msg struct {
		Role string `json:"role"`
	}

	tests := []struct {
		name       string
		messages   []msg
		wantUser   bool
		wantAssist bool
	}{
		{
			name:       "both roles",
			messages:   []msg{{Role: "user"}, {Role: "assistant"}},
			wantUser:   true,
			wantAssist: true,
		},
		{
			name:       "user only",
			messages:   []msg{{Role: "user"}},
			wantUser:   true,
			wantAssist: false,
		},
		{
			name:       "assistant only",
			messages:   []msg{{Role: "assistant"}},
			wantUser:   false,
			wantAssist: true,
		},
		{
			name:       "empty",
			messages:   []msg{},
			wantUser:   false,
			wantAssist: false,
		},
		{
			name:       "system only",
			messages:   []msg{{Role: "system"}},
			wantUser:   false,
			wantAssist: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Convert to the type classifyMessages accepts.
			typed := make([]struct {
				Role string `json:"role"`
			}, len(tc.messages))
			for i, m := range tc.messages {
				typed[i].Role = m.Role
			}
			gotUser, gotAssist := classifyMessages(typed)
			assert.Equal(t, tc.wantUser, gotUser)
			assert.Equal(t, tc.wantAssist, gotAssist)
		})
	}
}
