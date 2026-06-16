import type { Node as FlowNode, Edge as FlowEdge } from "@xyflow/react";
import type { WorkloadModel, WorkloadNode, WorkloadNodeKind } from "./types";
import { nodeSize } from "./node-sizes";

export interface WorkloadNodeData extends Record<string, unknown> {
  node: WorkloadNode;
  onClick?: (id: string) => void;
}

const KIND_TO_TYPE: Record<WorkloadNodeKind, string> = {
  agent: "workloadAgent",
  state: "workflowState",
  provider: "workloadProvider",
  tool: "workloadTool",
  skill: "workloadSkill",
  scenario: "workflowScenario",
  judge: "workflowJudge",
  initial: "workflowInitial",
  final: "workflowFinal",
  variable: "workflowVariable",
  artifact: "workflowArtifact",
  persona: "workflowPersona",
};

function edgeStyle(style?: WorkloadModel["edges"][number]["style"]): FlowEdge["style"] {
  if (style === "unresolved") return { strokeDasharray: "4 4", opacity: 0.5 };
  if (style === "loop") return { strokeDasharray: "6 3" };
  if (style === "provides") return { strokeDasharray: "2 4", opacity: 0.6 };
  if (style === "data") return { strokeDasharray: "3 3", stroke: "#0d9488", opacity: 0.8 };
  return undefined;
}

export function modelToFlow(
  model: WorkloadModel,
  onClick?: (id: string) => void,
): { nodes: FlowNode<WorkloadNodeData>[]; edges: FlowEdge[] } {
  const nodes: FlowNode<WorkloadNodeData>[] = model.nodes.map((node) => {
    const { width, height } = nodeSize(node.kind);
    return {
      id: node.id,
      type: KIND_TO_TYPE[node.kind],
      position: { x: 0, y: 0 }, // replaced by elk layout
      width,
      height,
      data: { node, onClick },
    };
  });

  const edges: FlowEdge[] = model.edges.map((e) => ({
    id: e.id,
    source: e.source,
    target: e.target,
    label: e.label,
    // default xyflow edge type = smooth bezier, connecting top/bottom handles
    animated: e.style === "loop",
    style: edgeStyle(e.style),
  }));

  return { nodes, edges };
}
