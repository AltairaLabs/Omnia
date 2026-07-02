import { describe, it, expect } from "vitest";
import { brandConfigToCssVars } from "./css-vars";
import { OMNIA_BRAND } from "./types";

describe("brandConfigToCssVars", () => {
  it("maps allowlisted color keys to CSS variables", () => {
    const vars = brandConfigToCssVars({
      ...OMNIA_BRAND,
      colors: {
        primary: "#ff0000",
        chart1: "#00ff00",
        category1: "#0000ff",
        category8: "#123456",
        success: "#0000ff",
      },
    });
    expect(vars["--primary"]).toBe("#ff0000");
    expect(vars["--chart-1"]).toBe("#00ff00");
    expect(vars["--category-1"]).toBe("#0000ff");
    expect(vars["--category-8"]).toBe("#123456");
    expect(vars["--success"]).toBe("#0000ff");
  });

  it("ignores unknown keys and empty values (no arbitrary var injection)", () => {
    const vars = brandConfigToCssVars({
      ...OMNIA_BRAND,
      colors: {
        primary: "",
        // @ts-expect-error unknown key must be ignored
        "evil--x": "url(javascript:alert(1))",
      },
    });
    expect(Object.keys(vars)).not.toContain("evil--x");
    expect(vars["--primary"]).toBeUndefined(); // empty string dropped
    expect(Object.keys(vars).every((k) => k.startsWith("--"))).toBe(true);
  });

  it("emits a font-family var when provided", () => {
    const vars = brandConfigToCssVars({ ...OMNIA_BRAND, fonts: { family: "Acme Sans" } });
    expect(vars["--brand-font-sans"]).toContain("Acme Sans");
  });

  it("returns no vars for a config with no colors or fonts", () => {
    expect(brandConfigToCssVars(OMNIA_BRAND)).toEqual({});
  });
});
