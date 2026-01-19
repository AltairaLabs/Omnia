/**
 * API route for getting a specific shared tool registry.
 *
 * GET /api/shared/toolregistries/:registryName - Get tool registry details
 *
 * Tool registries are cluster-wide resources that define available tools.
 * Accessible to all users (including anonymous) - these are read-only
 * configuration resources.
 */

import { NextRequest, NextResponse } from "next/server";
import { getSharedCrd } from "@/lib/k8s/crd-operations";
import { notFoundResponse, serverErrorResponse, SYSTEM_NAMESPACE } from "@/lib/k8s/workspace-route-helpers";
import type { ToolRegistry } from "@/lib/data/types";

interface RouteContext {
  params: Promise<{ registryName: string }>;
}

/**
 * GET /api/shared/toolregistries/:registryName
 *
 * Get a specific shared tool registry by name.
 * No authentication required - read-only configuration data.
 */
export async function GET(
  _request: NextRequest,
  context: RouteContext
): Promise<NextResponse> {
  try {
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
