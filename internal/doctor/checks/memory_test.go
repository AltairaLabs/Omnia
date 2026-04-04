package checks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"

	"github.com/altairalabs/omnia/internal/doctor"
	memoryhttpclient "github.com/altairalabs/omnia/internal/memory/httpclient"
	"github.com/altairalabs/omnia/internal/session"
)

const (
	testMemoryID       = "mem-test-abc"
	testWorkspace      = "ws-test-1"
	testContentDisp    = `attachment; filename="memories.json"`
	testMemoryDocsBody = `<html><h1>Memory API</h1></html>`
	testSearchBody     = `{"memories":[{"id":"mem-test-abc","content":"doctor smoke test value"}],"total":1}`
	testListBody       = `{"memories":[{"id":"mem-test-abc","content":"doctor smoke test value"}],"total":1}`
)

// mockMemoryServer builds an httptest.Server that serves a complete, happy-path
// memory-api. Individual test cases may override handlers for failure scenarios.
type mockMemoryServer struct {
	// handler fields — nil means use the default; non-nil overrides the route.
	docsHandler   http.HandlerFunc
	saveHandler   http.HandlerFunc
	searchHandler http.HandlerFunc
	listHandler   http.HandlerFunc
	deleteHandler http.HandlerFunc
	exportHandler http.HandlerFunc
}

func (m *mockMemoryServer) serve(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	docsH := m.docsHandler
	if docsH == nil {
		docsH = defaultDocsHandler
	}
	mux.HandleFunc("/docs", docsH)

	saveH := m.saveHandler
	if saveH == nil {
		saveH = defaultSaveHandler
	}

	searchH := m.searchHandler
	if searchH == nil {
		searchH = defaultSearchHandler
	}

	listH := m.listHandler
	if listH == nil {
		listH = defaultListHandler
	}

	deleteH := m.deleteHandler
	if deleteH == nil {
		deleteH = defaultDeleteHandler
	}

	exportH := m.exportHandler
	if exportH == nil {
		exportH = defaultExportHandler
	}

	// /api/v1/memories and its sub-paths.
	mux.HandleFunc("/api/v1/memories/search", searchH)
	mux.HandleFunc("/api/v1/memories/export", exportH)
	mux.HandleFunc("/api/v1/memories/", deleteH) // DELETE /api/v1/memories/{id}
	mux.HandleFunc("/api/v1/memories", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			saveH(w, r)
		case http.MethodGet:
			listH(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	return httptest.NewServer(mux)
}

func defaultDocsHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(testMemoryDocsBody))
}

func defaultSaveHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"memory":{"id":"` + testMemoryID + `"}}`))
}

func defaultSearchHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(testSearchBody))
}

func defaultListHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(testListBody))
}

func defaultDeleteHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{}`))
}

func defaultExportHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Disposition", testContentDisp)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`[]`))
}

// newCheckerForMemoryServer creates a MemoryChecker pointing at the given server.
func newCheckerForMemoryServer(srv *httptest.Server) *MemoryChecker {
	store := memoryhttpclient.NewStore(srv.URL, logr.Discard())
	return NewMemoryChecker(srv.URL, store, testWorkspace, nil)
}

// --- MemoryAPIDocsServed ---

func TestCheckMemoryDocs_Pass(t *testing.T) {
	srv := (&mockMemoryServer{}).serve(t)
	defer srv.Close()

	result := newCheckerForMemoryServer(srv).checkDocs(t.Context())
	assert.Equal(t, doctor.StatusPass, result.Status)
}

func TestCheckMemoryDocs_Fail_NoContent(t *testing.T) {
	srv := (&mockMemoryServer{
		docsHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<html>Some other page</html>"))
		},
	}).serve(t)
	defer srv.Close()

	result := newCheckerForMemoryServer(srv).checkDocs(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "Memory API")
}

func TestCheckMemoryDocs_Fail_ServerError(t *testing.T) {
	srv := (&mockMemoryServer{
		docsHandler: func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "oops", http.StatusInternalServerError)
		},
	}).serve(t)
	defer srv.Close()

	result := newCheckerForMemoryServer(srv).checkDocs(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
}

