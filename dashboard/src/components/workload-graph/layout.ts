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

interface ElkEdge {
  id: string;
  sources: string[];
  targets: string[];
  labels?: Array<{ text: string; width: number; height: number }>;
  sections?: Array<{ startPoint: Point; endPoint: Point; bendPoints?: Point[] }>;
}

interface ElkNode {
  id: string;
  width?: number;
  height?: number;
  layoutOptions?: Record<string, string>;
  children?: ElkNode[];
  edges?: ElkEdge[];
  x?: number;
  y?: number;
}

function routeFromEdge(e: ElkEdge): Point[] | undefined {
  const s = e.sections?.[0];
  if (!s) return undefined;
  return [s.startPoint, ...(s.bendPoints ?? []), s.endPoint].map((p) => ({ x: p.x, y: p.y }));
}

const CONTAINER_PADDING = "[top=36,left=12,bottom=12,right=12]";

const CONTAINER_TYPES = new Set(["composition", "compositionParallel"]);

function isContainerNode(n: Node): boolean {
  // to-flow tags containers by node type; the WorkloadNode also carries the
  // flag at data.node.isContainer. Accept data.isContainer too for direct tests.
  if (n.type && CONTAINER_TYPES.has(n.type)) return true;
  const data = n.data as { node?: { isContainer?: boolean }; isContainer?: boolean } | undefined;
  return Boolean(data?.node?.isContainer ?? data?.isContainer);
}

function containerDirection(n: Node): string {
  // parallel blocks fan out horizontally; compositions stack top-down
  return (n.data as { node?: { kind?: string } } | undefined)?.node?.kind === "stepParallel" ? "RIGHT" : "DOWN";
}

function elkEdge(e: Edge): ElkEdge {
  return {
    id: e.id,
    sources: [e.source],
    targets: [e.target],
    labels: labelWidth(e.label) ? [{ text: String(e.label), width: labelWidth(e.label), height: 14 }] : undefined,
  };
}

function buildElkTree<T extends Node>(
  node: T,
  childrenByParent: Map<string | undefined, T[]>,
  edgesByContainer: Map<string | undefined, Edge[]>,
): ElkNode {
  const kids = childrenByParent.get(node.id) ?? [];
  if (kids.length === 0) {
    return { id: node.id, width: node.width ?? NODE_W, height: node.height ?? NODE_H };
  }
  return {
    id: node.id,
    layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": containerDirection(node),
      "elk.padding": CONTAINER_PADDING,
      "elk.spacing.nodeNode": "28",
      "elk.layered.spacing.nodeNodeBetweenLayers": "44",
    },
    children: kids.map((k) => buildElkTree(k, childrenByParent, edgesByContainer)),
    edges: (edgesByContainer.get(node.id) ?? []).map(elkEdge),
  };
}

interface Laid { x: number; y: number; width: number; height: number; }

function collectLaid(node: ElkNode, out: Map<string, Laid>): void {
  out.set(node.id, { x: node.x ?? 0, y: node.y ?? 0, width: node.width ?? NODE_W, height: node.height ?? NODE_H });
  for (const c of node.children ?? []) collectLaid(c, out);
}

function collectRoutes(node: ElkNode, routes: Map<string, Point[]>): void {
  for (const e of node.edges ?? []) {
    const r = routeFromEdge(e);
    if (r) routes.set(e.id, r);
  }
  for (const c of node.children ?? []) collectRoutes(c, routes);
}

export async function layoutFlow<T extends Node>(
  nodes: T[],
  edges: Edge[],
): Promise<LayoutResult<T>> {
  if (nodes.length === 0) return { nodes, routes: new Map() };

  const byId = new Map(nodes.map((n) => [n.id, n]));
  const childrenByParent = new Map<string | undefined, T[]>();
  for (const n of nodes) {
    const key = n.parentId ?? undefined;
    const list = childrenByParent.get(key) ?? [];
    list.push(n);
    childrenByParent.set(key, list);
  }
  // edges live in the container that is the common parent of their endpoints.
  // Our builder only ever connects siblings, so source.parentId is that parent.
  const edgesByContainer = new Map<string | undefined, Edge[]>();
  for (const e of edges) {
    const key = byId.get(e.source)?.parentId ?? undefined;
    const list = edgesByContainer.get(key) ?? [];
    list.push(e);
    edgesByContainer.set(key, list);
  }

  const roots = childrenByParent.get(undefined) ?? [];
  const graph: ElkNode = {
    id: "root",
    layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": "DOWN",
      // Break feedback loops by DFS so forward flow is preserved and only the
      // genuine back-edges (need_more, revise) are reversed.
      "elk.layered.cycleBreaking.strategy": "DEPTH_FIRST",
      "elk.spacing.nodeNode": "44",
      "elk.layered.spacing.nodeNodeBetweenLayers": "64",
      "elk.layered.nodePlacement.strategy": "BRANDES_KOEPF",
      "elk.layered.nodePlacement.bk.fixedAlignment": "BALANCED",
      "elk.layered.crossingMinimization.strategy": "LAYER_SWEEP",
    },
    children: roots.map((r) => buildElkTree(r, childrenByParent, edgesByContainer)),
    edges: (edgesByContainer.get(undefined) ?? []).map(elkEdge),
  };

  const laid = (await elk.layout(graph as unknown as Parameters<typeof elk.layout>[0])) as unknown as ElkNode;
  const pos = new Map<string, Laid>();
  for (const c of laid.children ?? []) collectLaid(c, pos);
  const routes = new Map<string, Point[]>();
  collectRoutes(laid, routes);

  // elk gives child coords relative to their parent, which is exactly what React
  // Flow wants for nodes that declare parentId. Container width/height come from elk.
  const outNodes = nodes.map((n) => {
    const p = pos.get(n.id);
    if (!p) return n;
    const next = { ...n, position: { x: p.x, y: p.y } } as T;
    if (isContainerNode(n)) {
      (next as { width?: number }).width = p.width;
      (next as { height?: number }).height = p.height;
      (next as { style?: Record<string, unknown> }).style = { ...(n.style ?? {}), width: p.width, height: p.height };
    }
    return next;
  });

  return { nodes: outNodes, routes };
}
