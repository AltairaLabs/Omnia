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
import { resolveServiceURLs } from "@/lib/k8s/service-url-resolver";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";

type Params = { name: string };

export const GET = withWorkspaceAccess<Params>(
  "viewer",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext<Params>,
    _access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;

    const urls = await resolveServiceURLs(name);
    if (!urls) {
      return NextResponse.json(
        { error: "Session API not configured", results: [], total: 0, hasMore: false },
        { status: 503 }
      );
    }

    const baseUrl = urls.sessionURL.endsWith("/") ? urls.sessionURL.slice(0, -1) : urls.sessionURL;
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
