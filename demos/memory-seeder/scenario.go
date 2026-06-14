package main

// Scenario holds the configurable knobs for the "Hawkridge Cloud" seed.
type Scenario struct {
	WorkspaceUID      string
	AgentUID          string
	InstitutionalDocs int
	AgentMemories     int
	Users             int
	MemoriesPerUser   int
	HotEntities       int
	ObsPerHotEntity   int
	Seed              int64
}

// DefaultScenario returns realistic-scale defaults: ~300 docs → thousands of
// institutional chunks, ~300 agent memories, ~40 users × 12, ~20 hot entities
// × 15 observations for compaction fodder.
func DefaultScenario(workspaceUID string) Scenario {
	return Scenario{
		WorkspaceUID:      workspaceUID,
		InstitutionalDocs: 300,
		AgentMemories:     300,
		Users:             40,
		MemoriesPerUser:   12,
		HotEntities:       20,
		ObsPerHotEntity:   15,
		Seed:              1,
	}
}