// --- MemorySave ---

func TestCheckSave_Pass(t *testing.T) {
	srv := (&mockMemoryServer{}).serve(t)
	defer srv.Close()

	checker := newCheckerForMemoryServer(srv)
	result := checker.checkSave(t.Context())
	assert.Equal(t, doctor.StatusPass, result.Status)
	assert.Equal(t, testMemoryID, checker.savedMemoryID)
}

func TestCheckSave_Fail_ServerError(t *testing.T) {
	srv := (&mockMemoryServer{
		saveHandler: func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "oops", http.StatusInternalServerError)
		},
	}).serve(t)
	defer srv.Close()

	checker := newCheckerForMemoryServer(srv)
	result := checker.checkSave(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Empty(t, checker.savedMemoryID)
}

func TestCheckSave_Fail_MissingID(t *testing.T) {
	srv := (&mockMemoryServer{
		saveHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{}`))
		},
	}).serve(t)
	defer srv.Close()

	result := newCheckerForMemoryServer(srv).checkSave(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "missing id")
}

func TestCheckSave_SendsCorrectPayload(t *testing.T) {
	var captured pkmemory.Memory
	srv := (&mockMemoryServer{
		saveHandler: func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"memory":{"id":"x"}}`))
		},
	}).serve(t)
	defer srv.Close()

	_ = newCheckerForMemoryServer(srv).checkSave(t.Context())
	assert.Equal(t, memoryTestType, captured.Type)
	assert.Equal(t, memoryTestValue, captured.Content)
	assert.InDelta(t, 0.95, captured.Confidence, 0.001)
	assert.Equal(t, testWorkspace, captured.Scope["workspace_id"])
}

// --- MemoryRetrieve ---

func TestCheckRetrieve_Pass(t *testing.T) {
	srv := (&mockMemoryServer{}).serve(t)
	defer srv.Close()

	result := newCheckerForMemoryServer(srv).checkRetrieve(t.Context())
	assert.Equal(t, doctor.StatusPass, result.Status)
}

func TestCheckRetrieve_Fail_NoResults(t *testing.T) {
	srv := (&mockMemoryServer{
		searchHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results":[]}`))
		},
	}).serve(t)
	defer srv.Close()

	result := newCheckerForMemoryServer(srv).checkRetrieve(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "no results")
}

func TestCheckRetrieve_Fail_ServerError(t *testing.T) {
	srv := (&mockMemoryServer{
		searchHandler: func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "fail", http.StatusInternalServerError)
		},
	}).serve(t)
	defer srv.Close()

	result := newCheckerForMemoryServer(srv).checkRetrieve(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
}

// --- MemoryList ---

func TestCheckList_Pass(t *testing.T) {
	srv := (&mockMemoryServer{}).serve(t)
	defer srv.Close()

	result := newCheckerForMemoryServer(srv).checkList(t.Context())
	assert.Equal(t, doctor.StatusPass, result.Status)
}

