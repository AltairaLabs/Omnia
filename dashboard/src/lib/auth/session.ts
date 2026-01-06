/**
 * Session management using iron-session.
 *
 * Provides encrypted, stateless sessions stored in HTTP-only cookies.
 */

import { getIronSession, IronSession } from "iron-session";
import { cookies } from "next/headers";
import { getAuthConfig } from "./config";
import type { SessionData, User } from "./types";

/**
 * Get the session options from config.
 */
function getSessionOptions() {
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

/**
 * Get the current session.
 */
export async function getSession(): Promise<IronSession<SessionData>> {
  const cookieStore = await cookies();
  return getIronSession<SessionData>(cookieStore, getSessionOptions());
}

/**
 * Get the current user from session.
 * Returns null if not authenticated.
 */
export async function getCurrentUser(): Promise<User | null> {
  const session = await getSession();
  return session.user || null;
}

/**
 * Save user to session.
 */
export async function saveUserToSession(user: User): Promise<void> {
  const session = await getSession();
  session.user = user;
  session.createdAt = Date.now();
  await session.save();
}

/**
 * Clear the session (logout).
 */
export async function clearSession(): Promise<void> {
  const session = await getSession();
  session.destroy();
}

/**
 * Check if user is authenticated.
 */
export async function isAuthenticated(): Promise<boolean> {
  const user = await getCurrentUser();
  return user !== null && user.provider !== "anonymous";
}
