package main

import (
	"math/rand"
	"strings"
	"testing"
)

func TestAgentMemoriesUseAgentUID(t *testing.T) {
	s := DefaultScenario("ws")
	s.AgentUID = "11111111-2222-3333-4444-555555555555"
	out := Generate(s, rand.New(rand.NewSource(s.Seed)))
	for _, m := range out.AgentMemories {
		if m.AgentID != s.AgentUID {
			t.Fatalf("AgentID = %q, want %q", m.AgentID, s.AgentUID)
		}
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	s := DefaultScenario("ws")
	a := Generate(s, rand.New(rand.NewSource(s.Seed)))
	b := Generate(s, rand.New(rand.NewSource(s.Seed)))
	if len(a.Docs) != s.InstitutionalDocs {
		t.Fatalf("Docs = %d, want %d", len(a.Docs), s.InstitutionalDocs)
	}
	if len(a.AgentMemories) != s.AgentMemories {
		t.Fatalf("AgentMemories = %d, want %d", len(a.AgentMemories), s.AgentMemories)
	}
	if len(a.UserMemories) != s.Users*s.MemoriesPerUser {
		t.Fatalf("UserMemories = %d, want %d", len(a.UserMemories), s.Users*s.MemoriesPerUser)
	}
	wantObs := s.HotEntities * s.ObsPerHotEntity
	if len(a.HotObservations) != wantObs {
		t.Fatalf("HotObservations = %d, want %d", len(a.HotObservations), wantObs)
	}
	if a.Docs[0].Text != b.Docs[0].Text || a.Docs[0].URL != b.Docs[0].URL {
		t.Errorf("generation not deterministic for fixed seed")
	}
	if len(strings.Fields(a.Docs[0].Text)) < 250 {
		t.Errorf("doc text too short to chunk: %d words", len(strings.Fields(a.Docs[0].Text)))
	}
}

func TestUserMemoriesCoverAllCategories(t *testing.T) {
	s := DefaultScenario("ws")
	out := Generate(s, rand.New(rand.NewSource(s.Seed)))
	seen := map[string]bool{}
	for _, m := range out.UserMemories {
		seen[m.Category] = true
	}
	for _, c := range Categories {
		if !seen[c] {
			t.Errorf("category %q never generated", c)
		}
	}
}

func TestHotObservationsShareEntities(t *testing.T) {
	s := DefaultScenario("ws")
	out := Generate(s, rand.New(rand.NewSource(s.Seed)))
	byKey := map[string]int{}
	for _, o := range out.HotObservations {
		byKey[o.AboutKey]++
	}
	if len(byKey) != s.HotEntities {
		t.Fatalf("distinct hot entities = %d, want %d", len(byKey), s.HotEntities)
	}
	for k, n := range byKey {
		if n != s.ObsPerHotEntity {
			t.Errorf("entity %s has %d observations, want %d", k, n, s.ObsPerHotEntity)
		}
	}
}