func TestCheckList_Fail_Empty(t *testing.T) {
	srv := (&mockMemoryServer{
		listHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"items":[]}`))
		},
	}).serve(t)
	defer srv.Close()

	result := newCheckerForMemoryServer(srv).checkList(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "0 items")
}

func TestCheckList_Fail_ServerError(t *testing.T) {
	srv := (&mockMemoryServer{
		listHandler: func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "fail", http.StatusInternalServerError)
		},
	}).serve(t)
	defer srv.Close()

	result := newCheckerForMemoryServer(srv).checkList(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
}

// --- MemoryDelete ---

func TestCheckDelete_Pass(t *testing.T) {
	srv := (&mockMemoryServer{}).serve(t)
	defer srv.Close()

	checker := newCheckerForMemoryServer(srv)
	checker.savedMemoryID = testMemoryID
	result := checker.checkDelete(t.Context())
	assert.Equal(t, doctor.StatusPass, result.Status)
}

func TestCheckDelete_Skip_NoSavedID(t *testing.T) {
	srv := (&mockMemoryServer{}).serve(t)
	defer srv.Close()

	result := newCheckerForMemoryServer(srv).checkDelete(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
	assert.Contains(t, result.Detail, "no memory to delete")
}

func TestCheckDelete_Fail_ServerError(t *testing.T) {
	srv := (&mockMemoryServer{
		deleteHandler: func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "fail", http.StatusInternalServerError)
		},
	}).serve(t)
	defer srv.Close()

	checker := newCheckerForMemoryServer(srv)
	checker.savedMemoryID = testMemoryID
	result := checker.checkDelete(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
}

// --- MemoryExport ---

func TestCheckExport_Pass(t *testing.T) {
	srv := (&mockMemoryServer{}).serve(t)
	defer srv.Close()

	result := newCheckerForMemoryServer(srv).checkExport(t.Context())
	assert.Equal(t, doctor.StatusPass, result.Status)
	assert.Contains(t, result.Detail, "attachment")
}

func TestCheckExport_Fail_NoContentDisposition(t *testing.T) {
	srv := (&mockMemoryServer{
		exportHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		},
	}).serve(t)
	defer srv.Close()

	result := newCheckerForMemoryServer(srv).checkExport(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "Content-Disposition")
}

func TestCheckExport_Fail_ServerError(t *testing.T) {
	srv := (&mockMemoryServer{
		exportHandler: func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "fail", http.StatusInternalServerError)
		},
	}).serve(t)
	defer srv.Close()

	result := newCheckerForMemoryServer(srv).checkExport(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
}

// --- User Ownership tests ---

func TestCheckUserOwnership_Pass(t *testing.T) {
	srv := (&mockMemoryServer{
		searchHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"memories":[{"id":"` + testMemoryID + `","type":"doctor-test","content":"ownership test","scope":{"workspace_id":"ws1","user_id":"doctor-test-user"}}],"total":1}`))
		},
	}).serve(t)
	defer srv.Close()

	result := newCheckerForMemoryServer(srv).checkUserOwnership(t.Context())
	assert.Equal(t, doctor.StatusPass, result.Status)
	assert.Contains(t, result.Detail, "user_id scope")
}

func TestCheckUserOwnership_Fail_NoUserID(t *testing.T) {
	srv := (&mockMemoryServer{
		searchHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"memories":[{"id":"` + testMemoryID + `","type":"doctor-test","content":"ownership test","scope":{"workspace_id":"ws1"}}],"total":1}`))
		},
	}).serve(t)
	defer srv.Close()

	result := newCheckerForMemoryServer(srv).checkUserOwnership(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "without user_id")
}

func TestCheckUserOwnership_Skip_NoWorkspace(t *testing.T) {
	c := NewMemoryChecker("http://localhost:8080", nil, "", nil)
	result := c.checkUserOwnership(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
}

// --- User Isolation tests ---

func TestCheckUserIsolation_Pass(t *testing.T) {
	srv := (&mockMemoryServer{
		searchHandler: func(w http.ResponseWriter, r *http.Request) {
			uid := r.URL.Query().Get("user_id")
			if uid == memoryOtherUserID {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"memories":[],"total":0}`))
			} else {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"memories":[{"id":"iso-1"}],"total":1}`))
			}
		},
	}).serve(t)
	defer srv.Close()

	result := newCheckerForMemoryServer(srv).checkUserIsolation(t.Context())
	assert.Equal(t, doctor.StatusPass, result.Status)
	assert.Contains(t, result.Detail, "isolated")
}

func TestCheckUserIsolation_Fail_LeaksToOtherUser(t *testing.T) {
	srv := (&mockMemoryServer{
		searchHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"memories":[{"id":"iso-2","content":"isolation test secret"}],"total":1}`))
		},
	}).serve(t)
	defer srv.Close()

	result := newCheckerForMemoryServer(srv).checkUserIsolation(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "isolation violated")
}

