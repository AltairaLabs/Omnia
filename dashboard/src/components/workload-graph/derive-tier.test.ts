import { describe, it, expect } from "vitest";
import { deriveWorkloadTier } from "./derive-tier";
import type { PromptPackContent } from "@/lib/data/types";

describe("deriveWorkloadTier — solo", () => {
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

    expect(model.tier).toBe("solo");
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
