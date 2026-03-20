/**
 * Tests for job-wizard-utils pure functions.
 */

import { describe, it, expect } from "vitest";
import {
  validateForm,
  validateProviderMappings,
  validateProviderGroups,
  buildSpec,
  buildArenaFilePath,
  toProviderEntry,
  countTotalEntries,
  groupSummary,
  getStepIndicatorClassName,
  getInitialFormState,
  type JobWizardFormState,
} from "./job-wizard-utils";

function makeForm(overrides: Partial<JobWizardFormState> = {}): JobWizardFormState {
  return {
    name: "test-job",
    sourceRef: "my-source",
    rootPath: "",
    arenaFileName: "config.arena.yaml",
    providerGroups: {},
    providerMappings: {},
    selectedToolRegistries: [],
    workers: "1",
    verbose: false,
    ...overrides,
  };
}

// =============================================================================
// validateProviderMappings
// =============================================================================

describe("validateProviderMappings", () => {
  it("returns null for empty mappings", () => {
    expect(validateProviderMappings({})).toBeNull();
  });

  it("returns null when all entries have selections", () => {
    const result = validateProviderMappings({
      judges: {
        "judge-quality": { type: "provider", name: "haiku" },
        "judge-safety": { type: "agent", name: "safety-agent" },
      },
    });
    expect(result).toBeNull();
  });

  it("returns error when an entry is null", () => {
    const result = validateProviderMappings({
      judges: {
        "judge-quality": { type: "provider", name: "haiku" },
        "judge-safety": null,
      },
    });
    expect(result).toContain("judge-safety");
    expect(result).toContain("judges");
  });
});

// =============================================================================
// validateProviderGroups
// =============================================================================

describe("validateProviderGroups", () => {
  it("returns null when no required groups", () => {
    const form = makeForm();
    expect(validateProviderGroups(form, [])).toBeNull();
  });

  it("returns error when required array-mode group is empty", () => {
    const form = makeForm({ providerGroups: { default: [] } });
    const result = validateProviderGroups(form, ["default"]);
    expect(result).toContain("default");
  });

  it("skips required groups that are in providerMappings", () => {
    const form = makeForm({
      providerMappings: {
        judges: { "judge-quality": { type: "provider", name: "haiku" } },
      },
    });
    expect(validateProviderGroups(form, ["judges"])).toBeNull();
  });

  it("returns error for incomplete mapping entries", () => {
    const form = makeForm({
      providerMappings: {
        judges: { "judge-quality": null },
      },
    });
    const result = validateProviderGroups(form, ["judges"]);
    expect(result).toContain("judge-quality");
  });
});

// =============================================================================
// validateForm
// =============================================================================

describe("validateForm", () => {
  it("passes with valid form", () => {
    expect(validateForm(makeForm(), 0, [])).toBeNull();
  });

  it("fails with empty name", () => {
    expect(validateForm(makeForm({ name: "" }), 0, [])).toBe("Name is required");
  });

  it("fails with invalid name", () => {
    const result = validateForm(makeForm({ name: "UPPERCASE" }), 0, []);
    expect(result).toContain("lowercase");
  });

  it("fails with no sourceRef", () => {
    expect(validateForm(makeForm({ sourceRef: "" }), 0, [])).toBe("Source is required");
  });

  it("fails when workers exceed max", () => {
    const result = validateForm(makeForm({ workers: "5" }), 3, []);
    expect(result).toContain("3");
  });

  it("fails with invalid workers", () => {
    expect(validateForm(makeForm({ workers: "abc" }), 0, [])).toContain("positive integer");
  });

  it("delegates to provider group validation", () => {
    const form = makeForm({
      providerMappings: { judges: { "judge-quality": null } },
    });
    const result = validateForm(form, 0, ["judges"]);
    expect(result).toContain("judge-quality");
  });
});

// =============================================================================
// buildArenaFilePath
// =============================================================================

describe("buildArenaFilePath", () => {
  it("returns fileName when no rootPath", () => {
    expect(buildArenaFilePath("", "config.yaml")).toBe("config.yaml");
  });

  it("returns rootPath/default when no fileName", () => {
    expect(buildArenaFilePath("evals", "")).toBe("evals/config.arena.yaml");
  });

  it("joins rootPath and fileName", () => {
    expect(buildArenaFilePath("evals", "my.yaml")).toBe("evals/my.yaml");
  });

  it("returns undefined when both empty", () => {
    expect(buildArenaFilePath("", "")).toBeUndefined();
  });
});

// =============================================================================
// toProviderEntry
// =============================================================================

describe("toProviderEntry", () => {
  it("converts provider type to providerRef", () => {
    const result = toProviderEntry({ type: "provider", name: "claude", namespace: "ns" });
    expect(result).toEqual({ providerRef: { name: "claude", namespace: "ns" } });
  });

  it("converts agent type to agentRef", () => {
    const result = toProviderEntry({ type: "agent", name: "my-agent" });
    expect(result).toEqual({ agentRef: { name: "my-agent" } });
  });
});

