/** Random identifiers for the CLI browser-login handoff. */
import { randomBytes, randomUUID } from "node:crypto";

/** Opaque flow id carried in the picker URL. */
export function newFlowId(): string {
  return randomUUID();
}

/** One-time exchange code (URL-safe, 256-bit). */
export function newOneTimeCode(): string {
  return randomBytes(32).toString("base64url");
}

/** Short suffix to disambiguate minted token names. */
export function shortSuffix(): string {
  return randomBytes(4).toString("hex");
}
