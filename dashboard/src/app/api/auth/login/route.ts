/**
 * OAuth login endpoint.
 *
 * GET /api/auth/login — start the OAuth flow.
 *
 * The PKCE record lives in the server-side session store keyed by the
 * IdP `state` parameter. A tiny ephemeral `omnia_oauth_state` cookie
 * binds the login-initiating browser to the state value, so that the
 * callback can reject cross-origin login attempts even though PKCE is
 * no longer sealed into the user's session cookie.
 */

import { NextRequest, NextResponse } from "next/server";
import { getAuthConfig } from "@/lib/auth/config";
import { generatePKCE, buildAuthorizationUrl } from "@/lib/auth/oauth";
import { getSessionStore } from "@/lib/auth/session-store";

const STATE_COOKIE_NAME = "omnia_oauth_state";

export async function GET(request: NextRequest) {
  const config = getAuthConfig();

  if (config.mode !== "oauth") {
    return NextResponse.json({ error: "OAuth authentication is not enabled" }, { status: 400 });
  }

  try {
    const returnTo = request.nextUrl.searchParams.get("returnTo") || "/";
    const pkce = await generatePKCE(returnTo);

    await getSessionStore().putPkce(
      pkce.state,
      { ...pkce, createdAt: Date.now() },
      config.session.pkceTtl,
    );

    const authUrl = await buildAuthorizationUrl(pkce);

    const response = NextResponse.redirect(authUrl);
    response.cookies.set({
      name: STATE_COOKIE_NAME,
      value: pkce.state,
      httpOnly: true,
      secure: process.env.NODE_ENV === "production",
      sameSite: "lax",
      path: "/",
      maxAge: config.session.pkceTtl,
    });
    return response;
  } catch (error) {
    console.error("OAuth login error:", error);
    const message = error instanceof Error ? error.message : "Unknown error";
    return NextResponse.redirect(
      new URL(`/login?error=config_error&message=${encodeURIComponent(message)}`, config.baseUrl),
    );
  }
}
