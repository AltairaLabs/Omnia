import type { FieldConstraint } from "./constraint-types";

/** The DNS-1123 subdomain pattern k8s enforces on resource names. */
const DNS_SUBDOMAIN_PATTERN = "^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$";
/** The DNS-1123 label pattern used by most CRD name fields. */
const DNS_LABEL_PATTERN = "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$";

const DNS_FRIENDLY =
  "Use lowercase letters, numbers, and hyphens; must start and end with a letter or number.";

/** Friendly copy for well-known patterns; unknown patterns fall back to generic. */
const PATTERN_MESSAGES: Record<string, string> = {
  [DNS_LABEL_PATTERN]: DNS_FRIENDLY,
  [DNS_SUBDOMAIN_PATTERN]:
    "Use lowercase letters, numbers, hyphens, and dots; must start and end with a letter or number.",
};

/** Built-in constraint for the resource name (CRD schemas don't carry metadata.name). */
export const METADATA_NAME_CONSTRAINT: FieldConstraint = {
  type: "string",
  required: true,
  maxLength: 253,
  pattern: DNS_SUBDOMAIN_PATTERN,
};

export function getConstraint(
  constraints: Record<string, FieldConstraint>,
  path: string
): FieldConstraint | undefined {
  if (constraints[path]) return constraints[path];
  if (path === "metadata.name") return METADATA_NAME_CONSTRAINT;
  return undefined;
}

function isEmpty(value: unknown): boolean {
  return value === undefined || value === null || value === "";
}

function patternMessage(pattern: string): string {
  return PATTERN_MESSAGES[pattern] ?? "Invalid format.";
}

function checkString(value: string, c: FieldConstraint): string | null {
  if (c.enum && !c.enum.includes(value)) {
    return `Must be one of: ${c.enum.join(", ")}.`;
  }
  if (c.pattern && !new RegExp(c.pattern).test(value)) {
    return patternMessage(c.pattern);
  }
  if (c.maxLength !== undefined && value.length > c.maxLength) {
    return `Must be ${c.maxLength} characters or fewer.`;
  }
  if (c.minLength !== undefined && value.length < c.minLength) {
    return `Must be at least ${c.minLength} characters.`;
  }
  return null;
}

function checkNumber(value: number, c: FieldConstraint): string | null {
  if (c.minimum !== undefined && value < c.minimum) return `Must be at least ${c.minimum}.`;
  if (c.maximum !== undefined && value > c.maximum) return `Must be at most ${c.maximum}.`;
  return null;
}

/**
 * Validate a single value against a constraint. Returns a friendly message, or
 * null when valid. `required` is only enforced when opts.isSubmit is true, so a
 * pristine field isn't scolded as the user types.
 */
export function validateField(
  value: unknown,
  constraint: FieldConstraint,
  opts: { isSubmit?: boolean } = {}
): string | null {
  if (isEmpty(value)) {
    return opts.isSubmit && constraint.required ? "This field is required." : null;
  }
  if (typeof value === "string") return checkString(value, constraint);
  if (typeof value === "number") return checkNumber(value, constraint);
  return null;
}
