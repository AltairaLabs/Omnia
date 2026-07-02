"use client";

import { Loader2, AlertTriangle } from "lucide-react";
import { useArenaWorkloadModel } from "@/hooks/arena";
import { WorkloadGraph } from "@/components/workload-graph";

export function ArenaWorkloadView({ projectId }: Readonly<{ projectId: string }>) {
  const { model, loading, parseError } = useArenaWorkloadModel(projectId);

  if (loading && !model) {
    return (
      <div className="flex items-center justify-center h-full text-sm text-muted-foreground gap-2">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading workload…
      </div>
    );
  }

  if (!model || model.nodes.length === 0) {
    return (
      <div className="flex items-center justify-center h-full text-sm text-muted-foreground">
        No workload to show — add a prompt, workflow, or agents to the arena config.
      </div>
    );
  }

  return (
    <div className="h-full p-3 overflow-auto">
      {parseError && (
        <div className="mb-2 flex items-center gap-2 text-xs text-warning bg-warning/10 border border-warning/30 rounded px-2 py-1">
          <AlertTriangle className="h-3.5 w-3.5" />
          {parseError} — showing last valid workload.
        </div>
      )}
      <WorkloadGraph model={model} storageKey={`arena-workload:${projectId}`} />
    </div>
  );
}
