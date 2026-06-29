/**
 * Workspace-level privacy enforcement stats proxy route.
 *
 * GET /api/workspaces/{name}/privacy/enforcement-stats
 *   → PRIVACY_API_URL/api/v1/privacy/enforcement-stats?workspace={uid}
 *
 * Aggregate-only — returns workspace-scoped counts of opt-out write blocks
 * (piiBlocked) and PII redactions (redactions). Handler moved from memory-api
 * to privacy-api in Slice B (#1642).
 */

import { NextRequest, NextResponse } from "next/server";
import {
  withWorkspaceAccess,
  type WorkspaceRouteContext,
} from "@/lib/auth/workspace-guard";
import { resolveServiceURLs } from "@/lib/k8s/service-url-resolver";
import { serviceApiHeaders } from "@/lib/auth/session-api-token";
import { getWorkspace } from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";

const ERR_PRIVACY_API_NOT_CONFIGURED = "Privacy API not configured";

function privacyApiNotConfigured(): NextResponse {
  return NextResponse.json({ error: ERR_PRIVACY_API_NOT_CONFIGURED }, { status: 503 });
}

export const GET = withWorkspaceAccess(
  "viewer",
  async (
    _request: NextRequest,
    context: WorkspaceRouteContext,
    _access: WorkspaceAccess,
    _user: User,
  ): Promise<NextResponse> => {
    const { name } = await context.params;

    const urls = await resolveServiceURLs(name);
    if (!urls || !urls.privacyURL) {
      return privacyApiNotConfigured();
    }

    const workspace = await getWorkspace(name);
    const uid = workspace?.metadata?.uid;
    if (!uid) {
      return NextResponse.json({ error: "Workspace not found" }, { status: 404 });
    }

    const base = urls.privacyURL.endsWith("/") ? urls.privacyURL.slice(0, -1) : urls.privacyURL;
    const targetUrl = `${base}/api/v1/privacy/enforcement-stats?workspace=${encodeURIComponent(uid)}`;

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
          { status: 502 },
        );
      }
    } catch (error) {
      console.error("Enforcement stats API proxy error:", error);
      return NextResponse.json(
        {
          error: "Failed to connect to Privacy API",
          details: error instanceof Error ? error.message : String(error),
        },
        { status: 502 },
      );
    }
  },
);
