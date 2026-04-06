package checks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/doctor"
)

// newPrivacyCheckerForServer creates a PrivacyChecker pointing at the given test server.
func newPrivacyCheckerForServer(srv *httptest.Server) *PrivacyChecker {
	return NewPrivacyChecker(srv.URL, "", testWorkspace)
}

// privacySaveBody captures a decoded save request body for assertions.
type privacySaveBody struct {
	Type    string                 `json:"type"`
	Content string                 `json:"content"`
	Scope   map[string]interface{} `json:"scope"`
}

// mockPrivacyServer builds a minimal httptest.Server for privacy check tests.
// handlers map path strings to handler funcs; the save endpoint dispatches on method.
type mockPrivacyServer struct {
	saveHandler        http.HandlerFunc
	searchHandler      http.HandlerFunc
	batchDeleteHandler http.HandlerFunc
	auditHandler       http.HandlerFunc
}

func (m *mockPrivacyServer) serve(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	saveH := m.saveHandler
	if saveH == nil {
		saveH = func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"memory":{"id":"priv-test-id"}}`))
		}
	}

	searchH := m.searchHandler
	if searchH == nil {
		searchH = func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"memories":[],"total":0}`))
		}
	}

	mux.HandleFunc(privacyMemoriesPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			saveH(w, r)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc(privacyMemorySearchPath, searchH)

	if m.batchDeleteHandler != nil {
		mux.HandleFunc(privacyBatchDeletePath, m.batchDeleteHandler)
	}

	if m.auditHandler != nil {
		mux.HandleFunc("/api/v1/audit/memories", m.auditHandler)
	}

	return httptest.NewServer(mux)
}

// --- MemoryPIIRedaction ---

func TestCheckPIIRedaction_Pass(t *testing.T) {
	// Save succeeds; search returns content without SSN.
	srv := (&mockPrivacyServer{
		searchHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"memories":[{"id":"m1","content":"patient ssn is [REDACTED]"}],"total":1}`))
		},
	}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv).checkPIIRedaction(t.Context())
	assert.Equal(t, doctor.StatusPass, result.Status)
	assert.Contains(t, result.Detail, "redacted")
}

func TestCheckPIIRedaction_Fail_SSNPresent(t *testing.T) {
	// Search returns content still containing the SSN.
	srv := (&mockPrivacyServer{
		searchHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			body, _ := json.Marshal(map[string]interface{}{
				"memories": []map[string]interface{}{
					{"id": "m1", "content": "patient ssn is " + privacyTestSSN},
				},
				"total": 1,
			})
			_, _ = w.Write(body)
		},
	}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv).checkPIIRedaction(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "unredacted")
}

func TestCheckPIIRedaction_Fail_SaveError(t *testing.T) {
	srv := (&mockPrivacyServer{
		saveHandler: func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "internal error", http.StatusInternalServerError)
		},
	}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv).checkPIIRedaction(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
}

func TestCheckPIIRedaction_Skip_NoWorkspace(t *testing.T) {
	c := NewPrivacyChecker("http://localhost:8080", "", "")
	result := c.checkPIIRedaction(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
	assert.Contains(t, result.Detail, "workspace UID not resolved")
}

func TestCheckPIIRedaction_Skip_SearchUnavailable(t *testing.T) {
	// Save succeeds but search returns 500 — should skip (search unavailable).
	srv := (&mockPrivacyServer{
		searchHandler: func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "search down", http.StatusInternalServerError)
		},
	}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv).checkPIIRedaction(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
	assert.Contains(t, result.Detail, "unavailable")
}

func TestCheckPIIRedaction_SendsSSNInContent(t *testing.T) {
	var captured privacySaveBody
	srv := (&mockPrivacyServer{
		saveHandler: func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"memory":{"id":"x"}}`))
		},
	}).serve(t)
	defer srv.Close()

	_ = newPrivacyCheckerForServer(srv).checkPIIRedaction(t.Context())
	assert.Contains(t, captured.Content, privacyTestSSN)
}

// --- MemoryOptOutRespected ---

func TestCheckOptOutRespected_Pass(t *testing.T) {
	// Mock session-api accepts opt-out, mock memory-api rejects save with 204.
	sessionSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusNoContent)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer sessionSrv.Close()

	memorySrv := (&mockPrivacyServer{
		saveHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	}).serve(t)
	defer memorySrv.Close()

	c := NewPrivacyChecker(memorySrv.URL, sessionSrv.URL, testWorkspace)
	result := c.checkOptOutRespected(t.Context())
	assert.Equal(t, doctor.StatusPass, result.Status)
	assert.Contains(t, result.Detail, "204")
}

