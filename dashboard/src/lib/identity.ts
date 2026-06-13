/**
 * User identity pseudonymization for the dashboard.
 *
 * Produces the same deterministic hash as pkg/identity.PseudonymizeID in Go,
 * so dashboard queries match server-side stored pseudonyms.
 *
 * Algorithm: first 16 hex chars of HMAC-SHA256(raw) when
 * OMNIA_PSEUDONYM_HMAC_KEY is set; otherwise legacy SHA-256(raw).
 */

import { createHash, createHmac } from "crypto";

const PSEUDONYM_LENGTH = 16;
const PSEUDONYM_HMAC_KEY_ENV = "OMNIA_PSEUDONYM_HMAC_KEY";

/**
 * Returns a deterministic, non-reversible pseudonym for a user ID.
 * Empty input returns empty string.
 *
 * Must match Go's pkg/identity.PseudonymizeID exactly.
 */
export function pseudonymizeId(raw: string): string {
  if (!raw) return "";

  const hmacKey = process.env[PSEUDONYM_HMAC_KEY_ENV];
  if (hmacKey) {
    const hmac = createHmac("sha256", hmacKey).update(raw).digest("hex");
    return hmac.slice(0, PSEUDONYM_LENGTH);
  }

  const hash = createHash("sha256").update(raw).digest("hex");
  return hash.slice(0, PSEUDONYM_LENGTH);
}
