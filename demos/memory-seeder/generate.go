package main

import (
	"fmt"
	"math/rand"
	"strings"
)

// Categories are the six product consent categories used for user-tier memories.
var Categories = []string{
	"memory:identity", "memory:context", "memory:health",
	"memory:location", "memory:preferences", "memory:history",
}

type Doc struct {
	Title, URL, Site, Text string
}
type UserMemory struct {
	RawUserID, Category, Type, Content string
	Confidence                         float64
}
type AgentMemory struct {
	AgentID, Type, Content string
	Confidence             float64
}
type HotObservation struct {
	AboutKind, AboutKey, Content string
}

// Generated is the full deterministic seed payload.
type Generated struct {
	Docs            []Doc
	AgentMemories   []AgentMemory
	UserMemories    []UserMemory
	HotObservations []HotObservation
}

var docTopics = []string{
	"refund policy", "tenant reset runbook", "billing dispute process",
	"SSO troubleshooting", "rate-limit error E_QUOTA", "data export procedure",
	"incident severity matrix", "API deprecation schedule", "GDPR DSAR handling",
	"webhook retry semantics",
}

var paragraph = "Hawkridge Cloud support agents must follow the documented procedure. " +
	"Escalate to tier two when the customer is on the Enterprise plan or when data loss is suspected. " +
	"Record the ticket reference and the affected tenant id in the case notes. " +
	"Confirm the customer's identity before disclosing account details. "

// Generate produces the deterministic seed for s using r.
func Generate(s Scenario, r *rand.Rand) Generated {
	return Generated{
		Docs:            genDocs(s, r),
		AgentMemories:   genAgentMemories(s, r),
		UserMemories:    genUserMemories(s, r),
		HotObservations: genHotObservations(s),
	}
}

// paragraphsPerDoc makes each generated doc ~1200 words so the chunk strategy
// (200-word windows) fans it into ~8 institutional chunks — ~300 docs → ~2000+
// institutional memories.
const paragraphsPerDoc = 24

func genDocs(s Scenario, r *rand.Rand) []Doc {
	docs := make([]Doc, s.InstitutionalDocs)
	for i := range docs {
		topic := docTopics[r.Intn(len(docTopics))]
		text := strings.Repeat(fmt.Sprintf("%s (%s). ", paragraph, topic), paragraphsPerDoc)
		docs[i] = Doc{
			Title: fmt.Sprintf("KB-%04d: %s", i, topic),
			URL:   fmt.Sprintf("kb://hawkridge/%04d", i),
			Site:  "hawkridge-support",
			Text:  text,
		}
	}
	return docs
}

func genAgentMemories(s Scenario, r *rand.Rand) []AgentMemory {
	out := make([]AgentMemory, s.AgentMemories)
	for i := range out {
		topic := docTopics[r.Intn(len(docTopics))]
		out[i] = AgentMemory{
			AgentID:    s.AgentUID,
			Type:       "resolution_pattern",
			Content:    fmt.Sprintf("Customers hitting %s are usually resolved by step %d of the runbook.", topic, 1+r.Intn(5)),
			Confidence: 0.5 + r.Float64()*0.5,
		}
	}
	return out
}

func genUserMemories(s Scenario, r *rand.Rand) []UserMemory {
	out := make([]UserMemory, 0, s.Users*s.MemoriesPerUser)
	for u := 0; u < s.Users; u++ {
		raw := fmt.Sprintf("customer-%03d", u)
		for j := 0; j < s.MemoriesPerUser; j++ {
			cat := Categories[(u+j)%len(Categories)]
			out = append(out, UserMemory{
				RawUserID:  raw,
				Category:   cat,
				Type:       "profile",
				Content:    fmt.Sprintf("%s note for %s: %s", cat, raw, docTopics[r.Intn(len(docTopics))]),
				Confidence: 0.6 + r.Float64()*0.4,
			})
		}
	}
	return out
}

func genHotObservations(s Scenario) []HotObservation {
	out := make([]HotObservation, 0, s.HotEntities*s.ObsPerHotEntity)
	for e := 0; e < s.HotEntities; e++ {
		key := fmt.Sprintf("hot-entity-%02d", e)
		for o := 0; o < s.ObsPerHotEntity; o++ {
			out = append(out, HotObservation{
				AboutKind: aboutKindSupportTopic,
				AboutKey:  key,
				Content:   fmt.Sprintf("Observation %d about %s: customer reported recurring issue.", o, key),
			})
		}
	}
	return out
}
