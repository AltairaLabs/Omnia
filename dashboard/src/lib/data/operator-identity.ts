/**
 * Shared server-only helpers for minting operator identity JWTs and
 * resolving operator API base URLs.
 *
 * Both `content-api-service.ts` and `deploy-api-service.ts` call the
 * operator over HTTP with a short-lived RS256 identity JWT (carrying the
 * authenticated user's identity + groups) signed with the dashboard's
 * mgmt-plane key. The operator verifies the signature and recomputes
 * authorization server-side — it never trusts a role claim from the token.
 *
 * Extracted so the two services don't near-duplicate this logic (which
 * would trip SonarCloud's duplication gate).
 *
 * Server-only: reads the signing key off disk and never runs in the browser.
 */

import type { KeyObject } from "node:crypto";

import type { User } from "@/lib/auth/types";
// Shared CJS minter (server.js requires the same module); see invoke-token.ts.
import { loadSigningKey, mintIdentityToken } from "../../../lib/mgmt-plane-token";

/** Identity tokens are used immediately for a single request, so keep them short. */
const TOKEN_TTL_SECONDS = 60;

/**
 * Error carrying the operator's HTTP status so route handlers can pass
 * through 404 / 400 / 403 instead of collapsing everything to 500.
 */
export class OperatorApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
    this.name = "OperatorApiError";
  }
}

/**
 * Run `fn` and, if it throws the shared base OperatorApiError (a config failure
 * from signing-key / base-URL resolution), re-throw it as a service-specific
 * subclass via `wrap` so callers only ever see their own error type. Errors
 * that are already a subclass, or not an OperatorApiError at all, pass through
 * untouched. Shared so `content-api-service` and `deploy-api-service` don't
 * near-duplicate this wrapper (which would trip SonarCloud's duplication gate).
 */
export function asOperatorError<T>(
  fn: () => T,
  wrap: (message: string, status: number) => OperatorApiError,
): T {
  try {
    return fn();
  } catch (err) {
    if (err instanceof OperatorApiError && err.constructor === OperatorApiError) {
      throw wrap(err.message, err.status);
    }
    throw err;
  }
}

let cachedPath: string | undefined | null = undefined;
let cachedKey: KeyObject | null = null;

/** Load the signing key, caching by path so a changed path reloads. */
function signingKey(): KeyObject | null {
  const path = process.env.OMNIA_MGMT_PLANE_SIGNING_KEY_PATH || "";
  if (path === cachedPath) return cachedKey;
  cachedPath = path;
  cachedKey = path ? (loadSigningKey(path) as KeyObject) : null;
  return cachedKey;
}

/** Resolve the operator base URL for the given env var, without a trailing slash. */
export function operatorBaseURL(envVar: string): string {
  let url = process.env[envVar];
  if (!url) {
    throw new OperatorApiError(`${envVar} not configured`, 500);
  }
  while (url.endsWith("/")) {
    url = url.slice(0, -1);
  }
  return url;
}

function principalFor(user: User): { identity: string; groups: string[]; anonymous: boolean } {
  const anonymous = user.provider === "anonymous";
  return {
    identity: anonymous ? "" : user.email || user.username,
    groups: user.groups ?? [],
    anonymous,
  };
}

/** Mint a short-lived operator identity token (aud omnia-operator) for the given workspace + user. */
export function mintOperatorIdentityToken(workspace: string, user: User): string {
  const key = signingKey();
  if (!key) {
    throw new OperatorApiError("operator API auth not configured (no signing key)", 500);
  }
  const { identity, groups, anonymous } = principalFor(user);
  return mintIdentityToken({ key, workspace, identity, groups, anonymous, ttlSeconds: TOKEN_TTL_SECONDS });
}
