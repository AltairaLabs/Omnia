import { describe, it, expect } from "vitest";
import { modelToFlow } from "./to-flow";
import type { WorkloadModel } from "./types";

const model: WorkloadModel = {
  tier: "flow",
  altitude: "deployment",
  nodes: [
    { id: "a", kind: "state", label: "A", isEntry: true, badges: [], detail: {} },
    { id: "b", kind: "state", label: "B", isTerminal: true, badges: [], detail: {} },
    { id: "provider:default", kind: "provider", label: "default", badges: [], detail: { model: "m" } },
  ],
  edges: [
    { id: "e1", source: "a", target: "b", label: "go", style: "normal" },
    { id: "e2", source: "b", target: "a", label: "max visits", style: "loop" },
  ],
  meta: { counts: { agents: 1, tools: 0, skills: 0, states: 2 } },
};

describe("modelToFlow", () => {
  it("maps node kinds to xyflow node types and carries data", () => {
    const { nodes } = modelToFlow(model);
    expect(nodes.find((n) => n.id === "a")?.type).toBe("workloadState");
    expect(nodes.find((n) => n.id === "provider:default")?.type).toBe("workloadProvider");
    expect(nodes.find((n) => n.id === "a")?.data.node.isEntry).toBe(true);
  });

  it("maps edges, dashing loop/unresolved styles", () => {
    const { edges } = modelToFlow(model);
    expect(edges).toHaveLength(2);
    expect(edges.find((e) => e.id === "e2")?.animated).toBe(true);
    expect(edges.find((e) => e.id === "e1")?.label).toBe("go");
  });
});
