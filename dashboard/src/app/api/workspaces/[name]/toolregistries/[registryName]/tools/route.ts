/**
 * API route for fetching the live OpenAPI tool list from a ToolRegistry.
 *
 * GET /api/workspaces/:name/toolregistries/:registryName/tools?handler=<name>
 *   - Proxies to the operator's tool preview API
 *   - Returns { tools: [{name, description, inputSchema}], specURL, error }
 *   - HTTP 200 on success; 200 with error set on discovery failure (422 from operator)
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess } from "@/lib/auth/workspace-guard";
import { validateWorkspace } from "@/lib/k8s/workspace-route-helpers";
import { OPERATOR_TOOL_TEST_URL, operatorAuthToken } from "@/lib/tooltest/operator-client";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";

interface RouteParams {
  params: Promise<{ name: string; registryName: string }>;
}

export const GET = withWorkspaceAccess<{ name: string; registryName: string }>(
  "editor",
  async (
    request: NextRequest,
    context: RouteParams,
    access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    const { name, registryName } = await context.params;
    const handler = new URL(request.url).searchParams.get("handler") || "";

    const result = await validateWorkspace(name, access.role!);
    if (!result.ok) return result.response;
    const namespace = result.workspace.spec.namespace.name;

    const headers: Record<string, string> = {};
    const token = await operatorAuthToken();
    if (token) headers.Authorization = `Bearer ${token}`;

    const target = `${OPERATOR_TOOL_TEST_URL}/api/v1/namespaces/${namespace}/toolregistries/${registryName}/tools?handler=${encodeURIComponent(handler)}`;

    let response: Response;
    try {
      response = await fetch(target, { method: "GET", headers });
    } catch (fetchError) {
      const message = fetchError instanceof Error ? fetchError.message : "Unknown error";
      return NextResponse.json(
        { tools: [], error: `Tool preview API unreachable (${OPERATOR_TOOL_TEST_URL}): ${message}` },
        { status: 200 }
      );
    }

    const data = await response.json();
    return NextResponse.json(data, { status: response.status });
  }
);
