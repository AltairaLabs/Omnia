/**
 * Server-side proxy for the operator API.
 *
 * This route proxies requests from the browser to the operator API,
 * allowing the dashboard to work when deployed in-cluster without
 * exposing the operator API externally.
 *
 * The browser calls: /api/operator/v1/agents
 * This proxies to: http://operator-service:8082/api/v1/agents
 */

import { NextRequest, NextResponse } from "next/server";

// Server-side operator API URL (not exposed to browser)
const OPERATOR_API_URL =
  process.env.OPERATOR_API_URL ||
  process.env.NEXT_PUBLIC_OPERATOR_API_URL ||
  "http://localhost:8082";

type RouteContext = {
  params: Promise<{ path: string[] }>;
};

async function proxyRequest(
  request: NextRequest,
  context: RouteContext
): Promise<NextResponse> {
  const { path } = await context.params;
  const pathString = path.join("/");

  // Build the target URL
  const targetUrl = new URL(`/api/${pathString}`, OPERATOR_API_URL);

  // Forward query parameters
  request.nextUrl.searchParams.forEach((value, key) => {
    targetUrl.searchParams.append(key, value);
  });

  try {
    // Forward the request to the operator API
    const response = await fetch(targetUrl.toString(), {
      method: request.method,
      headers: {
        "Content-Type": "application/json",
        // Forward authorization if present
        ...(request.headers.get("authorization")
          ? { Authorization: request.headers.get("authorization")! }
          : {}),
      },
      // Forward body for POST/PUT/PATCH
      body: ["POST", "PUT", "PATCH"].includes(request.method)
        ? await request.text()
        : undefined,
    });

    // Get response data
    const data = await response.text();

    // Return proxied response
    return new NextResponse(data, {
      status: response.status,
      statusText: response.statusText,
      headers: {
        "Content-Type": response.headers.get("Content-Type") || "application/json",
      },
    });
  } catch (error) {
    console.error(`Proxy error for ${targetUrl}:`, error);
    return NextResponse.json(
      {
        error: "Failed to connect to operator API",
        details: error instanceof Error ? error.message : String(error),
      },
      { status: 502 }
    );
  }
}

export async function GET(request: NextRequest, context: RouteContext) {
  return proxyRequest(request, context);
}

export async function POST(request: NextRequest, context: RouteContext) {
  return proxyRequest(request, context);
}

export async function PUT(request: NextRequest, context: RouteContext) {
  return proxyRequest(request, context);
}

export async function PATCH(request: NextRequest, context: RouteContext) {
  return proxyRequest(request, context);
}

export async function DELETE(request: NextRequest, context: RouteContext) {
  return proxyRequest(request, context);
}
