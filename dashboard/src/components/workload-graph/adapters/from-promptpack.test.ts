import { describe, it, expect } from "vitest";
import { promptPackToWorkload } from "./from-promptpack";

describe("promptPackToWorkload", () => {
  it("returns the derived skeleton at definition altitude", () => {
    const model = promptPackToWorkload({
      id: "greeter",
      prompts: { main: { id: "main", name: "Greeter", system_template: "hi" } },
    });
    expect(model.altitude).toBe("definition");
    expect(model.tier).toBe("solo");
    expect(model.nodes).toHaveLength(1);
  });

  it("returns an empty-but-valid model for undefined content", () => {
    const model = promptPackToWorkload(undefined);
    expect(model.tier).toBe("solo");
    expect(model.nodes).toEqual([]);
  });

  it("appends a resolved SkillSource node from spec.skills", () => {
    const model = promptPackToWorkload(
      { id: "g", prompts: { main: { id: "main", name: "Greeter", system_template: "hi" } } },
      {
        skillRefs: [{ source: "anthropic", mountAs: "skills" }],
        skillSources: [
          {
            apiVersion: "omnia.altairalabs.ai/v1alpha1",
            kind: "SkillSource",
            metadata: { name: "anthropic" },
            spec: { type: "git", interval: "1h" },
            status: { phase: "Ready", skillCount: 9 },
          },
        ],
      },
    );
    const skill = model.nodes.find((n) => n.kind === "skill");
    expect(skill?.resolution).toBe("resolved");
    expect(skill?.detail.skillCount).toBe(9);
    expect(model.meta.counts.skills).toBe(1);
  });
});
