"use client";

import { Bot, FileText, Wrench, Activity, DollarSign, Coins } from "lucide-react";
import { Header } from "@/components/layout";
import { StatCard, RecentAgents, ActivityChart } from "@/components/dashboard";
import { SystemOverviewPanels } from "@/components/grafana";
import { useStats, useGrafana } from "@/hooks";
import { getMockAggregatedUsage } from "@/lib/mock-data";
import { calculateCost, formatCost, formatTokens } from "@/lib/pricing";

export default function Home() {
  const { data: stats, isLoading } = useStats();
  const grafana = useGrafana();

  // Get aggregated usage data for cost display
  const usage = getMockAggregatedUsage();

  // Calculate total estimated cost
  const totalCost = Object.entries(usage.byModel).reduce((total, [model, data]) => {
    return total + calculateCost(model, data.inputTokens, data.outputTokens);
  }, 0);

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Overview"
        description="Monitor and manage your AI agent infrastructure"
      />

      <div className="flex-1 p-6 space-y-6">
        {/* Stats Grid */}
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
          <StatCard
            title="Total Agents"
            value={stats?.agents.total ?? 0}
            description={
              <>
                <span className="text-green-600 dark:text-green-400">
                  {stats?.agents.running ?? 0} running
                </span>{" "}
                / {stats?.agents.pending ?? 0} pending
              </>
            }
            icon={Bot}
            loading={isLoading}
          />

          <StatCard
            title="PromptPacks"
            value={stats?.promptPacks.total ?? 0}
            description={
              <>
                <span className="text-violet-600 dark:text-violet-400">
                  {stats?.promptPacks.canary ?? 0} canary
                </span>{" "}
                deployments
              </>
            }
            icon={FileText}
            loading={isLoading}
          />

          <StatCard
            title="Tools"
            value={stats?.tools.total ?? 0}
            description={
              <>
                <span className="text-cyan-600 dark:text-cyan-400">
                  {stats?.tools.available ?? 0} available
                </span>{" "}
                / {stats?.tools.degraded ?? 0} degraded
              </>
            }
            icon={Wrench}
            loading={isLoading}
          />

          <StatCard
            title="Active Sessions"
            value={stats?.sessions.active.toLocaleString() ?? 0}
            description="+23% from last hour"
            icon={Activity}
            loading={isLoading}
          />

          <StatCard
            title="Est. Cost (24h)"
            value={formatCost(totalCost)}
            description={
              <>
                <span className="text-amber-600 dark:text-amber-400">
                  {usage.totalRequests.toLocaleString()}
                </span>{" "}
                requests
              </>
            }
            icon={DollarSign}
            loading={isLoading}
          />

          <StatCard
            title="Tokens (24h)"
            value={formatTokens(usage.totalTokens)}
            description={
              <>
                <span className="text-blue-600 dark:text-blue-400">
                  {formatTokens(usage.totalInputTokens)}
                </span>{" "}
                in / {formatTokens(usage.totalOutputTokens)} out
              </>
            }
            icon={Coins}
            loading={isLoading}
          />
        </div>

        {/* Grafana Metrics (if enabled) */}
        {grafana.enabled && <SystemOverviewPanels />}

        {/* Charts and Recent Activity */}
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-7">
          <ActivityChart />
          <RecentAgents />
        </div>
      </div>
    </div>
  );
}
