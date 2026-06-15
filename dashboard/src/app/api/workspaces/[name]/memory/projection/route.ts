/**
 * Memory projection proxy route.
 *
 * GET /api/workspaces/{name}/memory/projection
 * Proxies to memory-api GET /api/v1/memories/projection (the workspace-wide
 * Memory Galaxy layout). The backend pre-render worker keeps the layout warm;
 * large cold scopes come back as { status: "pending" } until it's rendered.
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
    user: User,
  ): Promise<NextResponse> => {
    const { name } = await context.params;
    // Workspace-wide operator view: pass empty params so the proxy sends only
    // ?workspace=<uid>. The default buildBackendParams would add the caller's
    // user_id, which the backend's buildScope would use to narrow the galaxy to
    // that single user — wrong for this institution-wide projection.
    return proxyToMemoryApi(request, name, "/api/v1/memories/projection", user, new URLSearchParams());
  },
);
