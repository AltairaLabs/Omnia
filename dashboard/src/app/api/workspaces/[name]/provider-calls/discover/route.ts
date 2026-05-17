/**
 * Workspace provider-calls discover proxy route.
 *
 * GET /api/workspaces/{name}/provider-calls/discover
 *   -> SESSION_API_URL/api/v1/provider-calls/discover?namespace={name}
 *
 * Returns the distinct (provider, model) pairs that exist in this
 * workspace's provider_calls rows. Replaces Prometheus label-value
 * discovery for provider/model dropdowns.
 *
 * Part of the observability split: see CLAUDE.md → Observability Boundaries.
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
    _request: NextRequest,
    context: WorkspaceRouteContext<Params>,
    _access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;

    const urls = await resolveServiceURLs(name);
    if (!urls) {
      return NextResponse.json(
        { error: "Session API not configured", providers: [], models: [] },
        { status: 503 }
      );
    }

    const baseUrl = urls.sessionURL.endsWith("/")
      ? urls.sessionURL.slice(0, -1)
      : urls.sessionURL;
    const targetUrl = `${baseUrl}/api/v1/provider-calls/discover?namespace=${encodeURIComponent(name)}`;

    try {
      const response = await fetch(targetUrl, {
        headers: { Accept: "application/json" },
      });
      const data = await response.json();
      return NextResponse.json(data, { status: response.status });
    } catch (error) {
      console.error("Session API provider-calls discover proxy error:", error);
      return NextResponse.json(
        {
          error: "Failed to connect to Session API",
          details: error instanceof Error ? error.message : String(error),
          providers: [],
          models: [],
        },
        { status: 502 }
      );
    }
  }
);
