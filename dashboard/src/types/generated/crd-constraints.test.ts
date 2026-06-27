import { describe, it, expect } from "vitest";
import { crdConstraints } from "./crd-constraints";

describe("crdConstraints (generated)", () => {
  it("captures the ToolRegistry handler-name pattern and length", () => {
    const c = crdConstraints.ToolRegistry["spec.handlers[].name"];
    expect(c.pattern).toBe("^[a-z0-9]([-a-z0-9]*[a-z0-9])?$");
    expect(c.maxLength).toBe(63);
    expect(c.required).toBe(true);
  });

  it("includes SkillSource (newly added to codegen)", () => {
    expect(crdConstraints.SkillSource).toBeDefined();
    expect(Object.keys(crdConstraints.SkillSource).length).toBeGreaterThan(0);
  });

  it("captures at least one enum constraint", () => {
    const allFields = Object.values(crdConstraints).flatMap((kind) => Object.values(kind));
    expect(allFields.some((f) => Array.isArray(f.enum) && f.enum.length > 0)).toBe(true);
  });
});
