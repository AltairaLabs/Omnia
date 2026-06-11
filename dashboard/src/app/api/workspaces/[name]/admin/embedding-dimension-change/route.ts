/**
 * Admin proxy route: record one-shot consent to change the memory embedding
 * vector dimension (#1309).
 *
 * POST /api/workspaces/{name}/admin/embedding-dimension-change
 *   → MEMORY_API_URL/admin/embedding-dimension-change   body {"target_dim": N}
 *
 * Owner-only: a dimension change discards existing embeddings and triggers a
 * full re-embed on the next memory-api restart.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import { proxyToMemoryApi } from "../../memory/proxy-helpers";

export const POST = withWorkspaceAccess(
  "owner",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext,
    _access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;
    return proxyToMemoryApi(request, name, "/admin/embedding-dimension-change", user);
  }
);