func TestCheckUserIsolation_Skip_NoWorkspace(t *testing.T) {
	c := NewMemoryChecker("http://localhost:8080", nil, "", nil)
	result := c.checkUserIsolation(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
}

// --- Checks() registration ---

func TestChecks_NoAgentChecker_ReturnsRestOnly(t *testing.T) {
	c := NewMemoryChecker("http://localhost:8080", nil, "ws1", nil)
	checks := c.Checks()
	require.Len(t, checks, 8)
	names := make([]string, len(checks))
	for i, ch := range checks {
		names[i] = ch.Name
	}
	assert.Equal(t, []string{
		"MemoryAPIDocsServed",
		"MemorySave",
		"MemoryRetrieve",
		"MemoryList",
		"MemoryDelete",
		"MemoryExport",
		"MemoryUserOwnership",
		"MemoryUserIsolation",
	}, names)
}

func TestChecks_WithAgentChecker_ReturnsAllChecks(t *testing.T) {
	agentChecker := NewAgentChecker(AgentConfig{})
	c := NewMemoryChecker("http://localhost:8080", nil, "ws1", agentChecker)
	checks := c.Checks()
	require.Len(t, checks, 11)
	assert.Equal(t, "MemoryUserOwnership", checks[6].Name)
	assert.Equal(t, "MemoryUserIsolation", checks[7].Name)
	assert.Equal(t, "MemoryToolsAvailable", checks[8].Name)
	assert.Equal(t, "MemoryRecall", checks[9].Name)
	assert.Equal(t, "MemoryPersistsAcrossSessions", checks[10].Name)
}

// --- Tool checks (memory agent via WebSocket) ---

func TestCheckMemoryToolsAvailable_Pass(t *testing.T) {
	// Mock facade: agent responds to "remember" prompt.
	facadeSrv := serveMockFacade(t, mockFacadeHandler{
		responses: []wsServerMessage{
			{Type: wsMessageTypeDone, Content: "Remembered."},
		},
	})
	defer facadeSrv.Close()

	// Mock memory-api: search returns the remembered value.
	memorySrv := (&mockMemoryServer{
		searchHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"memories":[{"id":"m1","content":"doctor test value is smoke-42"}],"total":1}`))
		},
	}).serve(t)
	defer memorySrv.Close()

	agentChecker := newCheckerForServer(facadeSrv)
	memStore := memoryhttpclient.NewStore(memorySrv.URL, logr.Discard())
	c := NewMemoryChecker(memorySrv.URL, memStore, testWorkspace, agentChecker)
	result := c.checkMemoryToolsAvailable(t.Context())
	assert.Equal(t, doctor.StatusPass, result.Status)
}

func TestCheckMemoryToolsAvailable_Fail_NotPersisted(t *testing.T) {
	facadeSrv := serveMockFacade(t, mockFacadeHandler{
		responses: []wsServerMessage{
			{Type: wsMessageTypeDone, Content: "OK."},
		},
	})
	defer facadeSrv.Close()

	// Memory-api returns no results — the remember didn't persist.
	memorySrv := (&mockMemoryServer{
		searchHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"memories":[],"total":0}`))
		},
	}).serve(t)
	defer memorySrv.Close()

	agentChecker := newCheckerForServer(facadeSrv)
	memStore := memoryhttpclient.NewStore(memorySrv.URL, logr.Discard())
	c := NewMemoryChecker(memorySrv.URL, memStore, testWorkspace, agentChecker)
	result := c.checkMemoryToolsAvailable(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "did not persist")
}

