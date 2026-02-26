/**
 * Shared Prometheus proxy logic for server-side API routes.
 *
 * Both /api/prometheus/query and /api/prometheus/query_range use the same
 * pattern: validate config, validate params, build URL, fetch with timeout,
 * and return JSON. This module extracts that shared logic.
 */

import { NextRequest, NextResponse } from "next/server";
import { PROMETHEUS_FETCH_TIMEOUT_MS } from "@/lib/query-config";

const PROMETHEUS_URL = process.env.PROMETHEUS_URL;

interface PrometheusProxyOptions {
  /** The Prometheus API path suffix (e.g., "query" or "query_range"). */
  endpoint: string;
  /** Extract and validate query parameters from the request. Returns params to forward or an error response. */
  extractParams: (request: NextRequest) => { params: Record<string, string> } | { error: NextResponse };
}

function notConfiguredResponse(): NextResponse {
  return NextResponse.json(
    { status: "error", errorType: "configuration", error: "Prometheus not configured" },
    { status: 503 },
  );
}

function timeoutResponse(): NextResponse {
  return NextResponse.json(
    { status: "error", errorType: "timeout", error: "Prometheus query timed out" },
    { status: 504 },
  );
}

function connectionErrorResponse(error: unknown): NextResponse {
  return NextResponse.json(
    {
      status: "error",
      errorType: "internal",
      error: "Failed to connect to Prometheus",
      details: error instanceof Error ? error.message : String(error),
    },
    { status: 502 },
  );
}

function missingParamResponse(message: string): NextResponse {
  return NextResponse.json(
    { status: "error", errorType: "bad_data", error: message },
    { status: 400 },
  );
}

/**
 * Create a GET handler that proxies to a Prometheus endpoint.
 */
export function createPrometheusProxy({ endpoint, extractParams }: PrometheusProxyOptions) {
  return async function GET(request: NextRequest): Promise<NextResponse> {
    if (!PROMETHEUS_URL) {
      return notConfiguredResponse();
    }

    const result = extractParams(request);
    if ("error" in result) {
      return result.error;
    }

    const baseUrl = PROMETHEUS_URL.endsWith("/") ? PROMETHEUS_URL.slice(0, -1) : PROMETHEUS_URL;
    const targetUrl = new URL(`${baseUrl}/api/v1/${endpoint}`);
    for (const [key, value] of Object.entries(result.params)) {
      targetUrl.searchParams.set(key, value);
    }

    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), PROMETHEUS_FETCH_TIMEOUT_MS);

    try {
      const response = await fetch(targetUrl.toString(), {
        headers: { Accept: "application/json" },
        signal: controller.signal,
      });

      const data = await response.json();
      return NextResponse.json(data, { status: response.status });
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") {
        return timeoutResponse();
      }
      console.error(`Prometheus ${endpoint} error:`, error);
      return connectionErrorResponse(error);
    } finally {
      clearTimeout(timeout);
    }
  };
}

export { missingParamResponse };
