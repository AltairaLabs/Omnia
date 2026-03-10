/**
 * Workspace eval-results proxy route.
 *
 * GET /api/workspaces/{name}/eval-results
 *   -> SESSION_API_URL/api/v1/eval-results
 *
 * Forwards query parameters (agentName, namespace, evalId, evalType, passed,
 * limit, offset) to the session-api backend.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";

const SESSION_API_URL = process.env.SESSION_API_URL;

type Params = { name: string };

export const GET = withWorkspaceAccess<Params>(
  "viewer",
  async (
    request: NextRequest,
    _context: WorkspaceRouteContext<Params>,
    _access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    if (!SESSION_API_URL) {
      return NextResponse.json(
        { error: "Session API not configured", results: [], total: 0, hasMore: false },
        { status: 503 }
      );
    }

    const baseUrl = SESSION_API_URL.endsWith("/") ? SESSION_API_URL.slice(0, -1) : SESSION_API_URL;
    const searchParams = request.nextUrl.searchParams.toString();
    const qs = searchParams ? `?${searchParams}` : "";
    const targetUrl = `${baseUrl}/api/v1/eval-results${qs}`;

    try {
      const response = await fetch(targetUrl, {
        headers: { Accept: "application/json" },
      });

      const data = await response.json();
      return NextResponse.json(data, { status: response.status });
    } catch (error) {
      console.error("Session API eval-results proxy error:", error);
      return NextResponse.json(
        {
          error: "Failed to connect to Session API",
          details: error instanceof Error ? error.message : String(error),
          results: [],
          total: 0,
          hasMore: false,
        },
        { status: 502 }
      );
    }
  }
);
