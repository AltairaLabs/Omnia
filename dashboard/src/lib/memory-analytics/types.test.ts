import { describe, it, expect } from "vitest";
import { TIERS, isTier } from "./types";

describe("TIERS", () => {
  it("lists all four tiers in canonical order", () => {
    expect(TIERS).toEqual([
      "institutional",
      "agent",
      "user",
      "user_for_agent",
    ]);
  });
});

describe("isTier", () => {
  it.each(["institutional", "agent", "user", "user_for_agent"])(
    "recognises %s as a tier",
    (k) => {
      expect(isTier(k)).toBe(true);
    },
  );

  it.each(["", "USER", "team", "memory:context", " agent ", "user-for-agent"])(
    "rejects %p as a tier",
    (k) => {
      expect(isTier(k)).toBe(false);
    },
  );
});
