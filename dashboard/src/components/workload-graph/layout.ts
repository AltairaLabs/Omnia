import ELK from "elkjs/lib/elk.bundled.js";
import type { Node, Edge } from "@xyflow/react";

const elk = new ELK();

const NODE_W = 220;
const NODE_H = 84;

// Rough px width of an edge label, so elk reserves room between layers and
// labels (e.g. "need_more", "max visits") don't collide with nodes.
function labelWidth(label: unknown): number {
  if (typeof label !== "string" || label.length === 0) return 0;
  return label.length * 6.5 + 12;
}

export async function layoutFlow<T extends Node>(nodes: T[], edges: Edge[]): Promise<T[]> {
  if (nodes.length === 0) return nodes;
  const graph = {
    id: "root",
    layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": "RIGHT",
      "elk.spacing.nodeNode": "34",
      "elk.layered.spacing.nodeNodeBetweenLayers": "64",
      "elk.layered.spacing.edgeNodeBetweenLayers": "20",
      "elk.layered.nodePlacement.strategy": "NETWORK_SIMPLEX",
    },
    children: nodes.map((n) => ({ id: n.id, width: NODE_W, height: NODE_H })),
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
  return nodes.map((n) => ({ ...n, position: pos.get(n.id) ?? n.position }));
}
