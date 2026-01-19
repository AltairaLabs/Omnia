/**
 * API route for listing shared tool registries.
 *
 * GET /api/shared/toolregistries - List all shared tool registries
 *
 * Tool registries are cluster-wide resources that define available tools.
 * Accessible to all users (including anonymous) - these are read-only
 * configuration resources.
 */

import { NextResponse } from "next/server";
import { listSharedCrd } from "@/lib/k8s/crd-operations";
import { serverErrorResponse, SYSTEM_NAMESPACE } from "@/lib/k8s/workspace-route-helpers";
import type { ToolRegistry } from "@/lib/data/types";

/**
 * GET /api/shared/toolregistries
 *
 * List all shared tool registries in the system namespace.
 * No authentication required - read-only configuration data.
 */
export async function GET(): Promise<NextResponse> {
  try {
    const toolRegistries = await listSharedCrd<ToolRegistry>(
      "toolregistries",
      SYSTEM_NAMESPACE
    );

    return NextResponse.json(toolRegistries);
  } catch (error) {
    return serverErrorResponse(error, "Failed to list tool registries");
  }
}
