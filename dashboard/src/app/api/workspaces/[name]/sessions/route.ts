/**
 * Session list/search proxy route.
 *
 * GET /api/workspaces/{name}/sessions
 *   → SESSION_API_URL/api/v1/sessions?workspace={name}&...
 *
 * When `q` param is present, routes to the /search backend endpoint.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";

const SESSION_API_URL = process.env.SESSION_API_URL;

export const GET = withWorkspaceAccess(
  "viewer",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext,
    _access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;

    if (!SESSION_API_URL) {
      return NextResponse.json(
        { error: "Session API not configured", sessions: [], total: 0, hasMore: false },
        { status: 503 }
      );
    }

    // Forward query params to the backend
    const params = new URLSearchParams();
    params.set("workspace", name);

    const forwardParams = ["agent", "status", "from", "to", "limit", "offset", "q"];
    for (const key of forwardParams) {
      const value = request.nextUrl.searchParams.get(key);
      if (value) params.set(key, value);
    }

    // Route to /search endpoint when q param is present
    const hasQuery = request.nextUrl.searchParams.has("q");
    const endpoint = hasQuery ? "sessions/search" : "sessions";

    const baseUrl = SESSION_API_URL.endsWith("/") ? SESSION_API_URL.slice(0, -1) : SESSION_API_URL;
    const targetUrl = `${baseUrl}/api/v1/${endpoint}?${params.toString()}`;

    try {
      const response = await fetch(targetUrl, {
        headers: { Accept: "application/json" },
      });

      const data = await response.json();
      return NextResponse.json(data, { status: response.status });
    } catch (error) {
      console.error("Session API proxy error:", error);
      return NextResponse.json(
        {
          error: "Failed to connect to Session API",
          details: error instanceof Error ? error.message : String(error),
          sessions: [],
          total: 0,
          hasMore: false,
        },
        { status: 502 }
      );
    }
  }
);
