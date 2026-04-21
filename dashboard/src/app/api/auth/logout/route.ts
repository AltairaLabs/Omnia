/**
 * OAuth logout endpoint.
 *
 * Deletes the server-side session record (true revocation — impossible
 * with cookie-sealed sessions), clears the session cookie, and returns
 * the IdP end-session URL when the provider supports RP-initiated logout.
 */

import { NextResponse } from "next/server";
import { getAuthConfig } from "@/lib/auth/config";
import { clearSession, getSessionRecord } from "@/lib/auth/session";
import { buildEndSessionUrl } from "@/lib/auth/oauth";

async function resolveRedirect(): Promise<string> {
  const config = getAuthConfig();
  if (config.mode !== "oauth") return "/login";
  const record = await getSessionRecord();
  const idToken = record?.oauth?.idToken;
  if (!idToken) return "/login";
  const end = await buildEndSessionUrl(idToken);
  return end ?? "/login";
}

export async function POST() {
  const redirectUrl = await resolveRedirect();
  await clearSession();
  return NextResponse.json({ redirectUrl });
}

export async function GET() {
  const config = getAuthConfig();
  const redirectUrl = await resolveRedirect();
  await clearSession();
  return NextResponse.redirect(new URL(redirectUrl, config.baseUrl));
}
