/**
 * OAuth callback endpoint.
 *
 * GET /api/auth/callback - Handle OAuth provider callback
 *
 * Query params (from IdP):
 * - code: Authorization code
 * - state: State parameter for CSRF validation
 * - error: Error code (if auth failed)
 * - error_description: Error description (if auth failed)
 */

import { NextRequest, NextResponse } from "next/server";
import { getAuthConfig } from "@/lib/auth/config";
import { getSession } from "@/lib/auth/session";
import {
  exchangeCodeForTokens,
  extractClaims,
  mapClaimsToUser,
  getUserInfo,
  validateClaims,
} from "@/lib/auth/oauth";

export async function GET(request: NextRequest) {
  const config = getAuthConfig();
  const session = await getSession();
  const { searchParams } = request.nextUrl;

  // Check for error from IdP
  const error = searchParams.get("error");
  if (error) {
    const description = searchParams.get("error_description") || error;
    console.error("OAuth error from IdP:", error, description);
    return NextResponse.redirect(
      new URL(`/login?error=${encodeURIComponent(error)}`, request.url)
    );
  }

  // Verify we have PKCE data from login
  if (!session.pkce) {
    console.error("OAuth callback: No PKCE data in session");
    return NextResponse.redirect(
      new URL("/login?error=invalid_state", request.url)
    );
  }

  // Verify state parameter
  const state = searchParams.get("state");
  if (!state || state !== session.pkce.state) {
    console.error("OAuth callback: State mismatch");
    return NextResponse.redirect(
      new URL("/login?error=invalid_state", request.url)
    );
  }

  // Get authorization code
  const code = searchParams.get("code");
  if (!code) {
    console.error("OAuth callback: No authorization code");
    return NextResponse.redirect(
      new URL("/login?error=no_code", request.url)
    );
  }

  try {
    // Exchange code for tokens
    const tokens = await exchangeCodeForTokens(code, session.pkce);

    // Extract claims from ID token
    let claims = extractClaims(tokens);

    // If no claims in ID token, fetch from UserInfo endpoint
    if (!validateClaims(claims) && tokens.access_token) {
      // For UserInfo, we need a subject - use sub from partial claims or skip check
      const sub = (claims.sub as string) || "";
      const userInfo = await getUserInfo(tokens.access_token, sub);
      claims = userInfo as Record<string, unknown>;
    }

    // Validate we have required claims
    if (!validateClaims(claims)) {
      console.error("OAuth callback: Missing required claims");
      return NextResponse.redirect(
        new URL("/login?error=invalid_claims", request.url)
      );
    }

    // Map claims to user
    const user = mapClaimsToUser(claims, config);

    // Store user and tokens in session
    session.user = user;
    session.createdAt = Date.now();
    session.oauth = {
      accessToken: tokens.access_token,
      refreshToken: tokens.refresh_token,
      idToken: tokens.id_token,
      expiresAt: typeof tokens.expires_at === "number" ? tokens.expires_at : undefined,
      provider: config.oauth.provider,
    };

    // Get return URL and clear PKCE data
    const returnTo = session.pkce.returnTo || "/";
    delete session.pkce;

    await session.save();

    // Redirect to original destination
    return NextResponse.redirect(new URL(returnTo, request.url));
  } catch (error) {
    console.error("OAuth callback error:", error);
    const message = error instanceof Error ? error.message : "Unknown error";
    return NextResponse.redirect(
      new URL(`/login?error=callback_failed&message=${encodeURIComponent(message)}`, request.url)
    );
  }
}
