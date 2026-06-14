"use client";

import { useMemo, useState, type ReactNode } from "react";
import Link from "next/link";
import dynamic from "next/dynamic";
import { Header } from "@/components/layout";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useWorkspace } from "@/contexts/workspace-context";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Brain, Search, AlertCircle, Library } from "lucide-react";
import { useMemoryProjection } from "@/hooks/use-memory-projection";
import { usePersistedViewMode } from "@/hooks/use-persisted-view-mode";
import { MemoryDetailPanel } from "@/components/memories/memory-detail-panel";
import { TierRail } from "@/components/memories/tier-rail";
import {
  legendCounts,
  parseHiddenTiers,
  serializeHiddenTiers,
} from "@/lib/memory-galaxy/galaxy-math";
import type { GalaxyPoint } from "@/lib/memory-galaxy/types";
import type { Tier } from "@/lib/memory-analytics/types";
import type { MemoryEntity } from "@/lib/data/types";
import { useDeleteMemory } from "@/hooks/use-memory-mutations";

const MemoryGalaxy = dynamic(
  () => import("@/components/memories/memory-galaxy").then((m) => m.MemoryGalaxy),
  { ssr: false },
);

const CATEGORIES = [
  { value: "all", label: "All Categories" },
  { value: "memory:identity", label: "Identity" },
  { value: "memory:context", label: "Context" },
  { value: "memory:health", label: "Health" },
  { value: "memory:location", label: "Location" },
  { value: "memory:preferences", label: "Preferences" },
  { value: "memory:history", label: "History" },
];

function pointToMemory(p: GalaxyPoint): MemoryEntity {
  return {
    id: p.id,
    type: "observation",
    content: p.preview ?? p.title ?? "",
    confidence: p.confidence,
    scope: {},
    metadata: { consent_category: p.category },
    createdAt: p.observedAt ?? "",
    tier: p.tier,
  };
}

interface GalaxyBodyState {
  hasWorkspace: boolean;
  error: unknown;
  isLoading: boolean;
  points: GalaxyPoint[];
  colorBy: "tier" | "category";
  hidden: Set<string>;
  filters: { category: string; search: string };
  onSelect: (p: GalaxyPoint) => void;
}

function renderGalaxyBody(s: GalaxyBodyState): ReactNode {
  if (!s.hasWorkspace) {
    return (
      <Alert data-testid="no-workspace-notice">
        <AlertCircle className="h-4 w-4" />
        <AlertTitle>No workspace selected</AlertTitle>
        <AlertDescription>
          Select a workspace to view its memory galaxy.
        </AlertDescription>
      </Alert>
    );
  }
  if (s.error) {
    return (
      <Alert variant="destructive" data-testid="memory-error">
        <AlertCircle className="h-4 w-4" />
        <AlertTitle>Could not load the memory galaxy</AlertTitle>
        <AlertDescription>
          {s.error instanceof Error ? s.error.message : "Failed to reach the Memory API."}
        </AlertDescription>
      </Alert>
    );
  }
  if (s.isLoading) {
    return <Skeleton className="h-[600px] w-full rounded-lg" data-testid="galaxy-loading" />;
  }
  if (s.points.length === 0) {
    return (
      <div
        className="flex h-[400px] flex-col items-center justify-center text-muted-foreground"
        data-testid="empty-state"
      >
        <Brain className="mb-4 h-16 w-16 opacity-30" />
        <h3 className="mb-1 text-lg font-medium">No memories yet</h3>
        <p className="text-sm">As your agents interact, memories appear here.</p>
      </div>
    );
  }
  return (
    <MemoryGalaxy
      points={s.points}
      colorBy={s.colorBy}
      hiddenTiers={s.hidden}
      filters={s.filters}
      onSelect={s.onSelect}
    />
  );
}

