/**
 * Session detail proxy route.
 *
 * GET /api/workspaces/{name}/sessions/{sessionId}
 *   → SESSION_API_URL/api/v1/sessions/{sessionId}
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";

const SESSION_API_URL = process.env.SESSION_API_URL;

type Params = { name: string; sessionId: string };

export const GET = withWorkspaceAccess<Params>(
  "viewer",
  async (
    _request: NextRequest,
    context: WorkspaceRouteContext<Params>,
    _access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    const { sessionId } = await context.params;

    if (!SESSION_API_URL) {
      return NextResponse.json(
        { error: "Session API not configured" },
        { status: 503 }
      );
    }

    const baseUrl = SESSION_API_URL.endsWith("/") ? SESSION_API_URL.slice(0, -1) : SESSION_API_URL;
    const targetUrl = `${baseUrl}/api/v1/sessions/${encodeURIComponent(sessionId)}`;

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
        },
        { status: 502 }
      );
    }
  }
);
