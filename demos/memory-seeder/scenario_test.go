package main

import "testing"

func TestDefaultScenarioCounts(t *testing.T) {
	s := DefaultScenario("ws-uid-123")
	if s.WorkspaceUID != "ws-uid-123" {
		t.Fatalf("WorkspaceUID = %q, want ws-uid-123", s.WorkspaceUID)
	}
	checks := map[string]int{
		"InstitutionalDocs": s.InstitutionalDocs,
		"AgentMemories":     s.AgentMemories,
		"Users":             s.Users,
		"MemoriesPerUser":   s.MemoriesPerUser,
		"HotEntities":       s.HotEntities,
		"ObsPerHotEntity":   s.ObsPerHotEntity,
	}
	for name, v := range checks {
		if v <= 0 {
			t.Errorf("%s = %d, want > 0", name, v)
		}
	}
	if s.ObsPerHotEntity < 10 {
		t.Errorf("ObsPerHotEntity = %d, want >= 10 (worker min group size)", s.ObsPerHotEntity)
	}
}
