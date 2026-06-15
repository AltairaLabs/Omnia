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

// DefaultScenario returns a galaxy-friendly scale: a few thousand DIVERSE
// points spread across six semantic topics, dominated by categorised user
// memories (so colour + clusters show) rather than duplicated boilerplate.
// 36 distinct topical docs (≈6 per topic) chunk into a modest, varied
// institutional set instead of one blob.
func DefaultScenario(workspaceUID string) Scenario {
	return Scenario{
		WorkspaceUID:      workspaceUID,
		InstitutionalDocs: 36,
		AgentMemories:     240,
		Users:             60,
		MemoriesPerUser:   18,
		HotEntities:       12,
		ObsPerHotEntity:   12,
		Seed:              1,
	}
}
