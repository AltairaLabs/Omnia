/**
 * Eval results summary proxy route.
 *
 * GET /api/workspaces/{name}/eval-results/summary
 *   → SESSION_API_URL/api/v1/eval-results/summary?namespace={name}&...
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
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
        { error: "Session API not configured", summaries: [] },
        { status: 503 }
      );
    }

    const params = new URLSearchParams();
    params.set("namespace", name);

    const forwardParams = ["agentName", "createdAfter", "createdBefore"];
    for (const key of forwardParams) {
      const value = request.nextUrl.searchParams.get(key);
      if (value) params.set(key, value);
    }

    const baseUrl = SESSION_API_URL.endsWith("/") ? SESSION_API_URL.slice(0, -1) : SESSION_API_URL;
    const targetUrl = `${baseUrl}/api/v1/eval-results/summary?${params.toString()}`;

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
          summaries: [],
        },
        { status: 502 }
      );
    }
  }
);
