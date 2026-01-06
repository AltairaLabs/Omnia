/**
 * Grafana proxy route.
 *
 * Proxies requests to Grafana with authentication headers.
 * This enables auth proxy mode where the dashboard authenticates
 * users and passes their identity to Grafana.
 */

import { NextRequest, NextResponse } from "next/server";
import { getUser, getAuthConfig } from "@/lib/auth";
import { getGrafanaAuthHeaders } from "@/lib/auth/proxy";

const GRAFANA_URL = process.env.NEXT_PUBLIC_GRAFANA_URL;

/**
 * Get the remote Grafana path, normalized to start and end with /.
 * Defaults to /grafana/ if not configured.
 */
function getGrafanaRemotePath(): string {
  let path = (process.env.NEXT_PUBLIC_GRAFANA_PATH || "/grafana/").trim();
  if (!path.startsWith("/")) {
    path = "/" + path;
  }
  if (!path.endsWith("/")) {
    path = path + "/";
  }
  return path;
}

/**
 * Proxy all HTTP methods to Grafana.
 */
async function proxyToGrafana(
  request: NextRequest,
  { params }: { params: Promise<{ path: string[] }> }
): Promise<NextResponse> {
  if (!GRAFANA_URL) {
    return NextResponse.json(
      { error: "Grafana URL not configured" },
      { status: 503 }
    );
  }

  const { path } = await params;
  const remotePath = getGrafanaRemotePath();
  const grafanaPath = `${remotePath}${path.join("/")}`;
  const url = new URL(grafanaPath, GRAFANA_URL);

  // Forward query parameters
  request.nextUrl.searchParams.forEach((value, key) => {
    url.searchParams.set(key, value);
  });

  // Get current user for auth headers
  const config = getAuthConfig();
  const headers = new Headers();

  // Copy relevant request headers
  const headersToForward = [
    "accept",
    "accept-encoding",
    "accept-language",
    "cache-control",
    "content-type",
  ];

  for (const header of headersToForward) {
    const value = request.headers.get(header);
    if (value) {
      headers.set(header, value);
    }
  }

  // Add auth headers if proxy auth is enabled
  if (config.mode === "proxy") {
    try {
      const user = await getUser();
      if (user.provider !== "anonymous") {
        const authHeaders = getGrafanaAuthHeaders(user);
        for (const [key, value] of Object.entries(authHeaders)) {
          headers.set(key, value);
        }
      }
    } catch {
      // Continue without auth headers if user fetch fails
    }
  }

  try {
    const response = await fetch(url.toString(), {
      method: request.method,
      headers,
      body: request.method !== "GET" && request.method !== "HEAD"
        ? await request.arrayBuffer()
        : undefined,
    });

    // Create response with Grafana's response
    const responseHeaders = new Headers();

    // Copy safe response headers
    const safeHeaders = [
      "content-type",
      "cache-control",
      "etag",
      "last-modified",
    ];

    for (const header of safeHeaders) {
      const value = response.headers.get(header);
      if (value) {
        responseHeaders.set(header, value);
      }
    }

    // Allow iframe embedding from same origin
    responseHeaders.set("X-Frame-Options", "SAMEORIGIN");

    return new NextResponse(response.body, {
      status: response.status,
      statusText: response.statusText,
      headers: responseHeaders,
    });
  } catch (error) {
    console.error("Grafana proxy error:", error);
    return NextResponse.json(
      { error: "Failed to connect to Grafana" },
      { status: 502 }
    );
  }
}

export async function GET(
  request: NextRequest,
  context: { params: Promise<{ path: string[] }> }
) {
  return proxyToGrafana(request, context);
}

export async function POST(
  request: NextRequest,
  context: { params: Promise<{ path: string[] }> }
) {
  return proxyToGrafana(request, context);
}

export async function PUT(
  request: NextRequest,
  context: { params: Promise<{ path: string[] }> }
) {
  return proxyToGrafana(request, context);
}

export async function DELETE(
  request: NextRequest,
  context: { params: Promise<{ path: string[] }> }
) {
  return proxyToGrafana(request, context);
}