func TestCheckMemoryToolsAvailable_Fail_ToolCallError(t *testing.T) {
	facadeSrv := serveMockFacade(t, mockFacadeHandler{
		responses: []wsServerMessage{
			{Type: wsMessageTypeDone, Content: "I tried to remember."},
		},
	})
	defer facadeSrv.Close()

	// Memory-api returns no results (remember failed).
	memorySrv := (&mockMemoryServer{
		searchHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"memories":[],"total":0}`))
		},
	}).serve(t)
	defer memorySrv.Close()

	agentChecker := newCheckerForServer(facadeSrv)
	agentChecker.config.SessionStore = &MockStore{
		ToolCalls: []session.ToolCall{
			{Name: "memory__remember", Status: session.ToolCallStatusError, ErrorMessage: "validation error: metadata: Invalid type"},
		},
	}
	memStore := memoryhttpclient.NewStore(memorySrv.URL, logr.Discard())
	c := NewMemoryChecker(memorySrv.URL, memStore, testWorkspace, agentChecker)
	result := c.checkMemoryToolsAvailable(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "validation error")
}

func TestCheckMemoryToolsAvailable_Fail_ToolCallErrorFallbackResult(t *testing.T) {
	facadeSrv := serveMockFacade(t, mockFacadeHandler{
		responses: []wsServerMessage{
			{Type: wsMessageTypeDone, Content: "OK."},
		},
	})
	defer facadeSrv.Close()

	memorySrv := (&mockMemoryServer{
		searchHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"memories":[],"total":0}`))
		},
	}).serve(t)
	defer memorySrv.Close()

	agentChecker := newCheckerForServer(facadeSrv)
	agentChecker.config.SessionStore = &MockStore{
		ToolCalls: []session.ToolCall{
			{Name: "memory__remember", Status: session.ToolCallStatusError, Result: "Tool execution failed: some error"},
		},
	}
	memStore := memoryhttpclient.NewStore(memorySrv.URL, logr.Discard())
	c := NewMemoryChecker(memorySrv.URL, memStore, testWorkspace, agentChecker)
	result := c.checkMemoryToolsAvailable(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "Tool execution failed")
}

func TestCheckMemoryToolsAvailable_Fail_ConnectionError(t *testing.T) {
	agentChecker := NewAgentChecker(AgentConfig{FacadeURL: "http://127.0.0.1:1", AgentName: "x", Namespace: "y"})
	memStore := memoryhttpclient.NewStore("http://localhost:9999", logr.Discard())
	c := NewMemoryChecker("http://localhost:9999", memStore, testWorkspace, agentChecker)
	result := c.checkMemoryToolsAvailable(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
}

func TestCheckMemoryToolsAvailable_Skip_NoWorkspace(t *testing.T) {
	agentChecker := NewAgentChecker(AgentConfig{})
	c := NewMemoryChecker("http://localhost:8080", nil, "", agentChecker)
	result := c.checkMemoryToolsAvailable(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
}

func TestCheckMemoryRecall_Pass(t *testing.T) {
	srv := serveMockFacade(t, mockFacadeHandler{
		responses: []wsServerMessage{
			{Type: wsMessageTypeChunk, Content: "Your doctor test value is "},
			{Type: wsMessageTypeDone, Content: "smoke-42"},
		},
	})
	defer srv.Close()

	agentChecker := newCheckerForServer(srv)
	c := NewMemoryChecker("", nil, testWorkspace, agentChecker)
	result := c.checkMemoryRecall(t.Context())
	assert.Equal(t, doctor.StatusPass, result.Status)
}

func TestCheckMemoryRecall_Fail_ValueNotInResponse(t *testing.T) {
	srv := serveMockFacade(t, mockFacadeHandler{
		responses: []wsServerMessage{
			{Type: wsMessageTypeDone, Content: "I don't recall anything."},
		},
	})
	defer srv.Close()

	agentChecker := newCheckerForServer(srv)
	c := NewMemoryChecker("", nil, testWorkspace, agentChecker)
	result := c.checkMemoryRecall(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "smoke-42")
}

func TestCheckMemoryRecall_Fail_ConnectionError(t *testing.T) {
	agentChecker := NewAgentChecker(AgentConfig{FacadeURL: "http://127.0.0.1:1", AgentName: "x", Namespace: "y"})
	c := NewMemoryChecker("", nil, testWorkspace, agentChecker)
	result := c.checkMemoryRecall(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
}

// --- MemoryPersistsAcrossSessions ---

func TestCheckMemoryPersistsAcrossSessions_Pass(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close() //nolint:errcheck

		connected := wsServerMessage{Type: wsMessageTypeConnected, SessionID: fmt.Sprintf("sess-%d", callCount)}
		require.NoError(t, conn.WriteJSON(connected))
		callCount++

		// Read one client message.
		_, _, err = conn.ReadMessage()
		if err != nil {
			return
		}

		// Session 1: acknowledge remember; Session 2: recall with the value.
		if callCount == 1 {
			require.NoError(t, conn.WriteJSON(wsServerMessage{Type: wsMessageTypeDone, Content: "Remembered."}))
		} else {
			require.NoError(t, conn.WriteJSON(wsServerMessage{Type: wsMessageTypeDone, Content: "Your value is persist-ok."}))
		}
	}))
	defer srv.Close()

	agentChecker := newCheckerForServer(srv)

	// Mock memory-api that returns persist-ok when searched.
	memSrv := (&mockMemoryServer{
		searchHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"memories":[{"id":"p1","content":"persist-ok","scope":{"workspace_id":"ws1","user_id":"test"}}],"total":1}`))
		},
	}).serve(t)
	defer memSrv.Close()

	memStore := memoryhttpclient.NewStore(memSrv.URL, logr.Discard())
	c := NewMemoryChecker(memSrv.URL, memStore, testWorkspace, agentChecker)
	result := c.checkMemoryPersistsAcrossSessions(t.Context())
	assert.Equal(t, doctor.StatusPass, result.Status)
	assert.Contains(t, result.Detail, "persisted across sessions")
}

func TestCheckMemoryPersistsAcrossSessions_Fail_NotInStore(t *testing.T) {
	srv := serveMockFacade(t, mockFacadeHandler{
		responses: []wsServerMessage{
			{Type: wsMessageTypeDone, Content: "Remembered."},
		},
	})
	defer srv.Close()

	agentChecker := newCheckerForServer(srv)

	// Mock memory-api that returns empty results.
	memSrv := (&mockMemoryServer{
		searchHandler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"memories":[],"total":0}`))
		},
	}).serve(t)
	defer memSrv.Close()

	memStore := memoryhttpclient.NewStore(memSrv.URL, logr.Discard())
	c := NewMemoryChecker(memSrv.URL, memStore, testWorkspace, agentChecker)
	result := c.checkMemoryPersistsAcrossSessions(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "not found in memory store")
}

