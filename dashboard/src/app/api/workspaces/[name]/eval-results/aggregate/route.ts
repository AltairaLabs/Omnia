/**
 * Workspace eval-results aggregate proxy route.
 *
 * GET /api/workspaces/{name}/eval-results/aggregate
 *   -> SESSION_API_URL/api/v1/eval-results/aggregate?namespace={name}&...
 *
 * Forwards `groupBy`, `metric`, optional filters (`agentName`, `evalId`,
 * `evalType`), and time bounds (`from`, `to`) to session-api. The workspace
 * `name` from the URL is injected as the `namespace` query param so the
 * caller cannot accidentally read another workspace's eval data.
 *
 * Part of the observability split: see CLAUDE.md → Observability Boundaries.
 * Replaces direct Prometheus reads for product-class dashboard views.
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

    // Pin namespace to the URL-scoped workspace so a caller can't read
    // another workspace's data by setting ?namespace=other.
    const params = new URLSearchParams(request.nextUrl.searchParams);
    params.set("namespace", name);
    const targetUrl = `${baseUrl}/api/v1/eval-results/aggregate?${params.toString()}`;

    try {
      const response = await fetch(targetUrl, {
        headers: { Accept: "application/json" },
      });
      const data = await response.json();
      return NextResponse.json(data, { status: response.status });
    } catch (error) {
      console.error("Session API eval-results aggregate proxy error:", error);
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
