/**
 * Workspace single function-invocation proxy route.
 *
 * GET /api/workspaces/{name}/function-invocations/{id}
 *   -> SESSION_API_URL/api/v1/function-invocations/{id}?namespace={name}
 *
 * Returns one invocation row. Cross-workspace reads (an id that belongs
 * to a different namespace) resolve to 404 — the session-api layer
 * enforces tenant isolation by treating namespace mismatch as missing,
 * not unauthorised, so existence does not leak across tenants.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { resolveServiceURLs } from "@/lib/k8s/service-url-resolver";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";

type Params = { name: string; id: string };

export const GET = withWorkspaceAccess<Params>(
  "viewer",
  async (
    _request: NextRequest,
    context: WorkspaceRouteContext<Params>,
    _access: WorkspaceAccess,
    _user: User,
  ): Promise<NextResponse> => {
    const { name, id } = await context.params;

    const urls = await resolveServiceURLs(name);
    if (!urls) {
      return NextResponse.json(
        { error: "Session API not configured" },
        { status: 503 },
      );
    }

    const baseUrl = urls.sessionURL.endsWith("/")
      ? urls.sessionURL.slice(0, -1)
      : urls.sessionURL;
    const targetUrl =
      `${baseUrl}/api/v1/function-invocations/${encodeURIComponent(id)}` +
      `?namespace=${encodeURIComponent(name)}`;

    try {
      const response = await fetch(targetUrl, {
        headers: { Accept: "application/json" },
      });
      const data = await response.json();
      return NextResponse.json(data, { status: response.status });
    } catch (error) {
      console.error("Session API function-invocation proxy error:", error);
      return NextResponse.json(
        {
          error: "Failed to connect to Session API",
          details: error instanceof Error ? error.message : String(error),
        },
        { status: 502 },
      );
    }
  },
);
