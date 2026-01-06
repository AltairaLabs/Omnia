/**
 * OAuth login endpoint.
 *
 * GET /api/auth/login - Initiate OAuth flow
 *
 * Query params:
 * - returnTo: URL to redirect to after successful login (optional)
 */

import { NextRequest, NextResponse } from "next/server";
import { getAuthConfig } from "@/lib/auth/config";
import { getSession } from "@/lib/auth/session";
import { generatePKCE, buildAuthorizationUrl } from "@/lib/auth/oauth";

export async function GET(request: NextRequest) {
  const config = getAuthConfig();

  // Only available in OAuth mode
  if (config.mode !== "oauth") {
    return NextResponse.json(
      { error: "OAuth authentication is not enabled" },
      { status: 400 }
    );
  }

  try {
    // Get return URL from query params
    const returnTo = request.nextUrl.searchParams.get("returnTo") || "/";

    // Generate PKCE challenge and state
    const pkce = await generatePKCE(returnTo);

    // Store PKCE data in session for callback validation
    const session = await getSession();
    session.pkce = pkce;
    await session.save();

    // Build authorization URL and redirect
    const authUrl = await buildAuthorizationUrl(pkce);

    return NextResponse.redirect(authUrl);
  } catch (error) {
    console.error("OAuth login error:", error);
    const message = error instanceof Error ? error.message : "Unknown error";
    return NextResponse.redirect(
      new URL(`/login?error=config_error&message=${encodeURIComponent(message)}`, request.url)
    );
  }
}
