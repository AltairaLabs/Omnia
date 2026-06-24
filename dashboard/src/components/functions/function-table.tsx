"use client";

/**
 * FunctionTable — the table/list view of the /functions catalog, mirroring
 * AgentTable but with function-specific columns (input/output field counts in
 * place of replicas). Cost reuses the same Prometheus-backed hook.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { StatusBadge } from "@/components/agents/status-badge";
import { FrameworkBadge } from "@/components/agents/framework-badge";
import { CostBadge } from "@/components/cost";
import { useAgentCost } from "@/hooks/agents";
import { useProvider } from "@/hooks/resources";
import { useWorkspace } from "@/contexts/workspace-context";
import { getDefaultProviderRef } from "@/types/agent-runtime";
import type { AgentRuntime } from "@/types";
import { schemaFieldCount } from "./function-card";

interface FunctionTableProps {
  functions: AgentRuntime[];
}

/** Provider cell — resolves the provider TYPE (e.g. "anthropic"), matching the
 * card. Falls back to the ref name, then "-". */
function FunctionProviderCell({
  namespace,
  providerRefName,
}: Readonly<{ namespace: string; providerRefName?: string }>) {
  const { data: provider } = useProvider(providerRefName, namespace);
  return <span className="capitalize">{provider?.spec?.type || providerRefName || "-"}</span>;
}

function formatAge(timestamp?: string): string {
  if (!timestamp) return "-";
  const diff = new Date().getTime() - new Date(timestamp).getTime();
  const days = Math.floor(diff / (1000 * 60 * 60 * 24));
  const hours = Math.floor((diff % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60));
  if (days > 0) return `${days}d`;
  if (hours > 0) return `${hours}h`;
  return "<1h";
}

function fieldsLabel(n: number): string {
  return `${n} field${n === 1 ? "" : "s"}`;
}

/** Cost cell, reusing the Prometheus-backed agent cost hook. Cost is keyed by
 * workspace NAME (the API resolves the backing namespace); the page is
 * workspace-scoped, so the current workspace applies to every row (#1572). */
function FunctionCostCell({ name }: Readonly<{ name: string }>) {
  const { currentWorkspace } = useWorkspace();
  const { data: costData } = useAgentCost(currentWorkspace?.name ?? "", name);
  if (!costData?.available) {
    return <span className="text-muted-foreground">-</span>;
  }
  return <CostBadge inputTokens={costData.inputTokens} outputTokens={costData.outputTokens} model="" />;
}

export function FunctionTable({ functions }: Readonly<FunctionTableProps>) {
  const router = useRouter();
  return (
    <div className="rounded-md border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Namespace</TableHead>
            <TableHead>Status</TableHead>
            <TableHead>Framework</TableHead>
            <TableHead>Input</TableHead>
            <TableHead>Output</TableHead>
            <TableHead>Provider</TableHead>
            <TableHead>Cost (24h)</TableHead>
            <TableHead>Age</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {functions.map((fn) => {
            const href = `/functions/${fn.metadata.name}?namespace=${fn.metadata.namespace ?? "default"}`;
            return (
              <TableRow
                key={fn.metadata.uid ?? fn.metadata.name}
                data-testid="function-row"
                className="cursor-pointer"
                onClick={() => router.push(href)}
              >
                <TableCell>
                  <Link
                    href={href}
                    className="font-medium hover:underline"
                    onClick={(e) => e.stopPropagation()}
                  >
                    {fn.metadata.name}
                  </Link>
                </TableCell>
                <TableCell className="text-muted-foreground">{fn.metadata.namespace}</TableCell>
                <TableCell>
                  <StatusBadge phase={fn.status?.phase} />
                </TableCell>
                <TableCell>
                  <FrameworkBadge framework={fn.spec.framework?.type} />
                </TableCell>
                <TableCell>{fieldsLabel(schemaFieldCount(fn.spec.inputSchema))}</TableCell>
                <TableCell>{fieldsLabel(schemaFieldCount(fn.spec.outputSchema))}</TableCell>
                <TableCell>
                  <FunctionProviderCell
                    namespace={fn.metadata.namespace || "default"}
                    providerRefName={getDefaultProviderRef(fn.spec)?.name}
                  />
                </TableCell>
                <TableCell>
                  <FunctionCostCell name={fn.metadata.name || ""} />
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {formatAge(fn.metadata.creationTimestamp)}
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </div>
  );
}
