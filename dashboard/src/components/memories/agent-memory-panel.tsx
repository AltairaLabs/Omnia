"use client";

/**
 * AgentMemoryPanel — read-only browser for (workspace, agent)-scoped memory.
 *
 * Renders the rows the agent has accumulated across sessions (resolution
 * patterns, learned playbooks, etc.) as a flat list with tier + category
 * badges. Mounted on the agent detail page's Memory tab. Read-only for v1
 * — operator-curated agent memories are written via the memory-api directly
 * (or by the runtime as it learns); editing them in-dashboard is a follow-up.
 */

import { Brain } from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@/components/ui/alert";
import { CategoryBadge } from "./category-badge";
import { TierBadge } from "./tier-badge";
import { useAgentMemories } from "@/hooks/use-agent-memories";
import type { MemoryEntity } from "@/lib/data/types";

interface AgentMemoryPanelProps {
  agentId: string | undefined;
}

export function AgentMemoryPanel({ agentId }: Readonly<AgentMemoryPanelProps>) {
  const { data, isLoading, error } = useAgentMemories({ agentId });

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Brain className="h-5 w-5" />
          Agent memory
        </CardTitle>
        <CardDescription>
          Patterns and facts this agent has accumulated across sessions, shared
          with every user it talks to. Read-only — entries land here from the
          runtime as the agent learns.
        </CardDescription>
      </CardHeader>
      <CardContent>{renderBody({ isLoading, error, data })}</CardContent>
    </Card>
  );
}

interface BodyProps {
  isLoading: boolean;
  error: unknown;
  data: { memories: MemoryEntity[]; total: number } | undefined;
}

function renderBody({ isLoading, error, data }: BodyProps) {
  if (error) {
    return (
      <Alert variant="destructive" data-testid="agent-memory-error">
        <AlertTitle>Could not load</AlertTitle>
        <AlertDescription>
          {error instanceof Error
            ? error.message
            : "Failed to load agent memories."}
        </AlertDescription>
      </Alert>
    );
  }
  if (isLoading) {
    return <Skeleton className="w-full h-32 rounded" />;
  }
  const memories = data?.memories ?? [];
  if (memories.length === 0) {
    return (
      <p
        className="text-sm text-muted-foreground"
        data-testid="agent-memory-empty"
      >
        No memories for this agent yet. They will appear here as the agent
        accumulates patterns from sessions, or when an operator seeds them via
        the memory-api directly.
      </p>
    );
  }
  return (
    <ul className="space-y-2" data-testid="agent-memory-list">
      {memories.map((m) => (
        <AgentMemoryRow key={m.id} memory={m} />
      ))}
    </ul>
  );
}

function AgentMemoryRow({ memory }: { memory: MemoryEntity }) {
  const category = memory.metadata?.consent_category as string | undefined;
  const confidence = Math.round((memory.confidence ?? 0) * 100);
  return (
    <li className="rounded border bg-card p-3 space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        <TierBadge tier={memory.tier} />
        <CategoryBadge category={category} />
        <span className="text-xs font-mono text-muted-foreground">
          {memory.type}
        </span>
        <span className="ml-auto text-xs text-muted-foreground">
          {confidence}% confidence
        </span>
      </div>
      <p className="text-sm whitespace-pre-wrap break-words">{memory.content}</p>
    </li>
  );
}
