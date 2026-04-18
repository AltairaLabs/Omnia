package checks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	eev1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/doctor"
)

const (
	privacyCategory         = "Privacy"
	privacyTestSSN          = "123-45-6789"
	privacyTestUserID       = "doctor-privacy-test"
	privacyBatchDeletePath  = "/api/v1/memories/batch"
	privacyMemoriesPath     = "/api/v1/memories"
	privacyMemorySearchPath = "/api/v1/memories/search"

	// HTTP header/media-type constants (extracted to satisfy go:S1192).
	headerContentType = "Content-Type"
	contentTypeJSON   = "application/json"
)

// PrivacyChecker runs privacy-related checks against the memory-api service.
// Privacy features (PII redaction, opt-out, audit) are enterprise-only.
// Each check verifies a valid enterprise license before running.
type PrivacyChecker struct {
	memoryAPIURL  string
	sessionAPIURL string
	workspace     string
	arenaURL      string
	k8sClient     client.Client
}

// NewPrivacyChecker creates a new PrivacyChecker. arenaURL is the base URL of
// the arena controller, used to fetch the license for enterprise detection.
func NewPrivacyChecker(memoryAPIURL, sessionAPIURL, workspace, arenaURL string) *PrivacyChecker {
	return &PrivacyChecker{
		memoryAPIURL:  memoryAPIURL,
		sessionAPIURL: sessionAPIURL,
		workspace:     workspace,
		arenaURL:      arenaURL,
	}
}

// WithK8sClient sets the Kubernetes client used by the SessionEncryptionAtRest check.
// When nil, the encryption check reports a skip (k8s not available).
func (p *PrivacyChecker) WithK8sClient(c client.Client) *PrivacyChecker {
	p.k8sClient = c
	return p
}

// Checks returns the list of privacy checks to run.
func (p *PrivacyChecker) Checks() []doctor.Check {
	return []doctor.Check{
		{Name: "MemoryPIIRedaction", Category: privacyCategory, Run: p.checkPIIRedaction},
		{Name: "MemoryOptOutRespected", Category: privacyCategory, Run: p.checkOptOutRespected},
		{Name: "MemoryDeletionCascade", Category: privacyCategory, Run: p.checkDeletionCascade},
		{Name: "AuditLogWritten", Category: privacyCategory, Run: p.checkAuditLogWritten},
		{Name: "SessionEncryptionAtRest", Category: privacyCategory, Run: p.checkSessionEncryption},
	}
}

