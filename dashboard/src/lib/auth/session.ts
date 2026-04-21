/**
 * Session helpers backed by the `SessionStore`.
 *
 * The iron-session cookie carries only `{ sid }`; the full session
 * record (user, tokens, metadata) lives in the server-side store.
 * This keeps the cookie fixed-size across every IDP and lets logout
 * actually revoke — neither is possible with cookie-sealed sessions.
 */

import { randomBytes } from "node:crypto";
import { getIronSession, IronSession } from "iron-session";
import { cookies } from "next/headers";
import { getAuthConfig } from "./config";
import { getSessionStore } from "./session-store";
import type { SessionRecord } from "./session-store/types";
import type { OAuthTokens } from "./oauth/types";
import type { SessionCookieData, User } from "./types";

function getCookieOptions() {
  const config = getAuthConfig();
  return {
    password: config.session.secret,
    cookieName: config.session.cookieName,
    cookieOptions: {
      secure: process.env.NODE_ENV === "production",
      httpOnly: true,
      sameSite: "lax" as const,
      maxAge: config.session.ttl,
    },
  };
}

async function getCookieSession(): Promise<IronSession<SessionCookieData>> {
  const cookieStore = await cookies();
  return getIronSession<SessionCookieData>(cookieStore, getCookieOptions());
}

function newSid(): string {
  return randomBytes(32).toString("base64url");
}

/** Read the server-side record for the current browser session, or null. */
export async function getSessionRecord(): Promise<SessionRecord | null> {
  const cookie = await getCookieSession();
  if (!cookie.sid) return null;
  return getSessionStore().getSession(cookie.sid);
}

/** Current user, or null if not authenticated. */
export async function getCurrentUser(): Promise<User | null> {
  const record = await getSessionRecord();
  return record?.user ?? null;
}

/**
 * Mint a fresh session id, write the record to the store, seal the sid
 * into the cookie. Always rotates the sid — defends against session
 * fixation at the login boundary.
 */
export async function saveUserToSession(user: User, oauth?: OAuthTokens): Promise<void> {
  const config = getAuthConfig();
  const sid = newSid();
  const record: SessionRecord = {
    user,
    oauth,
    createdAt: Date.now(),
  };
  await getSessionStore().putSession(sid, record, config.session.ttl);
  const cookie = await getCookieSession();
  cookie.sid = sid;
  await cookie.save();
}

/**
 * Update the OAuth tokens on the current session (refresh flow). No-op
 * if the session no longer exists.
 */
export async function updateSessionOAuth(oauth: OAuthTokens, user?: User): Promise<void> {
  const cookie = await getCookieSession();
  if (!cookie.sid) return;
  const config = getAuthConfig();
  const store = getSessionStore();
  const current = await store.getSession(cookie.sid);
  if (!current) return;
  const next: SessionRecord = {
    ...current,
    oauth,
    user: user ?? current.user,
  };
  await store.putSession(cookie.sid, next, config.session.ttl);
}

/**
 * Returns the current iron-session cookie session.
 * Use `getSessionRecord()` to read user/oauth data from the server-side store.
 */
export async function getSession(): Promise<IronSession<SessionCookieData>> {
  return getCookieSession();
}

/** Delete the server-side record and clear the cookie. */
export async function clearSession(): Promise<void> {
  const cookie = await getCookieSession();
  const sid = cookie.sid;
  if (sid) {
    await getSessionStore().deleteSession(sid);
  }
  cookie.destroy();
}

export async function isAuthenticated(): Promise<boolean> {
  const user = await getCurrentUser();
  return user !== null && user.provider !== "anonymous";
}
