package checks

import (
	"bytes"
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
	memoryAPITimeout = 10 * time.Second
	memoryCategory   = "Memory"
	memoryAPIPrefix  = "/api/v1/memories"
	memoryTestType   = "doctor-test"
	memoryTestValue  = "doctor smoke test value"
	memoryTestMarker = "smoke-42"
	workspaceParam   = "workspace"
)

// MemoryChecker runs REST API checks against the memory-api service, and
// optionally agent tool-calling checks if an AgentChecker is provided.
type MemoryChecker struct {
	memoryAPIURL  string
	workspace     string
	agentChecker  *AgentChecker
	savedMemoryID string
}

// NewMemoryChecker creates a new MemoryChecker.
func NewMemoryChecker(memoryAPIURL, workspace string, agentChecker *AgentChecker) *MemoryChecker {
	return &MemoryChecker{
		memoryAPIURL: memoryAPIURL,
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
		)
	}
	return checks
}

// memoryClient returns an HTTP client with the configured timeout.
func memoryClient() *http.Client {
	return &http.Client{Timeout: memoryAPITimeout}
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

// memorySaveRequest is the body sent to POST /api/v1/memories.
type memorySaveRequest struct {
	Type       string      `json:"type"`
	Content    string      `json:"content"`
	Confidence float64     `json:"confidence"`
	Scope      memoryScope `json:"scope"`
}

// memoryScope identifies the workspace the memory belongs to.
type memoryScope struct {
	WorkspaceID string `json:"workspace_id"`
}

// memorySaveResponse is the expected response from POST /api/v1/memories.
type memorySaveResponse struct {
	Memory struct {
		ID string `json:"id"`
	} `json:"memory"`
}

// checkSave POSTs a test memory and stores the returned ID for later deletion.
func (m *MemoryChecker) checkSave(ctx context.Context) doctor.TestResult {
	if r := m.requireWorkspace(); r != nil {
		return *r
	}
	payload := memorySaveRequest{
		Type:       memoryTestType,
		Content:    memoryTestValue,
		Confidence: 0.95,
		Scope:      memoryScope{WorkspaceID: m.workspace},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: fmt.Sprintf("marshal: %v", err)}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.memoryAPIURL+memoryAPIPrefix, bytes.NewReader(data))
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := memoryClient().Do(req)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: fmt.Sprintf("expected HTTP 201, got %d", resp.StatusCode),
		}
	}

	var result memorySaveResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: fmt.Sprintf("decode response: %v", err)}
	}
	if result.Memory.ID == "" {
		return doctor.TestResult{Status: doctor.StatusFail, Detail: "response missing id field"}
	}

	m.savedMemoryID = result.Memory.ID
	return doctor.TestResult{Status: doctor.StatusPass, Detail: fmt.Sprintf("saved memory id=%s", result.Memory.ID)}
}

// memorySearchResponse is the shape returned by GET /api/v1/memories/search.
type memorySearchResponse struct {
	Memories []memoryItem `json:"memories"`
}

// memoryItem is a single memory returned by list or search.
type memoryItem struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

// checkRetrieve searches for the previously saved test memory.
func (m *MemoryChecker) checkRetrieve(ctx context.Context) doctor.TestResult {
	if r := m.requireWorkspace(); r != nil {
		return *r
	}
	url := fmt.Sprintf("%s%s/search?q=%s&%s=%s",
		m.memoryAPIURL, memoryAPIPrefix,
		"doctor+smoke",
		workspaceParam, m.workspace,
	)
	body, err := fetchBody(ctx, memoryClient(), url)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error()}
	}

	var result memorySearchResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: fmt.Sprintf("decode: %v", err)}
	}
	if len(result.Memories) == 0 {
		return doctor.TestResult{Status: doctor.StatusFail, Detail: "search returned no results"}
	}
	return doctor.TestResult{
		Status: doctor.StatusPass,
		Detail: fmt.Sprintf("found %d result(s)", len(result.Memories)),
	}
}

// memoryListResponse is the shape returned by GET /api/v1/memories.
type memoryListResponse struct {
	Memories []memoryItem `json:"memories"`
}

// checkList lists memories for the workspace and verifies at least one exists.
func (m *MemoryChecker) checkList(ctx context.Context) doctor.TestResult {
	if r := m.requireWorkspace(); r != nil {
		return *r
	}
	url := fmt.Sprintf("%s%s?%s=%s", m.memoryAPIURL, memoryAPIPrefix, workspaceParam, m.workspace)
	body, err := fetchBody(ctx, memoryClient(), url)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error()}
	}

	var result memoryListResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: fmt.Sprintf("decode: %v", err)}
	}
	if len(result.Memories) == 0 {
		return doctor.TestResult{Status: doctor.StatusFail, Detail: "list returned 0 items"}
	}
	return doctor.TestResult{
		Status: doctor.StatusPass,
		Detail: fmt.Sprintf("%d item(s) in workspace", len(result.Memories)),
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

	url := fmt.Sprintf("%s%s/%s?%s=%s",
		m.memoryAPIURL, memoryAPIPrefix, m.savedMemoryID, workspaceParam, m.workspace)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
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
	return doctor.TestResult{Status: doctor.StatusPass, Detail: "memory deleted"}
}

