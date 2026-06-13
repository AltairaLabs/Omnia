import { describe, it, expect } from "vitest";
import { modelToFlow } from "./to-flow";
import type { WorkloadModel } from "./types";

const model: WorkloadModel = {
  tier: "workflow",
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

  it("applies dashed styling to unresolved edges and leaves normal edges unstyled", () => {
    const m: WorkloadModel = {
      ...model,
      edges: [
        { id: "u", source: "a", target: "ghost", style: "unresolved" },
        { id: "n", source: "a", target: "b", style: "normal" },
      ],
    };
    const { edges } = modelToFlow(m);
    expect(edges.find((e) => e.id === "u")?.style).toMatchObject({ strokeDasharray: "4 4" });
    expect(edges.find((e) => e.id === "u")?.animated).toBe(false);
    expect(edges.find((e) => e.id === "n")?.style).toBeUndefined();
  });

  it("passes an onClick handler through to node data", () => {
    const onClick = () => {};
    const { nodes } = modelToFlow(model, onClick);
    expect(nodes[0].data.onClick).toBe(onClick);
  });
});
