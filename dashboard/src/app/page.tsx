"use client";

import { Bot, FileText, Wrench, Activity, DollarSign, Coins } from "lucide-react";
import { Header } from "@/components/layout";
import { StatCard, RecentAgents, ActivityChart, SystemMetrics } from "@/components/dashboard";
import { useStats, useCosts } from "@/hooks";
import { formatCost, formatTokens } from "@/lib/pricing";

export default function Home() {
  const { data: stats, isLoading: statsLoading } = useStats();
  const { data: costs, isLoading: costsLoading } = useCosts();

  // Check if cost data is available
  const costsAvailable = costs?.available ?? false;
  const summary = costs?.summary;

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
            loading={statsLoading}
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
            loading={statsLoading}
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
            loading={statsLoading}
          />

          <StatCard
            title="Active Sessions"
            value={stats?.sessions.active.toLocaleString() ?? 0}
            description={
              stats?.sessions.trend !== null && stats?.sessions.trend !== undefined ? (
                <>
                  <span className={stats.sessions.trend >= 0 ? "text-green-600 dark:text-green-400" : "text-red-600 dark:text-red-400"}>
                    {stats.sessions.trend >= 0 ? "+" : ""}{stats.sessions.trend.toFixed(0)}%
                  </span>{" "}
                  from last hour
                </>
              ) : (
                "Currently connected"
              )
            }
            icon={Activity}
            loading={statsLoading}
          />

          <StatCard
            title="Est. Cost (24h)"
            value={costsAvailable ? formatCost(summary?.totalCost ?? 0) : "--"}
            description={
              costsAvailable ? (
                <>
                  <span className="text-amber-600 dark:text-amber-400">
                    {(summary?.totalRequests ?? 0).toLocaleString()}
                  </span>{" "}
                  requests
                </>
              ) : (
                "Prometheus not configured"
              )
            }
            icon={DollarSign}
            loading={costsLoading}
          />

          <StatCard
            title="Tokens (24h)"
            value={costsAvailable ? formatTokens(summary?.totalTokens ?? 0) : "--"}
            description={
              costsAvailable ? (
                <>
                  <span className="text-blue-600 dark:text-blue-400">
                    {formatTokens(summary?.inputTokens ?? 0)}
                  </span>{" "}
                  in / {formatTokens(summary?.outputTokens ?? 0)} out
                </>
              ) : (
                "Prometheus not configured"
              )
            }
            icon={Coins}
            loading={costsLoading}
          />
        </div>

        {/* Live Metrics from Prometheus */}
        <SystemMetrics />

        {/* Charts and Recent Activity */}
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-7">
          <ActivityChart />
          <RecentAgents />
        </div>
      </div>
    </div>
  );
}
