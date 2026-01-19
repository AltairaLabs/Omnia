/**
 * Server-side proxy for the operator API.
 *
 * DEPRECATED: This proxy route is deprecated and will be removed in a future release.
 * Use the new workspace-scoped API routes instead:
 *   - /api/workspaces/:name/agents
 *   - /api/workspaces/:name/promptpacks
 *   - /api/shared/toolregistries
 *   - /api/shared/providers
 *
 * See #278 for migration details.
 *
 * Proxy Mode Configuration:
 * - OMNIA_PROXY_MODE=strict (default): Returns 410 Gone for all proxy requests
 * - OMNIA_PROXY_MODE=compat: Allows proxy requests with deprecation warnings
 *
 * The browser calls: /api/operator/api/v1/agents
 * This proxies to: http://operator-service:8082/api/v1/agents
 *
 * Note: Demo mode is handled by the DataService layer.
 * When demo mode is enabled, MockDataService is used and this
 * proxy is never called.
 */

import { NextRequest, NextResponse } from "next/server";
import { logProxyUsage } from "@/lib/audit";

// Server-side operator API URL (not exposed to browser)
const OPERATOR_API_URL =
  process.env.OPERATOR_API_URL ||
  process.env.NEXT_PUBLIC_OPERATOR_API_URL ||
  "http://localhost:8082";

/**
 * Proxy mode configuration.
 * - "strict" (default): Proxy is disabled, returns 410 Gone
 * - "compat": Proxy is enabled with deprecation warnings (for migration period)
 */
const PROXY_MODE = process.env.OMNIA_PROXY_MODE || "strict";

type RouteContext = {
  params: Promise<{ path: string[] }>;
};

/**
 * Check if proxy mode allows requests.
 */
function isProxyEnabled(): boolean {
  return PROXY_MODE === "compat";
}

/**
 * Return 410 Gone response when proxy is disabled (strict mode).
 */
function proxyDisabledResponse(method: string, path: string): NextResponse {
  return NextResponse.json(
    {
      error: "Gone",
      message:
        "The operator proxy has been deprecated and is disabled by default. " +
        "Please migrate to workspace-scoped API routes. " +
        "See https://github.com/AltairaLabs/Omnia/issues/278 for migration guide. " +
        "To temporarily re-enable the proxy during migration, set OMNIA_PROXY_MODE=compat.",
      deprecatedPath: `/api/operator/${path}`,
      migrationGuide: "https://github.com/AltairaLabs/Omnia/issues/278",
      alternatives: [
        "/api/workspaces/:name/agents",
        "/api/workspaces/:name/promptpacks",
        "/api/shared/toolregistries",
        "/api/shared/providers",
      ],
    },
    { status: 410 }
  );
}

async function proxyRequest(
  request: NextRequest,
  context: RouteContext
): Promise<NextResponse> {
  const { path } = await context.params;
  const pathString = path.join("/");

  // Get user info for audit logging
  const user = request.headers.get("x-forwarded-user") ||
               request.headers.get("x-user-email") ||
               "unknown";
  const userAgent = request.headers.get("user-agent") || undefined;

  // Log proxy usage for audit trail
  logProxyUsage(request.method, pathString, user, userAgent);

  // Check if proxy is enabled
  if (!isProxyEnabled()) {
    console.warn(
      `[PROXY DISABLED] Blocked proxy request: ${request.method} /api/operator/${pathString}. ` +
      `OMNIA_PROXY_MODE=${PROXY_MODE}. Set OMNIA_PROXY_MODE=compat to allow during migration.`
    );
    return proxyDisabledResponse(request.method, pathString);
  }

  // DEPRECATION WARNING: This proxy route is deprecated.
  // Use the new workspace-scoped API routes instead:
  //   - /api/workspaces/:name/agents
  //   - /api/workspaces/:name/promptpacks
  //   - /api/shared/toolregistries
  //   - /api/shared/providers
  // See #278 for migration details.
  console.warn(
    `[DEPRECATED] Operator proxy route called: ${request.method} /api/operator/${pathString}. ` +
    `User: ${user}. ` +
    `Please migrate to workspace-scoped API routes (see #278). ` +
    `This proxy will be removed in a future release.`
  );

  // Build the target URL - pathString already includes 'api/v1/...'
  const targetUrl = new URL(`/${pathString}`, OPERATOR_API_URL);

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

    // Return proxied response with deprecation header
    return new NextResponse(data, {
      status: response.status,
      statusText: response.statusText,
      headers: {
        "Content-Type": response.headers.get("Content-Type") || "application/json",
        "X-Deprecated": "true",
        "X-Deprecation-Notice": "Operator proxy is deprecated. Migrate to /api/workspaces/:name/* routes.",
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
