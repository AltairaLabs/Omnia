/**
 * Authoritative user-scoping for per-user proxy routes.
 *
 * Server-side enforcement of *whose* data a request may act on. For an
 * authenticated user the scope is ALWAYS their session identity (`user.id`) —
 * a client-supplied `?userId` is ignored — so a workspace viewer cannot read,
 * export, delete, or overwrite another user's data by passing someone else's
 * id (#1263). Anonymous users have no session identity, so their device id
 * (sent as `userId`) is the only available scope, matching the memory write
 * path's device scoping.
 */

import type { User } from "./types";

/**
 * Returns the user id the request is allowed to act on, or null when no
 * identity is available (anonymous request with no device id).
 */
export function resolveScopedUserId(
  searchParams: URLSearchParams,
  user: User
): string | null {
  if (user.provider === "anonymous") {
    return searchParams.get("userId");
  }
  const clientUserId = searchParams.get("userId");
  if (clientUserId && clientUserId !== user.id) {
    console.warn(
      "[scoped-user] ignoring client-supplied userId; scoping to authenticated session user"
    );
  }
  return user.id;
}
