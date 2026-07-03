"use client";
// Memory Galaxy — workspace-scoped operator view of the memory projection.

import { useMemo, useState, type ReactNode } from "react";
import dynamic from "next/dynamic";
import { Header } from "@/components/layout";
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
import { Brain, Search, AlertCircle, Loader2 } from "lucide-react";
import { useMemoryProjection } from "@/hooks/use-memory-projection";
import { usePersistedViewMode } from "@/hooks/use-persisted-view-mode";
import { useServiceBannerCulprit } from "@/hooks/use-service-banner-culprit";
import { FacetRail, type Facet } from "@/components/memories/facet-rail";
import { facetCounts, parseHiddenTiers } from "@/lib/memory-galaxy/galaxy-math";
import { countMatches } from "@/lib/memory-galaxy/galaxy-search";
import {
  TIER_LABELS,
  TIER_DESCRIPTIONS,
} from "@/lib/memory-analytics/colors";
import { categoryColorVar, tierColorVar } from "@/lib/colors/category";
import type { GalaxyPoint } from "@/lib/memory-galaxy/types";
import type { Tier } from "@/lib/memory-analytics/types";
import { useDeleteMemory } from "@/hooks/use-memory-mutations";
import { EnterpriseGate } from "@/components/license/license-gate";
import { ServiceUnreadyBanner } from "@/components/sessions/service-unready-banner";

const MemoryGalaxy = dynamic(
  () => import("@/components/memories/memory-galaxy").then((m) => m.MemoryGalaxy),
  { ssr: false },
);

const TIER_KEYS: Tier[] = ["institutional", "agent", "user", "user_for_agent"];
const CATEGORY_KEYS = [
  "memory:identity",
  "memory:context",
  "memory:health",
  "memory:location",
  "memory:preferences",
  "memory:history",
];

function categoryLabel(cat: string): string {
  const s = cat.replace(/^memory:/, "");
  return s.charAt(0).toUpperCase() + s.slice(1);
}

function buildFacets(points: GalaxyPoint[], colorBy: "tier" | "category"): Facet[] {
  const counts = facetCounts(points, colorBy);
  if (colorBy === "tier") {
    return TIER_KEYS.map((t) => ({
      key: t,
      label: TIER_LABELS[t],
      color: tierColorVar(t),
      count: counts[t] ?? 0,
      description: TIER_DESCRIPTIONS[t],
    }));
  }
  return CATEGORY_KEYS.map((c) => ({
    key: c,
    label: categoryLabel(c),
    color: categoryColorVar(c),
    count: counts[c] ?? 0,
  }));
}

interface GalaxyBodyState {
  hasWorkspace: boolean;
  error: unknown;
  isLoading: boolean;
  /** A culprit service was already identified by the banner above — the
   * error alert and the loading skeleton both defer to it instead of
   * duplicating (or, for loading, hanging on) the same message. */
  hasCulprit: boolean;
  status?: "ready" | "pending";
  total: number;
  points: GalaxyPoint[];
  colorBy: "tier" | "category";
  hidden: Set<string>;
  search: string;
  onDelete: (id: string) => void;
}

