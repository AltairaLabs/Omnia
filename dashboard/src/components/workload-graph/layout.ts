import ELK from "elkjs/lib/elk.bundled.js";
import type { Node, Edge } from "@xyflow/react";

const elk = new ELK();

// Must match the fixed rendered node size in workload-nodes.tsx (w-[200px]
// h-[68px], border-box) so elk's edge routes terminate exactly on the borders.
const NODE_W = 200;
const NODE_H = 68;

export interface Point {
  x: number;
  y: number;
}

export interface LayoutResult<T> {
  nodes: T[];
  // elk's crossing-minimized orthogonal route per edge id (absolute flow coords).
  routes: Map<string, Point[]>;
}

// Rough px width of an edge label, so elk reserves room between layers and
// labels (e.g. "need_more", "max visits") don't collide with nodes.
function labelWidth(label: unknown): number {
  if (typeof label !== "string" || label.length === 0) return 0;
  return label.length * 5 + 6;
}

interface ElkLaidEdge {
  id: string;
  sections?: Array<{ startPoint: Point; endPoint: Point; bendPoints?: Point[] }>;
}

function routeFromEdge(e: ElkLaidEdge): Point[] | undefined {
  const s = e.sections?.[0];
  if (!s) return undefined;
  return [s.startPoint, ...(s.bendPoints ?? []), s.endPoint].map((p) => ({ x: p.x, y: p.y }));
}

export async function layoutFlow<T extends Node>(
  nodes: T[],
  edges: Edge[],
): Promise<LayoutResult<T>> {
  if (nodes.length === 0) return { nodes, routes: new Map() };
  const graph = {
    id: "root",
    layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": "DOWN",
      "elk.edgeRouting": "ORTHOGONAL",
      "elk.spacing.nodeNode": "20",
      "elk.layered.spacing.nodeNodeBetweenLayers": "44",
      "elk.layered.spacing.edgeNodeBetweenLayers": "14",
      "elk.layered.spacing.edgeEdgeBetweenLayers": "10",
      "elk.layered.nodePlacement.strategy": "NETWORK_SIMPLEX",
      "elk.layered.crossingMinimization.strategy": "LAYER_SWEEP",
    },
    children: nodes.map((n) => ({
      id: n.id,
      width: (n as { width?: number }).width ?? NODE_W,
      height: (n as { height?: number }).height ?? NODE_H,
    })),
    edges: edges.map((e) => ({
      id: e.id,
      sources: [e.source],
      targets: [e.target],
      labels: labelWidth(e.label)
        ? [{ text: String(e.label), width: labelWidth(e.label), height: 14 }]
        : undefined,
    })),
  };
  const laid = await elk.layout(graph);
  const pos = new Map((laid.children ?? []).map((c) => [c.id, { x: c.x ?? 0, y: c.y ?? 0 }]));
  const routes = new Map<string, Point[]>();
  for (const e of (laid.edges ?? []) as unknown as ElkLaidEdge[]) {
    const r = routeFromEdge(e);
    if (r) routes.set(e.id, r);
  }
  return {
    nodes: nodes.map((n) => ({ ...n, position: pos.get(n.id) ?? n.position })),
    routes,
  };
}
