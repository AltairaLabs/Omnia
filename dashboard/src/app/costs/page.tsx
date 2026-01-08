"use client";

import { Header } from "@/components/layout";
import { StatCard } from "@/components/dashboard";
import {
  CostByProviderChart,
  CostByModelChart,
  CostOverTimeChart,
  CostBreakdownTable,
  CostUnavailableBanner,
} from "@/components/cost";
import { formatCost, formatTokens } from "@/lib/pricing";
import { DollarSign, TrendingUp, Coins, PiggyBank, Loader2 } from "lucide-react";
import { useCosts } from "@/hooks";
import { Skeleton } from "@/components/ui/skeleton";

const NO_DATA_AVAILABLE = "No data available";

function LoadingSkeleton() {
  return (
    <div className="flex flex-col h-full">
      <Header
        title="Costs"
        description="LLM cost tracking and allocation across agents"
      />
      <div className="flex-1 p-6 space-y-6 overflow-auto">
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          {[...Array(4)].map((_, i) => (
            <Skeleton key={i} className="h-32" />
          ))}
        </div>
        <Skeleton className="h-80" />
        <div className="grid gap-4 md:grid-cols-2">
          <Skeleton className="h-64" />
          <Skeleton className="h-64" />
        </div>
        <Skeleton className="h-96" />
      </div>
    </div>
  );
}

export default function CostsPage() {
  const { data: costData, isLoading, error } = useCosts();

  if (isLoading) {
    return <LoadingSkeleton />;
  }

  if (error) {
    return (
      <div className="flex flex-col h-full">
        <Header
          title="Costs"
          description="LLM cost tracking and allocation across agents"
        />
        <div className="flex-1 p-6">
          <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4">
            <p className="text-sm font-medium text-red-800 dark:text-red-200">
              Failed to load cost data
            </p>
            <p className="text-sm text-red-700 dark:text-red-300">
              {error instanceof Error ? error.message : "Unknown error"}
            </p>
          </div>
        </div>
      </div>
    );
  }

  const { available, reason, summary, byAgent, byProvider, byModel, timeSeries, grafanaUrl } = costData!;

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Costs"
        description="LLM cost tracking and allocation across agents"
      />

      <div className="flex-1 p-6 space-y-6 overflow-auto">
        {/* Unavailable Banner */}
        {!available && <CostUnavailableBanner reason={reason} />}

        {/* Summary Stats */}
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          <StatCard
            title="Total Cost (24h)"
            value={available ? formatCost(summary.totalCost) : "--"}
            description={
              available ? (
                <>
                  <span className="text-orange-600 dark:text-orange-400">
                    {formatCost(summary.anthropicCost)} Anthropic
                  </span>{" "}
                  / {formatCost(summary.openaiCost)} OpenAI
                </>
              ) : (
                NO_DATA_AVAILABLE
              )
            }
            icon={DollarSign}
          />

          <StatCard
            title="Projected Monthly"
            value={available ? formatCost(summary.projectedMonthlyCost) : "--"}
            description={available ? "Based on current 24h usage" : NO_DATA_AVAILABLE}
            icon={TrendingUp}
          />

          <StatCard
            title="Total Tokens (24h)"
            value={available ? formatTokens(summary.totalTokens) : "--"}
            description={
              available ? (
                <>
                  <span className="text-blue-600 dark:text-blue-400">
                    {summary.inputPercent.toFixed(0)}% input
                  </span>{" "}
                  / {summary.outputPercent.toFixed(0)}% output
                </>
              ) : (
                NO_DATA_AVAILABLE
              )
            }
            icon={Coins}
          />

          <StatCard
            title="Cache Savings"
            value={available ? formatCost(summary.totalCacheSavings) : "--"}
            description={available ? "Saved via prompt caching" : NO_DATA_AVAILABLE}
            icon={PiggyBank}
          />
        </div>

        {/* Cost Over Time Chart - Full Width */}
        {available && timeSeries.length > 0 && (
          <CostOverTimeChart data={timeSeries} grafanaUrl={grafanaUrl} />
        )}

        {/* Provider and Model Charts */}
        {available && (byProvider.length > 0 || byModel.length > 0) && (
          <div className="grid gap-4 md:grid-cols-2">
            <CostByProviderChart data={byProvider} />
            <CostByModelChart data={byModel} />
          </div>
        )}

        {/* Detailed Breakdown Table */}
        {available && byAgent.length > 0 && (
          <CostBreakdownTable data={byAgent} />
        )}

        {/* Empty state when available but no data */}
        {available && byAgent.length === 0 && (
          <div className="text-center py-12 text-muted-foreground">
            <Loader2 className="h-8 w-8 animate-spin mx-auto mb-4 opacity-50" />
            <p>No cost data yet. Start using your agents to see cost metrics here.</p>
          </div>
        )}
      </div>
    </div>
  );
}
