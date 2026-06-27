import { describe, it, expect } from "vitest";
import { validateField, getConstraint, METADATA_NAME_CONSTRAINT } from "./crd-validator";
import type { FieldConstraint } from "./constraint-types";

const DNS_LABEL: FieldConstraint = {
  type: "string",
  required: true,
  maxLength: 63,
  pattern: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$",
};

describe("validateField", () => {
  it("returns null for a valid value", () => {
    expect(validateField("my-tool", DNS_LABEL)).toBeNull();
  });

  it("gives a friendly DNS-label message for a pattern violation", () => {
    expect(validateField("Api", DNS_LABEL)).toBe(
      "Use lowercase letters, numbers, and hyphens; must start and end with a letter or number."
    );
  });

  it("does not flag a pristine empty field outside submit", () => {
    expect(validateField("", DNS_LABEL)).toBeNull();
  });

  it("flags required only on submit", () => {
    expect(validateField("", DNS_LABEL, { isSubmit: true })).toBe("This field is required.");
  });

  it("enforces maxLength with a friendly message", () => {
    expect(validateField("a".repeat(64), { type: "string", maxLength: 63 })).toBe(
      "Must be 63 characters or fewer."
    );
  });

  it("enforces enum membership", () => {
    expect(validateField("grpc", { enum: ["http", "mcp"] })).toBe("Must be one of: http, mcp.");
    expect(validateField("http", { enum: ["http", "mcp"] })).toBeNull();
  });

  it("enforces numeric minimum/maximum", () => {
    expect(validateField(0, { type: "integer", minimum: 1 })).toBe("Must be at least 1.");
    expect(validateField(10, { type: "integer", maximum: 5 })).toBe("Must be at most 5.");
  });

  it("falls back to a generic message for an unknown pattern", () => {
    expect(validateField("??", { pattern: "^[A-Z]+$" })).toBe("Invalid format.");
  });

  it("enforces minLength with a friendly message", () => {
    expect(validateField("ab", { type: "string", minLength: 5 })).toBe(
      "Must be at least 5 characters."
    );
    expect(validateField("abcde", { type: "string", minLength: 5 })).toBeNull();
  });

  it("gives a DNS-subdomain friendly message for metadata.name pattern violations", () => {
    const result = validateField("Bad.Name", METADATA_NAME_CONSTRAINT);
    expect(result).toContain("hyphens, and dots");
  });
});

describe("getConstraint", () => {
  it("returns the spec constraint when present", () => {
    const map = { "spec.x": { type: "string" as const } };
    expect(getConstraint(map, "spec.x")).toEqual({ type: "string" });
  });

  it("falls back to the built-in metadata.name constraint", () => {
    expect(getConstraint({}, "metadata.name")).toBe(METADATA_NAME_CONSTRAINT);
  });

  it("returns undefined for an unconstrained field", () => {
    expect(getConstraint({}, "spec.description")).toBeUndefined();
  });
});
