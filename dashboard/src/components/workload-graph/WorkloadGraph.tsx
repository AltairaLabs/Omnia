"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import {
  ReactFlow,
  Background,
  Controls,
  Panel,
  BackgroundVariant,
  MarkerType,
  useNodesState,
  useEdgesState,
  type ReactFlowInstance,
  type Node as FlowNode,
  type Edge as FlowEdge,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { Info } from "lucide-react";
import { modelToFlow, type WorkloadNodeData } from "./to-flow";
import { layoutFlow } from "./layout";
import { workloadNodeTypes } from "./workload-nodes";
import { NodeDrawer } from "./node-drawer";
import { usePersistedNodeLayout } from "@/hooks/use-persisted-node-layout";
import type { WorkloadModel, WorkloadNode, WorkloadBudget } from "./types";

type WorkloadFlowInstance = ReactFlowInstance<FlowNode<WorkloadNodeData>, FlowEdge>;

// elk repositions nodes after mount; re-fit once they're measured (next paint)
// so the whole graph scales to the canvas instead of staying at the initial zoom.
export function fitViewAfterPaint(inst: WorkloadFlowInstance | null): void {
  if (!inst) return;
  requestAnimationFrame(() =>
    requestAnimationFrame(() => inst.fitView({ padding: 0.08, duration: 250 })),
  );
}

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

function testBanner(model: WorkloadModel): string {
  const providers = model.nodes.filter((n) => n.kind === "provider").length;
  const scenarioNode = model.nodes.find((n) => n.kind === "scenario");
  const scenarios = scenarioNode?.detail.scenarios?.length ?? 0;
  const p = `${providers} provider${providers === 1 ? "" : "s"}`;
  const s = `${scenarios} scenario${scenarios === 1 ? "" : "s"}`;
  return `Arena test topology · ${p} × ${s}`;
}

function bannerLabel(model: WorkloadModel): string {
  if (model.altitude === "deployment") return deploymentBanner(model);
  if (model.altitude === "test") return testBanner(model);
  return "Definition · abstract workload";
}

export function WorkloadGraph({
  model,
  className,
  namespace,
  storageKey,
}: Readonly<{ model: WorkloadModel; className?: string; namespace?: string; storageKey?: string }>) {
  const [selectedId, setSelectedId] = useState<string | undefined>();
  const [showData, setShowData] = useState(true);
  const flow = useMemo(() => modelToFlow(model, setSelectedId), [model]);
  const [nodes, setNodes, onNodesChange] = useNodesState(flow.nodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(flow.edges);
  const rf = useRef<WorkloadFlowInstance | null>(null);
  const { applyLayout, onNodeDragStop, reset } = usePersistedNodeLayout(storageKey ?? "");
  const [layoutNonce, setLayoutNonce] = useState(0);

  // Fill the viewport below the graph's own top edge, so the box accounts for
  // whatever sits above it (the dev licensing banner, page header, tabs) instead
  // of guessing with a vh fraction that overshoots.
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [height, setHeight] = useState(560);
  useEffect(() => {
    const compute = () => {
      const el = containerRef.current;
      if (!el) return;
      const top = el.getBoundingClientRect().top;
      setHeight(Math.max(360, window.innerHeight - top - 16));
    };
    compute();
    const raf = requestAnimationFrame(compute);
    window.addEventListener("resize", compute);
    return () => {
      cancelAnimationFrame(raf);
      window.removeEventListener("resize", compute);
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    layoutFlow(flow.nodes, flow.edges).then(({ nodes: laid }) => {
      if (cancelled) return;
      // overlay any saved drag positions on top of the fresh elk layout
      setNodes(applyLayout(laid));
      setEdges(flow.edges);
      fitViewAfterPaint(rf.current);
    });
    return () => { cancelled = true; };
  }, [flow, setNodes, setEdges, applyLayout, layoutNonce]);

  // The data-flow toggle HIDES variable/artifact nodes (and their edges) rather
  // than rebuilding the graph — so node positions, including manual drags, survive
  // the toggle. The UML ●/◉ markers are control flow and always stay.
  const dataNodeIds = useMemo(
    () => new Set(model.nodes.filter((n) => n.kind === "variable" || n.kind === "artifact").map((n) => n.id)),
    [model],
  );
  const displayNodes = useMemo(
    () => nodes.map((n) => (dataNodeIds.has(n.id) ? { ...n, hidden: !showData } : n)),
    [nodes, dataNodeIds, showData],
  );
  const displayEdges = useMemo(
    () =>
      edges.map((e) =>
        dataNodeIds.has(e.source) || dataNodeIds.has(e.target) ? { ...e, hidden: !showData } : e,
      ),
    [edges, dataNodeIds, showData],
  );

  const selected: WorkloadNode | undefined = model.nodes.find((n) => n.id === selectedId);
  const hasData = dataNodeIds.size > 0;
  const banner = bannerLabel(model);

  if (model.nodes.length === 0) {
    return (
      <div className="flex items-center justify-center h-[300px] text-sm text-muted-foreground border rounded-lg">
        No workload to display — this pack declares no prompts.
      </div>
    );
  }

  return (
    <div className={className}>
      <div
        ref={containerRef}
        className="relative border rounded-lg"
        style={{ width: "100%", height: `${height}px` }}
      >
        <ReactFlow
          nodes={displayNodes}
          edges={displayEdges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onNodeDragStop={onNodeDragStop}
          nodeTypes={workloadNodeTypes}
          onInit={(inst) => { rf.current = inst; }}
          fitView
          fitViewOptions={{ padding: 0.08 }}
          minZoom={0.2}
          maxZoom={2}
          defaultEdgeOptions={{
            markerEnd: { type: MarkerType.ArrowClosed, width: 18, height: 18 },
          }}
        >
          <Panel position="top-left">
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground bg-background/85 backdrop-blur rounded border px-2 py-1">
              <Info className="h-3.5 w-3.5" />
              <span>{banner}</span>
            </div>
          </Panel>
          {model.meta.budget && (
            <Panel position="top-right">
              <div className="text-xs bg-background/85 backdrop-blur rounded border px-2 py-1">
                <span className="font-medium text-foreground">Budget</span>{" "}
                <span className="text-muted-foreground">{budgetLabel(model.meta.budget)}</span>
              </div>
            </Panel>
          )}
          {hasData && (
            <Panel position="bottom-left">
              <label className="flex items-center gap-1.5 text-xs text-muted-foreground bg-background/85 backdrop-blur rounded border px-2 py-1 cursor-pointer">
                <input
                  type="checkbox"
                  role="switch"
                  aria-label="Data flow"
                  checked={showData}
                  onChange={(e) => setShowData(e.target.checked)}
                />
                Data flow
              </label>
            </Panel>
          )}
          {storageKey && (
            <Panel position="bottom-right">
              <button
                type="button"
                onClick={() => { reset(); setLayoutNonce((n) => n + 1); }}
                className="text-xs text-muted-foreground bg-background/85 backdrop-blur rounded border px-2 py-1 hover:text-foreground"
              >
                Reset layout
              </button>
            </Panel>
          )}
          <Controls />
          <Background variant={BackgroundVariant.Dots} gap={12} size={1} />
        </ReactFlow>
        <NodeDrawer
          node={selected}
          onClose={() => setSelectedId(undefined)}
          namespace={namespace}
        />
      </div>
    </div>
  );
}
