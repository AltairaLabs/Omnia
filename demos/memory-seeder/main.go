package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
)

// run executes the seed in dependency order and links a sample of entities.
func run(ctx context.Context, c *Client, g Generated) error {
	for _, d := range g.Docs {
		if err := c.Ingest(ctx, d); err != nil {
			return fmt.Errorf("ingest: %w", err)
		}
	}
	for _, m := range g.AgentMemories {
		if _, err := c.SaveAgentMemory(ctx, m); err != nil {
			return fmt.Errorf("agent memory: %w", err)
		}
	}
	instIDs, err := seedInstitutionalFacts(ctx, c)
	if err != nil {
		return err
	}
	userIDs, err := seedUserMemories(ctx, c, g.UserMemories)
	if err != nil {
		return err
	}
	for _, o := range g.HotObservations {
		if _, err := c.SaveObservation(ctx, o); err != nil {
			return fmt.Errorf("observation: %w", err)
		}
	}
	return linkSample(ctx, c, instIDs, userIDs)
}

// seedInstitutionalFacts writes the institutional policy facts, skipping any
// consent-suppressed (204 → empty id) writes so they aren't linked.
func seedInstitutionalFacts(ctx context.Context, c *Client) ([]string, error) {
	var ids []string
	for i := range 20 {
		id, err := c.SaveInstitutional(ctx, "policy",
			fmt.Sprintf("Hawkridge policy fact %d", i), 0.9)
		if err != nil {
			return nil, fmt.Errorf("institutional: %w", err)
		}
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// seedUserMemories writes the user-tier memories, skipping consent-suppressed
// (204 → empty id) writes so they aren't linked.
func seedUserMemories(ctx context.Context, c *Client, mems []UserMemory) ([]string, error) {
	var ids []string
	for _, m := range mems {
		id, err := c.SaveUserMemory(ctx, m)
		if err != nil {
			return nil, fmt.Errorf("user memory: %w", err)
		}
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// linkSample wires a few hundred edges so the graph has structure.
func linkSample(ctx context.Context, c *Client, instIDs, userIDs []string) error {
	for i := 1; i < len(instIDs); i++ {
		if err := c.Link(ctx, instIDs[i-1], instIDs[i], "RELATED_TO", 1.0); err != nil {
			return fmt.Errorf("link inst: %w", err)
		}
	}
	for i, uid := range userIDs {
		if len(instIDs) == 0 {
			break
		}
		if err := c.Link(ctx, uid, instIDs[i%len(instIDs)], "MENTIONS", 0.7); err != nil {
			return fmt.Errorf("link user: %w", err)
		}
	}
	return nil
}

func main() {
	base := flag.String("memory-api", "http://localhost:8080", "memory-api base URL")
	wsUID := flag.String("workspace-uid", "", "workspace metadata.uid (required)")
	agentUID := flag.String("agent-uid", "", "AgentRuntime metadata.uid to scope agent-tier memories to (required)")
	seed := flag.Int64("seed", 1, "rng seed for deterministic output")
	flag.Parse()
	if *wsUID == "" {
		log.Fatal("--workspace-uid is required " +
			"(kubectl get workspace dev-agents -n dev-agents -o jsonpath='{.metadata.uid}')")
	}
	if *agentUID == "" {
		log.Fatal("--agent-uid is required " +
			"(kubectl get agentruntime demo-agent -n dev-agents -o jsonpath='{.metadata.uid}')")
	}
	s := DefaultScenario(*wsUID)
	s.AgentUID = *agentUID
	s.Seed = *seed
	g := Generate(s, rand.New(rand.NewSource(s.Seed)))
	c := NewClient(*base, *wsUID)
	if err := run(context.Background(), c, g); err != nil {
		fmt.Fprintln(os.Stderr, "seed failed:", err)
		os.Exit(1)
	}
	fmt.Printf("seeded: %d docs, %d agent, %d user, %d observations\n",
		len(g.Docs), len(g.AgentMemories), len(g.UserMemories), len(g.HotObservations))
}
