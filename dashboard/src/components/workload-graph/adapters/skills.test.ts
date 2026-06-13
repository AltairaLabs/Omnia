import { describe, it, expect } from "vitest";
import { skillNodesAndEdges, attachSkills } from "./skills";
import type { SkillSource, SkillSourcePhase } from "@/types/skill-source";
import type { WorkloadModel } from "../types";

function source(name: string, phase?: SkillSourcePhase, skillCount?: number): SkillSource {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "SkillSource",
    metadata: { name },
    spec: { type: "git", interval: "1h" },
    status: phase ? { phase, skillCount } : undefined,
  } as SkillSource;
}

describe("skillNodesAndEdges", () => {
  it("resolves a Ready source to a resolved node with count, attached to entry", () => {
    const sources = new Map([["anthropic", source("anthropic", "Ready", 12)]]);
    const { nodes, edges } = skillNodesAndEdges(
      [{ source: "anthropic", include: ["pdf"], mountAs: "skills" }],
      sources,
      "plan",
    );
    expect(nodes).toHaveLength(1);
    expect(nodes[0]).toMatchObject({
      id: "skill:anthropic",
      kind: "skill",
      resolution: "resolved",
      detail: { skillSource: "anthropic", include: ["pdf"], mountAs: "skills", skillCount: 12, skillPhase: "Ready" },
    });
    expect(nodes[0].badges[0].label).toBe("Ready · 12");
    expect(edges).toEqual([
      { id: "plan--provides-->skill:anthropic", source: "plan", target: "skill:anthropic", style: "provides" },
    ]);
  });

  it("maps Fetching to unresolved and Error to unavailable", () => {
    const sources = new Map([
      ["a", source("a", "Fetching")],
      ["b", source("b", "Error")],
    ]);
    const { nodes } = skillNodesAndEdges(
      [{ source: "a" }, { source: "b" }],
      sources,
      "entry",
    );
    expect(nodes[0].resolution).toBe("unresolved");
    expect(nodes[1].resolution).toBe("unavailable");
  });

  it("marks a missing source unavailable with phase 'missing'", () => {
    const { nodes } = skillNodesAndEdges([{ source: "ghost" }], new Map(), "entry");
    expect(nodes[0].resolution).toBe("unavailable");
    expect(nodes[0].detail.skillPhase).toBe("missing");
    expect(nodes[0].badges[0].label).toBe("missing");
  });

  it("emits no edges when there is no entry node", () => {
    const sources = new Map([["a", source("a", "Ready", 1)]]);
    const { edges } = skillNodesAndEdges([{ source: "a" }], sources, undefined);
    expect(edges).toEqual([]);
  });

  it("returns nothing for no refs", () => {
    expect(skillNodesAndEdges(undefined, new Map(), "e")).toEqual({ nodes: [], edges: [] });
  });
});

const base: WorkloadModel = {
  tier: "flow",
  altitude: "definition",
  nodes: [{ id: "plan", kind: "state", label: "Plan", isEntry: true, badges: [], detail: {} }],
  edges: [],
  meta: { counts: { agents: 1, tools: 0, skills: 0, states: 1 } },
};

describe("attachSkills", () => {
  it("is a no-op when there are no skill refs", () => {
    expect(attachSkills(base, undefined, [])).toBe(base);
    expect(attachSkills(base, [], [])).toBe(base);
  });

  it("appends skill nodes/edges and bumps the skill count", () => {
    const out = attachSkills(base, [{ source: "anthropic" }], [source("anthropic", "Ready", 5)]);
    expect(out.nodes).toHaveLength(2);
    expect(out.edges).toHaveLength(1);
    expect(out.meta.counts.skills).toBe(1);
    expect(out.nodes[1].id).toBe("skill:anthropic");
  });

  it("falls back to the first node when no entry is flagged", () => {
    const noEntry: WorkloadModel = {
      ...base,
      nodes: [{ id: "first", kind: "agent", label: "First", badges: [], detail: {} }],
    };
    const out = attachSkills(noEntry, [{ source: "a" }], [source("a", "Ready", 1)]);
    expect(out.edges[0].source).toBe("first");
  });
});
