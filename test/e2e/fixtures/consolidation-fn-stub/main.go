/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

consolidation-fn-stub is a tiny HTTP server fronted by /functions/{name}
that synthesizes a deterministic action list referencing the observation
IDs in the request body. The E2E test deploys it as a Service in the
consolidation-e2e namespace and points the consolidation worker at it
via MemoryFunctionRef. Avoids spinning up a real function-mode
AgentRuntime (mock-provider can't emit valid dynamic JSON) while still
exercising the worker → HTTP → validator → applier → audit pipeline.
*/

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

// inputScope mirrors consolidation.Scope (we don't import the package
// to keep the fixture binary tiny — no omnia module dependencies).
type inputScope struct {
	WorkspaceID string `json:"workspaceID"`
	AgentID     string `json:"agentID,omitempty"`
	UserID      string `json:"userID,omitempty"`
}

type inputEntry struct {
	ID    string     `json:"id"`
	Scope inputScope `json:"scope"`
}

type inputBucket struct {
	Entries []inputEntry `json:"entries"`
}

type input struct {
	WorkspaceID string        `json:"workspaceID"`
	Buckets     []inputBucket `json:"buckets"`
}

func handler(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	var in input
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var ids []string
	for _, b := range in.Buckets {
		for _, e := range b.Entries {
			ids = append(ids, e.ID)
		}
	}
	if len(ids) == 0 {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, "[]")
		return
	}

	scope := inputScope{WorkspaceID: in.WorkspaceID}
	// Single create_summary over every input ID. The applier's
	// cross-reference resolution (supersede.withID handle) is exercised
	// when the applier walks the result list — we don't need a supersede
	// here since the goal is to verify the full pipeline lands at least
	// one row.
	out := []map[string]any{
		{
			"action":  "create_summary",
			"fromIDs": ids,
			"scope":   scope,
			"content": "stub-synthesized summary from " + fmt.Sprint(len(ids)) + " observations",
		},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func health(w http.ResponseWriter, _ *http.Request) {
	_, _ = fmt.Fprint(w, "ok")
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/functions/", handler)
	mux.HandleFunc("/healthz", health)
	log.Println("consolidation-fn-stub listening on :8080")
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	log.Fatal(server.ListenAndServe())
}
