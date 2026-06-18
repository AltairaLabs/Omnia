import { describe, it, expect, vi } from "vitest";
import { modelToFlow } from "./to-flow";
import type { WorkloadModel, WorkloadNode } from "./types";

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
    expect(nodes.find((n) => n.id === "a")?.type).toBe("workflowState");
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
    const { nodes } = modelToFlow(model, { onClick });
    expect(nodes[0].data.onClick).toBe(onClick);
  });
});

function compositionState(): WorkloadNode {
  return {
    id: "main", kind: "state", label: "main", badges: [{ label: "composition" }],
    detail: {
      compositionName: "analyze", stepCount: 2,
      composition: {
        name: "analyze",
        nodes: [
          { id: "main::a", parentId: "main", kind: "stepPrompt", label: "a", badges: [], detail: {} },
          { id: "main::b", parentId: "main", kind: "stepPrompt", label: "b", badges: [], detail: {} },
        ],
        edges: [{ id: "main::a->b", source: "main::a", target: "main::b" }],
      },
    },
  };
}

const compModel = (): WorkloadModel => ({
  tier: "workflow", altitude: "definition",
  nodes: [compositionState()], edges: [],
  meta: { counts: { agents: 1, tools: 0, skills: 0, states: 1 } },
});

describe("modelToFlow — composition collapsed", () => {
  it("emits a single workflowState node flagged expandable when not expanded", () => {
    const { nodes } = modelToFlow(compModel(), { expanded: new Set() });
    expect(nodes).toHaveLength(1);
    expect(nodes[0].type).toBe("workflowState");
    expect((nodes[0].data as { expandable?: boolean }).expandable).toBe(true);
  });
});

describe("modelToFlow — composition expanded", () => {
  it("emits a container before its children, children carry parentId + extent", () => {
    const { nodes, edges } = modelToFlow(compModel(), { expanded: new Set(["main"]) });
    const ids = nodes.map((n) => n.id);
    expect(ids).toEqual(["main", "main::a", "main::b"]); // container first
    expect(nodes[0].type).toBe("composition");
    expect(nodes[1].parentId).toBe("main");
    expect(nodes[1].extent).toBe("parent");
    expect(edges.some((e) => e.source === "main::a" && e.target === "main::b")).toBe(true);
  });

  it("wires onToggle into container and collapsed node data", () => {
    const onToggle = vi.fn();
    const collapsed = modelToFlow(compModel(), { expanded: new Set(), onToggle });
    (collapsed.nodes[0].data as { onToggle?: (id: string) => void }).onToggle?.("main");
    expect(onToggle).toHaveBeenCalledWith("main");
  });
});

describe("modelToFlow — data-flow kinds", () => {
  it("maps new kinds to node types, sets size, and styles data edges", () => {
    const m: WorkloadModel = {
      tier: "workflow", altitude: "definition",
      nodes: [
        { id: "initial", kind: "initial", label: "", badges: [], detail: {} },
        { id: "var:topic", kind: "variable", label: "topic", badges: [], detail: {} },
        { id: "artifact:notes", kind: "artifact", label: "notes", badges: [], detail: {} },
      ],
      edges: [{ id: "e", source: "var:topic", target: "initial", style: "data" }],
      meta: { counts: { agents: 0, tools: 0, skills: 0, states: 0 } },
    };
    const { nodes, edges } = modelToFlow(m);
    expect(nodes.find((n) => n.id === "initial")!.type).toBe("workflowInitial");
    expect(nodes.find((n) => n.id === "var:topic")!.width).toBe(120);
    expect(nodes.find((n) => n.id === "artifact:notes")!.type).toBe("workflowArtifact");
    expect(edges[0].style).toMatchObject({ strokeDasharray: "3 3" });
  });

  it("maps arena harness kinds to their node types", () => {
    const m: WorkloadModel = {
      tier: "single", altitude: "test",
      nodes: [
        { id: "scenarios", kind: "scenario", label: "3 scenarios", badges: [], detail: {} },
        { id: "judge:r", kind: "judge", label: "r", badges: [], detail: {} },
        { id: "persona:p", kind: "persona", label: "p", badges: [], detail: {} },
      ],
      edges: [],
      meta: { counts: { agents: 1, tools: 0, skills: 0, states: 0 } },
    };
    const { nodes } = modelToFlow(m);
    expect(nodes.find((n) => n.id === "scenarios")!.type).toBe("workflowScenario");
    expect(nodes.find((n) => n.id === "judge:r")!.type).toBe("workflowJudge");
    expect(nodes.find((n) => n.id === "persona:p")!.type).toBe("workflowPersona");
    expect(nodes.find((n) => n.id === "scenarios")!.width).toBe(170);
  });
});
