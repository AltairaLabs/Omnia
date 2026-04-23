/**
 * Institutional memory list/create proxy route.
 *
 * GET  /api/workspaces/{name}/institutional-memory → MEMORY_API_URL/api/v1/institutional/memories?workspace={uid}
 * POST /api/workspaces/{name}/institutional-memory → MEMORY_API_URL/api/v1/institutional/memories
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import { resolveWorkspaceUID } from "../memory/proxy-helpers";
import { resolveServiceURLs } from "@/lib/k8s/service-url-resolver";

export const GET = withWorkspaceAccess(
  "viewer",
  async (
    _request: NextRequest,
    context: WorkspaceRouteContext,
    _access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;
    return proxyList(name, _request);
  }
);

export const POST = withWorkspaceAccess(
  "editor",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext,
    _access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;
    return proxyCreate(name, request);
  }
);

async function proxyList(workspaceName: string, request: NextRequest): Promise<NextResponse> {
  const urls = await resolveServiceURLs(workspaceName);
  if (!urls) {
    return NextResponse.json(
      { error: "Memory API not configured", memories: [], total: 0 },
      { status: 503 }
    );
  }
  const workspaceUID = await resolveWorkspaceUID(workspaceName);
  if (!workspaceUID) {
    return NextResponse.json(
      { error: "Workspace not found", memories: [], total: 0 },
      { status: 404 }
    );
  }

  const params = new URLSearchParams({ workspace: workspaceUID });
  for (const key of ["limit", "offset"]) {
    const v = request.nextUrl.searchParams.get(key);
    if (v) params.set(key, v);
  }

  const baseUrl = urls.memoryURL.endsWith("/") ? urls.memoryURL.slice(0, -1) : urls.memoryURL;
  const targetUrl = `${baseUrl}/api/v1/institutional/memories?${params.toString()}`;

  return fetchAndForward(targetUrl, { method: "GET", headers: { Accept: "application/json" } });
}

async function proxyCreate(workspaceName: string, request: NextRequest): Promise<NextResponse> {
  const urls = await resolveServiceURLs(workspaceName);
  if (!urls) {
    return NextResponse.json({ error: "Memory API not configured" }, { status: 503 });
  }
  const workspaceUID = await resolveWorkspaceUID(workspaceName);
  if (!workspaceUID) {
    return NextResponse.json({ error: "Workspace not found" }, { status: 404 });
  }

  // Parse the incoming body and substitute workspace_id with the UID so
  // callers cannot write into a workspace they don't have access to.
  let body: Record<string, unknown>;
  try {
    body = await request.json();
  } catch {
    return NextResponse.json({ error: "Invalid JSON body" }, { status: 400 });
  }
  body.workspace_id = workspaceUID;

  const baseUrl = urls.memoryURL.endsWith("/") ? urls.memoryURL.slice(0, -1) : urls.memoryURL;
  const targetUrl = `${baseUrl}/api/v1/institutional/memories`;

  return fetchAndForward(targetUrl, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(body),
  });
}

async function fetchAndForward(targetUrl: string, init: RequestInit): Promise<NextResponse> {
  try {
    const response = await fetch(targetUrl, init);
    const text = await response.text();
    try {
      const data = JSON.parse(text);
      return NextResponse.json(data, { status: response.status });
    } catch {
      return NextResponse.json(
        { error: `Memory API returned non-JSON (HTTP ${response.status})` },
        { status: 502 }
      );
    }
  } catch (error) {
    console.error("Institutional memory API proxy error:", error);
    return NextResponse.json(
      { error: "Failed to connect to Memory API" },
      { status: 502 }
    );
  }
}
