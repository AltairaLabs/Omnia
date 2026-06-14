/**
 * Workspace-level privacy enforcement stats proxy route.
 *
 * GET /api/workspaces/{name}/privacy/enforcement-stats
 *   → MEMORY_API_URL/api/v1/privacy/enforcement-stats?workspace={uid}
 *
 * Aggregate-only — returns workspace-scoped counts of opt-out write blocks
 * (piiBlocked) and PII redactions (redactions). The handler itself is mounted
 * on memory-api (registered in cmd/memory-api/main.go inside
 * wrapPrivacyMiddleware), so we reuse the memory proxy helper.
 */

import { NextRequest, NextResponse } from "next/server";
import {
  withWorkspaceAccess,
  type WorkspaceRouteContext,
} from "@/lib/auth/workspace-guard";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import { proxyToMemoryApi } from "../../memory/proxy-helpers";

export const GET = withWorkspaceAccess(
  "viewer",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext,
    _access: WorkspaceAccess,
    user: User,
  ): Promise<NextResponse> => {
    const { name } = await context.params;
    return proxyToMemoryApi(
      request,
      name,
      "/api/v1/privacy/enforcement-stats",
      user,
      new URLSearchParams(),
    );
  },
);
