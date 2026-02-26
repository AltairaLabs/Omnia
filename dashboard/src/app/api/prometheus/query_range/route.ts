/**
 * Server-side proxy for Prometheus range queries.
 *
 * Proxies requests to the Prometheus /api/v1/query_range endpoint.
 * Returns 503 if Prometheus is not configured.
 */

import { createPrometheusProxy, missingParamResponse } from "@/lib/prometheus-proxy";

export const GET = createPrometheusProxy({
  endpoint: "query_range",
  extractParams: (request) => {
    const query = request.nextUrl.searchParams.get("query");
    if (!query) {
      return { error: missingParamResponse("Missing required parameter: query") };
    }

    const start = request.nextUrl.searchParams.get("start");
    const end = request.nextUrl.searchParams.get("end");
    if (!start || !end) {
      return { error: missingParamResponse("Missing required parameters: start and end") };
    }

    const step = request.nextUrl.searchParams.get("step") || "1h";
    return { params: { query, start, end, step } };
  },
});
