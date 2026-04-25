"use client";

import { Database, Users, Calendar, ShieldOff } from "lucide-react";
import { StatCard } from "@/components/dashboard/stat-card";

interface SummaryCardsProps {
  totalMemories: number;
  activeUsers: number;
  memoriesToday: number;
  piiBlocked: number;
  loading?: boolean;
}

export function SummaryCards({
  totalMemories,
  activeUsers,
  memoriesToday,
  piiBlocked,
  loading,
}: Readonly<SummaryCardsProps>) {
  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
      <StatCard
        title="Total memories"
        value={totalMemories.toLocaleString()}
        icon={Database}
        loading={loading}
      />
      <StatCard
        title="Active users"
        value={activeUsers.toLocaleString()}
        description="Users with stored memory"
        icon={Users}
        loading={loading}
      />
      <StatCard
        title="Created today"
        value={memoriesToday.toLocaleString()}
        icon={Calendar}
        loading={loading}
      />
      <StatCard
        title="PII blocked"
        value={piiBlocked.toLocaleString()}
        description="Memories rejected by privacy filter"
        icon={ShieldOff}
        loading={loading}
      />
    </div>
  );
}
