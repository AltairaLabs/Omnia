/**
 * Server-side mgmt-plane auth for the function invoke proxy.
 *
 * Mirrors the Console WS proxy (server.js): when a mgmt-plane signing key is
 * mounted (OMNIA_MGMT_PLANE_SIGNING_KEY_PATH), mint a short-lived RS256 JWT and
 * attach it as `Authorization: Bearer` so the facade's mgmt-plane validator
 * admits the request. When no key is configured (dev/CI), return no header —
 * the facade's allow-unauthenticated fallback admits it, exactly like the
 * Console's unauthenticated default.
 */

import type { KeyObject } from "node:crypto";
// The minter is shared CJS (server.js requires the same module).
import { loadSigningKey, mintToken } from "../../../lib/mgmt-plane-token";

let cachedPath: string | undefined | null = undefined;
let cachedKey: KeyObject | null = null;

/** Load the signing key, caching by path so a changed path reloads. */
function signingKey(): KeyObject | null {
  const path = process.env.OMNIA_MGMT_PLANE_SIGNING_KEY_PATH || "";
  if (path === cachedPath) return cachedKey;
  cachedPath = path;
  if (!path) {
    cachedKey = null;
    return null;
  }
  try {
    cachedKey = loadSigningKey(path) as KeyObject;
  } catch {
    cachedKey = null;
  }
  return cachedKey;
}

/**
 * Build the upstream auth headers for invoking a function's facade. Empty when
 * no signing key is configured (dev) or minting fails — never throws.
 */
export function mgmtPlaneAuthHeaders(
  fnName: string,
  workspace: string,
  subject: string,
): Record<string, string> {
  const key = signingKey();
  if (!key) return {};
  try {
    const token = mintToken({ key, subject, agent: fnName, workspace });
    return { Authorization: `Bearer ${token}` };
  } catch {
    return {};
  }
}
