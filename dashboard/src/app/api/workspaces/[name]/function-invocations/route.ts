/**
 * Workspace function-invocations list proxy route.
 *
 * GET /api/workspaces/{name}/function-invocations[?function=&from=&to=&limit=]
 *   -> SESSION_API_URL/api/v1/function-invocations?namespace={name}[&...]
 *
 * Returns the per-call audit rows persisted by function-mode
 * AgentRuntimes (Functions Phase 1, #1102 / #1103 PR 5). The workspace
 * name pins the session-api namespace filter, so cross-workspace reads
 * are impossible at the proxy layer.
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

/** Query-string params we forward to session-api. We deliberately
 * re-build the URL rather than passing the caller's raw query string
 * so a malicious caller can't override `namespace` to read another
 * tenant's rows. */
const FORWARDED_PARAMS = ["function", "from", "to", "limit"] as const;

export const GET = withWorkspaceAccess<Params>(
  "viewer",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext<Params>,
    _access: WorkspaceAccess,
    _user: User,
  ): Promise<NextResponse> => {
    const { name } = await context.params;

    const urls = await resolveServiceURLs(name);
    if (!urls) {
      return NextResponse.json(
        { error: "Session API not configured", rows: [] },
        { status: 503 },
      );
    }

    const incoming = request.nextUrl.searchParams;
    const qs = new URLSearchParams();
    qs.set("namespace", name);
    for (const key of FORWARDED_PARAMS) {
      const v = incoming.get(key);
      if (v) qs.set(key, v);
    }

    const baseUrl = urls.sessionURL.endsWith("/")
      ? urls.sessionURL.slice(0, -1)
      : urls.sessionURL;
    const targetUrl = `${baseUrl}/api/v1/function-invocations?${qs}`;

    try {
      const response = await fetch(targetUrl, {
        headers: { Accept: "application/json" },
      });
      const data = await response.json();
      return NextResponse.json(data, { status: response.status });
    } catch (error) {
      console.error("Session API function-invocations list proxy error:", error);
      return NextResponse.json(
        {
          error: "Failed to connect to Session API",
          details: error instanceof Error ? error.message : String(error),
          rows: [],
        },
        { status: 502 },
      );
    }
  },
);
