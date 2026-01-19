/**
 * API routes for workspace costs.
 *
 * GET /api/workspaces/:name/costs - Get cost data for workspace
 *
 * Queries Prometheus for LLM cost metrics filtered by workspace namespace.
 * Protected by workspace access checks.
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
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import { PrometheusService } from "@/lib/data/prometheus-service";
import type { CostData } from "@/lib/data/types";

const prometheusService = new PrometheusService();

/**
 * Workspace cost response with budget information.
 */
interface WorkspaceCostResponse extends CostData {
  /** Budget configuration from workspace (if set) */
  budget?: {
    dailyBudget?: string;
    monthlyBudget?: string;
    dailyUsedPercent?: number;
    monthlyUsedPercent?: number;
  };
}

export const GET = withWorkspaceAccess(
  "viewer",
  async (
    _request: NextRequest,
    context: WorkspaceRouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;
    let auditCtx;

    try {
      const result = await validateWorkspace(name, access.role!);
      if (!result.ok) return result.response;

      const namespace = result.workspace.spec.namespace.name;

      auditCtx = createAuditContext(
        name,
        namespace,
        user,
        access.role!,
        "Cost"
      );

      // Fetch costs from Prometheus filtered by workspace namespace
      const costs = await prometheusService.getCosts({ namespace });

      // Build response with budget info if configured
      const response: WorkspaceCostResponse = {
        ...costs,
      };

      // Add budget information if workspace has cost controls configured
      const costControls = result.workspace.spec.costControls;
      if (costControls) {
        const dailyBudget = costControls.dailyBudget
          ? parseFloat(costControls.dailyBudget)
          : undefined;
        const monthlyBudget = costControls.monthlyBudget
          ? parseFloat(costControls.monthlyBudget)
          : undefined;

        response.budget = {
          dailyBudget: costControls.dailyBudget,
          monthlyBudget: costControls.monthlyBudget,
        };

        // Calculate usage percentages based on current spend
        // The prometheus service returns 24h costs in summary.totalCost
        if (dailyBudget && dailyBudget > 0) {
          response.budget.dailyUsedPercent = Math.min(
            (costs.summary.totalCost / dailyBudget) * 100,
            100
          );
        }

        // For monthly, project based on daily average
        if (monthlyBudget && monthlyBudget > 0) {
          response.budget.monthlyUsedPercent = Math.min(
            (costs.summary.projectedMonthlyCost / monthlyBudget) * 100,
            100
          );
        }
      }

      auditSuccess(auditCtx, "get", undefined, {
        available: costs.available,
        totalCost: costs.summary.totalCost,
      });
      return NextResponse.json(response);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", undefined, error, 500);
      }
      return serverErrorResponse(error, "Failed to fetch workspace costs");
    }
  }
);
