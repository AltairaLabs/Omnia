/**
 * API route for testing tool calls against a ToolRegistry.
 *
 * POST /api/workspaces/:name/toolregistries/:registryName/test
 *   - Proxies to the operator's tool test API server
 *   - Uses PromptKit executors for real tool execution
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess } from "@/lib/auth/workspace-guard";
import {
  validateWorkspace,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";

const SERVICE_DOMAIN = process.env.SERVICE_DOMAIN || "svc.cluster.local";
const OPERATOR_TOOL_TEST_URL =
  process.env.OPERATOR_TOOL_TEST_URL ||
  `http://omnia-operator.omnia-system.${SERVICE_DOMAIN}:8083`;

interface RouteParams {
  params: Promise<{ name: string; registryName: string }>;
}

export const POST = withWorkspaceAccess<{
  name: string;
  registryName: string;
}>(
  "editor",
  async (
    request: NextRequest,
    context: RouteParams,
    access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    const { name, registryName } = await context.params;

    try {
      const result = await validateWorkspace(name, access.role!);
      if (!result.ok) return result.response;

      const namespace = result.workspace.spec.namespace.name;
      const rawBody = await request.text();
      let body: unknown;
      try {
        body = JSON.parse(rawBody);
      } catch (parseErr) {
        return NextResponse.json(
          {
            success: false,
            error: `Invalid request body: ${parseErr instanceof Error ? parseErr.message : "invalid JSON"}`,
            durationMs: 0,
            handlerType: "unknown",
          },
          { status: 400 }
        );
      }

      let response: Response;
      try {
        response = await fetch(
          `${OPERATOR_TOOL_TEST_URL}/api/v1/namespaces/${namespace}/toolregistries/${registryName}/test`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(body),
          }
        );
      } catch (fetchError) {
        // Operator API server unreachable
        const message = fetchError instanceof Error ? fetchError.message : "Unknown error";
        return NextResponse.json(
          {
            success: false,
            error: `Tool test API server unreachable (${OPERATOR_TOOL_TEST_URL}): ${message}. Ensure the operator is started with --api-bind-address.`,
            durationMs: 0,
            handlerType: "unknown",
          },
          { status: 200 }
        );
      }

      const data = await response.json();
      return NextResponse.json(data, { status: response.status });
    } catch (error) {
      const message = error instanceof Error ? error.message : "Internal server error";
      return NextResponse.json(
        {
          success: false,
          error: message,
          durationMs: 0,
          handlerType: "unknown",
        },
        { status: 500 }
      );
    }
  }
);
