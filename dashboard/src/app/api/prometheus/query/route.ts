/**
 * Server-side proxy for Prometheus instant queries.
 *
 * Proxies requests to the Prometheus /api/v1/query endpoint.
 * Returns 503 if Prometheus is not configured.
 */

import { NextRequest, NextResponse } from "next/server";

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
  const time = request.nextUrl.searchParams.get("time");

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

  // Build the Prometheus query URL
  // PROMETHEUS_URL may include a path prefix (e.g., /prometheus), so we append to it
  const baseUrl = PROMETHEUS_URL.endsWith("/") ? PROMETHEUS_URL.slice(0, -1) : PROMETHEUS_URL;
  const targetUrl = new URL(`${baseUrl}/api/v1/query`);
  targetUrl.searchParams.set("query", query);
  if (time) {
    targetUrl.searchParams.set("time", time);
  }

  try {
    const response = await fetch(targetUrl.toString(), {
      headers: {
        Accept: "application/json",
      },
    });

    const data = await response.json();

    return NextResponse.json(data, {
      status: response.status,
    });
  } catch (error) {
    console.error("Prometheus query error:", error);
    return NextResponse.json(
      {
        status: "error",
        errorType: "internal",
        error: "Failed to connect to Prometheus",
        details: error instanceof Error ? error.message : String(error),
      },
      { status: 502 }
    );
  }
}
