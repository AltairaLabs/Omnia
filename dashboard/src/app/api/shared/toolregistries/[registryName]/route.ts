/**
 * API route for getting a specific shared tool registry.
 *
 * GET /api/shared/toolregistries/:registryName - Get tool registry details
 *
 * Tool registries are cluster-wide resources that define available tools.
 * Any authenticated user can view shared tool registries.
 */

import { NextRequest, NextResponse } from "next/server";
import { getSharedCrd } from "@/lib/k8s/crd-operations";
import { requireAuth, notFoundResponse, serverErrorResponse, SYSTEM_NAMESPACE } from "@/lib/k8s/workspace-route-helpers";
import type { ToolRegistry } from "@/lib/data/types";

interface RouteContext {
  params: Promise<{ registryName: string }>;
}

/**
 * GET /api/shared/toolregistries/:registryName
 *
 * Get a specific shared tool registry by name.
 * Requires authentication (any role).
 */
export async function GET(
  _request: NextRequest,
  context: RouteContext
): Promise<NextResponse> {
  try {
    const auth = await requireAuth();
    if (!auth.ok) return auth.response;

    const { registryName } = await context.params;

    const toolRegistry = await getSharedCrd<ToolRegistry>(
      "toolregistries",
      SYSTEM_NAMESPACE,
      registryName
    );

    if (!toolRegistry) {
      return notFoundResponse(`Tool registry not found: ${registryName}`);
    }

    return NextResponse.json(toolRegistry);
  } catch (error) {
    return serverErrorResponse(error, "Failed to get tool registry");
  }
}
