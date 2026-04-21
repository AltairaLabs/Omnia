import { NextRequest, NextResponse } from "next/server";
import { unsealData } from "iron-session";
import type { SessionCookieData } from "@/lib/auth/types";
import { getSessionStore } from "@/lib/auth/session-store";
import { applySecurityHeaders } from "@/lib/security-headers";

/**
 * Auth middleware — enforces authentication when OMNIA_AUTH_MODE is
 * anything other than "anonymous". Without this, unauthenticated
 * visitors to oauth/proxy/builtin-mode deployments silently get an
 * anonymous viewer session via `handleOAuthAuth` in lib/auth/index.ts,
 * which contradicts the chosen auth mode.
 *
 * Flow:
 *   - `anonymous` mode: pass everything through.
 *   - Otherwise: allow public paths (login, auth API, health, static
 *     assets) + requests carrying a valid session; redirect pages
 *     without a valid session to /login?returnTo=<path>, and return
 *     401 JSON for unauthenticated API requests (so JSON clients don't
 *     receive an HTML redirect).
 *
 * For `oauth` and `builtin` modes we unseal the iron-session cookie to
 * extract the `sid`, then look up the session record in the server-side
 * store and verify it carries a user whose `provider` matches the active
 * mode. A present-but-bogus cookie (stale, tampered, mode-mismatched,
 * or expired from the store) is treated as unauthenticated and cleared
 * from the response.
 *
 * For `proxy` mode we keep the presence check: proxy deployments
 * mint a session on the first authenticated request and the proxy
 * itself is the trust anchor, so re-decrypting here would regress
 * cold-start behaviour.
 */

const PUBLIC_PATH_PREFIXES: readonly string[] = [
  "/login",
  "/api/auth/login",
  "/api/auth/callback",
  "/api/auth/logout",
  "/api/auth/refresh",
  "/api/auth/builtin/", // signup / forgot-password / reset-password / verify-email
  "/api/health",
  "/api/config", // needed by the login page to pick the provider button
  "/api/license",
  "/_next/",
  "/favicon",
];

function isPublicPath(pathname: string): boolean {
  for (const prefix of PUBLIC_PATH_PREFIXES) {
    if (pathname === prefix || pathname.startsWith(prefix)) {
      return true;
    }
  }
  return false;
}

// Kept in sync with lib/auth/config.ts:generateDevSecret(). iron-session
// requires a password ≥ 32 chars; this lets local dev work when
// OMNIA_SESSION_SECRET is unset and matches the fallback used by the
// app so cookies written by the app decode here.
const DEV_SESSION_SECRET = "omnia-dev-secret-do-not-use-in-production-32";

function getSessionOptions() {
  return {
    password: process.env.OMNIA_SESSION_SECRET || DEV_SESSION_SECRET,
    cookieName: process.env.OMNIA_SESSION_COOKIE_NAME ?? "omnia_session",
  };
}

async function hasValidSession(
  req: NextRequest,
  mode: "oauth" | "builtin",
): Promise<boolean> {
  const opts = getSessionOptions();
  const cookie = req.cookies.get(opts.cookieName);
  if (!cookie) return false;
  try {
    // Unseal the slim cookie payload to extract the session ID, then
    // look up the full session record in the server-side store. This
    // keeps the cookie small (≤ 4 KB) and ensures the session is valid
    // and hasn't been revoked (e.g. by logout).
    const { sid } = await unsealData<SessionCookieData>(cookie.value, {
      password: opts.password,
    });
    if (!sid) return false;
    const record = await getSessionStore().getSession(sid);
    if (!record?.user) return false;
    return record.user.provider === mode;
  } catch {
    // Bad signature / wrong password / corrupt ciphertext — treat as no session.
    return false;
  }
}

function unauthenticatedResponse(
  req: NextRequest,
  cookieName: string,
): NextResponse {
  const { pathname } = req.nextUrl;
  let response: NextResponse;
  if (pathname.startsWith("/api/")) {
    response = NextResponse.json({ error: "unauthenticated" }, { status: 401 });
  } else {
    const loginUrl = new URL("/login", req.url);
    loginUrl.searchParams.set("returnTo", pathname + req.nextUrl.search);
    response = NextResponse.redirect(loginUrl);
  }
  // Clean up the invalid cookie so the next request doesn't repeat the
  // dance. `response.cookies.delete(name)` issues a Set-Cookie with empty
  // value + past expiry but omits HttpOnly/Secure/SameSite — we set them
  // explicitly so the clearing cookie carries the same security attributes
  // as the original (pen-test H-2).
  response.cookies.set({
    name: cookieName,
    value: "",
    path: "/",
    maxAge: 0,
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
  });
  return response;
}

export async function middleware(req: NextRequest) {
  const mode = process.env.OMNIA_AUTH_MODE ?? "anonymous";
  if (mode === "anonymous") {
    return applySecurityHeaders(NextResponse.next());
  }

  const { pathname } = req.nextUrl;
  if (isPublicPath(pathname)) {
    return applySecurityHeaders(NextResponse.next());
  }

  const cookieName = process.env.OMNIA_SESSION_COOKIE_NAME ?? "omnia_session";

  if (mode === "oauth" || mode === "builtin") {
    const ok = await hasValidSession(req, mode);
    if (ok) return applySecurityHeaders(NextResponse.next());
    return applySecurityHeaders(unauthenticatedResponse(req, cookieName));
  }

  // proxy (and any future mode): presence-check is the safest behaviour.
  // A fresh proxy request with headers but no cookie gets through and
  // handleProxyAuth in lib/auth/index.ts mints the session.
  if (req.cookies.has(cookieName)) {
    return applySecurityHeaders(NextResponse.next());
  }
  return applySecurityHeaders(unauthenticatedResponse(req, cookieName));
}

export const config = {
  // Exclude Next.js internal assets up front so they never hit the
  // middleware function at all. The isPublicPath() belt-and-braces
  // check covers the remainder.
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
  // Node runtime is required because the session store can transitively
  // import ioredis (Node-only). The Edge runtime would fail to bundle it
  // and every request would throw at module-load time.
  runtime: "nodejs",
};