func TestCheckOptOutRespected_Fail_SavedDespiteOptOut(t *testing.T) {
	// Session-api accepts opt-out, but memory-api saves anyway (middleware broken).
	sessionSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer sessionSrv.Close()

	memorySrv := (&mockPrivacyServer{}).serve(t)
	defer memorySrv.Close()

	c := NewPrivacyChecker(memorySrv.URL, sessionSrv.URL, testWorkspace)
	result := c.checkOptOutRespected(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "despite")
}

func TestCheckOptOutRespected_Skip_SessionAPIUnreachable(t *testing.T) {
	memorySrv := (&mockPrivacyServer{}).serve(t)
	defer memorySrv.Close()

	c := NewPrivacyChecker(memorySrv.URL, "https://127.0.0.1:1", testWorkspace)
	result := c.checkOptOutRespected(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
}

func TestCheckOptOutRespected_Skip_NoWorkspace(t *testing.T) {
	c := NewPrivacyChecker("https://localhost:8080", "", "")
	result := c.checkOptOutRespected(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
}

// --- MemoryDeletionCascade ---

func TestCheckDeletionCascade_Pass(t *testing.T) {
	srv := (&mockPrivacyServer{
		batchDeleteHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"deleted":1}`))
		},
		// search after delete returns empty.
		searchHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"memories":[],"total":0}`))
		},
	}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv).checkDeletionCascade(t.Context())
	assert.Equal(t, doctor.StatusPass, result.Status)
	assert.Contains(t, result.Detail, "absent")
}

func TestCheckDeletionCascade_Skip_BatchEndpointNotFound(t *testing.T) {
	srv := (&mockPrivacyServer{
		batchDeleteHandler: func(w http.ResponseWriter, _ *http.Request) {
			http.NotFound(w, nil)
		},
	}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv).checkDeletionCascade(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
	assert.Contains(t, result.Detail, "batch delete endpoint not available")
}

func TestCheckDeletionCascade_Fail_MemoryStillPresent(t *testing.T) {
	srv := (&mockPrivacyServer{
		batchDeleteHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"deleted":0}`))
		},
		searchHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"memories":[{"id":"m1","content":"deletion cascade test"}],"total":1}`))
		},
	}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv).checkDeletionCascade(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "still present")
}

func TestCheckDeletionCascade_Fail_SaveFails(t *testing.T) {
	srv := (&mockPrivacyServer{
		saveHandler: func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "save error", http.StatusInternalServerError)
		},
	}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv).checkDeletionCascade(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "save failed")
}

func TestCheckDeletionCascade_Fail_BatchDeleteError(t *testing.T) {
	srv := (&mockPrivacyServer{
		batchDeleteHandler: func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "delete error", http.StatusInternalServerError)
		},
	}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv).checkDeletionCascade(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
}

func TestCheckDeletionCascade_Skip_NoWorkspace(t *testing.T) {
	c := NewPrivacyChecker("http://localhost:8080", "", "")
	result := c.checkDeletionCascade(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
}

// --- AuditLogWritten ---

func TestCheckAuditLogWritten_Pass(t *testing.T) {
	srv := (&mockPrivacyServer{
		auditHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			// Return entry with memory_id matching the test save response ("priv-test-id").
			_, _ = w.Write([]byte(`{"entries":[{"eventType":"memory_created","memory_id":"priv-test-id"}],"total":1,"hasMore":false}`))
		},
	}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv).checkAuditLogWritten(t.Context())
	assert.Equal(t, doctor.StatusPass, result.Status)
	assert.Contains(t, result.Detail, "priv-test-id")
}

func TestCheckAuditLogWritten_Fail_NoEvents(t *testing.T) {
	srv := (&mockPrivacyServer{
		auditHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"entries":[],"total":0,"hasMore":false}`))
		},
	}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv).checkAuditLogWritten(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "no audit event found")
}

func TestCheckAuditLogWritten_Skip_EndpointNotAvailable(t *testing.T) {
	// No audit handler registered → 404 → skip.
	srv := (&mockPrivacyServer{}).serve(t)
	defer srv.Close()

	result := newPrivacyCheckerForServer(srv).checkAuditLogWritten(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
	assert.Contains(t, result.Detail, "audit endpoint not available")
}

func TestCheckAuditLogWritten_Skip_NoWorkspace(t *testing.T) {
	c := NewPrivacyChecker("http://localhost:8080", "", "")
	result := c.checkAuditLogWritten(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
}

// --- Checks() registration ---

func TestPrivacyChecker_Checks_ReturnsFour(t *testing.T) {
	c := NewPrivacyChecker("http://localhost:8080", "", "ws1")
	cs := c.Checks()
	require.Len(t, cs, 4)
	names := make([]string, len(cs))
	for i, ch := range cs {
		names[i] = ch.Name
	}
	assert.Equal(t, []string{
		"MemoryPIIRedaction",
		"MemoryOptOutRespected",
		"MemoryDeletionCascade",
		"AuditLogWritten",
	}, names)
	for _, ch := range cs {
		assert.Equal(t, privacyCategory, ch.Category)
	}
}
