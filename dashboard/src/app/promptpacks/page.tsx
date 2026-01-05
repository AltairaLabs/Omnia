"use client";

import { useState } from "react";
import { Plus } from "lucide-react";
import { Header } from "@/components/layout";
import { PromptPackCard } from "@/components/promptpacks";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Skeleton } from "@/components/ui/skeleton";
import { usePromptPacks } from "@/hooks";
import type { PromptPackPhase } from "@/types";

type FilterPhase = "all" | PromptPackPhase;

export default function PromptPacksPage() {
  const [filterPhase, setFilterPhase] = useState<FilterPhase>("all");

  const { data: promptPacks, isLoading } = usePromptPacks();

  const filteredPacks =
    filterPhase === "all"
      ? promptPacks
      : promptPacks?.filter((p) => p.status?.phase === filterPhase);

  const phaseCounts = promptPacks?.reduce(
    (acc, pack) => {
      const phase = pack.status?.phase;
      if (phase === "Active") acc.active++;
      else if (phase === "Canary") acc.canary++;
      else if (phase === "Failed") acc.failed++;
      else if (phase === "Pending") acc.pending++;
      return acc;
    },
    { active: 0, canary: 0, pending: 0, failed: 0 }
  );

  return (
    <div className="flex flex-col h-full">
      <Header
        title="PromptPacks"
        description="Manage your prompt templates and configurations"
      />

      <div className="flex-1 p-6 space-y-6">
        {/* Toolbar */}
        <div className="flex items-center justify-between">
          <Tabs
            value={filterPhase}
            onValueChange={(v) => setFilterPhase(v as FilterPhase)}
          >
            <TabsList>
              <TabsTrigger value="all">
                All ({promptPacks?.length ?? 0})
              </TabsTrigger>
              <TabsTrigger value="Active">
                Active ({phaseCounts?.active ?? 0})
              </TabsTrigger>
              <TabsTrigger value="Canary">
                Canary ({phaseCounts?.canary ?? 0})
              </TabsTrigger>
              <TabsTrigger value="Pending">
                Pending ({phaseCounts?.pending ?? 0})
              </TabsTrigger>
              <TabsTrigger value="Failed">
                Failed ({phaseCounts?.failed ?? 0})
              </TabsTrigger>
            </TabsList>
          </Tabs>

          <Button size="sm">
            <Plus className="mr-2 h-4 w-4" />
            New PromptPack
          </Button>
        </div>

        {/* Content */}
        {isLoading ? (
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
            {[...Array(4)].map((_, i) => (
              <Skeleton key={i} className="h-[200px] rounded-lg" />
            ))}
          </div>
        ) : (
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
            {filteredPacks?.map((pack) => (
              <PromptPackCard key={pack.metadata.uid} promptPack={pack} />
            ))}
          </div>
        )}

        {!isLoading && filteredPacks?.length === 0 && (
          <div className="flex flex-col items-center justify-center py-12 text-center">
            <p className="text-muted-foreground">No PromptPacks found</p>
            <Button variant="outline" className="mt-4">
              <Plus className="mr-2 h-4 w-4" />
              Create your first PromptPack
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}
