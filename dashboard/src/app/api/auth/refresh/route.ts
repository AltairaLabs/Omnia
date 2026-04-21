/**
 * OAuth token refresh endpoint.
 *
 * Reads the current session record from the store, performs the refresh,
 * and writes the updated record back under the same sid.
 */

import { NextResponse } from "next/server";
import { getAuthConfig } from "@/lib/auth/config";
import { getSessionRecord, updateSessionOAuth } from "@/lib/auth/session";
import {
  refreshAccessToken,
  extractClaims,
  mapClaimsToUser,
  validateClaims,
} from "@/lib/auth/oauth";

export async function POST() {
  const config = getAuthConfig();
  const record = await getSessionRecord();

  if (config.mode !== "oauth") {
    return NextResponse.json({ error: "OAuth authentication is not enabled" }, { status: 400 });
  }
  if (!record?.oauth?.refreshToken) {
    return NextResponse.json({ error: "No refresh token available" }, { status: 400 });
  }

  try {
    const tokens = await refreshAccessToken(record.oauth.refreshToken);
    const nextOAuth = {
      ...record.oauth,
      refreshToken: tokens.refresh_token || record.oauth.refreshToken,
      idToken: tokens.id_token || record.oauth.idToken,
      expiresAt:
        typeof tokens.expires_at === "number" ? tokens.expires_at : record.oauth.expiresAt,
    };

    let nextUser = record.user;
    if (tokens.id_token) {
      const claims = extractClaims(tokens);
      if (validateClaims(claims)) {
        nextUser = mapClaimsToUser(claims, config);
      }
    }

    await updateSessionOAuth(nextOAuth, nextUser);

    return NextResponse.json({ success: true, expiresAt: tokens.expires_at });
  } catch (err) {
    console.error("Token refresh error:", err);
    return NextResponse.json(
      { error: "Token refresh failed", requiresLogin: true },
      { status: 401 },
    );
  }
}
