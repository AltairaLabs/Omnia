/**
 * A single field's validation constraints, extracted from a CRD OpenAPI schema.
 * Shared by the generated constraint map and the runtime validator.
 */
export interface FieldConstraint {
  required?: boolean;
  type?: "string" | "integer" | "number" | "boolean" | "array" | "object";
  pattern?: string;
  enum?: string[];
  minLength?: number;
  maxLength?: number;
  minimum?: number;
  maximum?: number;
}

/** kind → field path (e.g. "spec.handlers[].name") → constraint. */
export type CrdConstraintMap = Record<string, Record<string, FieldConstraint>>;
