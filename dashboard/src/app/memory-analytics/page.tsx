"use client";

import { useMemo, useState } from "react";
import { Header } from "@/components/layout";
import { useMemoryAggregate } from "@/hooks/use-memory-aggregate";
import { useConsentStats } from "@/hooks/use-consent-stats";
import { useEnforcementStats } from "@/hooks/use-enforcement-stats";
import { useAgents } from "@/hooks/use-agents";
import { TierLegend } from "@/components/memory-analytics/tier-legend";
import { TierQuadCard } from "@/components/memory-analytics/tier-quad-card";
import { ConsolidationSection } from "@/components/memory-analytics/consolidation-section";
import { useConsolidationStats } from "@/hooks/use-consolidation-stats";
import { SummaryCards } from "@/components/memory-analytics/summary-cards";
import { CategoryDonut } from "@/components/memory-analytics/category-donut";
import {
  GrowthChart,
  type RangeDays,
} from "@/components/memory-analytics/growth-chart";
import { AgentChart } from "@/components/memory-analytics/agent-chart";
import { PrivacyPosture } from "@/components/memory-analytics/privacy-posture";
import { isTier } from "@/lib/memory-analytics/types";
import {
  agentNameByUidMap,
  resolveAgentRows,
} from "@/lib/memory-analytics/agent-names";
import { EnterpriseGate } from "@/components/license/license-gate";

const DEFAULT_RANGE_DAYS: RangeDays = 30;

const EMPTY_CONSENT = {
  totalUsers: 0,
  optedOutAll: 0,
  grantsByCategory: {},
};

const EMPTY_ENFORCEMENT = {
  piiBlocked: 0,
  redactions: 0,
};

function toUtcMidnight(date: Date): string {
  const d = new Date(
    Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate()),
  );
  return d.toISOString();
}

function todayWindow(): { from: string; to: string } {
  const now = new Date();
  const from = toUtcMidnight(now);
  const tomorrow = new Date(now);
  tomorrow.setUTCDate(tomorrow.getUTCDate() + 1);
  const to = toUtcMidnight(tomorrow);
  return { from, to };
}

function rangeWindow(days: RangeDays): { from: string; to: string } {
  const now = new Date();
  const from = new Date(now);
  from.setUTCDate(from.getUTCDate() - days);
  return { from: from.toISOString(), to: now.toISOString() };
}

function MemoryAnalyticsContent() {
  const [rangeDays, setRangeDays] = useState<RangeDays>(DEFAULT_RANGE_DAYS);

  const tierQuery = useMemoryAggregate({ groupBy: "tier" });
  const categoryQuery = useMemoryAggregate({ groupBy: "category" });
  const agentQuery = useMemoryAggregate({ groupBy: "agent" });
  const today = useMemo(() => todayWindow(), []);
  const todayQuery = useMemoryAggregate({
    groupBy: "day",
    from: today.from,
    to: today.to,
  });
  const range = useMemo(() => rangeWindow(rangeDays), [rangeDays]);
  const dayQuery = useMemoryAggregate({
    groupBy: "day",
    from: range.from,
    to: range.to,
  });
  const activeUsersQuery = useMemoryAggregate({
    groupBy: "tier",
    metric: "distinct_users",
  });
  const consentQuery = useConsentStats();
  const enforcementQuery = useEnforcementStats();
  const agentsQuery = useAgents();
  const consolidationQuery = useConsolidationStats({ rangeDays });

  const agentNameByUid = useMemo(
    () => agentNameByUidMap(agentsQuery.data ?? []),
    [agentsQuery.data],
  );

  const agentRows = useMemo(
    () => resolveAgentRows(agentQuery.data ?? [], agentNameByUid),
    [agentQuery.data, agentNameByUid],
  );

  const totalMemories = (categoryQuery.data ?? []).reduce(
    (acc, r) => acc + r.value,
    0,
  );
  const memoriesToday = (todayQuery.data ?? []).reduce(
    (acc, r) => acc + r.value,
    0,
  );
  const activeUsersRow = (activeUsersQuery.data ?? []).find(
    (r) => isTier(r.key) && r.key === "user",
  );
  const activeUsers = activeUsersRow?.value ?? 0;

  const enforcement = enforcementQuery.data ?? EMPTY_ENFORCEMENT;

  const summaryLoading =
    categoryQuery.isLoading ||
    activeUsersQuery.isLoading ||
    todayQuery.isLoading ||
    enforcementQuery.isLoading;

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Memory analytics"
        description="How memory is being collected, distributed, and consented to across this workspace."
      />

      <main className="flex-1 overflow-auto p-6 space-y-6">
        <TierLegend />

        <SummaryCards
          totalMemories={totalMemories}
          activeUsers={activeUsers}
          memoriesToday={memoriesToday}
          piiBlocked={enforcement.piiBlocked}
          loading={summaryLoading}
        />

        <TierQuadCard
          rows={tierQuery.data ?? []}
          loading={tierQuery.isLoading}
        />

        <ConsolidationSection
          stats={consolidationQuery.data}
          loading={consolidationQuery.isLoading}
        />

        <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <CategoryDonut rows={categoryQuery.data ?? []} />
          <GrowthChart
            rows={dayQuery.data ?? []}
            rangeDays={rangeDays}
            onRangeChange={setRangeDays}
          />
        </div>

        <AgentChart rows={agentRows} />

        <PrivacyPosture
          stats={consentQuery.data ?? EMPTY_CONSENT}
          redactions={enforcement.redactions}
        />
      </main>
    </div>
  );
}

export default function MemoryAnalyticsPage() {
  return (
    <EnterpriseGate featureName="Memory analytics">
      <MemoryAnalyticsContent />
    </EnterpriseGate>
  );
}
