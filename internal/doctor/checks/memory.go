package checks

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"

	"github.com/altairalabs/omnia/internal/doctor"
	memoryhttpclient "github.com/altairalabs/omnia/internal/memory/httpclient"
	"github.com/altairalabs/omnia/internal/session"
)

const (
	memoryAPITimeout = 10 * time.Second
	memoryCategory   = "Memory"
	memoryTestType   = "doctor-test"
	memoryTestValue  = "doctor smoke test value"
	memoryTestMarker = "smoke-42"
)

// MemoryChecker runs REST API checks against the memory-api service, and
// optionally agent tool-calling checks if an AgentChecker is provided.
type MemoryChecker struct {
	memoryAPIURL  string
	memoryStore   *memoryhttpclient.Store
	workspace     string
	agentChecker  *AgentChecker
	savedMemoryID string
}

// NewMemoryChecker creates a new MemoryChecker.
func NewMemoryChecker(memoryAPIURL string, memoryStore *memoryhttpclient.Store, workspace string, agentChecker *AgentChecker) *MemoryChecker {
	return &MemoryChecker{
		memoryAPIURL: memoryAPIURL,
		memoryStore:  memoryStore,
		workspace:    workspace,
		agentChecker: agentChecker,
	}
}

// Checks returns the list of memory checks to run.
// Tool checks are only included if an AgentChecker was provided.
func (m *MemoryChecker) Checks() []doctor.Check {
	checks := []doctor.Check{
		{Name: "MemoryAPIDocsServed", Category: memoryCategory, Run: m.checkDocs},
		{Name: "MemorySave", Category: memoryCategory, Run: m.checkSave},
		{Name: "MemoryRetrieve", Category: memoryCategory, Run: m.checkRetrieve},
		{Name: "MemoryList", Category: memoryCategory, Run: m.checkList},
		{Name: "MemoryDelete", Category: memoryCategory, Run: m.checkDelete},
		{Name: "MemoryExport", Category: memoryCategory, Run: m.checkExport},
	}
	if m.agentChecker != nil {
		checks = append(checks,
			doctor.Check{Name: "MemoryToolsAvailable", Category: memoryCategory, Run: m.checkMemoryToolsAvailable},
			doctor.Check{Name: "MemoryRecall", Category: memoryCategory, Run: m.checkMemoryRecall},
			doctor.Check{Name: "MemoryPersistsAcrossSessions", Category: memoryCategory, Run: m.checkMemoryPersistsAcrossSessions},
		)
	}
	return checks
}

// memoryClient returns an HTTP client with the configured timeout.
func memoryClient() *http.Client {
	return &http.Client{Timeout: memoryAPITimeout}
}

// scope returns the scope map for memory operations.
func (m *MemoryChecker) scope() map[string]string {
	return map[string]string{"workspace_id": m.workspace}
}

// requireWorkspace returns a skip result if the workspace UID is empty.
func (m *MemoryChecker) requireWorkspace() *doctor.TestResult {
	if m.workspace == "" {
		r := doctor.TestResult{Status: doctor.StatusSkip, Detail: "workspace UID not resolved"}
		return &r
	}
	return nil
}

// checkDocs verifies the memory-api docs endpoint is reachable and has expected content.
func (m *MemoryChecker) checkDocs(ctx context.Context) doctor.TestResult {
	body, err := fetchBody(ctx, memoryClient(), m.memoryAPIURL+"/docs")
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error()}
	}
	if !strings.Contains(body, "Memory API") {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: "docs page does not contain 'Memory API'",
		}
	}
	return doctor.TestResult{Status: doctor.StatusPass, Detail: "docs page served"}
}

// checkSave POSTs a test memory and stores the returned ID for later deletion.
func (m *MemoryChecker) checkSave(ctx context.Context) doctor.TestResult {
	if r := m.requireWorkspace(); r != nil {
		return *r
	}
	mem := &pkmemory.Memory{
		Type:       memoryTestType,
		Content:    memoryTestValue,
		Confidence: 0.95,
		Scope:      m.scope(),
	}
	if err := m.memoryStore.Save(ctx, mem); err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error()}
	}
	if mem.ID == "" {
		return doctor.TestResult{Status: doctor.StatusFail, Detail: "response missing id field"}
	}

	m.savedMemoryID = mem.ID
	return doctor.TestResult{Status: doctor.StatusPass, Detail: fmt.Sprintf("saved memory id=%s", mem.ID)}
}

