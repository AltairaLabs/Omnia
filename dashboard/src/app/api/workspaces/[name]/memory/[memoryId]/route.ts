/**
 * Single memory proxy route.
 *
 * DELETE /api/workspaces/{name}/memory/{memoryId} → MEMORY_API_URL/api/v1/memories/{memoryId}
 *
 * Scoped to the caller's own pseudonymous identity (#1268): memory-api only
 * forgets a row owned by the caller's virtual_user_id, so a workspace member
 * can't delete another user's memory by knowing its UUID. Mirrors the
 * user_id derivation of the read / delete-all paths.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import { resolveWorkspaceUID } from "../proxy-helpers";
import { resolveServiceURLs } from "@/lib/k8s/service-url-resolver";
import { resolveScopedUserId } from "@/lib/auth/scoped-user";
import { pseudonymizeId } from "@/lib/identity";

export const DELETE = withWorkspaceAccess<{ name: string; memoryId: string }>(
  "viewer",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext<{ name: string; memoryId: string }>,
    _access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, memoryId } = await context.params;

    const urls = await resolveServiceURLs(name);
    if (!urls) {
      return NextResponse.json({ error: "Memory API not configured" }, { status: 503 });
    }

    const workspaceUID = await resolveWorkspaceUID(name);
    if (!workspaceUID) {
      return NextResponse.json({ error: "Workspace not found" }, { status: 404 });
    }

    const params = new URLSearchParams();
    params.set("workspace", workspaceUID);
    // Scope the delete to the caller's own pseudonym so a member can't forget
    // another user's memory by its UUID (#1268). Same derivation as the read /
    // delete-all paths: session identity for authenticated users, the device
    // pseudonym for anonymous.
    const scopedUserId = resolveScopedUserId(request.nextUrl.searchParams, user);
    if (scopedUserId) params.set("user_id", pseudonymizeId(scopedUserId));

    const baseUrl = urls.memoryURL.endsWith("/") ? urls.memoryURL.slice(0, -1) : urls.memoryURL;
    const targetUrl = `${baseUrl}/api/v1/memories/${encodeURIComponent(memoryId)}?${params.toString()}`;

    try {
      const response = await fetch(targetUrl, { method: "DELETE" });
      if (!response.ok) {
        const data = await response.json().catch(() => ({ error: "Delete failed" }));
        return NextResponse.json(data, { status: response.status });
      }
      return new NextResponse(null, { status: 200 });
    } catch (error) {
      console.error("Memory API proxy error:", error);
      return NextResponse.json(
        { error: "Failed to connect to Memory API" },
        { status: 502 }
      );
    }
  }
);
