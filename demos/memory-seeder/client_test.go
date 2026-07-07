package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altairalabs/omnia/pkg/identity"
)

// Test-scoped constants for repeated literals (keeps goconst quiet).
const (
	fieldMemory = "memory"
	testWSUID   = "ws-uid"
)

func decode(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	body, _ := io.ReadAll(r.Body)
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	return m
}

func TestSaveUserMemorySuppressed204IsNotAnError(t *testing.T) {
	// The enterprise privacy middleware suppresses consent-violating writes
	// with 204 No Content. The seeder must treat that as a skip, not a failure.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, testWSUID)
	id, err := c.SaveUserMemory(context.Background(), UserMemory{
		RawUserID: "customer-001", Category: "memory:health",
		Type: "profile", Content: "suppressed", Confidence: 0.9,
	})
	if err != nil {
		t.Fatalf("SaveUserMemory on 204: unexpected error %v", err)
	}
	if id != "" {
		t.Errorf("id = %q, want \"\" (suppressed)", id)
	}
}

func TestSaveUserMemoryHashesUserAndScopes(t *testing.T) {
	var gotPath, gotWorkspace, gotUserParam string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotWorkspace = r.URL.Query().Get("workspace")
		gotUserParam = r.URL.Query().Get("virtual_user_id")
		gotBody = decode(t, r)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{fieldMemory: map[string]any{"id": "ent-1"}})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, testWSUID)
	id, err := c.SaveUserMemory(context.Background(), UserMemory{
		RawUserID: "customer-001", Category: "memory:identity",
		Type: "profile", Content: "hello", Confidence: 0.9,
	})
	if err != nil {
		t.Fatalf("SaveUserMemory: %v", err)
	}
	if id != "ent-1" {
		t.Errorf("id = %q, want ent-1", id)
	}
	if gotPath != "/api/v1/memories" {
		t.Errorf("path = %q", gotPath)
	}
	if gotWorkspace != testWSUID {
		t.Errorf("workspace param = %q", gotWorkspace)
	}
	wantHash := identity.PseudonymizeID("customer-001")
	if gotUserParam != wantHash {
		t.Errorf("user_id param = %q, want hashed %q", gotUserParam, wantHash)
	}
	scope, _ := gotBody["scope"].(map[string]any)
	if scope[fieldWorkspaceID] != testWSUID || scope["virtual_user_id"] != wantHash {
		t.Errorf("body scope = %v", scope)
	}
	if gotBody["category"] != "memory:identity" {
		t.Errorf("category = %v", gotBody["category"])
	}
	meta, _ := gotBody["metadata"].(map[string]any)
	if meta["provenance"] != "user_requested" {
		t.Errorf("provenance = %v, want user_requested", meta["provenance"])
	}
}

func TestIngestPostsDocAndAccepts202(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/institutional/ingest" {
			t.Errorf("path = %q", r.URL.Path)
		}
		gotBody = decode(t, r)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, testWSUID)
	err := c.Ingest(context.Background(), Doc{Title: "t", URL: "kb://1", Site: "s", Text: "body"})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if gotBody["workspace_id"] != "ws-uid" || gotBody["url"] != "kb://1" {
		t.Errorf("body = %v", gotBody)
	}
}

func TestSaveObservationRepeatsAboutKey(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody = decode(t, r)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{fieldMemory: map[string]any{"id": "ent-2"}})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, testWSUID)
	_, err := c.SaveObservation(context.Background(), HotObservation{
		AboutKind: aboutKindSupportTopic, AboutKey: "hot-entity-00", Content: "obs",
	})
	if err != nil {
		t.Fatalf("SaveObservation: %v", err)
	}
	about, _ := gotBody["about"].(map[string]any)
	if about["kind"] != aboutKindSupportTopic || about["key"] != "hot-entity-00" {
		t.Errorf("about = %v", about)
	}
	meta, _ := gotBody["metadata"].(map[string]any)
	if meta["provenance"] != "system_generated" {
		t.Errorf("provenance = %v, want system_generated", meta["provenance"])
	}
	// /api/v1/memories requires a user_id in scope; observations are owned by
	// a synthetic ops user so the write is accepted.
	scope, _ := gotBody["scope"].(map[string]any)
	wantOwner := identity.PseudonymizeID(observationOwnerUser)
	if scope[fieldUserID] != wantOwner {
		t.Errorf("scope user_id = %v, want %q", scope[fieldUserID], wantOwner)
	}
}

