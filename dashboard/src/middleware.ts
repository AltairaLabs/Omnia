import { NextRequest, NextResponse } from "next/server";

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
 *     assets) + requests carrying a session cookie; redirect pages
 *     without a session to /login?returnTo=<path>, and return 401 JSON
 *     for unauthenticated API requests (so JSON clients don't receive
 *     an HTML redirect).
 *
 * We deliberately only check *presence* of the session cookie here, not
 * validity. Expired or tampered cookies still reach the server-side
 * session reader, which treats them as anonymous — so the actual auth
 * guarantee still lives in lib/auth/session.ts. This middleware is the
 * "you must at least try to log in" guard.
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

export async function middleware(req: NextRequest) {
  const mode = process.env.OMNIA_AUTH_MODE ?? "anonymous";
  if (mode === "anonymous") {
    return NextResponse.next();
  }

  const { pathname } = req.nextUrl;
  if (isPublicPath(pathname)) {
    return NextResponse.next();
  }

  const cookieName = process.env.OMNIA_SESSION_COOKIE_NAME ?? "omnia_session";
  if (req.cookies.has(cookieName)) {
    return NextResponse.next();
  }

  if (pathname.startsWith("/api/")) {
    return NextResponse.json({ error: "unauthenticated" }, { status: 401 });
  }

  const loginUrl = new URL("/login", req.url);
  loginUrl.searchParams.set("returnTo", pathname + req.nextUrl.search);
  return NextResponse.redirect(loginUrl);
}

export const config = {
  // Exclude Next.js internal assets up front so they never hit the
  // middleware function at all. The isPublicPath() belt-and-braces
  // check covers the remainder.
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
