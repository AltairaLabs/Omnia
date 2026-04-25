import { describe, it, expect } from "vitest";
import {
  TIER_COLORS,
  TIER_LABELS,
  TIER_DESCRIPTIONS,
  CATEGORY_COLORS,
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

  it("provides a fallback color for unknown categories", () => {
    expect(CATEGORY_COLORS.unknown).toMatch(/^hsl\(/);
  });

  it("provides colors for the standard memory categories", () => {
    for (const category of [
      "memory:context",
      "memory:identity",
      "memory:health",
      "memory:location",
      "memory:preferences",
      "memory:history",
    ]) {
      expect(CATEGORY_COLORS[category]).toMatch(/^hsl\(/);
    }
  });
});