// checkExport downloads an export and verifies the Content-Disposition header.
func (m *MemoryChecker) checkExport(ctx context.Context) doctor.TestResult {
	if r := m.requireWorkspace(); r != nil {
		return *r
	}
	url := fmt.Sprintf("%s%s/export?%s=%s", m.memoryAPIURL, memoryAPIPrefix, workspaceParam, m.workspace)
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

// checkMemoryToolsAvailable tells the agent to remember a value, then verifies
// the memory was persisted by querying the memory-api directly. Memory tools are
// platform-level and not forwarded via WebSocket, so we verify by outcome.
func (m *MemoryChecker) checkMemoryToolsAvailable(ctx context.Context) doctor.TestResult {
	if r := m.requireWorkspace(); r != nil {
		return *r
	}

	toolCtx, cancel := context.WithTimeout(ctx, wsResponseTimeout)
	defer cancel()

	conn, sessionID, err := m.agentChecker.dial(toolCtx)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "connection failed"}
	}
	defer closeConn(conn)

	if err := sendMessage(conn, "Please remember that my doctor test value is smoke-42"); err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "send failed"}
	}

	// Wait for the agent to finish (memory__remember executes server-side).
	if _, err := collectResponse(toolCtx, conn); err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "receive failed"}
	}

	// Check session-api for tool call errors. The runtime event store writes
	// asynchronously, so poll until the tool call record appears.
	if m.agentChecker.config.SessionAPIURL != "" && sessionID != "" {
		for attempt := range 5 {
			if errDetail := m.checkToolCallErrors(ctx, sessionID, "memory__remember"); errDetail != "" {
				return doctor.TestResult{Status: doctor.StatusFail, Detail: errDetail}
			}
			calls, _ := m.agentChecker.fetchToolCalls(ctx, sessionID)
			if len(calls) > 0 {
				break
			}
			if attempt < 4 {
				time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
			}
		}
	}

	// Verify the memory was saved by searching the memory-api.
	url := fmt.Sprintf("%s%s/search?q=%s&%s=%s",
		m.memoryAPIURL, memoryAPIPrefix, memoryTestMarker, workspaceParam, m.workspace)
	body, err := fetchBody(ctx, memoryClient(), url)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "search after remember failed"}
	}

	var result memorySearchResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "decode search response"}
	}

	for _, mem := range result.Memories {
		if strings.Contains(mem.Content, memoryTestMarker) {
			return doctor.TestResult{Status: doctor.StatusPass, Detail: "memory__remember persisted 'smoke-42'"}
		}
	}
	return doctor.TestResult{
		Status: doctor.StatusFail,
		Detail: fmt.Sprintf("memory__remember did not persist 'smoke-42' (found %d memories)", len(result.Memories)),
	}
}

// checkToolCallErrors queries session-api for tool calls and returns an error detail
// string if any call matching toolName has status "error". Returns "" if no errors.
func (m *MemoryChecker) checkToolCallErrors(ctx context.Context, sessionID, toolName string) string {
	toolCalls, err := m.agentChecker.fetchToolCalls(ctx, sessionID)
	if err != nil || len(toolCalls) == 0 {
		return ""
	}
	for _, tc := range toolCalls {
		if tc.Name == toolName && tc.Status == "error" {
			errMsg := tc.ErrorMessage
			if errMsg == "" {
				errMsg = tc.Result
			}
			return fmt.Sprintf("%s tool call failed: %s", toolName, truncate(errMsg, 150))
		}
	}
	return ""
}

// checkMemoryRecall asks the agent to recall the stored value. Memory tools are
// platform-level, so we verify by checking the response text for the expected value.
func (m *MemoryChecker) checkMemoryRecall(ctx context.Context) doctor.TestResult {
	recallCtx, cancel := context.WithTimeout(ctx, wsResponseTimeout)
	defer cancel()

	conn, _, err := m.agentChecker.dial(recallCtx)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "connection failed"}
	}
	defer closeConn(conn)

	if err := sendMessage(conn, "What is my doctor test value? Use your memory tools to find it."); err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "send failed"}
	}

	msgs, err := collectResponse(recallCtx, conn)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "receive failed"}
	}

	text := assembleText(msgs)
	if strings.Contains(text, memoryTestMarker) {
		return doctor.TestResult{Status: doctor.StatusPass, Detail: "recalled 'smoke-42' from memory"}
	}
	return doctor.TestResult{
		Status: doctor.StatusFail,
		Detail: fmt.Sprintf("expected 'smoke-42' in response, got: %q", truncate(text, 200)),
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
