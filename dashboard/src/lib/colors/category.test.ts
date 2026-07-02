import { describe, it, expect } from "vitest";
import {
  categoryIndex,
  isKnownCategory,
  categoryColorVar,
  categoryColorHex,
  getCategoryClasses,
  getCategoryLabel,
} from "./category";

describe("category colors", () => {
  it("maps known memory categories to stable indices", () => {
    expect(categoryIndex("memory:identity")).toBe(1);
    expect(categoryIndex("memory:preferences")).toBe(2);
    expect(categoryIndex("memory:history")).toBe(4);
    expect(categoryIndex("memory:location")).toBe(5);
    expect(categoryIndex("memory:health")).toBe(7);
    expect(categoryIndex("memory:context")).toBe(8);
  });

  it("falls back to the neutral index for unknown/missing categories", () => {
    expect(categoryIndex("memory:nope")).toBe(8);
    expect(categoryIndex()).toBe(8);
  });

  it("reports whether a category is known", () => {
    expect(isKnownCategory("memory:identity")).toBe(true);
    expect(isKnownCategory("memory:nope")).toBe(false);
    expect(isKnownCategory()).toBe(false);
  });

  it("returns a category CSS variable", () => {
    expect(categoryColorVar("memory:identity")).toBe("var(--category-1)");
    expect(categoryColorVar()).toBe("var(--category-8)");
  });

  it("returns a concrete hex matching the token default (for canvas)", () => {
    expect(categoryColorHex("memory:identity")).toBe("#3B82F6");
    expect(categoryColorHex("memory:context")).toBe("#6B7280");
    expect(categoryColorHex()).toBe("#6B7280");
  });

  it("returns token utility classes, never a raw palette class", () => {
    const cls = getCategoryClasses("memory:identity");
    expect(cls.bg).toBe("bg-category-1/15");
    expect(cls.text).toBe("text-category-1");
    expect(JSON.stringify(cls)).not.toMatch(/-(blue|green|red|amber|purple|gray)-\d/);
  });

  it("labels known categories and defaults unknown to Context", () => {
    expect(getCategoryLabel("memory:identity")).toBe("Identity");
    expect(getCategoryLabel("memory:health")).toBe("Health");
    expect(getCategoryLabel("memory:nope")).toBe("Context");
    expect(getCategoryLabel()).toBe("Context");
  });
});
