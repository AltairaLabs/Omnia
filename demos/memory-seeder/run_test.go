package main

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestRunPostsAllTiers(t *testing.T) {
	var ingest, agent, user, obs, institutional, relations int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/institutional/ingest":
			atomic.AddInt64(&ingest, 1)
			w.WriteHeader(http.StatusAccepted)
			return
		case "/api/v1/institutional/memories":
			atomic.AddInt64(&institutional, 1)
		case "/api/v1/agent-memories":
			atomic.AddInt64(&agent, 1)
		case "/api/v1/relations":
			atomic.AddInt64(&relations, 1)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "rel"})
			return
		case "/api/v1/memories":
			body := decode(t, r)
			if _, ok := body["about"]; ok {
				atomic.AddInt64(&obs, 1)
			} else {
				atomic.AddInt64(&user, 1)
			}
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"memory": map[string]any{"id": "ent"}})
	}))
	defer srv.Close()

	s := Scenario{WorkspaceUID: "ws", InstitutionalDocs: 3, AgentMemories: 2,
		Users: 2, MemoriesPerUser: 2, HotEntities: 2, ObsPerHotEntity: 10, Seed: 1}
	g := Generate(s, rand.New(rand.NewSource(s.Seed)))
	c := NewClient(srv.URL, "ws")
	if err := run(context.Background(), c, g); err != nil {
		t.Fatalf("run: %v", err)
	}
	if ingest != 3 {
		t.Errorf("ingest = %d, want 3", ingest)
	}
	if agent != 2 {
		t.Errorf("agent = %d, want 2", agent)
	}
	if user != 4 {
		t.Errorf("user = %d, want 4", user)
	}
	if obs != 20 {
		t.Errorf("obs = %d, want 20", obs)
	}
	if relations == 0 {
		t.Errorf("relations = 0, want > 0")
	}
}
