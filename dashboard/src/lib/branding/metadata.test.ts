import { describe, it, expect } from "vitest";
import { buildBrandMetadata } from "./metadata";

describe("buildBrandMetadata", () => {
  it("titles and favicons from brand env", () => {
    const m = buildBrandMetadata({
      NEXT_PUBLIC_BRAND_PRODUCT_NAME: "Acme AI",
      NEXT_PUBLIC_BRAND_FAVICON: "/acme-fav.svg",
    });
    expect(m.title).toBe("Acme AI Dashboard");
    expect(m.icons).toEqual({ icon: "/acme-fav.svg" });
  });

  it("defaults to the Omnia title and favicon when unset", () => {
    const m = buildBrandMetadata({});
    expect(m.title).toBe("Omnia Dashboard");
    expect(m.icons).toEqual({ icon: "/favicon.svg" });
  });
});
