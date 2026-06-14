package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/altairalabs/omnia/pkg/identity"
	"golang.org/x/time/rate"
)

// JSON field-name and value constants shared across request bodies. These are
// wire-contract strings, extracted so repeated literals stay in one place.
const (
	fieldWorkspaceID      = "workspace_id"
	fieldConfidence       = "confidence"
	fieldType             = "type"
	fieldContent          = "content"
	fieldUserID           = "user_id"
	aboutKindSupportTopic = "support_topic"

	// observationOwnerUser owns the compaction-fodder observation clusters.
	// /api/v1/memories requires a user_id in scope, so the shared-entity
	// observations are attributed to one synthetic ops user.
	observationOwnerUser = "hawkridge-ops"
)

// seedRPS/seedBurst keep the seeder under memory-api's per-IP rate limit
// (default 100 rps / 200 burst, see internal/session/api/ratelimit.go).
const (
	seedRPS   = 60
	seedBurst = 30
)

// Client posts seed data to a memory-api instance for one workspace.
type Client struct {
	base         string
	workspaceUID string
	http         *http.Client
	limiter      *rate.Limiter
}

func NewClient(base, workspaceUID string) *Client {
	return &Client{
		base:         base,
		workspaceUID: workspaceUID,
		http:         &http.Client{},
		limiter:      rate.NewLimiter(seedRPS, seedBurst),
	}
}

type saveResp struct {
	Memory struct {
		ID string `json:"id"`
	} `json:"memory"`
}

func (c *Client) postJSON(
	ctx context.Context, path string, query url.Values, body any, wantStatus int,
) ([]byte, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	u := c.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	out := new(bytes.Buffer)
	_, _ = out.ReadFrom(resp.Body)
	if resp.StatusCode != wantStatus {
		return nil, fmt.Errorf("%s: status %d: %s", path, resp.StatusCode, out.String())
	}
	return out.Bytes(), nil
}

// Ingest posts a document for chunk-strategy institutional ingestion (202).
func (c *Client) Ingest(ctx context.Context, d Doc) error {
	body := map[string]any{
		fieldWorkspaceID: c.workspaceUID,
		"title":          d.Title, "url": d.URL, "site": d.Site, "text": d.Text,
	}
	_, err := c.postJSON(ctx, "/api/v1/institutional/ingest", nil, body, http.StatusAccepted)
	return err
}

// SaveInstitutional saves a single institutional fact directly (201).
func (c *Client) SaveInstitutional(ctx context.Context, typ, content string, confidence float64) (string, error) {
	body := map[string]any{
		fieldWorkspaceID: c.workspaceUID, fieldType: typ, fieldContent: content, fieldConfidence: confidence,
	}
	return c.saveID(ctx, "/api/v1/institutional/memories", nil, body)
}

// SaveAgentMemory saves an agent-tier memory (201).
func (c *Client) SaveAgentMemory(ctx context.Context, m AgentMemory) (string, error) {
	body := map[string]any{
		fieldWorkspaceID: c.workspaceUID, "agent_id": m.AgentID,
		fieldType: m.Type, fieldContent: m.Content, fieldConfidence: m.Confidence,
	}
	return c.saveID(ctx, "/api/v1/agent-memories", nil, body)
}

// SaveUserMemory saves a user-tier memory scoped to PseudonymizeID(RawUserID).
func (c *Client) SaveUserMemory(ctx context.Context, m UserMemory) (string, error) {
	hashed := identity.PseudonymizeID(m.RawUserID)
	q := url.Values{"workspace": {c.workspaceUID}, fieldUserID: {hashed}}
	body := map[string]any{
		fieldType: m.Type, fieldContent: m.Content, fieldConfidence: m.Confidence,
		"category": m.Category,
		"scope":    map[string]string{fieldWorkspaceID: c.workspaceUID, fieldUserID: hashed},
		"metadata": map[string]any{"provenance": "user_requested"},
	}
	return c.saveID(ctx, "/api/v1/memories", q, body)
}

// SaveObservation appends an observation to a shared entity (compaction fodder).
// /api/v1/memories requires a user_id in scope, so observations are owned by a
// fixed synthetic ops user while still clustering on a shared about key.
func (c *Client) SaveObservation(ctx context.Context, o HotObservation) (string, error) {
	hashed := identity.PseudonymizeID(observationOwnerUser)
	q := url.Values{"workspace": {c.workspaceUID}, fieldUserID: {hashed}}
	body := map[string]any{
		fieldType: aboutKindSupportTopic, fieldContent: o.Content, fieldConfidence: 0.5,
		"about":    map[string]string{"kind": o.AboutKind, "key": o.AboutKey},
		"scope":    map[string]string{fieldWorkspaceID: c.workspaceUID, fieldUserID: hashed},
		"metadata": map[string]any{"provenance": "system_generated"},
	}
	return c.saveID(ctx, "/api/v1/memories", q, body)
}

// Link creates a relation edge between two entities (201).
func (c *Client) Link(ctx context.Context, srcID, tgtID, relType string, weight float64) error {
	body := map[string]any{
		"source_id": srcID, "target_id": tgtID, "relation_type": relType,
		"weight": weight, "scope": map[string]string{fieldWorkspaceID: c.workspaceUID},
	}
	_, err := c.postJSON(ctx, "/api/v1/relations", nil, body, http.StatusCreated)
	return err
}

func (c *Client) saveID(ctx context.Context, path string, q url.Values, body any) (string, error) {
	raw, err := c.postJSON(ctx, path, q, body, http.StatusCreated)
	if err != nil {
		return "", err
	}
	var sr saveResp
	if err := json.Unmarshal(raw, &sr); err != nil {
		return "", err
	}
	return sr.Memory.ID, nil
}
