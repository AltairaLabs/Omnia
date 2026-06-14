/**
 * Workspace eval-results discover proxy route.
 *
 * GET /api/workspaces/{name}/eval-results/discover
 *   -> SESSION_API_URL/api/v1/eval-results/discover?namespace={name}
 *
 * Returns the distinct (eval_id, eval_type) pairs that exist in this
 * workspace's eval_results table. Replaces dashboard discovery via
 * Prometheus' /api/v1/metadata for product views.
 *
 * Part of the observability split: see CLAUDE.md → Observability Boundaries.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { resolveServiceURLs } from "@/lib/k8s/service-url-resolver";
import { serviceApiHeaders } from "@/lib/auth/session-api-token";
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
        { error: "Session API not configured", evals: [] },
        { status: 503 }
      );
    }

    const baseUrl = urls.sessionURL.endsWith("/")
      ? urls.sessionURL.slice(0, -1)
      : urls.sessionURL;
    // The backend filters by K8s namespace, which is NOT the workspace name
    // (workspace "default" -> namespace "omnia-default"). See #1257.
    const targetUrl = `${baseUrl}/api/v1/eval-results/discover?namespace=${encodeURIComponent(urls.namespace)}`;

    try {
      const response = await fetch(targetUrl, {
        headers: serviceApiHeaders({ Accept: "application/json" }),
      });
      const data = await response.json();
      return NextResponse.json(data, { status: response.status });
    } catch (error) {
      console.error("Session API eval-results discover proxy error:", error);
      return NextResponse.json(
        {
          error: "Failed to connect to Session API",
          details: error instanceof Error ? error.message : String(error),
          evals: [],
        },
        { status: 502 }
      );
    }
  }
);
