/**
 * OAuth token refresh endpoint.
 *
 * POST /api/auth/refresh - Refresh access token
 *
 * Uses the refresh token stored in the session to obtain new tokens.
 * Updates the session with new tokens.
 */

import { NextResponse } from "next/server";
import { getAuthConfig } from "@/lib/auth/config";
import { getSession } from "@/lib/auth/session";
import {
  refreshAccessToken,
  extractClaims,
  mapClaimsToUser,
  validateClaims,
} from "@/lib/auth/oauth";

export async function POST() {
  const config = getAuthConfig();
  const session = await getSession();

  // Check we're in OAuth mode
  if (config.mode !== "oauth") {
    return NextResponse.json(
      { error: "OAuth authentication is not enabled" },
      { status: 400 }
    );
  }

  // Check we have a refresh token
  if (!session.oauth?.refreshToken) {
    return NextResponse.json(
      { error: "No refresh token available" },
      { status: 400 }
    );
  }

  try {
    // Refresh the tokens
    const tokens = await refreshAccessToken(session.oauth.refreshToken);

    // Update session with new tokens
    session.oauth = {
      ...session.oauth,
      accessToken: tokens.access_token,
      // Use new refresh token if provided, otherwise keep the old one
      refreshToken: tokens.refresh_token || session.oauth.refreshToken,
      idToken: tokens.id_token || session.oauth.idToken,
      expiresAt: typeof tokens.expires_at === "number" ? tokens.expires_at : session.oauth.expiresAt,
    };

    // Optionally update user from new ID token claims
    if (tokens.id_token) {
      const claims = extractClaims(tokens);
      if (validateClaims(claims)) {
        session.user = mapClaimsToUser(claims, config);
      }
    }

    await session.save();

    return NextResponse.json({
      success: true,
      expiresAt: tokens.expires_at,
    });
  } catch (error) {
    console.error("Token refresh error:", error);

    // If refresh fails, the user needs to re-authenticate
    return NextResponse.json(
      { error: "Token refresh failed", requiresLogin: true },
      { status: 401 }
    );
  }
}
