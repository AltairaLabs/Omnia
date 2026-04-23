/**
 * Institutional single-memory proxy route.
 *
 * DELETE /api/workspaces/{name}/institutional-memory/{memoryId}
 *   → MEMORY_API_URL/api/v1/institutional/memories/{memoryId}?workspace={uid}
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import { resolveWorkspaceUID } from "../../memory/proxy-helpers";
import { resolveServiceURLs } from "@/lib/k8s/service-url-resolver";

export const DELETE = withWorkspaceAccess(
  "editor",
  async (
    _request: NextRequest,
    context: WorkspaceRouteContext,
    _access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    const { name, memoryId } = await context.params as { name: string; memoryId: string };

    const urls = await resolveServiceURLs(name);
    if (!urls) {
      return NextResponse.json({ error: "Memory API not configured" }, { status: 503 });
    }
    const workspaceUID = await resolveWorkspaceUID(name);
    if (!workspaceUID) {
      return NextResponse.json({ error: "Workspace not found" }, { status: 404 });
    }

    const baseUrl = urls.memoryURL.endsWith("/") ? urls.memoryURL.slice(0, -1) : urls.memoryURL;
    const targetUrl = `${baseUrl}/api/v1/institutional/memories/${encodeURIComponent(memoryId)}?workspace=${encodeURIComponent(workspaceUID)}`;

    try {
      const response = await fetch(targetUrl, { method: "DELETE" });
      if (!response.ok) {
        const data = await response.json().catch(() => ({ error: "Delete failed" }));
        return NextResponse.json(data, { status: response.status });
      }
      return new NextResponse(null, { status: 200 });
    } catch (error) {
      console.error("Institutional memory proxy error:", error);
      return NextResponse.json({ error: "Failed to connect to Memory API" }, { status: 502 });
    }
  }
);
