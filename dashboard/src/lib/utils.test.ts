import { describe, it, expect } from "vitest";
import { cn, generateId } from "./utils";

describe("cn utility", () => {
  it("should merge class names", () => {
    expect(cn("foo", "bar")).toBe("foo bar");
  });

  it("should handle conditional classes", () => {
    expect(cn("foo", false && "bar", "baz")).toBe("foo baz");
  });

  it("should handle undefined", () => {
    expect(cn("foo", undefined, "bar")).toBe("foo bar");
  });
});

describe("generateId", () => {
  it("should return a string", () => {
    expect(typeof generateId()).toBe("string");
  });

  it("should generate unique IDs", () => {
    const ids = new Set(Array.from({ length: 100 }, () => generateId()));
    expect(ids.size).toBe(100);
  });

  it("should contain timestamp and random components", () => {
    const id = generateId();
    const parts = id.split("-");
    // Format: timestamp-counter-randomUUID
    expect(parts.length).toBeGreaterThanOrEqual(3);
    expect(Number(parts[0])).toBeGreaterThan(0);
  });
});
