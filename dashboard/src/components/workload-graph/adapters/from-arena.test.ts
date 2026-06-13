import { describe, it, expect } from "vitest";
import { arenaProjectToWorkload } from "./from-arena";
import type { ArenaParsed } from "./arena-parse";

function base(): ArenaParsed {
  return {
    content: {
      prompts: {
        assistant: {
          id: "assistant",
          name: "Assistant",
          system_template: "Hi {{topic}}",
          variables: [{ name: "topic", type: "string" }],
        },
      },
    },
    providers: [{ id: "gpt", model: "gpt-4o", providerType: "openai", group: "default", resolved: true }],
    scenarios: [{ id: "qa" }, { id: "edge" }],
    judges: [{ id: "relevance", provider: "judge-gpt" }],
    persona: { id: "user", role: "user", provider: "selfplay" },
  };
}

describe("arenaProjectToWorkload", () => {
  it("centers on the prompt, wraps it with the harness, sets test altitude", () => {
    const m = arenaProjectToWorkload(base());
    expect(m.altitude).toBe("test");
    expect(m.tier).toBe("single");
    expect(m.nodes.find((n) => n.id === "assistant")?.kind).toBe("agent");
    expect(m.nodes.find((n) => n.kind === "variable")?.label).toBe("topic");
    const provider = m.nodes.find((n) => n.id === "provider:gpt");
    expect(provider?.kind).toBe("provider");
    expect(m.edges).toContainEqual({ id: "provider:gpt-->assistant", source: "provider:gpt", target: "assistant", style: "provides" });
    const scenarios = m.nodes.find((n) => n.kind === "scenario");
    expect(scenarios?.label).toBe("2 scenarios");
    expect(m.edges).toContainEqual({ id: "scenarios-->assistant", source: "scenarios", target: "assistant", style: "data" });
    expect(m.edges).toContainEqual({ id: "assistant-->judge:relevance", source: "assistant", target: "judge:relevance", style: "data" });
    const persona = m.nodes.find((n) => n.kind === "persona");
    expect(persona?.id).toBe("persona:user");
  });

  it("renders a workflow center with the data-flow overlay (initial/final/artifact)", () => {
    const p = base();
    p.content = {
      prompts: { writer: { id: "writer", system_template: "{{artifacts.notes}}", variables: [{ name: "topic", type: "string" }] } },
      workflow: { version: 1, entry: "s", states: { s: { prompt_task: "writer", terminal: true, artifacts: { notes: {} } } } },
    };
    const m = arenaProjectToWorkload(p);
    expect(m.altitude).toBe("test");
    expect(m.tier).toBe("workflow");
    const kinds = m.nodes.map((n) => n.kind);
    expect(kinds).toContain("initial");
    expect(kinds).toContain("final");
    expect(kinds).toContain("artifact");
    expect(m.edges).toContainEqual({ id: "provider:gpt-->s", source: "provider:gpt", target: "s", style: "provides" });
  });

  it("marks an unresolved provider and omits persona when absent", () => {
    const p = base();
    p.providers = [{ id: "gpt", resolved: false }];
    p.persona = undefined;
    const m = arenaProjectToWorkload(p);
    expect(m.nodes.find((n) => n.id === "provider:gpt")?.resolution).toBe("unresolved");
    expect(m.nodes.some((n) => n.kind === "persona")).toBe(false);
  });

  it("returns an empty model when there is no prompt/workflow/agents", () => {
    const m = arenaProjectToWorkload({ content: {}, providers: [], scenarios: [], judges: [] });
    expect(m.nodes).toEqual([]);
    expect(m.altitude).toBe("test");
  });
});
