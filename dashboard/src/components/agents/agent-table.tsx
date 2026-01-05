"use client";

import Link from "next/link";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { StatusBadge } from "./status-badge";
import { FrameworkBadge } from "./framework-badge";
import { CostBadge } from "@/components/cost";
import { getMockAgentUsage } from "@/lib/mock-data";
import type { AgentRuntime } from "@/types";

interface AgentTableProps {
  agents: AgentRuntime[];
}

function formatAge(timestamp?: string): string {
  if (!timestamp) return "-";

  const now = new Date();
  const created = new Date(timestamp);
  const diff = now.getTime() - created.getTime();

  const days = Math.floor(diff / (1000 * 60 * 60 * 24));
  const hours = Math.floor((diff % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60));

  if (days > 0) return `${days}d`;
  if (hours > 0) return `${hours}h`;
  return "<1h";
}

export function AgentTable({ agents }: AgentTableProps) {
  return (
    <div className="rounded-md border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Namespace</TableHead>
            <TableHead>Status</TableHead>
            <TableHead>Framework</TableHead>
            <TableHead>Replicas</TableHead>
            <TableHead>Provider</TableHead>
            <TableHead>Cost (24h)</TableHead>
            <TableHead>Age</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {agents.map((agent) => (
            <TableRow key={agent.metadata.uid}>
              <TableCell>
                <Link
                  href={`/agents/${agent.metadata.name}?namespace=${agent.metadata.namespace}`}
                  className="font-medium hover:underline"
                >
                  {agent.metadata.name}
                </Link>
              </TableCell>
              <TableCell className="text-muted-foreground">
                {agent.metadata.namespace}
              </TableCell>
              <TableCell>
                <StatusBadge phase={agent.status?.phase} />
              </TableCell>
              <TableCell>
                <FrameworkBadge framework={agent.spec.framework?.type} />
              </TableCell>
              <TableCell>
                {agent.status?.replicas?.ready ?? 0}/
                {agent.status?.replicas?.desired ?? agent.spec.runtime?.replicas ?? 1}
              </TableCell>
              <TableCell className="capitalize">
                {agent.spec.provider?.type || "claude"}
              </TableCell>
              <TableCell>
                {(() => {
                  const usage = getMockAgentUsage(
                    agent.metadata.namespace || "default",
                    agent.metadata.name || ""
                  );
                  if (!usage) return <span className="text-muted-foreground">-</span>;
                  return (
                    <CostBadge
                      inputTokens={usage.inputTokens}
                      outputTokens={usage.outputTokens}
                      model={usage.model}
                    />
                  );
                })()}
              </TableCell>
              <TableCell className="text-muted-foreground">
                {formatAge(agent.metadata.creationTimestamp)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
