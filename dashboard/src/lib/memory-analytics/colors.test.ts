import { describe, it, expect } from "vitest";
import { TIER_LABELS, TIER_DESCRIPTIONS } from "./colors";
import { TIERS } from "./types";

describe("memory-analytics colors", () => {
  it("provides a label and description for every tier", () => {
    for (const tier of TIERS) {
      expect(TIER_LABELS[tier]).toBeTruthy();
      expect(TIER_DESCRIPTIONS[tier]).toMatch(/.{20,}/);
    }
  });
});
