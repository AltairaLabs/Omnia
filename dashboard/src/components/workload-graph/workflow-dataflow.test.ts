import { describe, it, expect } from "vitest";
import {
  collectVariables,
  variableNodesAndEdges,
  pseudoStateNodesAndEdges,
  artifactNodesAndEdges,
  workflowDataflow,
} from "./workflow-dataflow";

describe("collectVariables", () => {
  it("unions variables across prompts, deduped by name (first wins)", () => {
    const vars = collectVariables({
      prompts: {
        a: { id: "a", variables: [{ name: "topic", type: "string", required: true }] },
        b: { id: "b", variables: [{ name: "topic", type: "string" }, { name: "lang", type: "string" }] },
      },
    });
    expect(vars.map((v) => v.name)).toEqual(["topic", "lang"]);
    expect(vars[0].required).toBe(true);
  });
});

describe("variableNodesAndEdges", () => {
  it("makes a variable node per var and a data edge to the target", () => {
    const { nodes, edges } = variableNodesAndEdges(
      [{ name: "topic", type: "string", required: true, example: "AI" }],
      "plan",
    );
    expect(nodes).toEqual([
      {
        id: "var:topic", kind: "variable", label: "topic", badges: [],
        detail: { varType: "string", required: true, example: "AI", values: undefined, description: undefined },
      },
    ]);
    expect(edges).toEqual([
      { id: "var:topic-->plan", source: "var:topic", target: "plan", style: "data" },
    ]);
  });

  it("omits edges when there is no target", () => {
    const { edges } = variableNodesAndEdges([{ name: "x", type: "string" }], undefined);
    expect(edges).toEqual([]);
  });
});

describe("pseudoStateNodesAndEdges", () => {
  const content = {
    prompts: { p: { id: "p" } },
    workflow: {
      version: 1, entry: "plan",
      states: {
        plan: { prompt_task: "p", on_event: { go: "done" } },
        done: { prompt_task: "p", terminal: true },
      },
    },
  };

  it("adds an initial node into the entry and a final from each terminal", () => {
    const { nodes, edges } = pseudoStateNodesAndEdges(content);
    expect(nodes.map((n) => n.kind).sort()).toEqual(["final", "initial"]);
    expect(edges).toContainEqual({ id: "initial-->plan", source: "initial", target: "plan", style: "normal" });
    expect(edges).toContainEqual({ id: "done-->final", source: "done", target: "final", style: "normal" });
  });

  it("omits the final node when there is no terminal state", () => {
    const { nodes } = pseudoStateNodesAndEdges({
      prompts: { p: { id: "p" } },
      workflow: { version: 1, entry: "plan", states: { plan: { prompt_task: "p", on_event: { x: "plan" } } } },
    });
    expect(nodes.find((n) => n.kind === "final")).toBeUndefined();
  });

  it("returns nothing without a workflow", () => {
    expect(pseudoStateNodesAndEdges({})).toEqual({ nodes: [], edges: [] });
  });
});

describe("artifactNodesAndEdges", () => {
  const content = {
    prompts: {
      writer: { id: "writer", system_template: "write notes" },
      reader: { id: "reader", system_template: "use {{artifacts.notes}} to answer" },
    },
    workflow: {
      version: 1, entry: "gather",
      states: {
        gather: { prompt_task: "writer", on_event: { go: "answer" }, artifacts: { notes: { mode: "append", type: "text/plain" } } },
        answer: { prompt_task: "reader", terminal: true },
      },
    },
  };

  it("wires producer-state -> artifact -> consumer-state on data edges", () => {
    const { nodes, edges } = artifactNodesAndEdges(content);
    expect(nodes).toHaveLength(1);
    expect(nodes[0]).toMatchObject({
      id: "artifact:notes", kind: "artifact", label: "notes",
      detail: { artifactMode: "append", artifactType: "text/plain", producers: ["gather"], consumers: ["answer"] },
    });
    expect(edges).toContainEqual({ id: "gather--art-->artifact:notes", source: "gather", target: "artifact:notes", style: "data" });
    expect(edges).toContainEqual({ id: "artifact:notes--art-->answer", source: "artifact:notes", target: "answer", style: "data" });
  });

  it("marks an artifact referenced but never declared as unresolved", () => {
    const { nodes } = artifactNodesAndEdges({
      prompts: { r: { id: "r", system_template: "read {{artifacts.ghost}}" } },
      workflow: { version: 1, entry: "s", states: { s: { prompt_task: "r", terminal: true } } },
    });
    const ghost = nodes.find((n) => n.id === "artifact:ghost");
    expect(ghost?.resolution).toBe("unresolved");
    expect(ghost?.detail.producers).toEqual([]);
  });
});

describe("workflowDataflow", () => {
  it("combines variables (to entry), pseudo-states, and artifacts", () => {
    const { nodes } = workflowDataflow({
      prompts: { p: { id: "p", variables: [{ name: "topic", type: "string" }], system_template: "{{artifacts.x}}" } },
      workflow: { version: 1, entry: "s", states: { s: { prompt_task: "p", terminal: true, artifacts: { x: { mode: "replace" } } } } },
    });
    const kinds = nodes.map((n) => n.kind);
    expect(kinds).toContain("variable");
    expect(kinds).toContain("initial");
    expect(kinds).toContain("final");
    expect(kinds).toContain("artifact");
  });
});
