/**
 * Shared helpers for memory API proxy routes.
 */

import { NextRequest, NextResponse } from "next/server";
import { getWorkspace } from "@/lib/k8s/workspace-route-helpers";

const MEMORY_API_URL = process.env.MEMORY_API_URL;

/** Resolve workspace name to UID for memory-api scoping. */
export async function resolveWorkspaceUID(name: string): Promise<string | null> {
  const workspace = await getWorkspace(name);
  if (!workspace) return null;
  // K8s API always returns metadata.uid even though our TS type doesn't declare it
  const meta = workspace.metadata as unknown as Record<string, unknown>;
  return (meta?.uid as string) ?? null;
}

/** Map dashboard query param names to backend param names. */
export function buildBackendParams(
  searchParams: URLSearchParams,
  workspaceUID: string
): URLSearchParams {
  const params = new URLSearchParams();
  params.set("workspace", workspaceUID);

  const userId = searchParams.get("userId");
  if (userId) params.set("user_id", userId);

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
  extraParams?: URLSearchParams
): Promise<NextResponse> {
  if (!MEMORY_API_URL) {
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

  const params = extraParams ?? buildBackendParams(request.nextUrl.searchParams, workspaceUID);
  if (!params.has("workspace")) {
    params.set("workspace", workspaceUID);
  }

  const baseUrl = MEMORY_API_URL.endsWith("/")
    ? MEMORY_API_URL.slice(0, -1)
    : MEMORY_API_URL;
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
