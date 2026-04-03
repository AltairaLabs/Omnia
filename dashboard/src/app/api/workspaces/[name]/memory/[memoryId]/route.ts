/**
 * Single memory proxy route.
 *
 * DELETE /api/workspaces/{name}/memory/{memoryId} → MEMORY_API_URL/api/v1/memories/{memoryId}
 */

import { NextRequest, NextResponse } from "next/server";
import { resolveWorkspaceUID } from "../proxy-helpers";

const MEMORY_API_URL = process.env.MEMORY_API_URL;

export async function DELETE(
  request: NextRequest,
  { params }: { params: Promise<{ name: string; memoryId: string }> }
): Promise<NextResponse> {
    const { name, memoryId } = await params;

    if (!MEMORY_API_URL) {
      return NextResponse.json({ error: "Memory API not configured" }, { status: 503 });
    }

    const workspaceUID = await resolveWorkspaceUID(name);
    if (!workspaceUID) {
      return NextResponse.json({ error: "Workspace not found" }, { status: 404 });
    }

    const baseUrl = MEMORY_API_URL.endsWith("/") ? MEMORY_API_URL.slice(0, -1) : MEMORY_API_URL;
    const targetUrl = `${baseUrl}/api/v1/memories/${encodeURIComponent(memoryId)}?workspace=${encodeURIComponent(workspaceUID)}`;

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
