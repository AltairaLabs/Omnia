import { describe, it, expect } from "vitest";
import { agentRuntimeToWorkload } from "./from-agent";
import type { PromptPackContent } from "@/lib/data/types";

const content: PromptPackContent = {
  id: "refunds",
  prompts: {
    triage: { id: "triage", name: "Triage", system_template: "t", tools: ["lookup", "ghost"] },
  },
  tools: { lookup: { name: "lookup" }, ghost: { name: "ghost" } },
  workflow: { entry: "triage", states: { triage: { prompt_task: "triage", terminal: true } } },
};

describe("agentRuntimeToWorkload", () => {
  it("sets deployment altitude and adds provider nodes with bound model", () => {
    const model = agentRuntimeToWorkload({
      content,
      providers: [{ name: "default", type: "claude", model: "claude-opus-4-8", role: "llm" }],
      discoveredTools: [{ name: "lookup", handlerName: "http", endpoint: "https://x", status: "Available" }],
    });
    expect(model.altitude).toBe("deployment");
    const provider = model.nodes.find((n) => n.kind === "provider")!;
    expect(provider.detail.model).toBe("claude-opus-4-8");
    expect(model.meta.binding?.providers[0].model).toBe("claude-opus-4-8");
  });

  it("resolves a referenced tool to its endpoint and marks a missing one unavailable", () => {
    const model = agentRuntimeToWorkload({
      content,
      providers: [],
      discoveredTools: [{ name: "lookup", handlerName: "http", endpoint: "https://x", status: "Available" }],
    });
    const state = model.nodes.find((n) => n.kind === "state")!;
    const tools = state.detail.tools ?? [];
    const lookup = tools.find((t) => t.name === "lookup")!;
    expect(lookup.status).toBe("resolved");
    expect(lookup.endpoint).toBe("https://x");
    const ghost = tools.find((t) => t.name === "ghost")!;
    expect(ghost.status).toBe("unavailable");
  });

  it("marks all tools unresolved when the registry has not discovered anything yet", () => {
    const model = agentRuntimeToWorkload({ content, providers: [], discoveredTools: [] });
    const state = model.nodes.find((n) => n.kind === "state")!;
    expect(state.detail.tools?.every((t) => t.status === "unresolved")).toBe(true);
  });

  it("adds one tool-registry node (named, with the discovered tools) wired to the entry", () => {
    const model = agentRuntimeToWorkload({
      content,
      providers: [],
      discoveredTools: [
        { name: "lookup", handlerName: "http", endpoint: "https://x", status: "Available" },
        { name: "refund", handlerName: "http", endpoint: "https://y", status: "Available" },
      ],
      toolRegistryName: "demo-tools",
    });
    const registry = model.nodes.find((n) => n.kind === "tool");
    expect(registry).toBeDefined();
    expect(registry!.label).toBe("demo-tools");
    expect(registry!.detail.tools?.map((t) => t.name).sort()).toEqual(["lookup", "refund"]);
    // wired from the entry node into the registry
    const entry = model.nodes.find((n) => n.isEntry)!;
    expect(model.edges.some((e) => e.source === entry.id && e.target === registry!.id)).toBe(true);
  });

  it("omits the tool-registry node when no registry is configured", () => {
    const model = agentRuntimeToWorkload({ content, providers: [], discoveredTools: [] });
    expect(model.nodes.find((n) => n.kind === "tool")).toBeUndefined();
  });

  it("wires each provider to the entry node with an edge", () => {
    const model = agentRuntimeToWorkload({
      content,
      providers: [{ name: "default", type: "claude", model: "claude-opus-4-8", role: "llm" }],
      discoveredTools: [],
    });
    const entry = model.nodes.find((n) => n.isEntry)!;
    const provider = model.nodes.find((n) => n.kind === "provider")!;
    expect(model.edges.some((e) => e.source === entry.id && e.target === provider.id)).toBe(true);
  });
});
