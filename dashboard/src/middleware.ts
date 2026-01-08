/**
 * Next.js middleware for authentication.
 *
 * Handles:
 * - OAuth session validation
 * - Proxy header validation
 * - Session refresh
 * - Protected route enforcement
 */

import { NextResponse, type NextRequest } from "next/server";

/**
 * Routes that don't require authentication.
 */
const PUBLIC_ROUTES = [
  "/api/health",
  "/api/auth/login",
  "/api/auth/callback",
  "/api/auth/logout",
  "/api/auth/builtin/login",
  "/api/auth/builtin/signup",
  "/api/auth/builtin/forgot-password",
  "/api/auth/builtin/reset-password",
  "/api/auth/builtin/verify-email",
  "/login",
  "/signup",
  "/forgot-password",
  "/reset-password",
  "/verify-email",
  "/_next",
  "/favicon.ico",
];

/**
 * Check if a path is public.
 */
function isPublicRoute(pathname: string): boolean {
  return PUBLIC_ROUTES.some((route) => pathname.startsWith(route));
}

export function middleware(request: NextRequest) {
  const { pathname } = request.nextUrl;

  // Skip public routes
  if (isPublicRoute(pathname)) {
    return NextResponse.next();
  }

  // Get auth mode from env
  const authMode = process.env.OMNIA_AUTH_MODE || "anonymous";

  // Anonymous mode - allow all
  if (authMode === "anonymous") {
    return NextResponse.next();
  }

  // Proxy mode - check for user header
  if (authMode === "proxy") {
    const headerName = process.env.OMNIA_AUTH_PROXY_HEADER_USER || "X-Forwarded-User";
    const username = request.headers.get(headerName);

    // If no user header for API routes (except health), return 401
    // For non-API routes, continue and let the app handle showing appropriate UI
    if (!username && pathname.startsWith("/api/") && !pathname.startsWith("/api/health")) {
      return NextResponse.json(
        { error: "Authentication required" },
        { status: 401 }
      );
    }
  }

  // OAuth and builtin modes - check for session cookie
  if (authMode === "oauth" || authMode === "builtin") {
    const cookieName = process.env.OMNIA_SESSION_COOKIE_NAME || "omnia_session";
    const sessionCookie = request.cookies.get(cookieName);

    if (!sessionCookie) {
      // For page routes, redirect to login
      if (!pathname.startsWith("/api/")) {
        const loginUrl = new URL("/login", request.url);
        loginUrl.searchParams.set("returnTo", pathname);
        return NextResponse.redirect(loginUrl);
      }

      // For API routes, return 401
      return NextResponse.json(
        { error: "Authentication required", loginRequired: true },
        { status: 401 }
      );
    }
  }

  return NextResponse.next();
}

export const config = {
  matcher: [
    // Match all paths except static files
    "/((?!_next/static|_next/image|favicon.ico).*)",
  ],
};
