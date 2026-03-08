import { describe, it, expect } from "vitest";
import { generateName } from "./name-generator";

describe("generateName", () => {
  it("returns a hyphenated two-word name", () => {
    const name = generateName();
    expect(name).toMatch(/^[a-z]+-[a-z]+$/);
  });

  it("prepends prefix when provided", () => {
    const name = generateName("eval");
    expect(name).toMatch(/^eval-[a-z]+-[a-z]+$/);
  });

  it("produces valid Kubernetes resource names", () => {
    // RFC 1123: lowercase alphanumeric + hyphens, start/end with alphanumeric
    const k8sNameRegex = /^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/;
    for (let i = 0; i < 100; i++) {
      expect(generateName()).toMatch(k8sNameRegex);
      expect(generateName("job")).toMatch(k8sNameRegex);
    }
  });

  it("generates varied names", () => {
    const names = new Set(Array.from({ length: 50 }, () => generateName()));
    expect(names.size).toBeGreaterThan(30);
  });
});
