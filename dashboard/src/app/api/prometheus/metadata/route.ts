/**
 * Server-side proxy for Prometheus metadata queries.
 *
 * Proxies requests to the Prometheus /api/v1/metadata endpoint.
 * Accepts an optional `metric` query param to filter by metric name.
 * Returns 503 if Prometheus is not configured.
 */

import { createPrometheusProxy } from "@/lib/prometheus-proxy";

export const GET = createPrometheusProxy({
  endpoint: "metadata",
  extractParams: (request) => {
    const params: Record<string, string> = {};
    const metric = request.nextUrl.searchParams.get("metric");
    if (metric) {
      params.metric = metric;
    }
    return { params };
  },
});
