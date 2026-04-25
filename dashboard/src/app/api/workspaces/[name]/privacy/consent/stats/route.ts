/**
 * Workspace-level consent stats proxy route.
 *
 * GET /api/workspaces/{name}/privacy/consent/stats
 *   → MEMORY_API_URL/api/v1/privacy/consent/stats?workspace={uid}
 *
 * Aggregate-only — does NOT expose individual user consent (that lives at
 * /api/workspaces/{name}/privacy/consent against session-api). The handler
 * itself is mounted on memory-api (registered in cmd/memory-api/main.go
 * inside wrapPrivacyMiddleware), so we reuse the memory proxy helper.
 */

import { NextRequest, NextResponse } from "next/server";
import {
  withWorkspaceAccess,
  type WorkspaceRouteContext,
} from "@/lib/auth/workspace-guard";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import { proxyToMemoryApi } from "../../../memory/proxy-helpers";

export const GET = withWorkspaceAccess(
  "viewer",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext,
    _access: WorkspaceAccess,
    _user: User,
  ): Promise<NextResponse> => {
    const { name } = await context.params;
    return proxyToMemoryApi(
      request,
      name,
      "/api/v1/privacy/consent/stats",
      new URLSearchParams(),
    );
  },
);
