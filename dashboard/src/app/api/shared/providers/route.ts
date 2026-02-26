/**
 * API route for listing shared providers.
 *
 * GET /api/shared/providers - List all shared providers
 *
 * Providers are cluster-wide resources that define available LLM providers.
 * Accessible to all users (including anonymous) - these are read-only
 * configuration resources.
 */

import { createSharedCollectionRoutes } from "@/lib/api/crd-route-factory";
import type { Provider } from "@/lib/data/types";

export const { GET } = createSharedCollectionRoutes<Provider>({
  plural: "providers",
  errorLabel: "providers",
});