export default function MemoriesPage() {
  // Workspace-scoped, operator/demo view. The Next.js → memory-api proxy
  // authenticates service-to-service, so the browser user does NOT need a
  // personal memory identity — anonymous sessions with a workspace can view it.
  const { currentWorkspace } = useWorkspace();
  const hasWorkspace = !!currentWorkspace;
  const { data, isLoading, error } = useMemoryProjection();

  const [selected, setSelected] = useState<GalaxyPoint | null>(null);
  const [categoryFilter, setCategoryFilter] = useState("all");
  const [searchQuery, setSearchQuery] = useState("");
  const [colorBy, setColorBy] = usePersistedViewMode<"tier" | "category">(
    "omnia-memory-galaxy-color-by",
    "tier",
  );
  const [hiddenCsv, setHiddenCsv] = usePersistedViewMode<string>(
    "omnia-memory-galaxy-hidden-tiers",
    "",
  );

  const points = useMemo(() => data?.points ?? [], [data?.points]);
  const hidden = useMemo(() => parseHiddenTiers(hiddenCsv), [hiddenCsv]);
  const counts = useMemo(() => legendCounts(points), [points]);

  const toggleTier = (tier: Tier) => {
    const next = new Set(hidden);
    if (next.has(tier)) next.delete(tier);
    else next.add(tier);
    setHiddenCsv(serializeHiddenTiers(next));
  };

  const deleteMemory = useDeleteMemory();
  const handleDelete = (id: string) => {
    deleteMemory.mutate(id);
    setSelected(null);
  };

  const clusterKind = data?.projectionInput === "tfidf" ? "lexical" : "semantic";

  return (
    <div className="flex h-full flex-col">
      <Header
        title="Memory Galaxy"
        description="A semantic map of everything your agents remember, across all four tiers."
      />

      <div className="flex-1 space-y-4 overflow-auto p-6">
        {hasWorkspace && (
          <div className="flex flex-wrap items-center gap-3" data-testid="memories-toolbar">
            <div className="relative min-w-[200px] max-w-sm flex-1">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Search memories..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="pl-9"
                data-testid="memory-search"
              />
            </div>

            <Select value={categoryFilter} onValueChange={setCategoryFilter}>
              <SelectTrigger className="w-[180px]" data-testid="category-filter">
                <SelectValue placeholder="Category" />
              </SelectTrigger>
              <SelectContent>
                {CATEGORIES.map((cat) => (
                  <SelectItem key={cat.value} value={cat.value}>
                    {cat.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            <Select value={colorBy} onValueChange={(v) => setColorBy(v as "tier" | "category")}>
              <SelectTrigger className="w-[150px]" data-testid="color-by">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="tier">Color: tier</SelectItem>
                <SelectItem value="category">Color: category</SelectItem>
              </SelectContent>
            </Select>

            <div className="flex-1" />

            {currentWorkspace && (
              <Button asChild variant="outline" size="sm" data-testid="workspace-knowledge-link">
                <Link href={`/workspaces/${encodeURIComponent(currentWorkspace.name)}/knowledge`}>
                  <Library className="mr-2 h-4 w-4" />
                  Workspace knowledge
                </Link>
              </Button>
            )}
          </div>
        )}

        {hasWorkspace && <TierRail counts={counts} hidden={hidden} onToggle={toggleTier} />}

        {renderGalaxyBody({
          hasWorkspace,
          error,
          isLoading,
          points,
          colorBy,
          hidden,
          filters: { category: categoryFilter, search: searchQuery },
          onSelect: setSelected,
        })}

        {!isLoading && (data?.total ?? 0) > 0 && (
          <p className="text-center text-xs text-muted-foreground">
            {data?.total} memories · {clusterKind} clustering
            {data?.capped ? " (showing a capped subset)" : ""}
          </p>
        )}
      </div>

      <MemoryDetailPanel
        memory={selected ? pointToMemory(selected) : null}
        onClose={() => setSelected(null)}
        onDelete={handleDelete}
      />
    </div>
  );
}
