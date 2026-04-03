/**
 * Memory search proxy route.
 *
 * GET /api/workspaces/{name}/memory/search → MEMORY_API_URL/api/v1/memories/search
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import { proxyToMemoryApi, buildBackendParams, resolveWorkspaceUID } from "../proxy-helpers";

export const GET = withWorkspaceAccess(
  "viewer",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext,
    _access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;

    const workspaceUID = await resolveWorkspaceUID(name);
    if (!workspaceUID) {
      return NextResponse.json({ error: "Workspace not found", memories: [], total: 0 }, { status: 404 });
    }

    const params = buildBackendParams(request.nextUrl.searchParams, workspaceUID);
    const q = request.nextUrl.searchParams.get("query") ?? request.nextUrl.searchParams.get("q");
    if (q) params.set("q", q);
    const minConf = request.nextUrl.searchParams.get("minConfidence");
    if (minConf) params.set("min_confidence", minConf);

    return proxyToMemoryApi(request, name, "/api/v1/memories/search", params);
  }
);
