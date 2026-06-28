/**
 * Opt-out proxy route.
 *
 * GET /api/workspaces/{name}/privacy/opt-out?userId=X
 *   → PRIVACY_API_URL/api/v1/privacy/preferences/{userId}   (full Preferences)
 *
 * POST /api/workspaces/{name}/privacy/opt-out?userId=X
 *   → PRIVACY_API_URL/api/v1/privacy/opt-out   body: { userId, scope, target }
 *
 * DELETE /api/workspaces/{name}/privacy/opt-out?userId=X
 *   → PRIVACY_API_URL/api/v1/privacy/opt-out   body: { userId, scope, target }
 *
 * The server-resolved hashed userId is always used — any client-supplied
 * userId in the request is ignored (#1263).
 *
 * Requires at least viewer role in the workspace.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { resolveServiceURLs } from "@/lib/k8s/service-url-resolver";
import { serviceApiHeaders } from "@/lib/auth/session-api-token";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import { pseudonymizeId } from "@/lib/identity";
import { resolveScopedUserId } from "@/lib/auth/scoped-user";

const ERR_PRIVACY_API_NOT_CONFIGURED = "Privacy API not configured";
const ERR_USER_ID_REQUIRED = "userId is required";
const ERR_CONNECT_PRIVACY_API = "Failed to connect to Privacy API";
const LOG_OPT_OUT_PROXY_ERROR = "Opt-out API proxy error:";

function privacyBase(privacyURL: string): string {
  return privacyURL.endsWith("/") ? privacyURL.slice(0, -1) : privacyURL;
}

function privacyApiNotConfigured(): NextResponse {
  return NextResponse.json({ error: ERR_PRIVACY_API_NOT_CONFIGURED }, { status: 503 });
}

export const GET = withWorkspaceAccess(
  "viewer",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext,
    _access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;

    const urls = await resolveServiceURLs(name);
    if (!urls) {
      return privacyApiNotConfigured();
    }

    const userId = resolveScopedUserId(request.nextUrl.searchParams, user);
    if (!userId) {
      return NextResponse.json({ error: ERR_USER_ID_REQUIRED }, { status: 400 });
    }

    const hashedId = pseudonymizeId(userId);
    const targetUrl = `${privacyBase(urls.privacyURL)}/api/v1/privacy/preferences/${encodeURIComponent(hashedId)}`;

    try {
      const response = await fetch(targetUrl, {
        headers: serviceApiHeaders({ Accept: "application/json" }),
      });
      const text = await response.text();
      try {
        const data = JSON.parse(text);
        return NextResponse.json(data, { status: response.status });
      } catch {
        return NextResponse.json(
          { error: `Privacy API returned non-JSON (HTTP ${response.status})` },
          { status: 502 }
        );
      }
    } catch (error) {
      console.error(LOG_OPT_OUT_PROXY_ERROR, error);
      return NextResponse.json(
        {
          error: ERR_CONNECT_PRIVACY_API,
          details: error instanceof Error ? error.message : String(error),
        },
        { status: 502 }
      );
    }
  }
);

export const POST = withWorkspaceAccess(
  "viewer",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext,
    _access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;

    const urls = await resolveServiceURLs(name);
    if (!urls) {
      return privacyApiNotConfigured();
    }

    // Scope to the authenticated session user — never a client-supplied
    // userId — so a viewer cannot register an opt-out on behalf of another
    // user (#1263).
    const userId = resolveScopedUserId(request.nextUrl.searchParams, user);
    if (!userId) {
      return NextResponse.json({ error: ERR_USER_ID_REQUIRED }, { status: 400 });
    }

    const hashedId = pseudonymizeId(userId);

    let rawBody: unknown;
    try {
      rawBody = await request.json();
    } catch {
      return NextResponse.json({ error: "Failed to read request body" }, { status: 400 });
    }

    const { scope, target } = rawBody as { scope?: unknown; target?: unknown };

    const outBody = JSON.stringify({ userId: hashedId, scope, target });
    const targetUrl = `${privacyBase(urls.privacyURL)}/api/v1/privacy/opt-out`;

    try {
      const response = await fetch(targetUrl, {
        method: "POST",
        headers: serviceApiHeaders({
          "Content-Type": "application/json",
          Accept: "application/json",
        }),
        body: outBody,
      });
      const text = await response.text();
      try {
        const data = JSON.parse(text);
        return NextResponse.json(data, { status: response.status });
      } catch {
        return NextResponse.json(
          { error: `Privacy API returned non-JSON (HTTP ${response.status})` },
          { status: 502 }
        );
      }
    } catch (error) {
      console.error(LOG_OPT_OUT_PROXY_ERROR, error);
      return NextResponse.json(
        {
          error: ERR_CONNECT_PRIVACY_API,
          details: error instanceof Error ? error.message : String(error),
        },
        { status: 502 }
      );
    }
  }
);

export const DELETE = withWorkspaceAccess(
  "viewer",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext,
    _access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;

    const urls = await resolveServiceURLs(name);
    if (!urls) {
      return privacyApiNotConfigured();
    }

    // Scope to the authenticated session user — never a client-supplied
    // userId — so a viewer cannot remove another user's opt-out (#1263).
    const userId = resolveScopedUserId(request.nextUrl.searchParams, user);
    if (!userId) {
      return NextResponse.json({ error: ERR_USER_ID_REQUIRED }, { status: 400 });
    }

    const hashedId = pseudonymizeId(userId);

    let rawBody: unknown;
    try {
      rawBody = await request.json();
    } catch {
      return NextResponse.json({ error: "Failed to read request body" }, { status: 400 });
    }

    const { scope, target } = rawBody as { scope?: unknown; target?: unknown };

    const outBody = JSON.stringify({ userId: hashedId, scope, target });
    const targetUrl = `${privacyBase(urls.privacyURL)}/api/v1/privacy/opt-out`;

    try {
      const response = await fetch(targetUrl, {
        method: "DELETE",
        headers: serviceApiHeaders({
          "Content-Type": "application/json",
          Accept: "application/json",
        }),
        body: outBody,
      });
      const text = await response.text();
      try {
        const data = JSON.parse(text);
        return NextResponse.json(data, { status: response.status });
      } catch {
        return NextResponse.json(
          { error: `Privacy API returned non-JSON (HTTP ${response.status})` },
          { status: 502 }
        );
      }
    } catch (error) {
      console.error(LOG_OPT_OUT_PROXY_ERROR, error);
      return NextResponse.json(
        {
          error: ERR_CONNECT_PRIVACY_API,
          details: error instanceof Error ? error.message : String(error),
        },
        { status: 502 }
      );
    }
  }
);
