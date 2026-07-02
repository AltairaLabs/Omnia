import { describe, it, expect } from "vitest";
import { resolveBrandFromEnv, resolveBrand, applyEntitlement } from "./resolve-server";
import { OMNIA_BRAND } from "./types";
import { OPEN_CORE_LICENSE, type License } from "@/types/license";

const ENTERPRISE: License = {
  ...OPEN_CORE_LICENSE,
  tier: "enterprise",
  features: { ...OPEN_CORE_LICENSE.features, whiteLabel: true },
};

describe("resolveBrand (preset-aware)", () => {
  it("resolves a named preset when presets are allowed (demo mode)", () => {
    const b = resolveBrand({ NEXT_PUBLIC_BRAND_PRESET: "acme" }, { allowPreset: true });
    expect(b.productName).toBe("Acme Cloud");
    expect(b.colors?.primary).toBe("#EA580C");
  });

  it("ignores the preset when presets are not allowed (production)", () => {
    const b = resolveBrand({ NEXT_PUBLIC_BRAND_PRESET: "acme" }, { allowPreset: false });
    expect(b.productName).toBe("Omnia");
  });

  it("falls back to env-based resolution for an unknown preset", () => {
    const b = resolveBrand(
      { NEXT_PUBLIC_BRAND_PRESET: "nope", NEXT_PUBLIC_BRAND_PRODUCT_NAME: "Env Co" },
      { allowPreset: true },
    );
    expect(b.productName).toBe("Env Co");
  });

  it("uses env-based resolution when no preset is set", () => {
    expect(resolveBrand({}, { allowPreset: true }).productName).toBe("Omnia");
  });
});

describe("resolveBrandFromEnv", () => {
  it("reads branding env vars", () => {
    const b = resolveBrandFromEnv({
      NEXT_PUBLIC_BRAND_PRODUCT_NAME: "Acme AI",
      NEXT_PUBLIC_BRAND_LOGO_LIGHT: "https://cdn/acme-light.svg",
      NEXT_PUBLIC_BRAND_COLOR_PRIMARY: "#ff0000",
    });
    expect(b.productName).toBe("Acme AI");
    expect(b.logo.light).toBe("https://cdn/acme-light.svg");
    expect(b.colors?.primary).toBe("#ff0000");
  });

  it("falls back to Omnia defaults when product name is unset", () => {
    expect(resolveBrandFromEnv({}).productName).toBe("Omnia");
  });

  it("keeps Omnia logo/favicon defaults when only some vars are set", () => {
    const b = resolveBrandFromEnv({
      NEXT_PUBLIC_BRAND_PRODUCT_NAME: "Acme AI",
    });
    expect(b.logo.dark).toBe(OMNIA_BRAND.logo.dark);
    expect(b.favicon).toBe(OMNIA_BRAND.favicon);
  });
});

describe("applyEntitlement", () => {
  it("returns the Omnia default when the license lacks whiteLabel", () => {
    const custom = { ...OMNIA_BRAND, productName: "Acme AI" };
    expect(applyEntitlement(custom, OPEN_CORE_LICENSE)).toEqual(OMNIA_BRAND);
  });

  it("passes the custom brand through when entitled", () => {
    const custom = { ...OMNIA_BRAND, productName: "Acme AI" };
    expect(applyEntitlement(custom, ENTERPRISE).productName).toBe("Acme AI");
  });
});
