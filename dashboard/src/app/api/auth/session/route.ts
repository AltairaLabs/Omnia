/**
 * Session liveness probe.
 *
 * GET /api/auth/session — returns 200 when the caller's session is still
 * valid, or 401 when it has expired / was never established.
 *
 * Used by the client-side SessionWatcher component to detect expiry and
 * redirect to the login page without requiring a hard navigation.
 *
 * Response shapes:
 *   200 { authenticated: true }  — session is live
 *   401 { authenticated: false } — session expired or not present
 *
 * In anonymous-mode deployments `getUser()` always returns an anonymous
 * user (provider === "anonymous"), so this endpoint returns 401.  The
 * SessionWatcher component skips polling when `authMode === "anonymous"`,
 * so the 401 is never acted on.
 */

import { NextResponse } from "next/server";
import { getUser } from "@/lib/auth";

export async function GET() {
  const user = await getUser();
  if (user.provider === "anonymous") {
    return NextResponse.json({ authenticated: false }, { status: 401 });
  }
  return NextResponse.json({ authenticated: true });
}
