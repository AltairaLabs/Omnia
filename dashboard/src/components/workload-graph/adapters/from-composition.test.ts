import { describe, it, expect } from "vitest";
import { compositionToWorkload, predicateText } from "./from-composition";
import type { CompositionDef } from "@/lib/data/types";

describe("compositionToWorkload — sequential", () => {
  it("chains steps in array order with one backbone edge each", () => {
    const comp: CompositionDef = {
      version: 1,
      steps: [
        { id: "classify", kind: "prompt", prompt_task: "doc_classifier", input: "${input.text}" },
        { id: "extract", kind: "prompt", prompt_task: "extractor" },
      ],
    };
    const g = compositionToWorkload("main", "analyze", comp);
    expect(g.name).toBe("analyze");
    const ids = g.nodes.map((n) => n.id).sort();
    expect(ids).toEqual(["main::classify", "main::extract"]);
    expect(g.nodes.every((n) => n.parentId === "main")).toBe(true);
    expect(g.nodes.find((n) => n.id === "main::classify")!.kind).toBe("stepPrompt");
    expect(g.nodes.find((n) => n.id === "main::classify")!.detail.promptTask).toBe("doc_classifier");
    expect(g.edges).toHaveLength(1);
    expect(g.edges[0]).toMatchObject({ source: "main::classify", target: "main::extract" });
  });
});

describe("compositionToWorkload — branch", () => {
  it("emits then/else edges and does not duplicate the backbone edge to a routed target", () => {
    const comp: CompositionDef = {
      version: 1,
      steps: [
        { id: "classify", kind: "prompt", prompt_task: "c" },
        { id: "route", kind: "branch", predicate: { path: "${classify.output.type}", op: "equals", value: "paper" }, then: "paper", else: "general" },
        { id: "paper", kind: "prompt", prompt_task: "p" },
        { id: "general", kind: "prompt", prompt_task: "g" },
      ],
    };
    const g = compositionToWorkload("main", "analyze", comp);
    const route = g.nodes.find((n) => n.id === "main::route")!;
    expect(route.kind).toBe("stepBranch");
    expect(route.detail.predicateText).toBe("${classify.output.type} equals paper");
    const thenEdge = g.edges.find((e) => e.source === "main::route" && e.target === "main::paper")!;
    expect(thenEdge.label).toBe("then");
    const elseEdge = g.edges.find((e) => e.source === "main::route" && e.target === "main::general")!;
    expect(elseEdge.label).toBe("else");
    // exactly one route->paper edge (no unlabeled backbone duplicate)
    expect(g.edges.filter((e) => e.source === "main::route" && e.target === "main::paper")).toHaveLength(1);
  });
});

describe("compositionToWorkload — parallel", () => {
  it("nests branch children under a parallel container with a reducer, and wires prev/next at composition level", () => {
    const comp: CompositionDef = {
      version: 1,
      steps: [
        {
          id: "meta",
          kind: "parallel",
          branches: [
            { id: "title", kind: "prompt", prompt_task: "title_extractor" },
            { id: "struct", kind: "tool", tool: "doc.parse", args: { content: "${input.text}" } },
          ],
          reduce: { strategy: "barrier", into: "metadata" },
        },
        { id: "synth", kind: "agent", prompt_task: "analyzer", tools: ["ref.search"], termination: { max_steps: 10 } },
      ],
    };
    const g = compositionToWorkload("main", "deep", comp);
    const par = g.nodes.find((n) => n.id === "main::meta")!;
    expect(par.kind).toBe("stepParallel");
    expect(par.isContainer).toBe(true);
    expect(par.parentId).toBe("main");
    expect(par.detail.reducer).toBe("barrier → metadata");
    const title = g.nodes.find((n) => n.id === "main::title")!;
    expect(title.parentId).toBe("main::meta");
    const struct = g.nodes.find((n) => n.id === "main::struct")!;
    expect(struct.kind).toBe("stepTool");
    expect(struct.detail.toolRef).toBe("doc.parse");
    // no edges between sibling branches
    expect(g.edges.some((e) => e.source === "main::title" && e.target === "main::struct")).toBe(false);
    // composition-level backbone: parallel -> synth
    expect(g.edges.some((e) => e.source === "main::meta" && e.target === "main::synth")).toBe(true);
    const synth = g.nodes.find((n) => n.id === "main::synth")!;
    expect(synth.kind).toBe("stepAgent");
    expect(synth.detail.termination).toBe("≤10 steps");
    expect(synth.detail.tools?.map((t) => t.name)).toEqual(["ref.search"]);
  });
});

describe("compositionToWorkload — depends_on", () => {
  it("uses explicit dependency edges instead of the array-order backbone", () => {
    const comp: CompositionDef = {
      version: 1,
      steps: [
        { id: "a", kind: "prompt", prompt_task: "a" },
        { id: "b", kind: "prompt", prompt_task: "b" },
        { id: "c", kind: "prompt", prompt_task: "c", depends_on: ["a"] },
      ],
    };
    const g = compositionToWorkload("m", "x", comp);
    expect(g.edges.some((e) => e.source === "m::a" && e.target === "m::c")).toBe(true);
    // c declares depends_on, so no backbone b->c
    expect(g.edges.some((e) => e.source === "m::b" && e.target === "m::c")).toBe(false);
  });
});

describe("predicateText", () => {
  it("renders compare, exists, and composite predicates", () => {
    expect(predicateText({ path: "${x.y}", op: "equals", value: "z" })).toBe("${x.y} equals z");
    expect(predicateText({ path: "${x}", exists: true })).toBe("${x} exists");
    expect(predicateText({ path: "${x}", exists: false })).toBe("${x} missing");
    expect(predicateText({ all_of: [{ path: "${a}", op: "in", value: ["1"] }, { path: "${b}", exists: true }] }))
      .toBe("(${a} in 1 AND ${b} exists)");
    expect(predicateText({ not: { path: "${a}", op: "equals", value: "1" } })).toBe("NOT (${a} equals 1)");
  });
});
