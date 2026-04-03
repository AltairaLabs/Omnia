/**
 * Consent proxy route.
 *
 * GET /api/workspaces/{name}/privacy/consent?userId=X
 *   → SESSION_API_URL/api/v1/privacy/preferences/{userId}/consent
 *
 * PUT /api/workspaces/{name}/privacy/consent?userId=X
 *   → SESSION_API_URL/api/v1/privacy/preferences/{userId}/consent
 *
 * Requires at least viewer role in the workspace.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";

const SESSION_API_URL = process.env.SESSION_API_URL;

const ERR_SESSION_API_NOT_CONFIGURED = "Session API not configured";

function buildTargetUrl(userId: string): string | null {
  if (!SESSION_API_URL) return null;
  const base = SESSION_API_URL.endsWith("/") ? SESSION_API_URL.slice(0, -1) : SESSION_API_URL;
  return `${base}/api/v1/privacy/preferences/${encodeURIComponent(userId)}/consent`;
}

function sessionApiNotConfigured(): NextResponse {
  return NextResponse.json({ error: ERR_SESSION_API_NOT_CONFIGURED }, { status: 503 });
}

export const GET = withWorkspaceAccess(
  "viewer",
  async (
    request: NextRequest,
    _context: WorkspaceRouteContext,
    _access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    if (!SESSION_API_URL) {
      return sessionApiNotConfigured();
    }

    const userId = request.nextUrl.searchParams.get("userId");
    if (!userId) {
      return NextResponse.json({ error: "userId is required" }, { status: 400 });
    }

    const targetUrl = buildTargetUrl(userId);
    if (!targetUrl) {
      return sessionApiNotConfigured();
    }

    try {
      const response = await fetch(targetUrl, {
        headers: { Accept: "application/json" },
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
          { error: `Session API returned non-JSON (HTTP ${response.status})` },
          { status: 502 }
        );
      }
    } catch (error) {
      console.error("Consent API proxy error:", error);
      return NextResponse.json(
        {
          error: "Failed to connect to Session API",
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
    _context: WorkspaceRouteContext,
    _access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    if (!SESSION_API_URL) {
      return sessionApiNotConfigured();
    }

    const userId = request.nextUrl.searchParams.get("userId");
    if (!userId) {
      return NextResponse.json({ error: "userId is required" }, { status: 400 });
    }

    const targetUrl = buildTargetUrl(userId);
    if (!targetUrl) {
      return sessionApiNotConfigured();
    }

    let body: string;
    try {
      body = await request.text();
    } catch {
      return NextResponse.json({ error: "Failed to read request body" }, { status: 400 });
    }

    try {
      const response = await fetch(targetUrl, {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
          Accept: "application/json",
        },
        body,
      });
      const text = await response.text();
      try {
        const data = JSON.parse(text);
        return NextResponse.json(data, { status: response.status });
      } catch {
        return NextResponse.json(
          { error: `Session API returned non-JSON (HTTP ${response.status})` },
          { status: 502 }
        );
      }
    } catch (error) {
      console.error("Consent API proxy error:", error);
      return NextResponse.json(
        {
          error: "Failed to connect to Session API",
          details: error instanceof Error ? error.message : String(error),
        },
        { status: 502 }
      );
    }
  }
);
