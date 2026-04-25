"use client";

import { useState, type ReactNode } from "react";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Brain, LogIn } from "lucide-react";
import Link from "next/link";
import { useAuth } from "@/hooks/use-auth";
import { useMemories } from "@/hooks/use-memories";
import { MemoryCard } from "./memory-card";
import { Skeleton } from "@/components/ui/skeleton";
import type { MemoryEntity } from "@/lib/data/types";
import { TIERS, type Tier } from "@/lib/memory-analytics/types";
import { TIER_LABELS } from "@/lib/memory-analytics/colors";

interface MemorySidebarProps {
  agentName: string;
  open: boolean;
  onClose: () => void;
}

const SKELETON_KEYS = ["sk-a", "sk-b", "sk-c"];

type TierFilter = Tier | "all";

const TIER_FILTERS: TierFilter[] = ["all", ...TIERS];

const TIER_FILTER_LABELS: Record<TierFilter, string> = {
  all: "All",
  ...TIER_LABELS,
};

function LoadingSkeletons() {
  return (
    <div className="space-y-2">
      {SKELETON_KEYS.map((key) => (
        <Skeleton key={key} className="h-20 w-full rounded-lg" />
      ))}
    </div>
  );
}

function EmptyState() {
  return (
    <div className="flex flex-col items-center justify-center h-40 text-muted-foreground">
      <Brain className="h-8 w-8 mb-2 opacity-30" />
      <p className="text-sm">No memories yet</p>
    </div>
  );
}

function AnonymousNotice() {
  return (
    <Alert className="mt-2" data-testid="memory-sidebar-anonymous-notice">
      <LogIn className="h-4 w-4" />
      <AlertTitle>Memories require sign-in</AlertTitle>
      <AlertDescription>
        Memories are scoped to authenticated identities so each user&apos;s data
        stays private. Sign in to start saving memories.
      </AlertDescription>
    </Alert>
  );
}

function TierFilterChips({
  active,
  onChange,
}: {
  active: TierFilter;
  onChange: (next: TierFilter) => void;
}) {
  return (
    <div className="flex flex-wrap gap-1 px-4 pb-2" data-testid="tier-filter-chips">
      {TIER_FILTERS.map((tier) => (
        <Button
          key={tier}
          size="sm"
          variant={tier === active ? "default" : "outline"}
          onClick={() => onChange(tier)}
          data-testid={`tier-filter-${tier}`}
        >
          {TIER_FILTER_LABELS[tier]}
        </Button>
      ))}
    </div>
  );
}

function applyTierFilter(memories: MemoryEntity[], filter: TierFilter): MemoryEntity[] {
  if (filter === "all") return memories;
  return memories.filter((m) => m.tier === filter);
}

function renderBody(
  isAuthenticated: boolean,
  isLoading: boolean,
  memories: MemoryEntity[]
): ReactNode {
  if (!isAuthenticated) return <AnonymousNotice />;
  if (isLoading) return <LoadingSkeletons />;
  if (memories.length === 0) return <EmptyState />;
  return (
    <div className="space-y-1 pb-4">
      {memories.map((memory) => (
        <MemoryCard key={memory.id} memory={memory} />
      ))}
    </div>
  );
}

export function MemorySidebar({ agentName: _agentName, open, onClose }: MemorySidebarProps) {
  const { hasMemoryIdentity, memoryUserId } = useAuth();
  const { data, isLoading } = useMemories({
    userId: memoryUserId,
    enabled: hasMemoryIdentity,
  });
  const [tierFilter, setTierFilter] = useState<TierFilter>("all");

  const memories = data?.memories ?? [];
  const filtered = applyTierFilter(memories, tierFilter);

  return (
    <Sheet open={open} onOpenChange={(o) => { if (!o) onClose(); }}>
      <SheetContent data-testid="memory-sidebar" className="w-[350px] sm:w-[400px] p-0">
        <SheetHeader className="p-4 pb-2">
          <SheetTitle className="flex items-center gap-2 text-base">
            <Brain className="h-4 w-4" />
            Agent Memories
          </SheetTitle>
          <p className="text-xs text-muted-foreground">
            What the agent remembers about you
          </p>
        </SheetHeader>

        {hasMemoryIdentity && (
          <TierFilterChips active={tierFilter} onChange={setTierFilter} />
        )}

        <ScrollArea className="h-[calc(100vh-160px)] px-4">
          {renderBody(hasMemoryIdentity, isLoading, filtered)}
        </ScrollArea>

        <div className="border-t p-3">
          <Link
            href="/memories"
            className="text-sm text-primary hover:underline flex items-center gap-1"
            data-testid="view-all-memories"
          >
            View all memories →
          </Link>
        </div>
      </SheetContent>
    </Sheet>
  );
}
