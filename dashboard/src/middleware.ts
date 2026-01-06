/**
 * Next.js middleware for authentication.
 *
 * Handles:
 * - Proxy header validation
 * - Session refresh
 * - Protected route enforcement
 */

import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

/**
 * Routes that don't require authentication.
 */
const PUBLIC_ROUTES = [
  "/api/health",
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

    // If no user header, the proxy should redirect to login
    // We just continue and let the app handle showing appropriate UI
    if (!username) {
      // For API routes, return 401
      if (pathname.startsWith("/api/") && !pathname.startsWith("/api/health")) {
        return NextResponse.json(
          { error: "Authentication required" },
          { status: 401 }
        );
      }
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
