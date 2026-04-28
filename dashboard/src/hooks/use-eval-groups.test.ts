import { describe, expect, it } from "vitest";
import { collectPackGroups, BUILTIN_EVAL_GROUPS } from "./use-eval-groups";
import type { PromptPackContent } from "@/lib/data/types";

// The hook itself is thin (delegates to usePromptPackContent and
// memoizes); we cover the pure pack-walk separately so a regression in
// the discovery rule is caught without spinning up React Query.

describe("collectPackGroups", () => {
  it("returns [] for null/undefined", () => {
    expect(collectPackGroups(null)).toEqual([]);
    expect(collectPackGroups(undefined)).toEqual([]);
  });

  it("collects pack-level eval groups", () => {
    const content: PromptPackContent = {
      id: "p",
      name: "p",
      version: "1",
      template_engine: { version: "v1", syntax: "{{}}" },
      evals: [
        { id: "e1", type: "contains", trigger: "always", groups: ["safety", "fast"] },
        { id: "e2", type: "regex", trigger: "always", groups: ["fast"] },
      ],
    };
    expect(collectPackGroups(content).sort()).toEqual(["fast", "fast", "safety"]);
  });

  it("collects prompt-level eval groups", () => {
    const content: PromptPackContent = {
      id: "p",
      name: "p",
      version: "1",
      template_engine: { version: "v1", syntax: "{{}}" },
      prompts: {
        default: {
          id: "default",
          evals: [
            { id: "e1", type: "judge", trigger: "always", groups: ["llm-judge"] },
          ],
        },
      },
    };
    expect(collectPackGroups(content)).toEqual(["llm-judge"]);
  });

  it("merges pack-level and prompt-level groups", () => {
    const content: PromptPackContent = {
      id: "p",
      name: "p",
      version: "1",
      template_engine: { version: "v1", syntax: "{{}}" },
      evals: [{ id: "e1", type: "contains", trigger: "always", groups: ["pack-g"] }],
      prompts: {
        default: {
          id: "default",
          evals: [
            { id: "e2", type: "judge", trigger: "always", groups: ["prompt-g"] },
          ],
        },
      },
    };
    expect(collectPackGroups(content).sort()).toEqual(["pack-g", "prompt-g"]);
  });

  it("ignores evals without groups", () => {
    const content: PromptPackContent = {
      id: "p",
      name: "p",
      version: "1",
      template_engine: { version: "v1", syntax: "{{}}" },
      evals: [{ id: "e1", type: "contains", trigger: "always" }],
    };
    expect(collectPackGroups(content)).toEqual([]);
  });
});

describe("BUILTIN_EVAL_GROUPS", () => {
  it("is the four built-in PromptKit group names — order-stable for snapshot", () => {
    expect(BUILTIN_EVAL_GROUPS).toEqual([
      "default",
      "fast-running",
      "long-running",
      "external",
    ]);
  });
});