// checkRetrieve searches for the previously saved test memory.
func (m *MemoryChecker) checkRetrieve(ctx context.Context) doctor.TestResult {
	if r := m.requireWorkspace(); r != nil {
		return *r
	}
	memories, err := m.memoryStore.Retrieve(ctx, m.scope(), "doctor smoke", pkmemory.RetrieveOptions{})
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error()}
	}
	if len(memories) == 0 {
		return doctor.TestResult{Status: doctor.StatusFail, Detail: "search returned no results"}
	}
	return doctor.TestResult{
		Status: doctor.StatusPass,
		Detail: fmt.Sprintf("found %d result(s)", len(memories)),
	}
}

// checkList lists memories for the workspace and verifies at least one exists.
func (m *MemoryChecker) checkList(ctx context.Context) doctor.TestResult {
	if r := m.requireWorkspace(); r != nil {
		return *r
	}
	memories, err := m.memoryStore.List(ctx, m.scope(), pkmemory.ListOptions{})
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error()}
	}
	if len(memories) == 0 {
		return doctor.TestResult{Status: doctor.StatusFail, Detail: "list returned 0 items"}
	}
	return doctor.TestResult{
		Status: doctor.StatusPass,
		Detail: fmt.Sprintf("%d item(s) in workspace", len(memories)),
	}
}

// checkDelete deletes the previously saved test memory.
func (m *MemoryChecker) checkDelete(ctx context.Context) doctor.TestResult {
	if m.savedMemoryID == "" {
		return doctor.TestResult{
			Status: doctor.StatusSkip,
			Detail: "no memory to delete (save failed?)",
		}
	}

	if err := m.memoryStore.Delete(ctx, m.scope(), m.savedMemoryID); err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error()}
	}
	return doctor.TestResult{Status: doctor.StatusPass, Detail: "memory deleted"}
}

// checkExport downloads an export and verifies the Content-Disposition header.
func (m *MemoryChecker) checkExport(ctx context.Context) doctor.TestResult {
	if r := m.requireWorkspace(); r != nil {
		return *r
	}
	url := fmt.Sprintf("%s/api/v1/memories/export?workspace=%s", m.memoryAPIURL, m.workspace)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error()}
	}

	resp, err := memoryClient().Do(req)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: fmt.Sprintf("expected HTTP 200, got %d", resp.StatusCode),
		}
	}

	cd := resp.Header.Get("Content-Disposition")
	if cd == "" {
		return doctor.TestResult{Status: doctor.StatusFail, Detail: "missing Content-Disposition header"}
	}
	return doctor.TestResult{Status: doctor.StatusPass, Detail: fmt.Sprintf("export ready (%s)", cd)}
}

// chatWithAgent dials the facade, sends a message, waits for the response, and returns
// the session ID, assembled response text, and any error as a TestResult.
// The connection is opened and closed within the helper; the caller's ctx is not modified.
func (m *MemoryChecker) chatWithAgent(ctx context.Context, message string) (sessionID, responseText string, fail *doctor.TestResult) {
	chatCtx, cancel := context.WithTimeout(ctx, wsResponseTimeout)
	defer cancel()

	conn, sid, err := m.agentChecker.dial(chatCtx)
	if err != nil {
		r := doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "connection failed"}
		return "", "", &r
	}
	defer closeConn(conn)

	if err := sendMessage(conn, message); err != nil {
		r := doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "send failed"}
		return sid, "", &r
	}

	msgs, err := collectResponse(chatCtx, conn)
	if err != nil {
		r := doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "receive failed"}
		return sid, assembleText(msgs), &r
	}

	return sid, assembleText(msgs), nil
}