func TestCheckMemoryPersistsAcrossSessions_Fail_ConnectionError(t *testing.T) {
	agentChecker := NewAgentChecker(AgentConfig{FacadeURL: "https://127.0.0.1:1", AgentName: "x", Namespace: "y"})
	c := NewMemoryChecker("", nil, testWorkspace, agentChecker)
	result := c.checkMemoryPersistsAcrossSessions(t.Context())
	assert.Equal(t, doctor.StatusFail, result.Status)
	assert.Contains(t, result.Detail, "connection failed")
}

func TestCheckMemoryPersistsAcrossSessions_Skip_NoWorkspace(t *testing.T) {
	agentChecker := NewAgentChecker(AgentConfig{})
	c := NewMemoryChecker("", nil, "", agentChecker)
	result := c.checkMemoryPersistsAcrossSessions(t.Context())
	assert.Equal(t, doctor.StatusSkip, result.Status)
}

// --- fetchBody helper ---

// errReader implements io.ReadCloser and always returns an error on Read.
type errReader struct{}

func (errReader) Read(_ []byte) (int, error) { return 0, fmt.Errorf("read error") }
func (errReader) Close() error               { return nil }

func TestFetchBody_Fail_InvalidURL(t *testing.T) {
	client := memoryClient()
	_, err := fetchBody(t.Context(), client, "http://\x00invalid")
	assert.Error(t, err)
}

func TestFetchBody_Fail_ReadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Hijack the connection to simulate a body read error: write headers manually
		// then close the connection mid-body.
		w.WriteHeader(http.StatusOK)
		// flush headers but don't write body — connection closes after handler returns,
		// causing io.ReadAll to see EOF not an error. Instead we override the body via
		// a custom transport in this test, so use an error-body server approach.
		// Actually the simplest approach: write partial content with a known-bad flush.
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	// Use a custom RoundTripper to inject an errReader body.
	client := &http.Client{
		Transport: &bodyErrorTransport{wrapped: http.DefaultTransport},
	}
	_, err := fetchBody(t.Context(), client, srv.URL)
	assert.Error(t, err)
}

// bodyErrorTransport wraps a RoundTripper and replaces response body with errReader.
type bodyErrorTransport struct {
	wrapped http.RoundTripper
}

func (t *bodyErrorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.wrapped.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	resp.Body = errReader{}
	return resp, nil
}
