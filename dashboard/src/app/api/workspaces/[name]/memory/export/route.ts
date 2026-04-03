/**
 * Memory export proxy route (DSAR).
 *
 * GET /api/workspaces/{name}/memory/export → MEMORY_API_URL/api/v1/memories/export
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import { proxyToMemoryApi } from "../proxy-helpers";

export const GET = withWorkspaceAccess(
  "viewer",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext,
    _access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;
    return proxyToMemoryApi(request, name, "/api/v1/memories/export");
  }
);
