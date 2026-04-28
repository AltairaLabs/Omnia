/**
 * Microsoft Entra (Azure AD) "groups overage" claim handler.
 *
 * When a user belongs to more than 200 Entra groups, the ID token's
 * `groups` claim is replaced by a `_claim_names.groups` pointer at a
 * Microsoft Graph endpoint that lists the full membership. The
 * out-of-the-box claim parser only reads inline `groups`, so overage
 * users silently resolve to `groups: []` → viewer role → no workspace
 * access (issue #855). This module detects the overage shape and
 * fetches the full list via Microsoft Graph.
 *
 * The original `_claim_sources` URL points at the legacy Azure AD
 * Graph (`graph.windows.net`), which is being deprecated. We hit
 * Microsoft Graph (`graph.microsoft.com`) instead — same data, current
 * API, no separate token scope needed when the access token already
 * has User.Read (which Entra grants by default for openid+profile).
 *
 * Returns objectId GUIDs in the same shape Entra puts in the `groups`
 * claim by default. Tenants that have configured `groups` to emit SAM
 * names instead would need different code, but those tenants are
 * almost always using AD groups + `optionalClaims.groupMembershipClaims`,
 * which has its own size-limit handling and rarely overflows.
 */

/**
 * GraphTransport is the minimal contract a `fetch`-like transport
 * needs to support. Tests inject mocks; production passes the global
 * `fetch`.
 */
export type GraphTransport = (
  url: string,
  init: { method: string; headers: Record<string, string>; body?: string },
) => Promise<{ ok: boolean; status: number; json: () => Promise<unknown> }>;

/**
 * GroupsOverflowResult describes how the resolution went. The caller
 * uses `resolved` for the group list and `kind` for logging /
 * telemetry. We never throw on a Graph error — the caller can decide
 * whether to fail closed (auth rejected) or fail open (empty groups,
 * viewer role) based on policy. Today the dashboard fails open with
 * a console.warn, matching the existing "missing groups → viewer"
 * behaviour for non-overage tokens.
 */
export interface GroupsOverflowResult {
  /** "inline" — no overage; caller should keep using the inline groups claim.
   *  "resolved" — overage detected, Graph returned full list (in `groups`).
   *  "graph_failed" — overage detected, Graph call failed (groups is empty).
   *  "no_token" — overage detected but caller has no access token to resolve. */
  kind: "inline" | "resolved" | "graph_failed" | "no_token";
  groups: string[];
  /** Operator-visible reason / endpoint when something went wrong, for
   *  console.warn output. */
  reason?: string;
}

/**
 * Maximum number of `@odata.nextLink` follow hops. Entra returns up
 * to 999 group IDs per page, so 10 hops covers ~10k group memberships
 * — well past any realistic ceiling. Bounded to prevent a server
 * sending us into an infinite redirect loop.
 */
const MAX_PAGES = 10;

/**
 * Per-call upper bound on Graph response time. Entra's Graph is
 * normally <500ms; 10s is generous for slow networks but tight enough
 * that a stuck call doesn't keep the whole login spinning.
 */
const REQUEST_TIMEOUT_MS = 10_000;

/**
 * resolveGroupsOverflow detects and resolves Entra's groups-overage
 * pointer. When the claims do NOT contain `_claim_names.groups`, it
 * returns `{ kind: "inline", groups: inlineGroups }` and the caller
 * proceeds with the inline groups unchanged.
 *
 * accessToken is the OAuth access token issued alongside the ID token
 * — Microsoft Graph requires `Bearer <access_token>`, NOT the ID
 * token. When Entra issues an ID token with the overage pointer it
 * also issues an access token with the User.Read scope, so the same
 * token works for the Graph call.
 */
