"use client";

import { useCallback, useState } from "react";
import { validateField, getConstraint } from "@/lib/validation/crd-validator";
import type { FieldConstraint } from "@/lib/validation/constraint-types";

/** Resolve the concrete error key, substituting an array index for "[]". */
function concretePath(path: string, index?: number): string {
  if (index === undefined) return path;
  return path.replace("[]", `[${index}]`);
}

export interface FieldInput {
  path: string;
  value: unknown;
  index?: number;
}

export interface UseFieldValidationResult {
  errors: Record<string, string>;
  validate: (path: string, value: unknown, opts?: { index?: number }) => void;
  validateAll: (fields: FieldInput[]) => boolean;
  clearErrors: () => void;
  hasErrors: boolean;
}

/**
 * Realtime field validation driven by a CRD constraint map. `validate` runs on
 * each keystroke (required deferred); `validateAll` runs on submit (required
 * enforced) and returns whether every field passed.
 */
export function useFieldValidation(
  constraints: Record<string, FieldConstraint>
): UseFieldValidationResult {
  const [errors, setErrors] = useState<Record<string, string>>({});

  const setError = useCallback((key: string, message: string | null) => {
    setErrors((prev) => {
      if (message) {
        if (prev[key] === message) return prev;
        return { ...prev, [key]: message };
      }
      if (!(key in prev)) return prev;
      const next = { ...prev };
      delete next[key];
      return next;
    });
  }, []);

  const validate = useCallback(
    (path: string, value: unknown, opts?: { index?: number }) => {
      const constraint = getConstraint(constraints, path);
      if (!constraint) return;
      const message = validateField(value, constraint);
      setError(concretePath(path, opts?.index), message);
    },
    [constraints, setError]
  );

  const validateAll = useCallback(
    (fields: FieldInput[]): boolean => {
      const next: Record<string, string> = {};
      for (const field of fields) {
        const constraint = getConstraint(constraints, field.path);
        if (!constraint) continue;
        const message = validateField(field.value, constraint, { isSubmit: true });
        if (message) next[concretePath(field.path, field.index)] = message;
      }
      setErrors(next);
      return Object.keys(next).length === 0;
    },
    [constraints]
  );

  const clearErrors = useCallback(() => setErrors({}), []);

  return {
    errors,
    validate,
    validateAll,
    clearErrors,
    hasErrors: Object.keys(errors).length > 0,
  };
}
