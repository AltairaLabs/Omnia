import { describe, it, expect } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useFieldValidation } from "./use-field-validation";
import type { FieldConstraint } from "@/lib/validation/constraint-types";

const constraints: Record<string, FieldConstraint> = {
  "spec.handlers[].name": {
    type: "string",
    required: true,
    pattern: "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$",
  },
};

describe("useFieldValidation", () => {
  it("flags a live pattern violation but not pristine-required", () => {
    const { result } = renderHook(() => useFieldValidation(constraints));

    act(() => result.current.validate("spec.handlers[].name", "Api", { index: 0 }));
    expect(result.current.errors["spec.handlers[0].name"]).toContain("lowercase");
    expect(result.current.hasErrors).toBe(true);

    act(() => result.current.validate("spec.handlers[].name", "", { index: 0 }));
    expect(result.current.errors["spec.handlers[0].name"]).toBeUndefined();
    expect(result.current.hasErrors).toBe(false);
  });

  it("keys array errors by concrete index", () => {
    const { result } = renderHook(() => useFieldValidation(constraints));
    act(() => result.current.validate("spec.handlers[].name", "Bad", { index: 1 }));
    expect(result.current.errors["spec.handlers[1].name"]).toBeDefined();
    expect(result.current.errors["spec.handlers[0].name"]).toBeUndefined();
  });

  it("validateAll enforces required and returns false on failure", () => {
    const { result } = renderHook(() => useFieldValidation(constraints));
    let ok = true;
    act(() => {
      ok = result.current.validateAll([
        { path: "spec.handlers[].name", value: "", index: 0 },
      ]);
    });
    expect(ok).toBe(false);
    expect(result.current.errors["spec.handlers[0].name"]).toBe("This field is required.");
  });

  it("validateAll returns true when all fields valid", () => {
    const { result } = renderHook(() => useFieldValidation(constraints));
    let ok = false;
    act(() => {
      ok = result.current.validateAll([
        { path: "spec.handlers[].name", value: "ok", index: 0 },
      ]);
    });
    expect(ok).toBe(true);
    expect(result.current.hasErrors).toBe(false);
  });

  it("uses the built-in metadata.name constraint", () => {
    const { result } = renderHook(() => useFieldValidation({}));
    act(() => result.current.validate("metadata.name", "Bad_Name"));
    expect(result.current.errors["metadata.name"]).toBeDefined();
  });
});
