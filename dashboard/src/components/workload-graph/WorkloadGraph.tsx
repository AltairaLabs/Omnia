"use client";

import { useEffect, useMemo, useState } from "react";
import {
  ReactFlow,
  Background,
  Controls,
  BackgroundVariant,
  useNodesState,
  useEdgesState,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { Info } from "lucide-react";
import { modelToFlow } from "./to-flow";
import { layoutFlow } from "./layout";
import { workloadNodeTypes } from "./workload-nodes";
import { NodeDrawer } from "./node-drawer";
import type { WorkloadModel, WorkloadNode, WorkloadBudget } from "./types";

function budgetLabel(b: WorkloadBudget): string {
  const parts: string[] = [];
  if (b.maxTotalVisits != null) parts.push(`≤${b.maxTotalVisits} visits`);
  if (b.maxToolCalls != null) parts.push(`≤${b.maxToolCalls} tools`);
  if (b.maxWallTimeSec != null) parts.push(`≤${b.maxWallTimeSec}s`);
  return parts.join(" · ");
}

function deploymentBanner(model: WorkloadModel): string {
  const models = model.meta.binding?.providers.map((p) => p.model || p.name).join(", ");
  return `Deployment · resolved against ${models || "—"}`;
}

export function WorkloadGraph({
  model,
  className,
}: Readonly<{ model: WorkloadModel; className?: string }>) {
  const [selectedId, setSelectedId] = useState<string | undefined>();
  const flow = useMemo(() => modelToFlow(model, setSelectedId), [model]);
  const [nodes, setNodes] = useNodesState(flow.nodes);
  const [edges, setEdges] = useEdgesState(flow.edges);

  useEffect(() => {
    let cancelled = false;
    layoutFlow(flow.nodes, flow.edges).then((laid) => {
      if (!cancelled) {
        setNodes(laid);
        setEdges(flow.edges);
      }
    });
    return () => { cancelled = true; };
  }, [flow, setNodes, setEdges]);

  const selected: WorkloadNode | undefined = model.nodes.find((n) => n.id === selectedId);
  const banner =
    model.altitude === "deployment" ? deploymentBanner(model) : "Definition · abstract workload";

  if (model.nodes.length === 0) {
    return (
      <div className="flex items-center justify-center h-[300px] text-sm text-muted-foreground border rounded-lg">
        No workload to display — this pack declares no prompts.
      </div>
    );
  }

  return (
    <div className={className}>
      <div className="flex items-center gap-1.5 text-xs text-muted-foreground mb-2">
        <Info className="h-3.5 w-3.5" />
        <span>{banner}</span>
      </div>
      <div className="relative border rounded-lg" style={{ width: "100%", height: "560px" }}>
        <ReactFlow
          nodes={nodes}
          edges={edges}
          nodeTypes={workloadNodeTypes}
          fitView
          fitViewOptions={{ padding: 0.2 }}
          minZoom={0.2}
          maxZoom={2}
          defaultEdgeOptions={{ type: "smoothstep" }}
        >
          <Controls />
          <Background variant={BackgroundVariant.Dots} gap={12} size={1} />
        </ReactFlow>
        <NodeDrawer node={selected} onClose={() => setSelectedId(undefined)} />
      </div>
      {model.meta.budget && (
        <div className="text-xs text-muted-foreground mt-2">
          budget: {budgetLabel(model.meta.budget)}
        </div>
      )}
    </div>
  );
}
