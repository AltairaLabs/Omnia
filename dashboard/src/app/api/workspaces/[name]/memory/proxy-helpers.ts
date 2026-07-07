/**
 * Shared helpers for memory API proxy routes.
 */

import { NextRequest, NextResponse } from "next/server";
import { getWorkspace } from "@/lib/k8s/workspace-route-helpers";
import { resolveServiceURLs } from "@/lib/k8s/service-url-resolver";
import { serviceApiHeaders } from "@/lib/auth/session-api-token";
import { pseudonymizeId } from "@/lib/identity";
import type { User } from "@/lib/auth/types";
import { resolveScopedUserId } from "@/lib/auth/scoped-user";
import { fetchWithTimeout } from "@/lib/fetch-with-timeout";

/** True when the upstream fetch failed because it exceeded fetchWithTimeout's deadline. */
function isUpstreamTimeout(error: unknown): boolean {
  return error instanceof Error && error.message === "upstream timeout";
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
  if (scopedUserId) params.set("virtual_user_id", pseudonymizeId(scopedUserId));

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

/** Builds the fetch() init for a proxied request, forwarding the body on writes. */
async function buildFetchInit(request: NextRequest): Promise<RequestInit> {
  const isWrite = request.method === "POST" || request.method === "PUT";
  const baseHeaders: Record<string, string> = { Accept: "application/json" };
  if (isWrite) baseHeaders["Content-Type"] = "application/json";

  const fetchInit: RequestInit = {
    method: request.method,
    headers: serviceApiHeaders(baseHeaders),
  };
  if (isWrite) fetchInit.body = await request.text();
  return fetchInit;
}

/** Parses the memory-api response body, falling back gracefully on non-JSON. */
async function parseMemoryApiResponse(response: Response): Promise<NextResponse> {
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
}

/** Maps a failed upstream fetch (including a fetchWithTimeout timeout) to a response. */
function memoryApiErrorResponse(error: unknown): NextResponse {
  console.error("Memory API proxy error:", error);
  const timedOut = isUpstreamTimeout(error);
  return NextResponse.json(
    {
      error: timedOut ? "Memory API timed out" : "Failed to connect to Memory API",
      details: error instanceof Error ? error.message : String(error),
      memories: [],
      total: 0,
    },
    { status: timedOut ? 504 : 502 }
  );
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
    const fetchInit = await buildFetchInit(request);
    const response = await fetchWithTimeout(targetUrl, fetchInit);
    return await parseMemoryApiResponse(response);
  } catch (error) {
    return memoryApiErrorResponse(error);
  }
}