// requireEnterprise fetches the license from the arena controller and checks
// whether it is an enterprise license. Returns a skip result if the arena
// controller is not deployed or the license is not enterprise tier.
func (p *PrivacyChecker) requireEnterprise(ctx context.Context) *doctor.TestResult {
	if p.arenaURL == "" {
		r := doctor.TestResult{Status: doctor.StatusSkip, Detail: "enterprise not configured (no arena URL)"}
		return &r
	}

	licenseURL := p.arenaURL + "/api/v1/license"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, licenseURL, nil)
	if err != nil {
		r := doctor.TestResult{Status: doctor.StatusSkip, Detail: "enterprise license check failed", Error: err.Error()}
		return &r
	}
	resp, err := memoryClient().Do(req)
	if err != nil {
		r := doctor.TestResult{Status: doctor.StatusSkip, Detail: "enterprise not available (arena controller unreachable)"}
		return &r
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		r := doctor.TestResult{Status: doctor.StatusSkip, Detail: fmt.Sprintf("license endpoint returned HTTP %d", resp.StatusCode)}
		return &r
	}

	var license struct {
		Tier string `json:"tier"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&license); err != nil {
		r := doctor.TestResult{Status: doctor.StatusSkip, Detail: "failed to decode license response", Error: err.Error()}
		return &r
	}

	if license.Tier != "enterprise" {
		r := doctor.TestResult{Status: doctor.StatusSkip, Detail: fmt.Sprintf("privacy checks require enterprise license (current: %s)", license.Tier)}
		return &r
	}
	return nil
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
		"type":       privacyTestUserID,
		"content":    content,
		"confidence": 0.9,
		"scope":      map[string]string{"workspace_id": p.workspace, "user_id": privacyTestUserID},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.memoryAPIURL+privacyMemoriesPath, bytes.NewReader(body))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set(headerContentType, contentTypeJSON)
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
	params := url.Values{"workspace": {p.workspace}, "q": {query}, "user_id": {privacyTestUserID}}
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
// Skips when the enterprise privacy middleware is not deployed.
func (p *PrivacyChecker) checkPIIRedaction(ctx context.Context) doctor.TestResult {
	if r := p.requireWorkspace(); r != nil {
		return *r
	}

	// Probe the enterprise consent endpoint to detect whether the privacy
	// middleware is deployed. This is an EE-only route; a 404 means
	// redaction is not available so the check should skip.
	if skip := p.requireEnterprise(ctx); skip != nil {
		return *skip
	}

	_, status, err := p.saveMemory(ctx, "patient ssn is "+privacyTestSSN, nil)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error()}
	}
	if status != http.StatusCreated {
		return doctor.TestResult{Status: doctor.StatusFail, Detail: fmt.Sprintf("expected HTTP 201, got %d", status)}
	}

	// Search for the memory we just saved. Use a broad query ("patient ssn")
	// that will match both redacted and unredacted content.
	memories, err := p.searchMemories(ctx, "patient ssn")
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusSkip, Detail: "memory-api search unavailable", Error: err.Error()}
	}

	if len(memories) == 0 {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: "saved memory not found in search results — cannot verify redaction",
		}
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
	return doctor.TestResult{Status: doctor.StatusPass, Detail: fmt.Sprintf("SSN redacted in %d retrieved memory(ies)", len(memories))}
}

// checkOptOutRespected sets an opt-out preference for the test user, attempts
// to save a memory, and verifies the save is rejected (204). Cleans up the
// opt-out preference afterward.
func (p *PrivacyChecker) checkOptOutRespected(ctx context.Context) doctor.TestResult {
	if r := p.requireWorkspace(); r != nil {
		return *r
	}
	if r := p.requireEnterprise(ctx); r != nil {
		return *r
	}

	// Set opt-out for the test user via the session-api preferences endpoint.
	optOutURL := fmt.Sprintf("%s/api/v1/privacy/opt-out", p.sessionAPIURL)
	optOutBody, _ := json.Marshal(map[string]string{
		"userId": privacyTestUserID,
		"scope":  "all",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, optOutURL, bytes.NewReader(optOutBody))
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error()}
	}
	req.Header.Set(headerContentType, contentTypeJSON)

	resp, err := memoryClient().Do(req)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusSkip, Detail: "session-api opt-out endpoint not reachable", Error: err.Error()}
	}
	resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusNoContent {
		return doctor.TestResult{
			Status: doctor.StatusSkip,
			Detail: fmt.Sprintf("opt-out endpoint returned HTTP %d (expected 204)", resp.StatusCode),
		}
	}

	// Clean up: remove opt-out after the test (best-effort).
	defer func() {
		delReq, _ := http.NewRequestWithContext(ctx, http.MethodDelete, optOutURL, bytes.NewReader(optOutBody))
		if delReq != nil {
			delReq.Header.Set(headerContentType, contentTypeJSON)
			r, e := memoryClient().Do(delReq)
			if e == nil {
				r.Body.Close() //nolint:errcheck
			}
		}
	}()

	// Now try to save a memory — should be rejected with 204 (opted out).
	_, status, err := p.saveMemory(ctx, "opt-out test content", nil)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: err.Error()}
	}

	switch status {
	case http.StatusNoContent:
		return doctor.TestResult{Status: doctor.StatusPass, Detail: "opted-out user's memory save rejected with 204"}
	case http.StatusCreated:
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: "memory saved despite user opt-out — privacy middleware not enforcing",
		}
	default:
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: fmt.Sprintf("unexpected HTTP %d (expected 204)", status),
		}
	}
}

// checkDeletionCascade saves a test memory, batch-deletes it, and verifies it
// is gone. Skips if the batch delete endpoint is not available (HTTP 404).
func (p *PrivacyChecker) checkDeletionCascade(ctx context.Context) doctor.TestResult {
	if r := p.requireWorkspace(); r != nil {
		return *r
	}
	if r := p.requireEnterprise(ctx); r != nil {
		return *r
	}

	_, status, err := p.saveMemory(ctx, "deletion cascade test", nil)
	if err != nil || status != http.StatusCreated {
		return doctor.TestResult{Status: doctor.StatusFail, Detail: "save failed before batch delete", Error: errString(err)}
	}

	batchURL := fmt.Sprintf("%s%s?workspace=%s&user_id=%s&limit=100",
		p.memoryAPIURL, privacyBatchDeletePath, p.workspace, privacyTestUserID)
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

// checkAuditLogWritten saves a memory and queries the audit endpoint to verify
// a memory_created event was recorded. Skips if the audit endpoint is not available.
func (p *PrivacyChecker) checkAuditLogWritten(ctx context.Context) doctor.TestResult {
	if r := p.requireWorkspace(); r != nil {
		return *r
	}
	if r := p.requireEnterprise(ctx); r != nil {
		return *r
	}

	// Probe the audit endpoint first.
	auditURL := p.memoryAPIURL + "/api/v1/audit/memories?workspace=" + p.workspace + "&limit=1"
	probeResp, err := fetchBody(ctx, memoryClient(), auditURL)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusSkip, Detail: "audit endpoint not available", Error: err.Error()}
	}
	_ = probeResp

	// Save a test memory to generate an audit event.
	memoryID, status, err := p.saveMemory(ctx, "audit log test "+time.Now().Format(time.RFC3339Nano), nil)
	if err != nil || status != http.StatusCreated {
		return doctor.TestResult{Status: doctor.StatusFail, Detail: "save failed before audit check", Error: errString(err)}
	}

	// Brief pause to let the async audit logger flush.
	time.Sleep(2 * time.Second)

	// Query for memory_created events, filtering by the specific memory ID
	// to avoid false passes from stale events left by previous runs.
	queryURL := p.memoryAPIURL + "/api/v1/audit/memories?workspace=" + p.workspace + "&eventTypes=memory_created&limit=20"
	body, err := fetchBody(ctx, memoryClient(), queryURL)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Detail: "audit query failed", Error: err.Error()}
	}

	var result struct {
		Entries []map[string]any `json:"entries"`
		Total   int64            `json:"total"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Detail: "invalid audit response", Error: err.Error()}
	}

	// Look for the specific memory ID in the audit entries.
	for _, entry := range result.Entries {
		if id, ok := entry["memory_id"].(string); ok && id == memoryID {
			return doctor.TestResult{
				Status: doctor.StatusPass,
				Detail: fmt.Sprintf("audit event found for memory %s", memoryID),
			}
		}
	}
	return doctor.TestResult{
		Status: doctor.StatusFail,
		Detail: fmt.Sprintf("no audit event found for memory %s (%d total events checked)", memoryID, len(result.Entries)),
	}
}

