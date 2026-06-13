import { describe, it, expect } from "vitest";
import { deriveWorkloadTier } from "./derive-tier";
import type { PromptPackContent } from "@/lib/data/types";

describe("deriveWorkloadTier — single", () => {
  it("projects a single-prompt pack to one agent node", () => {
    const content: PromptPackContent = {
      id: "greeter",
      prompts: {
        main: {
          id: "main",
          name: "Greeter",
          description: "Says hello",
          system_template: "You are a friendly greeter.",
          tools: ["search"],
        },
      },
      tools: { search: { name: "search", description: "Search the web" } },
    };

    const model = deriveWorkloadTier(content);

    expect(model.tier).toBe("single");
    expect(model.altitude).toBe("definition");
    expect(model.nodes).toHaveLength(1);
    expect(model.nodes[0].kind).toBe("agent");
    expect(model.nodes[0].label).toBe("Greeter");
    expect(model.nodes[0].isEntry).toBe(true);
    expect(model.nodes[0].detail.tools?.map((t) => t.name)).toEqual(["search"]);
    expect(model.edges).toEqual([]);
    expect(model.meta.counts).toEqual({ agents: 1, tools: 1, skills: 0, states: 0 });
  });
});

describe("deriveWorkloadTier — workflow", () => {
  it("projects workflow states to nodes and on_event to edges, with budget", () => {
    const content: PromptPackContent = {
      id: "refunds",
      prompts: {
        triage: { id: "triage", name: "Triage", system_template: "t", tools: ["lookup"] },
        refund: { id: "refund", name: "Refund", system_template: "r" },
      },
      tools: { lookup: { name: "lookup" } },
      workflow: {
        entry: "triage",
        states: {
          triage: { prompt_task: "triage", on_event: { approved: "refund" } },
          refund: {
            prompt_task: "refund",
            terminal: true,
            max_visits: 3,
            on_max_visits: "triage",
            on_event: { retry: "triage" },
          },
        },
        engine: { budget: { max_total_visits: 12, max_tool_calls: 30, max_wall_time_sec: 60 } },
      },
    };

    const model = deriveWorkloadTier(content);

    expect(model.tier).toBe("workflow");
    expect(model.nodes.map((n) => n.id).sort()).toEqual(["refund", "triage"]);
    const triage = model.nodes.find((n) => n.id === "triage")!;
    expect(triage.kind).toBe("state");
    expect(triage.isEntry).toBe(true);
    const refund = model.nodes.find((n) => n.id === "refund")!;
    expect(refund.isTerminal).toBe(true);
    expect(refund.badges.some((b) => b.icon === "loop")).toBe(true);

    // approved (triage→refund), retry (refund→triage), on_max_visits (refund→triage loop)
    expect(model.edges).toHaveLength(3);
    const loopEdge = model.edges.find((e) => e.style === "loop")!;
    expect(loopEdge.source).toBe("refund");
    expect(loopEdge.target).toBe("triage");
    expect(model.meta.budget).toEqual({ maxTotalVisits: 12, maxToolCalls: 30, maxWallTimeSec: 60 });
    expect(model.meta.counts.states).toBe(2);
  });
});

describe("deriveWorkloadTier — multiagent", () => {
  it("projects A2A agents to first-class nodes, overlaying workflow hand-offs", () => {
    const content: PromptPackContent = {
      id: "crew",
      prompts: {
        triage: { id: "triage", name: "Triage", system_template: "t", tools: ["lookup"] },
        refund: { id: "refund", name: "Refund", system_template: "r", tools: ["pay"] },
      },
      tools: { lookup: { name: "lookup" }, pay: { name: "pay" } },
      agents: {
        entry: "triage",
        members: {
          triage: { description: "Triages requests", tags: ["intake"], input_modes: ["text/plain"] },
          refund: { description: "Issues refunds", output_modes: ["application/json"] },
        },
      },
      workflow: {
        entry: "triage",
        states: {
          triage: { prompt_task: "triage", on_event: { approved: "refund" } },
          refund: { prompt_task: "refund", terminal: true },
        },
      },
    };

    const model = deriveWorkloadTier(content);

    expect(model.tier).toBe("multiagent");
    expect(model.nodes).toHaveLength(2);
    const triage = model.nodes.find((n) => n.id === "triage")!;
    expect(triage.kind).toBe("agent");
    expect(triage.isEntry).toBe(true);
    expect(triage.detail.ioModes?.input).toEqual(["text/plain"]);
    expect(triage.detail.description).toBe("Triages requests");
    // hand-off edge from workflow overlay
    expect(model.edges).toHaveLength(1);
    expect(model.edges[0]).toMatchObject({ source: "triage", target: "refund", label: "approved" });
    expect(model.meta.counts).toMatchObject({ agents: 2, states: 2 });
  });
});

describe("deriveWorkloadTier — malformed", () => {
  it("returns empty solo model for a pack with no prompts", () => {
    const model = deriveWorkloadTier({ id: "empty" });
    expect(model.tier).toBe("single");
    expect(model.nodes).toEqual([]);
    expect(model.meta.counts.agents).toBe(0);
  });

  it("marks on_event to an undefined state as an unresolved edge", () => {
    const model = deriveWorkloadTier({
      id: "x",
      prompts: { a: { id: "a", name: "A", system_template: "t" } },
      workflow: { entry: "a", states: { a: { prompt_task: "a", on_event: { go: "ghost" } } } },
    });
    const edge = model.edges.find((e) => e.target === "ghost")!;
    expect(edge.style).toBe("unresolved");
  });

  it("does not crash when a state references a missing prompt_task", () => {
    const model = deriveWorkloadTier({
      id: "x",
      prompts: {},
      workflow: { entry: "a", states: { a: { prompt_task: "missing" } } },
    });
    expect(model.tier).toBe("workflow");
    expect(model.nodes[0].label).toBe("missing"); // falls back to prompt_task
    expect(model.nodes[0].detail.tools).toEqual([]);
  });

  it("treats empty agents.members as not-crew (falls through to flow/solo)", () => {
    const model = deriveWorkloadTier({
      id: "x",
      prompts: { a: { id: "a", name: "A", system_template: "t" } },
      agents: { entry: "a", members: {} },
    });
    expect(model.tier).toBe("single");
  });
});
