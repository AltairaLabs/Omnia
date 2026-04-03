/**
 * User identity pseudonymization for the dashboard.
 *
 * Produces the same deterministic hash as pkg/identity.PseudonymizeID in Go,
 * so dashboard queries match server-side stored pseudonyms.
 *
 * Algorithm: first 16 hex chars of SHA-256(raw).
 */

import { createHash } from "crypto";

const PSEUDONYM_LENGTH = 16;

/**
 * Returns a deterministic, non-reversible pseudonym for a user ID.
 * Empty input returns empty string.
 *
 * Must match Go's pkg/identity.PseudonymizeID exactly.
 */
export function pseudonymizeId(raw: string): string {
  if (!raw) return "";
  const hash = createHash("sha256").update(raw).digest("hex");
  return hash.slice(0, PSEUDONYM_LENGTH);
}
