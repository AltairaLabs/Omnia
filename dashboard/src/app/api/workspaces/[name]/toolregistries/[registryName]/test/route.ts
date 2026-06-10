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
import { readFile } from "node:fs/promises";
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

// The operator's tool-test API authenticates the dashboard via TokenReview
// (#1303). Forward this pod's ServiceAccount token as a bearer credential.
// Projected SA tokens rotate in place, so read fresh per request rather than
// caching. Falls back to an explicit token env or none (local dev).
const SA_TOKEN_PATH =
  process.env.SA_TOKEN_PATH || "/var/run/secrets/kubernetes.io/serviceaccount/token";

async function operatorAuthToken(): Promise<string | null> {
  if (process.env.OPERATOR_TOOL_TEST_TOKEN) {
    return process.env.OPERATOR_TOOL_TEST_TOKEN;
  }
  try {
    return (await readFile(SA_TOKEN_PATH, "utf-8")).trim();
  } catch {
    return null; // not running in-cluster (local dev) — send no auth
  }
}

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

      const headers: Record<string, string> = { "Content-Type": "application/json" };
      const token = await operatorAuthToken();
      if (token) {
        headers.Authorization = `Bearer ${token}`;
      }

      let response: Response;
      try {
        response = await fetch(
          `${OPERATOR_TOOL_TEST_URL}/api/v1/namespaces/${namespace}/toolregistries/${registryName}/test`,
          {
            method: "POST",
            headers,
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
