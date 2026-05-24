/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package consolidation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClient_CallSendsAxisPayload(t *testing.T) {
	var received FunctionInput
	var receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"action":"create_summary","fromIDs":["a"],"scope":{"workspaceID":"ws-1"},"content":"x"}]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, 5*time.Second)
	input := FunctionInput{
		Axis:        AxisStaleObservations,
		WorkspaceID: testWorkspaceID,
		Buckets:     []Bucket{{Key: "k1", Entries: []BucketEntry{{ID: "a"}}}},
	}
	actions, err := c.Call(context.Background(), "demo", input)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if received.Axis != AxisStaleObservations || received.WorkspaceID != testWorkspaceID {
		t.Errorf("received payload mismatch: %+v", received)
	}
	if !strings.HasSuffix(receivedPath, "/functions/demo") {
		t.Errorf("path = %q, want suffix /functions/demo", receivedPath)
	}
	if len(actions) != 1 || actions[0].Kind() != ActionCreateSummary {
		t.Fatalf("actions = %+v", actions)
	}
}

func TestClient_TimeoutSurfaced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, 50*time.Millisecond)
	_, err := c.Call(context.Background(), "demo", FunctionInput{
		Axis: AxisStaleObservations, WorkspaceID: testWorkspaceID,
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestClient_NonOKStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`oops`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, time.Second)
	_, err := c.Call(context.Background(), "demo", FunctionInput{
		Axis: AxisStaleObservations, WorkspaceID: testWorkspaceID,
	})
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("expected 500 error, got %v", err)
	}
}

func TestClient_RejectsInvalidResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not an array`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, time.Second)
	_, err := c.Call(context.Background(), "demo", FunctionInput{
		Axis: AxisStaleObservations, WorkspaceID: testWorkspaceID,
	})
	if err == nil {
		t.Fatal("expected decode error")
	}
}
