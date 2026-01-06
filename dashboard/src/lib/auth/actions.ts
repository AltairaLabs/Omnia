"use server";

/**
 * Server actions for authentication.
 */

import { getUser, clearSession } from "./index";
import type { User } from "./types";

/**
 * Get the current user (server action).
 */
export async function fetchCurrentUser(): Promise<User> {
  return getUser();
}

/**
 * Logout the current user (server action).
 */
export async function logout(): Promise<void> {
  await clearSession();
}
