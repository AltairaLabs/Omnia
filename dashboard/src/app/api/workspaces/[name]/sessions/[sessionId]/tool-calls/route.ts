/**
 * Session tool calls proxy route.
 *
 * GET /api/workspaces/{name}/sessions/{sessionId}/tool-calls
 *   -> SESSION_API_URL/api/v1/sessions/{sessionId}/tool-calls
 *
 * Verifies the session belongs to the workspace's namespace before
 * returning data (prevents cross-workspace IDOR).
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { verifySessionNamespace } from "../../session-namespace-guard";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";

type Params = { name: string; sessionId: string };

export const GET = withWorkspaceAccess<Params>(
  "viewer",
  async (
    _request: NextRequest,
    context: WorkspaceRouteContext<Params>,
    _access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    const { name, sessionId } = await context.params;

    const guard = await verifySessionNamespace(name, sessionId);
    if (!guard.ok) return guard.response;

    const targetUrl = `${guard.baseUrl}/api/v1/sessions/${encodeURIComponent(sessionId)}/tool-calls`;

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
          toolCalls: [],
        },
        { status: 502 }
      );
    }
  }
);