function renderGalaxyBody(s: GalaxyBodyState): ReactNode {
  if (!s.hasWorkspace) {
    return (
      <Alert data-testid="no-workspace-notice">
        <AlertCircle className="h-4 w-4" />
        <AlertTitle>No workspace selected</AlertTitle>
        <AlertDescription>Select a workspace to view its memory galaxy.</AlertDescription>
      </Alert>
    );
  }
  if (s.error) {
    if (s.hasCulprit) return null;
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
    // A known culprit means the banner above already explains the hang —
    // an indefinite skeleton here would be the endless-spinner bug this
    // proactive check exists to fix.
    if (s.hasCulprit) return null;
    return <Skeleton className="h-[70vh] min-h-[360px] w-full rounded-lg" data-testid="galaxy-loading" />;
  }
  // Large workspace the pre-render worker hasn't finished yet — the hook polls
  // until the layout is ready; show progress instead of a misleading "empty".
  if (s.status === "pending") {
    return (
      <div
        className="flex h-[70vh] min-h-[360px] flex-col items-center justify-center text-muted-foreground"
        data-testid="galaxy-pending"
      >
        <Loader2 className="mb-4 h-12 w-12 animate-spin opacity-40" />
        <h3 className="mb-1 text-lg font-medium">Building galaxy…</h3>
        <p className="text-sm">
          Rendering {s.total.toLocaleString()} memories. This updates automatically.
        </p>
      </div>
    );
  }
  if (s.points.length === 0) {
    return (
      <div
        className="flex h-[70vh] min-h-[360px] flex-col items-center justify-center text-muted-foreground"
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
      hidden={s.hidden}
      filters={{ search: s.search }}
      onDelete={s.onDelete}
    />
  );
}

function MemoriesContent() {
  // Workspace-scoped, operator/demo view. The Next.js → memory-api proxy
  // authenticates service-to-service, so the browser user does NOT need a
  // personal memory identity — anonymous sessions with a workspace can view it.
  const { currentWorkspace } = useWorkspace();
  const hasWorkspace = !!currentWorkspace;
  const { data, isLoading, error } = useMemoryProjection();
  // A hung memory-api never surfaces an error on its own — check the
  // culprit banner proactively (while loading, not just on error) so a
  // dead backend doesn't leave the galaxy spinning forever.
  const { bannerCulprit, setBannerCulprit, showBanner } = useServiceBannerCulprit(
    currentWorkspace?.name,
    error,
    isLoading
  );
  const hasCulprit = bannerCulprit === true;

  const [searchQuery, setSearchQuery] = useState("");
  const [colorBy, setColorBy] = usePersistedViewMode<"tier" | "category">(
    "omnia-memory-galaxy-color-by",
    "tier",
  );
  const [hiddenTiersCsv, setHiddenTiersCsv] = usePersistedViewMode<string>(
    "omnia-memory-galaxy-hidden-tiers",
    "",
  );
  const [hiddenCatsCsv, setHiddenCatsCsv] = usePersistedViewMode<string>(
    "omnia-memory-galaxy-hidden-categories",
    "",
  );

  const points = useMemo(() => data?.points ?? [], [data?.points]);
  const matchCount = useMemo(() => countMatches(points, searchQuery), [points, searchQuery]);
  // The active filter dimension follows the color dropdown.
  const hiddenCsv = colorBy === "tier" ? hiddenTiersCsv : hiddenCatsCsv;
  const setHiddenCsv = colorBy === "tier" ? setHiddenTiersCsv : setHiddenCatsCsv;
  const hidden = useMemo(() => parseHiddenTiers(hiddenCsv), [hiddenCsv]);
  const facets = useMemo(() => buildFacets(points, colorBy), [points, colorBy]);

  const toggleFacet = (key: string) => {
    const next = new Set(hidden);
    if (next.has(key)) next.delete(key);
    else next.add(key);
    setHiddenCsv([...next].sort((a, b) => a.localeCompare(b)).join(","));
  };

  const deleteMemory = useDeleteMemory();
  const handleDelete = (id: string) => deleteMemory.mutate(id);

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

            <Select value={colorBy} onValueChange={(v) => setColorBy(v as "tier" | "category")}>
              <SelectTrigger className="w-[160px]" data-testid="color-by">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="tier">Color: tier</SelectItem>
                <SelectItem value="category">Color: category</SelectItem>
              </SelectContent>
            </Select>
          </div>
        )}

        {hasWorkspace && <FacetRail facets={facets} hidden={hidden} onToggle={toggleFacet} />}

        {showBanner && currentWorkspace && (
          <ServiceUnreadyBanner
            workspaceName={currentWorkspace.name}
            resourceLabel="memories"
            onResult={setBannerCulprit}
          />
        )}

        {renderGalaxyBody({
          hasWorkspace,
          error,
          isLoading,
          hasCulprit,
          status: data?.status,
          total: data?.total ?? 0,
          points,
          colorBy,
          hidden,
          search: searchQuery,
          onDelete: handleDelete,
        })}

        {!isLoading && data?.status !== "pending" && (data?.total ?? 0) > 0 && (
          <p className="text-center text-xs text-muted-foreground">
            {data?.total} memories · {clusterKind} clustering
            {data?.capped ? " (showing a capped subset)" : ""}
            {searchQuery.trim() ? ` · ${matchCount} match${matchCount === 1 ? "" : "es"}` : ""}
          </p>
        )}
      </div>
    </div>
  );
}

export default function MemoriesPage() {
  return (
    <EnterpriseGate featureName="Memory Galaxy">
      <MemoriesContent />
    </EnterpriseGate>
  );
}