export async function resolveGroupsOverflow(
  claims: Record<string, unknown>,
  inlineGroups: string[],
  accessToken: string | undefined,
  transport: GraphTransport = defaultTransport,
): Promise<GroupsOverflowResult> {
  const claimNames = claims._claim_names;
  if (!claimNames || typeof claimNames !== "object") {
    return { kind: "inline", groups: inlineGroups };
  }
  const groupsPointer = (claimNames as Record<string, unknown>).groups;
  if (typeof groupsPointer !== "string" || groupsPointer === "") {
    return { kind: "inline", groups: inlineGroups };
  }

  if (!accessToken) {
    return {
      kind: "no_token",
      groups: inlineGroups,
      reason:
        "Entra groups overage detected but no access_token available — " +
        "user will resolve to viewer. Token-refresh flow may be missing the User.Read scope.",
    };
  }

  // The user's objectId is the `oid` claim, present on every Entra
  // token. The legacy `_claim_sources` URL embeds the same value, so
  // we don't strictly need to parse it — going through Graph
  // directly is cleaner.
  const oid = typeof claims.oid === "string" ? claims.oid : undefined;
  const sub = typeof claims.sub === "string" ? claims.sub : undefined;
  const userKey = oid || sub;
  if (!userKey) {
    return {
      kind: "graph_failed",
      groups: inlineGroups,
      reason: "Entra groups overage: claims missing both oid and sub — cannot identify user",
    };
  }

  try {
    const groups = await fetchAllGroupIDs(userKey, accessToken, transport);
    return { kind: "resolved", groups };
  } catch (err) {
    return {
      kind: "graph_failed",
      groups: inlineGroups,
      reason: `Microsoft Graph getMemberObjects failed: ${err instanceof Error ? err.message : String(err)}`,
    };
  }
}

/**
 * fetchAllGroupIDs hits `POST /v1.0/users/{oid}/getMemberObjects`,
 * follows `@odata.nextLink` for pagination, returns the de-duplicated
 * set of objectIds. `securityEnabledOnly: false` returns ALL groups
 * (security + Microsoft 365), matching the inline `groups` claim's
 * default behaviour.
 *
 * Throws on transport failure or non-2xx status. The caller catches
 * and converts to a `graph_failed` result.
 */
async function fetchAllGroupIDs(
  userKey: string,
  accessToken: string,
  transport: GraphTransport,
): Promise<string[]> {
  const seen = new Set<string>();
  let url: string | null =
    `https://graph.microsoft.com/v1.0/users/${encodeURIComponent(userKey)}/getMemberObjects`;
  let pages = 0;
  let body: string | undefined = JSON.stringify({ securityEnabledOnly: false });
  let method = "POST";

  while (url && pages < MAX_PAGES) {
    pages++;
    const init: { method: string; headers: Record<string, string>; body?: string } = {
      method,
      headers: {
        Authorization: `Bearer ${accessToken}`,
        Accept: "application/json",
      },
    };
    if (body !== undefined) {
      init.headers["Content-Type"] = "application/json";
      init.body = body;
    }
    const resp = await callWithTimeout(transport, url, init, REQUEST_TIMEOUT_MS);
    if (!resp.ok) {
      throw new Error(`Graph returned status ${resp.status}`);
    }
    const data = (await resp.json()) as {
      value?: unknown;
      "@odata.nextLink"?: unknown;
    };
    if (Array.isArray(data.value)) {
      for (const v of data.value) {
        if (typeof v === "string") {
          seen.add(v);
        }
      }
    }
    const next = data["@odata.nextLink"];
    if (typeof next === "string" && next !== "") {
      url = next;
      // Pagination follow-up uses GET (Graph quirk: getMemberObjects
      // is POST for the initial call but returns GET-style next links).
      method = "GET";
      body = undefined;
    } else {
      url = null;
    }
  }
  return Array.from(seen);
}

/**
 * callWithTimeout adds an AbortController-driven timeout because the
 * Node fetch implementation does not honour the deprecated `timeout`
 * option in init. Without this a hung Graph endpoint would stall the
 * login until the OS-level TCP keepalive triggers (minutes).
 */
async function callWithTimeout(
  transport: GraphTransport,
  url: string,
  init: { method: string; headers: Record<string, string>; body?: string },
  timeoutMs: number,
): Promise<{ ok: boolean; status: number; json: () => Promise<unknown> }> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    return await transport(url, { ...init, ...{ signal: controller.signal } as object });
  } finally {
    clearTimeout(timer);
  }
}

/**
 * defaultTransport is a thin wrapper around the global `fetch`. Kept
 * narrow to make the test mocks trivial and prevent accidental
 * coupling to fetch-specific options.
 */
const defaultTransport: GraphTransport = async (url, init) => {
  const resp = await fetch(url, init as RequestInit);
  return {
    ok: resp.ok,
    status: resp.status,
    json: () => resp.json(),
  };
};
