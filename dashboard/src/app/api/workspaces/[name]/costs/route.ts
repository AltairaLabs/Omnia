/**
 * API routes for workspace costs.
 *
 * GET /api/workspaces/:name/costs - Get cost data for workspace
 *
 * Reads exact aggregated cost/token usage from the workspace's session-api
 * provider_calls tables (product data — see CLAUDE.md → Observability
 * Boundaries), not Prometheus. Protected by workspace access checks.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import {
  validateWorkspace,
  serverErrorResponse,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import { resolveServiceURLs } from "@/lib/k8s/service-url-resolver";
import { fetchWorkspaceCostData, type CostSource } from "@/lib/data/cost-from-session-api";
import { emptyCostData } from "@/lib/data/cost-aggregation";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { CostData } from "@/lib/data/types";

const ONE_DAY_MS = 24 * 60 * 60 * 1000;

interface WorkspaceCostResponse extends CostData {
  budget?: {
    dailyBudget?: string;
    monthlyBudget?: string;
    dailyUsedPercent?: number;
    monthlyUsedPercent?: number;
  };
}

function withBudget(
  costs: CostData,
  costControls: { dailyBudget?: string; monthlyBudget?: string } | undefined,
): WorkspaceCostResponse {
  const response: WorkspaceCostResponse = { ...costs };
  if (!costControls) return response;

  const daily = costControls.dailyBudget ? Number.parseFloat(costControls.dailyBudget) : undefined;
  const monthly = costControls.monthlyBudget ? Number.parseFloat(costControls.monthlyBudget) : undefined;
  response.budget = { dailyBudget: costControls.dailyBudget, monthlyBudget: costControls.monthlyBudget };
  if (daily && daily > 0) {
    response.budget.dailyUsedPercent = Math.min((costs.summary.totalCost / daily) * 100, 100);
  }
  if (monthly && monthly > 0) {
    response.budget.monthlyUsedPercent = Math.min((costs.summary.projectedMonthlyCost / monthly) * 100, 100);
  }
  return response;
}

export const GET = withWorkspaceAccess(
  "viewer",
  async (
    _request: NextRequest,
    context: WorkspaceRouteContext,
    access: WorkspaceAccess,
    user: User,
  ): Promise<NextResponse> => {
    const { name } = await context.params;
    let auditCtx;
    try {
      const result = await validateWorkspace(name, access.role!);
      if (!result.ok) return result.response;

      const namespace = result.workspace.spec.namespace.name;
      auditCtx = createAuditContext(name, namespace, user, access.role!, "Cost");

      const urls = await resolveServiceURLs(name);
      if (!urls) {
        auditSuccess(auditCtx, "get", undefined, { available: false });
        return NextResponse.json(emptyCostData("Session API not configured"));
      }

      const sources: CostSource[] = [{ sessionURL: urls.sessionURL, namespace: urls.namespace }];
      const to = new Date();
      const from = new Date(to.getTime() - ONE_DAY_MS);
      const costs = await fetchWorkspaceCostData(sources, from, to);

      const response = withBudget(costs, result.workspace.spec.costControls);
      auditSuccess(auditCtx, "get", undefined, {
        available: costs.available,
        totalCost: costs.summary.totalCost,
      });
      return NextResponse.json(response);
    } catch (error) {
      if (auditCtx) auditError(auditCtx, "get", undefined, error, 500);
      return serverErrorResponse(error, "Failed to fetch workspace costs");
    }
  },
);
