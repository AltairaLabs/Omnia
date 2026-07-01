import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { getEffectiveLicense, MOCK_ENTERPRISE_LICENSE } from "./resolve-server";
import { OPEN_CORE_LICENSE } from "@/types/license";

const ENV_KEYS = ["NEXT_PUBLIC_DEMO_MODE", "DEMO_ENTERPRISE_LICENSE", "OPERATOR_API_URL"];

describe("getEffectiveLicense", () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    for (const k of ENV_KEYS) delete process.env[k];
  });
  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("returns the mock enterprise license in demo mode with the enterprise flag", async () => {
    process.env.NEXT_PUBLIC_DEMO_MODE = "true";
    process.env.DEMO_ENTERPRISE_LICENSE = "true";
    expect(await getEffectiveLicense()).toEqual(MOCK_ENTERPRISE_LICENSE);
    expect(MOCK_ENTERPRISE_LICENSE.features.whiteLabel).toBe(true);
  });

  it("returns open-core in demo mode without the enterprise flag", async () => {
    process.env.NEXT_PUBLIC_DEMO_MODE = "true";
    expect(await getEffectiveLicense()).toEqual(OPEN_CORE_LICENSE);
  });

  it("returns the operator license when the operator responds", async () => {
    process.env.OPERATOR_API_URL = "https://operator";
    const operatorLicense = { ...OPEN_CORE_LICENSE, customer: "Acme" };
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => operatorLicense,
    }) as unknown as typeof fetch;
    expect(await getEffectiveLicense()).toEqual(operatorLicense);
  });

  it("falls back to open-core when the operator responds non-ok", async () => {
    process.env.OPERATOR_API_URL = "https://operator";
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 500 }) as unknown as typeof fetch;
    vi.spyOn(console, "warn").mockImplementation(() => {});
    expect(await getEffectiveLicense()).toEqual(OPEN_CORE_LICENSE);
  });

  it("falls back to open-core when no operator URL is configured", async () => {
    expect(await getEffectiveLicense()).toEqual(OPEN_CORE_LICENSE);
  });

  it("falls back to open-core when the operator fetch throws", async () => {
    process.env.OPERATOR_API_URL = "https://operator";
    globalThis.fetch = vi.fn().mockRejectedValue(new Error("network")) as unknown as typeof fetch;
    vi.spyOn(console, "warn").mockImplementation(() => {});
    expect(await getEffectiveLicense()).toEqual(OPEN_CORE_LICENSE);
  });
});
