/**
 * Workspace provider-calls aggregate proxy route.
 *
 * GET /api/workspaces/{name}/provider-calls/aggregate
 *   -> SESSION_API_URL/api/v1/provider-calls/aggregate?namespace={name}&...
 *
 * Forwards `groupBy`, `metric`, optional filters (`agentName`, `provider`,
 * `model`), and time bounds (`from`, `to`) to session-api. The workspace
 * `name` from the URL is injected as the `namespace` query param so callers
 * cannot read another workspace's data.
 *
 * Part of the observability split: see CLAUDE.md → Observability Boundaries.
 * Powers the cost/usage dashboard views without Prometheus.
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
        { error: "Session API not configured", rows: [] },
        { status: 503 }
      );
    }

    const baseUrl = urls.sessionURL.endsWith("/")
      ? urls.sessionURL.slice(0, -1)
      : urls.sessionURL;

    // Pin to the workspace's real backing namespace (not its name, #1257).
    const params = new URLSearchParams(request.nextUrl.searchParams);
    params.set("namespace", urls.namespace);
    const targetUrl = `${baseUrl}/api/v1/provider-calls/aggregate?${params.toString()}`;

    try {
      const response = await fetch(targetUrl, {
        headers: { Accept: "application/json" },
      });
      const data = await response.json();
      return NextResponse.json(data, { status: response.status });
    } catch (error) {
      console.error("Session API provider-calls aggregate proxy error:", error);
      return NextResponse.json(
        {
          error: "Failed to connect to Session API",
          details: error instanceof Error ? error.message : String(error),
          rows: [],
        },
        { status: 502 }
      );
    }
  }
);
