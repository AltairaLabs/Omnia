/**
 * Internal endpoint used by the custom WebSocket proxy (server.js) to
 * resolve the authenticated end-user id for a WS upgrade.
 *
 * The iron-session cookie carries only a `sid`; the user record lives in
 * the server-side session store. server.js (CJS, not part of the Next
 * bundle) cannot reach that store directly, so it forwards the request's
 * cookie here and reuses the real `getCurrentUser()` resolution path. That
 * keeps WS identity in lockstep with how the rest of the app identifies a
 * user — it cannot drift.
 *
 * Returns the RAW user id (not pseudonymised): the facade applies
 * `PseudonymizeID` itself, exactly as the memory read path sends the raw
 * id to its own proxy and hashes there. Raw-in-flight, hashed-at-rest is
 * the established identity contract (see #1255).
 *
 * Safe to expose: it only ever returns the caller's OWN identity, derived
 * from their session cookie. No session / anonymous → `{ userId: null }`,
 * which makes server.js omit the header and the facade fall back to the
 * device_id (anonymous) scoping it used before.
 */

import { NextResponse } from "next/server";
import { getCurrentUser } from "@/lib/auth/session";

export async function GET() {
  const user = await getCurrentUser();
  const userId = user && user.provider !== "anonymous" ? user.id : null;
  return NextResponse.json({ userId });
}
