/**
 * API route for listing shared tool registries.
 *
 * GET /api/shared/toolregistries - List all shared tool registries
 *
 * Tool registries are cluster-wide resources that define available tools.
 * Any authenticated user can list shared tool registries.
 */

import { NextResponse } from "next/server";
import { listSharedCrd } from "@/lib/k8s/crd-operations";
import { requireAuth, serverErrorResponse, SYSTEM_NAMESPACE } from "@/lib/k8s/workspace-route-helpers";
import type { ToolRegistry } from "@/lib/data/types";

/**
 * GET /api/shared/toolregistries
 *
 * List all shared tool registries in the system namespace.
 * Requires authentication (any role).
 */
export async function GET(): Promise<NextResponse> {
  try {
    const auth = await requireAuth();
    if (!auth.ok) return auth.response;

    const toolRegistries = await listSharedCrd<ToolRegistry>(
      "toolregistries",
      SYSTEM_NAMESPACE
    );

    return NextResponse.json(toolRegistries);
  } catch (error) {
    return serverErrorResponse(error, "Failed to list tool registries");
  }
}