// =============================================================================
// buildSpec — map mode output
// =============================================================================

describe("buildSpec", () => {
  it("outputs array-mode providers", () => {
    const form = makeForm({
      providerGroups: {
        default: [{ type: "provider", name: "claude" }],
      },
    });
    const spec = buildSpec(form);
    expect(spec.providers).toBeDefined();
    expect(Array.isArray(spec.providers!.default)).toBe(true);
    expect(spec.providers!.default).toEqual([
      { providerRef: { name: "claude", namespace: undefined } },
    ]);
  });

  it("outputs map-mode providers", () => {
    const form = makeForm({
      providerMappings: {
        judges: {
          "judge-quality": { type: "provider", name: "haiku" },
          "judge-safety": { type: "agent", name: "safety-agent" },
        },
      },
    });
    const spec = buildSpec(form);
    expect(spec.providers).toBeDefined();
    const judges = spec.providers!.judges;
    expect(Array.isArray(judges)).toBe(false);
    expect(judges).toEqual({
      "judge-quality": { providerRef: { name: "haiku", namespace: undefined } },
      "judge-safety": { agentRef: { name: "safety-agent" } },
    });
  });

  it("outputs mixed array + map providers", () => {
    const form = makeForm({
      providerGroups: {
        default: [{ type: "provider", name: "claude" }],
      },
      providerMappings: {
        judges: {
          "judge-quality": { type: "provider", name: "haiku" },
        },
      },
    });
    const spec = buildSpec(form);
    expect(Array.isArray(spec.providers!.default)).toBe(true);
    expect(Array.isArray(spec.providers!.judges)).toBe(false);
  });

  it("skips map groups with all null entries", () => {
    const form = makeForm({
      providerMappings: {
        judges: { "judge-quality": null },
      },
    });
    const spec = buildSpec(form);
    expect(spec.providers).toBeUndefined();
  });

  it("includes verbose when true", () => {
    const spec = buildSpec(makeForm({ verbose: true }));
    expect(spec.verbose).toBe(true);
  });

  it("omits verbose when false", () => {
    const spec = buildSpec(makeForm({ verbose: false }));
    expect(spec.verbose).toBeUndefined();
  });
});

// =============================================================================
// countTotalEntries
// =============================================================================

describe("countTotalEntries", () => {
  it("counts array-mode entries only", () => {
    expect(countTotalEntries(
      { default: [{ type: "provider", name: "a" }, { type: "provider", name: "b" }] },
    )).toBe(2);
  });

  it("ignores map-mode groups (not passed)", () => {
    // Map-mode groups are not included in the count because they
    // don't participate in the scenario × provider matrix
    expect(countTotalEntries({})).toBe(0);
  });

  it("counts only array entries across multiple groups", () => {
    expect(countTotalEntries({
      default: [{ type: "provider", name: "a" }],
      extra: [{ type: "agent", name: "b" }, { type: "provider", name: "c" }],
    })).toBe(3);
  });

  it("returns 0 for empty", () => {
    expect(countTotalEntries({})).toBe(0);
  });
});

// =============================================================================
// groupSummary
// =============================================================================

describe("groupSummary", () => {
  it("shows providers and agents", () => {
    const result = groupSummary([
      { type: "provider", name: "a" },
      { type: "provider", name: "b" },
      { type: "agent", name: "c" },
    ]);
    expect(result).toBe("2 providers, 1 agent");
  });

  it("handles singular", () => {
    expect(groupSummary([{ type: "provider", name: "a" }])).toBe("1 provider");
  });

  it("handles empty", () => {
    expect(groupSummary([])).toBe("");
  });
});

// =============================================================================
// getStepIndicatorClassName
// =============================================================================

describe("getStepIndicatorClassName", () => {
  it("returns completed class for past steps", () => {
    expect(getStepIndicatorClassName(0, 2)).toBe("bg-primary text-primary-foreground");
  });

  it("returns current class for current step", () => {
    expect(getStepIndicatorClassName(2, 2)).toBe("border-2 border-primary");
  });

  it("returns future class for upcoming steps", () => {
    expect(getStepIndicatorClassName(3, 1)).toBe("border border-muted-foreground/30");
  });
});

// =============================================================================
// getInitialFormState
// =============================================================================

describe("getInitialFormState", () => {
  it("returns defaults", () => {
    const state = getInitialFormState();
    expect(state.name).toBe("");
    expect(state.sourceRef).toBe("");
    expect(state.arenaFileName).toBe("config.arena.yaml");
    expect(state.providerGroups).toEqual({});
    expect(state.providerMappings).toEqual({});
    expect(state.workers).toBe("1");
    expect(state.verbose).toBe(false);
  });

  it("uses preselected source", () => {
    const state = getInitialFormState("my-source");
    expect(state.sourceRef).toBe("my-source");
  });

  it("uses default name", () => {
    const state = getInitialFormState(undefined, "swift-falcon");
    expect(state.name).toBe("swift-falcon");
  });
});
