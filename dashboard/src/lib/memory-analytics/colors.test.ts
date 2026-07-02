import { describe, it, expect } from "vitest";
import {
  TIER_COLORS,
  TIER_LABELS,
  TIER_DESCRIPTIONS,
} from "./colors";
import { TIERS } from "./types";

describe("memory-analytics colors", () => {
  it("provides a color, label, and description for every tier", () => {
    for (const tier of TIERS) {
      expect(TIER_COLORS[tier]).toMatch(/^hsl\(/);
      expect(TIER_LABELS[tier]).toBeTruthy();
      expect(TIER_DESCRIPTIONS[tier]).toMatch(/.{20,}/);
    }
  });

});
