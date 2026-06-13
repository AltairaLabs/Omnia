import type { Node as FlowNode, Edge as FlowEdge } from "@xyflow/react";
import type { WorkloadModel, WorkloadNode, WorkloadNodeKind } from "./types";

export interface WorkloadNodeData extends Record<string, unknown> {
  node: WorkloadNode;
  onClick?: (id: string) => void;
}

const KIND_TO_TYPE: Record<WorkloadNodeKind, string> = {
  agent: "workloadAgent",
  state: "workloadState",
  provider: "workloadProvider",
  tool: "workloadAgent",
  skill: "workloadSkill",
  scenario: "workloadAgent",
  judge: "workloadAgent",
};

function edgeStyle(style?: WorkloadModel["edges"][number]["style"]): FlowEdge["style"] {
  if (style === "unresolved") return { strokeDasharray: "4 4", opacity: 0.5 };
  if (style === "loop") return { strokeDasharray: "6 3" };
  if (style === "provides") return { strokeDasharray: "2 4", opacity: 0.6 };
  return undefined;
}

export function modelToFlow(
  model: WorkloadModel,
  onClick?: (id: string) => void,
): { nodes: FlowNode<WorkloadNodeData>[]; edges: FlowEdge[] } {
  const nodes: FlowNode<WorkloadNodeData>[] = model.nodes.map((node) => ({
    id: node.id,
    type: KIND_TO_TYPE[node.kind],
    position: { x: 0, y: 0 }, // replaced by elk layout
    data: { node, onClick },
  }));

  const edges: FlowEdge[] = model.edges.map((e) => ({
    id: e.id,
    source: e.source,
    target: e.target,
    label: e.label,
    type: "workloadEdge",
    animated: e.style === "loop",
    style: edgeStyle(e.style),
    data: {}, // elk route points are injected after layout
  }));

  return { nodes, edges };
}
