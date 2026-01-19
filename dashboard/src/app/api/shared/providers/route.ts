/**
 * API route for listing shared providers.
 *
 * GET /api/shared/providers - List all shared providers
 *
 * Providers are cluster-wide resources that define available LLM providers.
 * Accessible to all users (including anonymous) - these are read-only
 * configuration resources.
 */

import { NextResponse } from "next/server";
import { listSharedCrd } from "@/lib/k8s/crd-operations";
import { serverErrorResponse, SYSTEM_NAMESPACE } from "@/lib/k8s/workspace-route-helpers";
import type { Provider } from "@/lib/data/types";

/**
 * GET /api/shared/providers
 *
 * List all shared providers in the system namespace.
 * No authentication required - read-only configuration data.
 */
export async function GET(): Promise<NextResponse> {
  try {
    const providers = await listSharedCrd<Provider>(
      "providers",
      SYSTEM_NAMESPACE
    );

    return NextResponse.json(providers);
  } catch (error) {
    return serverErrorResponse(error, "Failed to list providers");
  }
}
