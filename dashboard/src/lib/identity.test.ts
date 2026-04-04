import { describe, it, expect } from "vitest";
import { pseudonymizeId } from "./identity";

describe("pseudonymizeId", () => {
  it("produces deterministic output", () => {
    expect(pseudonymizeId("user@example.com")).toBe(pseudonymizeId("user@example.com"));
  });

  it("produces different output for different inputs", () => {
    expect(pseudonymizeId("alice")).not.toBe(pseudonymizeId("bob"));
  });

  it("produces 16 hex characters", () => {
    const result = pseudonymizeId("test-user");
    expect(result).toHaveLength(16);
    expect(result).toMatch(/^[0-9a-f]{16}$/);
  });

  it("returns empty string for empty input", () => {
    expect(pseudonymizeId("")).toBe("");
  });

  it("does not contain original input", () => {
    const result = pseudonymizeId("user@example.com");
    expect(result).not.toContain("user");
    expect(result).not.toContain("example");
  });

  it("matches Go pkg/identity.PseudonymizeID output", () => {
    // Pre-computed: echo -n "test-user" | shasum -a 256 | cut -c1-16
    expect(pseudonymizeId("test-user")).toBe("f85ac825d102b9f2");
  });
});
