import { describe, it, expect } from "vitest";
import { getProviderColor, getProviderDisplayName, PROVIDER_COLORS } from "./provider-utils";

describe("getProviderColor", () => {
  it("returns the brand color for a known provider (case-insensitive)", () => {
    expect(getProviderColor("anthropic")).toBe(PROVIDER_COLORS.anthropic);
    expect(getProviderColor("OpenAI")).toBe(PROVIDER_COLORS.openai);
  });

  it("falls back by index for unknown providers", () => {
    const a = getProviderColor("unknown-a", 0);
    const b = getProviderColor("unknown-b", 1);
    expect(a).not.toBe(b);
    expect(a).toMatch(/^#[0-9A-Fa-f]{6}$/);
  });

  it("returns a stable fallback for an empty provider", () => {
    expect(getProviderColor("")).toBe(getProviderColor("also-unknown", 0));
  });
});

describe("getProviderDisplayName", () => {
  it("maps known providers and the claude alias", () => {
    expect(getProviderDisplayName("anthropic")).toBe("Anthropic");
    expect(getProviderDisplayName("claude")).toBe("Anthropic");
    expect(getProviderDisplayName("openai")).toBe("OpenAI");
  });

  it("title-cases unknown providers", () => {
    expect(getProviderDisplayName("acme")).toBe("Acme");
  });
});
