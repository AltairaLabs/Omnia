/**
 * API route for getting a specific shared provider.
 *
 * GET /api/shared/providers/:providerName - Get provider details
 *
 * Providers are cluster-wide resources that define available LLM providers.
 * Accessible to all users (including anonymous) - these are read-only
 * configuration resources.
 */

import { createSharedItemRoutes } from "@/lib/api/crd-route-factory";
import type { Provider } from "@/lib/data/types";

export const { GET } = createSharedItemRoutes<Provider>({
  plural: "providers",
  paramKey: "providerName",
  resourceLabel: "Provider",
  errorLabel: "provider",
});
