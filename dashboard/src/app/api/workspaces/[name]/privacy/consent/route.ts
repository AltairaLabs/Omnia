/**
 * Consent proxy route.
 *
 * GET /api/workspaces/{name}/privacy/consent?userId=X
 *   → PRIVACY_API_URL/api/v1/privacy/preferences/{userId}/consent
 *
 * PUT /api/workspaces/{name}/privacy/consent?userId=X
 *   → PRIVACY_API_URL/api/v1/privacy/preferences/{userId}/consent
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

function buildTargetUrl(privacyURL: string, userId: string): string {
  const base = privacyURL.endsWith("/") ? privacyURL.slice(0, -1) : privacyURL;
  const hashedId = pseudonymizeId(userId);
  return `${base}/api/v1/privacy/preferences/${encodeURIComponent(hashedId)}/consent`;
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

    // Scope to the authenticated session user — never a client-supplied
    // userId — so a viewer cannot read another user's consent (#1263).
    const userId = resolveScopedUserId(request.nextUrl.searchParams, user);
    if (!userId) {
      return NextResponse.json({ error: "userId is required" }, { status: 400 });
    }

    const targetUrl = buildTargetUrl(urls.privacyURL, userId);

    try {
      const response = await fetch(targetUrl, {
        headers: serviceApiHeaders({ Accept: "application/json" }),
      });
      const text = await response.text();
      try {
        const data = JSON.parse(text);
        return NextResponse.json(data, { status: response.status });
      } catch {
        // Non-JSON response (e.g. 404 HTML page — consent endpoint not deployed yet)
        if (response.status === 404) {
          return NextResponse.json(
            { grants: [], defaults: [], denied: [] },
            { status: 200 }
          );
        }
        return NextResponse.json(
          { error: `Privacy API returned non-JSON (HTTP ${response.status})` },
          { status: 502 }
        );
      }
    } catch (error) {
      console.error("Consent API proxy error:", error);
      return NextResponse.json(
        {
          error: "Failed to connect to Privacy API",
          details: error instanceof Error ? error.message : String(error),
        },
        { status: 502 }
      );
    }
  }
);

export const PUT = withWorkspaceAccess(
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
    // userId — so a viewer cannot overwrite another user's consent (#1263).
    const userId = resolveScopedUserId(request.nextUrl.searchParams, user);
    if (!userId) {
      return NextResponse.json({ error: "userId is required" }, { status: 400 });
    }

    const targetUrl = buildTargetUrl(urls.privacyURL, userId);

    let body: string;
    try {
      body = await request.text();
    } catch {
      return NextResponse.json({ error: "Failed to read request body" }, { status: 400 });
    }

    try {
      const response = await fetch(targetUrl, {
        method: "PUT",
        headers: serviceApiHeaders({
          "Content-Type": "application/json",
          Accept: "application/json",
        }),
        body,
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
      console.error("Consent API proxy error:", error);
      return NextResponse.json(
        {
          error: "Failed to connect to Privacy API",
          details: error instanceof Error ? error.message : String(error),
        },
        { status: 502 }
      );
    }
  }
);
