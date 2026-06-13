import { describe, it, expect } from "vitest";
import { attachSkills } from "./skills";
import type { SkillSource, SkillSourcePhase } from "@/types/skill-source";
import type { WorkloadModel } from "../types";

function source(name: string, phase?: SkillSourcePhase, skillCount?: number): SkillSource {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "SkillSource",
    metadata: { name },
    spec: { type: "git", interval: "1h" },
    status: phase ? { phase, skillCount } : undefined,
  } as SkillSource;
}

const base: WorkloadModel = {
  tier: "flow",
  altitude: "definition",
  nodes: [
    {
      id: "plan",
      kind: "state",
      label: "Plan",
      isEntry: true,
      badges: [
        { icon: "tool", label: "0" },
        { icon: "skill", label: "0" },
      ],
      detail: {},
    },
    { id: "provider:x", kind: "provider", label: "x", badges: [], detail: {} },
  ],
  edges: [],
  meta: { counts: { agents: 1, tools: 0, skills: 0, states: 1 } },
};

describe("attachSkills", () => {
  it("is a no-op when there are no skill refs", () => {
    expect(attachSkills(base, undefined, [])).toBe(base);
    expect(attachSkills(base, [], [])).toBe(base);
  });

  it("decorates agent/state nodes with the resolved skill source, not a separate node", () => {
    const out = attachSkills(
      base,
      [{ source: "anthropic", mountAs: "skills", include: ["pdf"] }],
      [source("anthropic", "Ready", 18)],
    );
    // no new nodes — the provider node is left alone, no skill node added
    expect(out.nodes).toHaveLength(2);
    const plan = out.nodes.find((n) => n.id === "plan")!;
    expect(plan.detail.skillSource).toBe("anthropic");
    expect(plan.detail.mountAs).toBe("skills");
    expect(plan.detail.include).toEqual(["pdf"]);
    expect(plan.detail.skillCount).toBe(18);
    expect(plan.detail.skillPhase).toBe("Ready");
    // the existing skill badge reflects the resolved count
    expect(plan.badges.find((b) => b.icon === "skill")?.label).toBe("18");
    // provider node untouched
    expect(out.nodes.find((n) => n.id === "provider:x")!.detail.skillSource).toBeUndefined();
    expect(out.meta.counts.skills).toBe(18);
  });

  it("falls back to the phase label when the source is missing or unsynced", () => {
    const out = attachSkills(base, [{ source: "ghost" }], []);
    const plan = out.nodes.find((n) => n.id === "plan")!;
    expect(plan.detail.skillPhase).toBe("missing");
    expect(plan.badges.find((b) => b.icon === "skill")?.label).toBe("missing");
  });
});
