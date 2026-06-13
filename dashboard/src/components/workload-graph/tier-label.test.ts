import { describe, it, expect } from "vitest";
import { workloadTierLabel } from "./tier-label";

describe("workloadTierLabel", () => {
  it("maps each tier to its display label", () => {
    expect(workloadTierLabel("single")).toBe("Single");
    expect(workloadTierLabel("workflow")).toBe("Workflow");
    expect(workloadTierLabel("multiagent")).toBe("Multi-agent");
  });
});