func TestSaveInstitutionalPostsFact(t *testing.T) {
	var gotPath, gotWorkspace string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotWorkspace = r.URL.Query().Get("workspace")
		gotBody = decode(t, r)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{fieldMemory: map[string]any{"id": "inst-1"}})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, testWSUID)
	id, err := c.SaveInstitutional(context.Background(), "policy", "always escalate", 0.8)
	if err != nil {
		t.Fatalf("SaveInstitutional: %v", err)
	}
	if id != "inst-1" {
		t.Errorf("id = %q, want inst-1", id)
	}
	if gotPath != "/api/v1/institutional/memories" {
		t.Errorf("path = %q", gotPath)
	}
	// The handler reads the workspace from the query param, not the body.
	if gotWorkspace != testWSUID {
		t.Errorf("workspace query = %q, want %q", gotWorkspace, testWSUID)
	}
	if gotBody[fieldWorkspaceID] != testWSUID {
		t.Errorf("workspace_id = %v", gotBody[fieldWorkspaceID])
	}
	if gotBody["type"] != "policy" {
		t.Errorf("type = %v", gotBody["type"])
	}
	if gotBody["content"] != "always escalate" {
		t.Errorf("content = %v", gotBody["content"])
	}
}

func TestSaveAgentMemoryPostsFact(t *testing.T) {
	var gotPath, gotWorkspace string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotWorkspace = r.URL.Query().Get("workspace")
		gotBody = decode(t, r)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{fieldMemory: map[string]any{"id": "agent-1"}})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, testWSUID)
	id, err := c.SaveAgentMemory(context.Background(), AgentMemory{
		AgentID: "support-agent", Type: "resolution_pattern", Content: "step 3", Confidence: 0.7,
	})
	if err != nil {
		t.Fatalf("SaveAgentMemory: %v", err)
	}
	if id != "agent-1" {
		t.Errorf("id = %q, want agent-1", id)
	}
	if gotPath != "/api/v1/agent-memories" {
		t.Errorf("path = %q", gotPath)
	}
	// The handler reads the workspace from the query param, not the body.
	if gotWorkspace != testWSUID {
		t.Errorf("workspace query = %q, want %q", gotWorkspace, testWSUID)
	}
	if gotBody[fieldWorkspaceID] != testWSUID || gotBody["agent_id"] != "support-agent" {
		t.Errorf("body = %v", gotBody)
	}
}

func TestLinkPostsRelation(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotBody = decode(t, r)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, testWSUID)
	err := c.Link(context.Background(), "src-1", "tgt-1", "relates_to", 0.4)
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if gotPath != "/api/v1/relations" {
		t.Errorf("path = %q", gotPath)
	}
	if gotBody["source_id"] != "src-1" || gotBody["target_id"] != "tgt-1" {
		t.Errorf("body ids = %v", gotBody)
	}
	if gotBody["relation_type"] != "relates_to" {
		t.Errorf("relation_type = %v", gotBody["relation_type"])
	}
	if gotBody["weight"] != 0.4 {
		t.Errorf("weight = %v", gotBody["weight"])
	}
	scope, _ := gotBody["scope"].(map[string]any)
	if scope[fieldWorkspaceID] != testWSUID {
		t.Errorf("scope = %v", scope)
	}
}

func TestPostJSONErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, testWSUID)
	_, err := c.SaveInstitutional(context.Background(), "policy", "x", 0.5)
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
}
