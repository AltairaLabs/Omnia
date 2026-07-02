/**
 * Tests for the brand preset registry — the named brand configs used by the
 * dev/demo preset switcher and the /dev/theme preview route.
 */

import { describe, it, expect } from "vitest";
import {
  BRAND_PRESETS,
  PRESET_NAMES,
  getBrandPreset,
} from "./index";
import { brandConfigToCssVars } from "../css-vars";

describe("brand preset registry", () => {
  it("exposes the omnia, acme, and nebula presets", () => {
    expect(PRESET_NAMES).toEqual(expect.arrayContaining(["omnia", "acme", "nebula"]));
  });

  it("every preset is a valid BrandConfig with a product name and logo", () => {
    for (const name of PRESET_NAMES) {
      const preset = BRAND_PRESETS[name];
      expect(preset.productName.length).toBeGreaterThan(0);
      expect(preset.logo.light.length).toBeGreaterThan(0);
      expect(preset.logo.dark.length).toBeGreaterThan(0);
      expect(preset.favicon.length).toBeGreaterThan(0);
    }
  });

  it("getBrandPreset resolves a known name and returns undefined otherwise", () => {
    expect(getBrandPreset("acme")?.productName).toBe("Acme Cloud");
    expect(getBrandPreset("nope")).toBeUndefined();
    expect(getBrandPreset(undefined)).toBeUndefined();
  });

  it("acme and nebula override brand color tokens so the theme visibly changes", () => {
    const acme = brandConfigToCssVars(BRAND_PRESETS.acme);
    const nebula = brandConfigToCssVars(BRAND_PRESETS.nebula);
    // Each themed preset sets a primary and at least one categorical token.
    expect(acme["--primary"]).toBeTruthy();
    expect(nebula["--primary"]).toBeTruthy();
    expect(acme["--primary"]).not.toBe(nebula["--primary"]);
    expect(acme["--category-1"]).toBeTruthy();
    expect(nebula["--chart-1"]).toBeTruthy();
  });

  it("the omnia preset resets to theme defaults (no color overrides)", () => {
    // Selecting Omnia clears brand color vars so the base globals.css tokens win.
    const omnia = brandConfigToCssVars(BRAND_PRESETS.omnia);
    expect(omnia["--primary"]).toBeUndefined();
  });
});
