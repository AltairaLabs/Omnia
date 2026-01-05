"use client";

import { Header } from "@/components/layout";
import { StatCard } from "@/components/dashboard";
import {
  CostByProviderChart,
  CostByModelChart,
  CostOverTimeChart,
  CostBreakdownTable,
} from "@/components/cost";
import {
  mockCostAllocation,
  mockCostTimeSeries,
  getMockCostByProvider,
  getMockCostByModel,
  getMockCostSummary,
} from "@/lib/mock-data";
import { formatCost, formatTokens } from "@/lib/pricing";
import { DollarSign, TrendingUp, Coins, PiggyBank } from "lucide-react";

export default function CostsPage() {
  const summary = getMockCostSummary();
  const costByProvider = getMockCostByProvider();
  const costByModel = getMockCostByModel();

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Costs"
        description="LLM cost tracking and allocation across agents"
      />

      <div className="flex-1 p-6 space-y-6 overflow-auto">
        {/* Summary Stats */}
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          <StatCard
            title="Total Cost (24h)"
            value={formatCost(summary.totalCost)}
            description={
              <>
                <span className="text-orange-600 dark:text-orange-400">
                  {formatCost(summary.anthropicCost)} Anthropic
                </span>{" "}
                / {formatCost(summary.openaiCost)} OpenAI
              </>
            }
            icon={DollarSign}
          />

          <StatCard
            title="Projected Monthly"
            value={formatCost(summary.projectedMonthlyCost)}
            description="Based on current 24h usage"
            icon={TrendingUp}
          />

          <StatCard
            title="Total Tokens (24h)"
            value={formatTokens(summary.totalTokens)}
            description={
              <>
                <span className="text-blue-600 dark:text-blue-400">
                  {summary.inputPercent.toFixed(0)}% input
                </span>{" "}
                / {summary.outputPercent.toFixed(0)}% output
              </>
            }
            icon={Coins}
          />

          <StatCard
            title="Cache Savings"
            value={formatCost(summary.totalCacheSavings)}
            description="Saved via prompt caching"
            icon={PiggyBank}
          />
        </div>

        {/* Cost Over Time Chart - Full Width */}
        <CostOverTimeChart data={mockCostTimeSeries} />

        {/* Provider and Model Charts */}
        <div className="grid gap-4 md:grid-cols-2">
          <CostByProviderChart data={costByProvider} />
          <CostByModelChart data={costByModel} />
        </div>

        {/* Detailed Breakdown Table */}
        <CostBreakdownTable data={mockCostAllocation} />
      </div>
    </div>
  );
}
