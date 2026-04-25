/**
 * Memory aggregate proxy route.
 *
 * GET /api/workspaces/{name}/memory/aggregate?groupBy=tier|category|agent|day&metric=count|distinct_users&from=...&to=...&limit=...
 *   → MEMORY_API_URL/api/v1/memories/aggregate?workspace={uid}&...
 *
 * Validates groupBy + metric whitelists at the proxy boundary so the
 * backend doesn't 400 on an obvious typo. Reuses proxyToMemoryApi from
 * the sibling memory/proxy-helpers.ts which handles workspace UID
 * resolution and the upstream fetch.
 */

import { NextRequest, NextResponse } from "next/server";
import {
  withWorkspaceAccess,
  type WorkspaceRouteContext,
} from "@/lib/auth/workspace-guard";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import { proxyToMemoryApi } from "../proxy-helpers";

const ALLOWED_GROUP_BY = ["category", "agent", "day", "tier"] as const;
const ALLOWED_METRIC = ["count", "distinct_users"] as const;

type AllowedGroupBy = (typeof ALLOWED_GROUP_BY)[number];
type AllowedMetric = (typeof ALLOWED_METRIC)[number];

function isAllowedGroupBy(v: string): v is AllowedGroupBy {
  return (ALLOWED_GROUP_BY as readonly string[]).includes(v);
}

function isAllowedMetric(v: string): v is AllowedMetric {
  return (ALLOWED_METRIC as readonly string[]).includes(v);
}

export const GET = withWorkspaceAccess(
  "viewer",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext,
    _access: WorkspaceAccess,
    _user: User,
  ): Promise<NextResponse> => {
    const { name } = await context.params;
    const search = request.nextUrl.searchParams;

    const groupBy = search.get("groupBy") ?? "";
    if (!isAllowedGroupBy(groupBy)) {
      return NextResponse.json(
        { error: "groupBy must be one of: category, agent, day, tier" },
        { status: 400 },
      );
    }
    const metric = search.get("metric");
    if (metric && !isAllowedMetric(metric)) {
      return NextResponse.json(
        { error: "metric must be one of: count, distinct_users" },
        { status: 400 },
      );
    }

    const params = new URLSearchParams();
    params.set("groupBy", groupBy);
    if (metric) params.set("metric", metric);
    for (const key of ["from", "to", "limit"] as const) {
      const v = search.get(key);
      if (v) params.set(key, v);
    }

    return proxyToMemoryApi(
      request,
      name,
      "/api/v1/memories/aggregate",
      params,
    );
  },
);