// checkMemoryToolsAvailable tells the agent to remember a value, then verifies
// the memory was persisted by querying the memory-api directly. Memory tools are
// platform-level and not forwarded via WebSocket, so we verify by outcome.
func (m *MemoryChecker) checkMemoryToolsAvailable(ctx context.Context) doctor.TestResult {
	if r := m.requireWorkspace(); r != nil {
		return *r
	}

	sessionID, _, fail := m.chatWithAgent(ctx, "Please remember that my doctor test value is smoke-42")
	if fail != nil {
		return *fail
	}

	// Check session store for tool call errors first.
	if m.agentChecker.config.SessionStore != nil && sessionID != "" {
		if errDetail := m.checkToolCallErrors(ctx, sessionID, "memory__remember"); errDetail != "" {
			return doctor.TestResult{Status: doctor.StatusFail, Detail: errDetail}
		}
	}

	// Verify the memory was saved by searching the memory-api.
	memories, err := m.memoryStore.Retrieve(ctx, m.scope(), memoryTestMarker, pkmemory.RetrieveOptions{})
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "search after remember failed"}
	}

	for _, mem := range memories {
		if strings.Contains(mem.Content, memoryTestMarker) {
			return doctor.TestResult{Status: doctor.StatusPass, Detail: "memory__remember persisted 'smoke-42'"}
		}
	}
	return doctor.TestResult{
		Status: doctor.StatusFail,
		Detail: fmt.Sprintf("memory__remember did not persist 'smoke-42' (found %d memories)", len(memories)),
	}
}

// checkToolCallErrors queries the session store for tool calls and returns an error detail
// string if any call matching toolName has status "error". Returns "" if no errors.
func (m *MemoryChecker) checkToolCallErrors(ctx context.Context, sessionID, toolName string) string {
	store := m.agentChecker.config.SessionStore
	if store == nil {
		return ""
	}
	toolCalls, err := store.GetToolCalls(ctx, sessionID, 0, 0)
	if err != nil || len(toolCalls) == 0 {
		return ""
	}
	for _, tc := range toolCalls {
		if tc.Name == toolName && tc.Status == session.ToolCallStatusError {
			errMsg := tc.ErrorMessage
			if errMsg == "" {
				errMsg = toolCallResultString(tc.Result)
			}
			return fmt.Sprintf("%s tool call failed: %s", toolName, truncate(errMsg, 150))
		}
	}
	return ""
}

// checkMemoryRecall asks the agent to recall the stored value. Memory tools are
// platform-level, so we verify by checking the response text for the expected value.
func (m *MemoryChecker) checkMemoryRecall(ctx context.Context) doctor.TestResult {
	_, text, fail := m.chatWithAgent(ctx, "What is my doctor test value? Use your memory tools to find it.")
	if fail != nil {
		return *fail
	}

	if strings.Contains(text, memoryTestMarker) {
		return doctor.TestResult{Status: doctor.StatusPass, Detail: "recalled 'smoke-42' from memory"}
	}
	return doctor.TestResult{
		Status: doctor.StatusFail,
		Detail: fmt.Sprintf("expected 'smoke-42' in response, got: %q", truncate(text, 200)),
	}
}

const memoryPersistTestValue = "persist-ok"

// checkMemoryPersistsAcrossSessions verifies memories survive across WebSocket sessions.
// It opens a connection, asks the agent to remember a value, closes it, then opens a
// new connection and asks the agent to recall the value.
func (m *MemoryChecker) checkMemoryPersistsAcrossSessions(ctx context.Context) doctor.TestResult {
	if r := m.requireWorkspace(); r != nil {
		return *r
	}

	// Session 1: ask the agent to remember a value.
	if _, _, fail := m.chatWithAgent(ctx, "Please remember that my doctor persistence test value is persist-ok"); fail != nil {
		return *fail
	}

	// Session 2: ask the agent to recall the value.
	_, text, fail := m.chatWithAgent(ctx, "What is my doctor persistence test value?")
	if fail != nil {
		return *fail
	}

	if strings.Contains(text, memoryPersistTestValue) {
		return doctor.TestResult{Status: doctor.StatusPass, Detail: "memory persisted across sessions"}
	}
	return doctor.TestResult{
		Status: doctor.StatusFail,
		Detail: fmt.Sprintf("expected '%s' in response, got: %q", memoryPersistTestValue, truncate(text, 200)),
	}
}

// fetchBody performs a GET and returns the response body as a string.
// It returns an error for non-200 responses.
func fetchBody(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
