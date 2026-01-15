import { describe, it, expect } from "vitest";
import { cn, formatTokens } from "./utils";

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

describe("formatTokens", () => {
  it("should format millions with M suffix", () => {
    expect(formatTokens(1000000)).toBe("1.0M");
    expect(formatTokens(2500000)).toBe("2.5M");
    expect(formatTokens(10500000)).toBe("10.5M");
  });

  it("should format thousands with K suffix", () => {
    expect(formatTokens(1000)).toBe("1.0K");
    expect(formatTokens(2500)).toBe("2.5K");
    expect(formatTokens(999999)).toBe("1000.0K");
  });

  it("should format small numbers without suffix", () => {
    expect(formatTokens(0)).toBe("0");
    expect(formatTokens(500)).toBe("500");
    expect(formatTokens(999)).toBe("999");
  });

  it("should handle decimal numbers", () => {
    expect(formatTokens(1500)).toBe("1.5K");
    expect(formatTokens(1500000)).toBe("1.5M");
    expect(formatTokens(123.456)).toBe("123");
  });
});
