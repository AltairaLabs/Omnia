/**
 * Memory list/create/delete-all proxy route.
 *
 * GET    /api/workspaces/{name}/memory → MEMORY_API_URL/api/v1/memories?workspace={uid}&...
 * DELETE /api/workspaces/{name}/memory → MEMORY_API_URL/api/v1/memories?workspace={uid}&...
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import { proxyToMemoryApi } from "./proxy-helpers";

export const GET = withWorkspaceAccess(
  "viewer",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext,
    _access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;
    return proxyToMemoryApi(request, name, "/api/v1/memories");
  }
);

export const DELETE = withWorkspaceAccess(
  "viewer",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext,
    _access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;
    return proxyToMemoryApi(request, name, "/api/v1/memories");
  }
);
