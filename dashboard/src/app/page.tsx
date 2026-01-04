"use client";

import { Bot, FileText, Wrench, Activity } from "lucide-react";
import { Header } from "@/components/layout";
import { StatCard, RecentAgents, ActivityChart } from "@/components/dashboard";
import { useStats } from "@/hooks";

export default function Home() {
  const { data: stats, isLoading } = useStats();

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Overview"
        description="Monitor and manage your AI agent infrastructure"
      />

      <div className="flex-1 p-6 space-y-6">
        {/* Stats Grid */}
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
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
        </div>

        {/* Charts and Recent Activity */}
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-7">
          <ActivityChart />
          <RecentAgents />
        </div>
      </div>
    </div>
  );
}
