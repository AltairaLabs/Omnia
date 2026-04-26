/**
 * Agent-tier memory list proxy route.
 *
 * GET /api/workspaces/{name}/agent-memories?agent={agentId}&type=&limit=&offset=
 *   → MEMORY_API_URL/api/v1/agent-memories?workspace={uid}&agent={agentId}&...
 *
 * Returns workspace+agent-scoped memory rows (no user_id), each carrying a
 * derived `tier: "agent"` field. Used by the agent detail page's Memory tab.
 */

import { NextRequest, NextResponse } from "next/server";
import {
  withWorkspaceAccess,
  type WorkspaceRouteContext,
} from "@/lib/auth/workspace-guard";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import { proxyToMemoryApi } from "../memory/proxy-helpers";

export const GET = withWorkspaceAccess(
  "viewer",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext,
    _access: WorkspaceAccess,
    _user: User,
  ): Promise<NextResponse> => {
    const { name } = await context.params;
    const search = request.nextUrl.searchParams;

    const agent = search.get("agent") ?? "";
    if (!agent) {
      return NextResponse.json(
        { error: "agent query param is required" },
        { status: 400 },
      );
    }

    const params = new URLSearchParams();
    params.set("agent", agent);
    for (const key of ["type", "limit", "offset"] as const) {
      const v = search.get(key);
      if (v) params.set(key, v);
    }

    return proxyToMemoryApi(request, name, "/api/v1/agent-memories", params);
  },
);
