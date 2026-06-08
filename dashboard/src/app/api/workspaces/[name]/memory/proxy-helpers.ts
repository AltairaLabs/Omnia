/**
 * Shared helpers for memory API proxy routes.
 */

import { NextRequest, NextResponse } from "next/server";
import { getWorkspace } from "@/lib/k8s/workspace-route-helpers";
import { resolveServiceURLs } from "@/lib/k8s/service-url-resolver";
import { pseudonymizeId } from "@/lib/identity";
import type { User } from "@/lib/auth/types";

/**
 * Resolve the memory-scoping user id for a request, authoritatively.
 *
 * For an authenticated user the scope is ALWAYS their session identity
 * (`user.id`) — never a client-supplied `?userId` — so a workspace viewer
 * cannot read, export, or delete another user's memories by passing someone
 * else's id (#1263). Anonymous users have no session identity, so their
 * device id (sent as `userId`) is the only available scope, matching the
 * write path's device scoping.
 */
function resolveScopedUserId(searchParams: URLSearchParams, user: User): string | null {
  if (user.provider === "anonymous") {
    return searchParams.get("userId");
  }
  const clientUserId = searchParams.get("userId");
  if (clientUserId && clientUserId !== user.id) {
    console.warn(
      "[memory proxy] ignoring client-supplied userId; scoping to authenticated session user"
    );
  }
  return user.id;
}

/** Resolve workspace name to UID for memory-api scoping. */
export async function resolveWorkspaceUID(name: string): Promise<string | null> {
  const workspace = await getWorkspace(name);
  if (!workspace) {
    console.warn("resolveWorkspaceUID: workspace not found", { name });
    return null;
  }
  const uid = workspace.metadata?.uid ?? null;
  if (!uid) {
    console.warn("resolveWorkspaceUID: workspace has no UID", { name, metadata: workspace.metadata });
  }
  return uid;
}

/** Map dashboard query param names to backend param names. */
export function buildBackendParams(
  searchParams: URLSearchParams,
  workspaceUID: string,
  user: User
): URLSearchParams {
  const params = new URLSearchParams();
  params.set("workspace", workspaceUID);

  const scopedUserId = resolveScopedUserId(searchParams, user);
  if (scopedUserId) params.set("user_id", pseudonymizeId(scopedUserId));

  // "Visible to me" mode — institutional + agent + the user's own.
  if (searchParams.get("includeShared") === "true") {
    params.set("include_shared", "true");
  }

  for (const key of ["type", "limit", "offset"]) {
    const value = searchParams.get(key);
    if (value) params.set(key, value);
  }

  return params;
}

/** Proxy a request to the memory-api backend. */
export async function proxyToMemoryApi(
  request: NextRequest,
  workspaceName: string,
  backendPath: string,
  user: User,
  extraParams?: URLSearchParams
): Promise<NextResponse> {
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

  const params = extraParams ?? buildBackendParams(request.nextUrl.searchParams, workspaceUID, user);
  if (!params.has("workspace")) {
    params.set("workspace", workspaceUID);
  }

  const baseUrl = urls.memoryURL.endsWith("/")
    ? urls.memoryURL.slice(0, -1)
    : urls.memoryURL;
  const targetUrl = `${baseUrl}${backendPath}?${params.toString()}`;

  try {
    const fetchInit: RequestInit = {
      method: request.method,
      headers: { Accept: "application/json" },
    };

    if (request.method === "POST" || request.method === "PUT") {
      fetchInit.body = await request.text();
      fetchInit.headers = {
        ...fetchInit.headers,
        "Content-Type": "application/json",
      };
    }

    const response = await fetch(targetUrl, fetchInit);
    const text = await response.text();
    try {
      const data = JSON.parse(text);
      return NextResponse.json(data, { status: response.status });
    } catch {
      if (response.status === 404) {
        return NextResponse.json({ memories: [], total: 0 }, { status: 200 });
      }
      return NextResponse.json(
        { error: `Memory API returned non-JSON (HTTP ${response.status})`, memories: [], total: 0 },
        { status: 502 }
      );
    }
  } catch (error) {
    console.error("Memory API proxy error:", error);
    return NextResponse.json(
      {
        error: "Failed to connect to Memory API",
        details: error instanceof Error ? error.message : String(error),
        memories: [],
        total: 0,
      },
      { status: 502 }
    );
  }
}
