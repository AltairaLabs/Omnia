/**
 * API routes for a specific workspace-scoped provider.
 *
 * GET /api/workspaces/:name/providers/:providerName - Get provider details
 * PUT /api/workspaces/:name/providers/:providerName - Update provider
 *
 * Protected by workspace access checks.
 */

import { createItemRoutes } from "@/lib/api/crd-route-factory";
import { CRD_PROVIDERS } from "@/lib/k8s/workspace-route-helpers";
import type { Provider } from "@/lib/data/types";

const routes = createItemRoutes<Provider>({
  kind: "Provider",
  plural: CRD_PROVIDERS,
  resourceLabel: "Provider",
  paramKey: "providerName",
  errorLabel: "provider",
});

export const GET = routes.GET;
export const PUT = routes.PUT;
