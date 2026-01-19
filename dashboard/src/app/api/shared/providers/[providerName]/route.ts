/**
 * API route for getting a specific shared provider.
 *
 * GET /api/shared/providers/:providerName - Get provider details
 *
 * Providers are cluster-wide resources that define available LLM providers.
 * Accessible to all users (including anonymous) - these are read-only
 * configuration resources.
 */

import { NextRequest, NextResponse } from "next/server";
import { getSharedCrd } from "@/lib/k8s/crd-operations";
import { notFoundResponse, serverErrorResponse, SYSTEM_NAMESPACE } from "@/lib/k8s/workspace-route-helpers";
import type { Provider } from "@/lib/data/types";

interface RouteContext {
  params: Promise<{ providerName: string }>;
}

/**
 * GET /api/shared/providers/:providerName
 *
 * Get a specific shared provider by name.
 * No authentication required - read-only configuration data.
 */
export async function GET(
  _request: NextRequest,
  context: RouteContext
): Promise<NextResponse> {
  try {
    const { providerName } = await context.params;

    const provider = await getSharedCrd<Provider>(
      "providers",
      SYSTEM_NAMESPACE,
      providerName
    );

    if (!provider) {
      return notFoundResponse(`Provider not found: ${providerName}`);
    }

    return NextResponse.json(provider);
  } catch (error) {
    return serverErrorResponse(error, "Failed to get provider");
  }
}
