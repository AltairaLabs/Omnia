/**
 * Server-side proxy for Prometheus instant queries.
 *
 * Proxies requests to the Prometheus /api/v1/query endpoint.
 * Returns 503 if Prometheus is not configured.
 */

import { createPrometheusProxy, missingParamResponse } from "@/lib/prometheus-proxy";

export const GET = createPrometheusProxy({
  endpoint: "query",
  extractParams: (request) => {
    const query = request.nextUrl.searchParams.get("query");
    if (!query) {
      return { error: missingParamResponse("Missing required parameter: query") };
    }

    const params: Record<string, string> = { query };
    const time = request.nextUrl.searchParams.get("time");
    if (time) {
      params.time = time;
    }

    return { params };
  },
});
