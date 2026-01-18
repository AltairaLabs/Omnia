/**
 * API route for listing shared providers.
 *
 * GET /api/shared/providers - List all shared providers
 *
 * Providers are cluster-wide resources that define available LLM providers.
 * Any authenticated user can list shared providers.
 */

import { NextResponse } from "next/server";
import { listSharedCrd } from "@/lib/k8s/crd-operations";
import { requireAuth, serverErrorResponse, SYSTEM_NAMESPACE } from "@/lib/k8s/workspace-route-helpers";
import type { Provider } from "@/lib/data/types";

/**
 * GET /api/shared/providers
 *
 * List all shared providers in the system namespace.
 * Requires authentication (any role).
 */
export async function GET(): Promise<NextResponse> {
  try {
    const auth = await requireAuth();
    if (!auth.ok) return auth.response;

    const providers = await listSharedCrd<Provider>(
      "providers",
      SYSTEM_NAMESPACE
    );

    return NextResponse.json(providers);
  } catch (error) {
    return serverErrorResponse(error, "Failed to list providers");
  }
}
