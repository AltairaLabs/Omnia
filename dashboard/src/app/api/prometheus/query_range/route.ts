/**
 * Server-side proxy for Prometheus range queries.
 *
 * Proxies requests to the Prometheus /api/v1/query_range endpoint.
 * Returns 503 if Prometheus is not configured.
 */

import { NextRequest, NextResponse } from "next/server";
import { PROMETHEUS_FETCH_TIMEOUT_MS } from "@/lib/query-config";

const PROMETHEUS_URL = process.env.PROMETHEUS_URL;

export async function GET(request: NextRequest): Promise<NextResponse> {
  if (!PROMETHEUS_URL) {
    return NextResponse.json(
      {
        status: "error",
        errorType: "configuration",
        error: "Prometheus not configured",
      },
      { status: 503 }
    );
  }

  const query = request.nextUrl.searchParams.get("query");
  const start = request.nextUrl.searchParams.get("start");
  const end = request.nextUrl.searchParams.get("end");
  const step = request.nextUrl.searchParams.get("step") || "1h";

  if (!query) {
    return NextResponse.json(
      {
        status: "error",
        errorType: "bad_data",
        error: "Missing required parameter: query",
      },
      { status: 400 }
    );
  }

  if (!start || !end) {
    return NextResponse.json(
      {
        status: "error",
        errorType: "bad_data",
        error: "Missing required parameters: start and end",
      },
      { status: 400 }
    );
  }

  // Build the Prometheus query_range URL
  // PROMETHEUS_URL may include a path prefix (e.g., /prometheus), so we append to it
  const baseUrl = PROMETHEUS_URL.endsWith("/") ? PROMETHEUS_URL.slice(0, -1) : PROMETHEUS_URL;
  const targetUrl = new URL(`${baseUrl}/api/v1/query_range`);
  targetUrl.searchParams.set("query", query);
  targetUrl.searchParams.set("start", start);
  targetUrl.searchParams.set("end", end);
  targetUrl.searchParams.set("step", step);

  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), PROMETHEUS_FETCH_TIMEOUT_MS);

  try {
    const response = await fetch(targetUrl.toString(), {
      headers: {
        Accept: "application/json",
      },
      signal: controller.signal,
    });

    const data = await response.json();

    return NextResponse.json(data, {
      status: response.status,
    });
  } catch (error) {
    if (error instanceof DOMException && error.name === "AbortError") {
      return NextResponse.json(
        {
          status: "error",
          errorType: "timeout",
          error: "Prometheus query timed out",
        },
        { status: 504 }
      );
    }

    console.error("Prometheus query_range error:", error);
    return NextResponse.json(
      {
        status: "error",
        errorType: "internal",
        error: "Failed to connect to Prometheus",
        details: error instanceof Error ? error.message : String(error),
      },
      { status: 502 }
    );
  } finally {
    clearTimeout(timeout);
  }
}
