import { describe, it, expect } from "vitest";
import { TIERS, isTier } from "./types";

describe("TIERS", () => {
  it("lists all three tiers in canonical order", () => {
    expect(TIERS).toEqual(["institutional", "agent", "user"]);
  });
});

describe("isTier", () => {
  it.each(["institutional", "agent", "user"])("recognises %s as a tier", (k) => {
    expect(isTier(k)).toBe(true);
  });

  it.each(["", "USER", "team", "memory:context", " agent "])(
    "rejects %p as a tier",
    (k) => {
      expect(isTier(k)).toBe(false);
    },
  );
});
