import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("@/lib/license/resolve-server", () => ({
  getEffectiveLicense: vi.fn(),
}));

import { getEffectiveLicense } from "@/lib/license/resolve-server";
import { OPEN_CORE_LICENSE } from "@/types/license";
import { GET } from "./route";

const mockedGetLicense = vi.mocked(getEffectiveLicense);

describe("GET /api/config brand", () => {
  beforeEach(() => {
    process.env.NEXT_PUBLIC_BRAND_PRODUCT_NAME = "Acme AI";
  });
  afterEach(() => {
    delete process.env.NEXT_PUBLIC_BRAND_PRODUCT_NAME;
    vi.clearAllMocks();
  });

  it("returns the Omnia brand when the license lacks whiteLabel", async () => {
    mockedGetLicense.mockResolvedValue(OPEN_CORE_LICENSE);
    const body = await (await GET()).json();
    expect(body.brand.productName).toBe("Omnia");
  });

  it("returns the custom brand when the license grants whiteLabel", async () => {
    mockedGetLicense.mockResolvedValue({
      ...OPEN_CORE_LICENSE,
      features: { ...OPEN_CORE_LICENSE.features, whiteLabel: true },
    });
    const body = await (await GET()).json();
    expect(body.brand.productName).toBe("Acme AI");
  });
});
