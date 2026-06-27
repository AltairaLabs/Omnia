"use client";

/**
 * SessionWatcher — polls /api/auth/session on a configurable interval and
 * redirects to /login if the session has expired.
 *
 * Behaviour:
 * - No-op when the user is not authenticated (anonymous mode or before login).
 * - No-op when authMode is "anonymous" (no sessions to expire).
 * - Polls every `pollIntervalSeconds` seconds (minimum 15).
 * - Also fires on window focus and on visibility-change back to visible,
 *   so a tab left in the background detects expiry as soon as it regains focus.
 * - Only one redirect fires per page lifetime (idempotent via redirectToLogin).
 *
 * Mount this inside AuthProvider (e.g. in AuthWrapper) so it has access to
 * the auth context.
 */

import { useEffect } from "react";
import { useAuth } from "@/hooks/use-auth";
import { useRuntimeConfig } from "@/hooks/use-runtime-config";
import { redirectToLogin } from "@/lib/auth/redirect-to-login";

const MIN_POLL_INTERVAL_MS = 15_000;

async function checkSession(): Promise<boolean> {
  try {
    const res = await fetch("/api/auth/session");
    return res.status !== 401;
  } catch {
    // Network error — assume session is still alive to avoid false positives.
    return true;
  }
}

export function SessionWatcher() {
  const { isAuthenticated } = useAuth();
  const { config } = useRuntimeConfig();
  const { authMode, sessionPollIntervalSeconds } = config;

  const intervalMs = Math.max(
    MIN_POLL_INTERVAL_MS,
    sessionPollIntervalSeconds * 1000,
  );

  const shouldWatch = isAuthenticated && authMode !== "anonymous";

  useEffect(() => {
    if (!shouldWatch) return;

    async function poll() {
      const ok = await checkSession();
      if (!ok) {
        redirectToLogin();
      }
    }

    function onFocus() {
      void poll();
    }

    function onVisibilityChange() {
      if (!document.hidden) {
        void poll();
      }
    }

    const timer = setInterval(() => {
      void poll();
    }, intervalMs);

    window.addEventListener("focus", onFocus);
    document.addEventListener("visibilitychange", onVisibilityChange);

    return () => {
      clearInterval(timer);
      window.removeEventListener("focus", onFocus);
      document.removeEventListener("visibilitychange", onVisibilityChange);
    };
  }, [shouldWatch, intervalMs]);

  return null;
}
