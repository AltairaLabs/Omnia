/**
 * API routes for workspace-scoped providers.
 *
 * GET /api/workspaces/:name/providers - List providers in workspace
 * POST /api/workspaces/:name/providers - Create a new provider
 *
 * Providers can be workspace-scoped (in workspace namespace) or
 * shared (in omnia-system namespace). This endpoint returns workspace-scoped ones.
 */

import { createCollectionRoutes } from "@/lib/api/crd-route-factory";
import { CRD_PROVIDERS } from "@/lib/k8s/workspace-route-helpers";
import type { Provider } from "@/lib/data/types";

export const { GET, POST } = createCollectionRoutes<Provider>({
  kind: "Provider",
  plural: CRD_PROVIDERS,
  errorLabel: "providers",
});
