/**
 * OAuth callback endpoint.
 *
 * GET /api/auth/callback — complete the OAuth flow.
 *
 * Two-factor validation of the IdP `state` parameter:
 *   1. The URL state must match the `omnia_oauth_state` cookie set at
 *      /login (same-browser binding; defence against cross-origin CSRF
 *      now that PKCE is not sealed in the user's cookie).
 *   2. The server-side PKCE record at `pkce:<state>` must still exist
 *      and is consumed atomically via GETDEL (single-use replay defence).
 */

import { NextRequest, NextResponse } from "next/server";
import { getAuthConfig } from "@/lib/auth/config";
import { saveUserToSession } from "@/lib/auth/session";
import { getSessionStore } from "@/lib/auth/session-store";
import {
  exchangeCodeForTokens,
  extractClaims,
  mapClaimsToUserAsync,
  getUserInfo,
  validateClaims,
} from "@/lib/auth/oauth";

const STATE_COOKIE_NAME = "omnia_oauth_state";

function clearState(res: NextResponse): void {
  res.cookies.set({
    name: STATE_COOKIE_NAME,
    value: "",
    path: "/",
    maxAge: 0,
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
  });
}

export async function GET(request: NextRequest) {
  const config = getAuthConfig();
  const { searchParams } = request.nextUrl;

  const loginRedirect = (params: string, clearStateCookie = true) => {
    const res = NextResponse.redirect(new URL(`/login${params}`, config.baseUrl));
    if (clearStateCookie) clearState(res);
    return res;
  };

  const error = searchParams.get("error");
  if (error) {
    console.error("OAuth error from IdP:", error, searchParams.get("error_description"));
    return loginRedirect(`?error=${encodeURIComponent(error)}`);
  }

  const urlState = searchParams.get("state");
  const cookieState = request.cookies.get(STATE_COOKIE_NAME)?.value;
  if (!urlState || !cookieState || urlState !== cookieState) {
    console.error("OAuth callback: state cookie mismatch");
    return loginRedirect("?error=invalid_state");
  }

  const pkce = await getSessionStore().consumePkce(urlState);
  if (!pkce) {
    console.error("OAuth callback: pkce not found or already consumed");
    return loginRedirect("?error=invalid_state");
  }

  const code = searchParams.get("code");
  if (!code) return loginRedirect("?error=no_code");

  try {
    // Hand the full incoming URL to the token exchange so RFC 9207 iss
    // (emitted by Google / Google Workspace via
    // authorization_response_iss_parameter_supported=true) survives
    // openid-client's strict response validation. Issue #948.
    const tokens = await exchangeCodeForTokens(code, pkce, request.nextUrl);

    let claims = extractClaims(tokens);
    if (!validateClaims(claims) && tokens.access_token) {
      const sub = (claims.sub as string) || "";
      claims = (await getUserInfo(tokens.access_token, sub)) as Record<string, unknown>;
    }
    if (!validateClaims(claims)) {
      console.error("OAuth callback: missing required claims");
      return loginRedirect("?error=invalid_claims");
    }

    // mapClaimsToUserAsync resolves Entra groups-overage via Microsoft
    // Graph when the ID token has _claim_names.groups instead of an
    // inline list (issue #855). Needs the access_token, NOT the id_token.
    const user = await mapClaimsToUserAsync(claims, config, tokens.access_token);
    await saveUserToSession(user, {
      refreshToken: tokens.refresh_token,
      idToken: tokens.id_token,
      expiresAt: typeof tokens.expires_at === "number" ? tokens.expires_at : undefined,
      provider: config.oauth.provider,
    });

    const returnTo = pkce.returnTo || "/";
    const res = NextResponse.redirect(new URL(returnTo, config.baseUrl));
    clearState(res);
    return res;
  } catch (err) {
    console.error("OAuth callback error:", err);
    const message = err instanceof Error ? err.message : "Unknown error";
    return loginRedirect(`?error=callback_failed&message=${encodeURIComponent(message)}`);
  }
}
