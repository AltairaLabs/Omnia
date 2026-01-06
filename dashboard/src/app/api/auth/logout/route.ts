/**
 * OAuth logout endpoint.
 *
 * POST /api/auth/logout - Log out user and optionally redirect to IdP logout
 *
 * Returns:
 * - { redirectUrl: string } - URL to redirect to (IdP logout or login page)
 */

import { NextResponse } from "next/server";
import { getAuthConfig } from "@/lib/auth/config";
import { getSession, clearSession } from "@/lib/auth/session";
import { buildEndSessionUrl } from "@/lib/auth/oauth";

export async function POST() {
  const config = getAuthConfig();
  const session = await getSession();

  let redirectUrl = "/login";

  // For OAuth mode, try to get IdP logout URL (single sign-out)
  if (config.mode === "oauth" && session.oauth?.idToken) {
    const endSessionUrl = await buildEndSessionUrl(session.oauth.idToken);
    if (endSessionUrl) {
      redirectUrl = endSessionUrl;
    }
  }

  // Clear local session
  await clearSession();

  return NextResponse.json({ redirectUrl });
}

/**
 * GET handler for logout (for simple redirects).
 */
export async function GET() {
  const config = getAuthConfig();
  const session = await getSession();

  let redirectUrl = "/login";

  // For OAuth mode, try to get IdP logout URL
  if (config.mode === "oauth" && session.oauth?.idToken) {
    const endSessionUrl = await buildEndSessionUrl(session.oauth.idToken);
    if (endSessionUrl) {
      redirectUrl = endSessionUrl;
    }
  }

  // Clear local session
  await clearSession();

  return NextResponse.redirect(new URL(redirectUrl, config.baseUrl));
}
