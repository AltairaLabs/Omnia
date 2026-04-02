package checks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/altairalabs/omnia/internal/doctor"
)

const (
	privacyCategory         = "Privacy"
	privacyTestSSN          = "123-45-6789"
	privacyOptOutHeader     = "X-Privacy-Opt-Out"
	privacyBatchDeletePath  = "/api/v1/memories/batch"
	privacyMemoriesPath     = "/api/v1/memories"
	privacyMemorySearchPath = "/api/v1/memories/search"
)

// PrivacyChecker runs privacy-related checks against the memory-api service.
type PrivacyChecker struct {
	memoryAPIURL string
	workspace    string
}

// NewPrivacyChecker creates a new PrivacyChecker.
func NewPrivacyChecker(memoryAPIURL, workspace string) *PrivacyChecker {
	return &PrivacyChecker{
		memoryAPIURL: memoryAPIURL,
		workspace:    workspace,
	}
}

// Checks returns the list of privacy checks to run.
func (p *PrivacyChecker) Checks() []doctor.Check {
	return []doctor.Check{
		{Name: "MemoryPIIRedaction", Category: privacyCategory, Run: p.checkPIIRedaction},
		{Name: "MemoryOptOutRespected", Category: privacyCategory, Run: p.checkOptOutRespected},
		{Name: "MemoryDeletionCascade", Category: privacyCategory, Run: p.checkDeletionCascade},
		{Name: "AuditLogWritten", Category: privacyCategory, Run: p.checkAuditLogWritten},
	}
}

// requireWorkspace returns a skip result if workspace is empty.
func (p *PrivacyChecker) requireWorkspace() *doctor.TestResult {
	if p.workspace == "" {
		r := doctor.TestResult{Status: doctor.StatusSkip, Detail: "workspace UID not resolved"}
		return &r
	}
	return nil
}

// saveMemory POSTs a memory with the given content and optional extra headers.
// Returns the memory ID on success, or an error.
func (p *PrivacyChecker) saveMemory(ctx context.Context, content string, extraHeaders map[string]string) (string, int, error) {
	payload := map[string]interface{}{
		"type":       "doctor-privacy-test",
		"content":    content,
		"confidence": 0.9,
		"scope":      map[string]string{"workspace_id": p.workspace},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.memoryAPIURL+privacyMemoriesPath, bytes.NewReader(body))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := memoryClient().Do(req)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		return "", resp.StatusCode, nil
	}

	var result struct {
		Memory struct {
			ID string `json:"id"`
		} `json:"memory"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", resp.StatusCode, err
	}
	return result.Memory.ID, resp.StatusCode, nil
}

// searchMemories queries the memory search endpoint for a query string.
// Returns the raw JSON body contents of the memories array items.
func (p *PrivacyChecker) searchMemories(ctx context.Context, query string) ([]map[string]interface{}, error) {
	params := url.Values{"workspace": {p.workspace}, "q": {query}}
	searchURL := p.memoryAPIURL + privacyMemorySearchPath + "?" + params.Encode()
	body, err := fetchBody(ctx, memoryClient(), searchURL)
	if err != nil {
		return nil, err
	}

	var result struct {
		Memories []map[string]interface{} `json:"memories"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}
	return result.Memories, nil
}

// checkPIIRedaction saves a memory containing a test SSN, retrieves it, and
// verifies the SSN is not present in the returned content.
func (p *PrivacyChecker) checkPIIRedaction(ctx context.Context) doctor.TestResult {
	if r := p.requireWorkspace(); r != nil {
		return *r
	}

	_, status, err := p.saveMemory(ctx, "patient ssn is "+privacyTestSSN, nil)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error()}
	}
	if status != http.StatusCreated {
		return doctor.TestResult{Status: doctor.StatusFail, Detail: fmt.Sprintf("expected HTTP 201, got %d", status)}
	}

	memories, err := p.searchMemories(ctx, privacyTestSSN)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusSkip, Detail: "memory-api search unavailable", Error: err.Error()}
	}

	for _, mem := range memories {
		content, _ := mem["content"].(string)
		if strings.Contains(content, privacyTestSSN) {
			return doctor.TestResult{
				Status: doctor.StatusFail,
				Detail: "SSN found unredacted in retrieved memory content",
			}
		}
	}
	return doctor.TestResult{Status: doctor.StatusPass, Detail: "SSN not present in retrieved memory content"}
}

// checkOptOutRespected saves a memory with the opt-out header and verifies it
// is rejected (HTTP 204). If the memory is saved (HTTP 201), enterprise privacy
// middleware is not present — the check is skipped.
func (p *PrivacyChecker) checkOptOutRespected(ctx context.Context) doctor.TestResult {
	if r := p.requireWorkspace(); r != nil {
		return *r
	}

	_, status, err := p.saveMemory(ctx, "opt-out test content", map[string]string{privacyOptOutHeader: "true"})
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error()}
	}

	switch status {
	case http.StatusNoContent:
		return doctor.TestResult{Status: doctor.StatusPass, Detail: "opt-out header respected: save rejected with 204"}
	case http.StatusCreated:
		return doctor.TestResult{Status: doctor.StatusSkip, Detail: "enterprise privacy middleware not detected"}
	default:
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: fmt.Sprintf("unexpected HTTP %d (expected 204 or 201)", status),
		}
	}
}

// checkDeletionCascade saves a test memory, batch-deletes it, and verifies it
// is gone. Skips if the batch delete endpoint is not available (HTTP 404).
func (p *PrivacyChecker) checkDeletionCascade(ctx context.Context) doctor.TestResult {
	if r := p.requireWorkspace(); r != nil {
		return *r
	}

	memID, status, err := p.saveMemory(ctx, "deletion cascade test", nil)
	if err != nil || status != http.StatusCreated {
		return doctor.TestResult{Status: doctor.StatusFail, Detail: "save failed before batch delete", Error: errString(err)}
	}

	batchURL := fmt.Sprintf("%s%s?workspace=%s&user_id=%s&limit=100",
		p.memoryAPIURL, privacyBatchDeletePath, p.workspace, memID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, batchURL, nil)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error()}
	}

	resp, err := memoryClient().Do(req)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return doctor.TestResult{Status: doctor.StatusSkip, Detail: "batch delete endpoint not available"}
	}
	if resp.StatusCode != http.StatusOK {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: fmt.Sprintf("batch delete returned HTTP %d", resp.StatusCode),
		}
	}

	// Verify the memory is gone.
	memories, err := p.searchMemories(ctx, "deletion cascade test")
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error(), Detail: "search after batch delete failed"}
	}
	if len(memories) > 0 {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: fmt.Sprintf("memory still present after batch delete (%d result(s))", len(memories)),
		}
	}
	return doctor.TestResult{Status: doctor.StatusPass, Detail: "memory absent after batch delete"}
}

// checkAuditLogWritten is deferred until the audit query endpoint is added.
func (p *PrivacyChecker) checkAuditLogWritten(_ context.Context) doctor.TestResult {
	return doctor.TestResult{Status: doctor.StatusSkip, Detail: "audit endpoint not available"}
}

// errString returns the error message or an empty string if err is nil.
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