// encryptionSummary describes a single (workspace, serviceGroup, policy) triple.
type encryptionSummary struct {
	workspace    string
	serviceGroup string
	policyName   string
	kmsProvider  string
	keyID        string
}

// checkSessionEncryption walks all Workspaces and their service groups, resolves
// each privacyPolicyRef, and reports which (workspace, service-group) pairs have
// encryption-at-rest enabled and with what KMS provider/key.
//
// This is a configuration-summary check: it reads CRDs only — no probe writes or
// raw DB reads. It answers the question "does my intent match what's in the CRDs?"
// without requiring DB access from within the doctor.
//
// Exit status:
//   - skip: k8s client unavailable, or no Workspaces, or no service groups with a privacyPolicyRef
//   - pass: at least one service group has encryption enabled (summary listed in detail)
//   - fail: a privacyPolicyRef names a policy that does not exist in the cluster
func (p *PrivacyChecker) checkSessionEncryption(ctx context.Context) doctor.TestResult {
	if p.k8sClient == nil {
		return doctor.TestResult{Status: doctor.StatusSkip, Detail: "k8s client not available — CRD check skipped"}
	}

	var wsList omniav1alpha1.WorkspaceList
	if err := p.k8sClient.List(ctx, &wsList); err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Detail: "list Workspaces failed", Error: err.Error()}
	}
	if len(wsList.Items) == 0 {
		return doctor.TestResult{Status: doctor.StatusSkip, Detail: "no Workspaces found — encryption configuration cannot be verified"}
	}

	var (
		encrypted    []encryptionSummary
		missingRefs  []string
		checkedCount int
	)

	for i := range wsList.Items {
		ws := &wsList.Items[i]
		for j := range ws.Spec.Services {
			grp := &ws.Spec.Services[j]
			if grp.PrivacyPolicyRef == nil {
				continue
			}
			checkedCount++
			ref := grp.PrivacyPolicyRef
			policyNS := ws.Spec.Namespace.Name

			var policy eev1alpha1.SessionPrivacyPolicy
			key := types.NamespacedName{Name: ref.Name, Namespace: policyNS}
			if err := p.k8sClient.Get(ctx, key, &policy); err != nil {
				missingRefs = append(missingRefs,
					fmt.Sprintf("workspace %s / service group %s → ref %s/%s not found",
						ws.Name, grp.Name, policyNS, ref.Name))
				continue
			}

			if policy.Spec.Encryption == nil || !policy.Spec.Encryption.Enabled {
				continue
			}
			encrypted = append(encrypted, encryptionSummary{
				workspace:    ws.Name,
				serviceGroup: grp.Name,
				policyName:   ref.Name,
				kmsProvider:  string(policy.Spec.Encryption.KMSProvider),
				keyID:        policy.Spec.Encryption.KeyID,
			})
		}
	}

	if len(missingRefs) > 0 {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: fmt.Sprintf("unresolvable privacyPolicyRef(s): %s", strings.Join(missingRefs, "; ")),
		}
	}

	if checkedCount == 0 {
		return doctor.TestResult{
			Status: doctor.StatusSkip,
			Detail: "no service groups have a privacyPolicyRef — all sessions stored in plaintext (valid configuration)",
		}
	}

	if len(encrypted) == 0 {
		return doctor.TestResult{
			Status: doctor.StatusSkip,
			Detail: fmt.Sprintf("checked %d service group(s); none have encryption enabled — sessions stored in plaintext", checkedCount),
		}
	}

	parts := make([]string, 0, len(encrypted))
	for _, s := range encrypted {
		parts = append(parts, fmt.Sprintf("workspace=%s group=%s policy=%s kmsProvider=%s keyID=%s",
			s.workspace, s.serviceGroup, s.policyName, s.kmsProvider, s.keyID))
	}
	return doctor.TestResult{
		Status: doctor.StatusPass,
		Detail: fmt.Sprintf("%d encryption-enabled service group(s): %s", len(encrypted), strings.Join(parts, "; ")),
	}
}

// errString returns the error message or an empty string if err is nil.
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
