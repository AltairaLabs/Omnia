/**
 * Memory projection proxy route.
 *
 * GET /api/workspaces/{name}/memory/projection
 * TODO(#1418): replace the mock with
 *   proxyToMemoryApi(request, name, "/api/v1/memories/projection", user, params)
 * once the backend endpoint exists. The response contract is identical.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import { resolveWorkspaceUID } from "../proxy-helpers";
import { generateMockProjection } from "@/lib/memory-galaxy/mock-projection";

const MOCK_COUNT = 600;

export const GET = withWorkspaceAccess(
  "viewer",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext,
    _access: WorkspaceAccess,
    _user: User,
  ): Promise<NextResponse> => {
    const { name } = await context.params;
    const workspaceUID = await resolveWorkspaceUID(name);
    if (!workspaceUID) {
      return NextResponse.json({ error: "Workspace not found" }, { status: 404 });
    }
    const seed = Array.from(workspaceUID).reduce((a, c) => (a + c.charCodeAt(0)) | 0, 0);
    const body = generateMockProjection({ seed: Math.abs(seed) || 1, count: MOCK_COUNT });
    return NextResponse.json(body);
  },
);
