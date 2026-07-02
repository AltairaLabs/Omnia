import type { Node as FlowNode, Edge as FlowEdge } from "@xyflow/react";
import type { WorkloadModel, WorkloadNode, WorkloadNodeKind } from "./types";
import { nodeSize } from "./node-sizes";

export interface WorkloadNodeData extends Record<string, unknown> {
  node: WorkloadNode;
  onClick?: (id: string) => void;
  onToggle?: (id: string) => void;
  expandable?: boolean;
  expanded?: boolean;
}

export interface ModelToFlowOptions {
  onClick?: (id: string) => void;
  onToggle?: (id: string) => void;
  expanded?: Set<string>;
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
  composition: "composition",
  stepPrompt: "compositionStep",
  stepAgent: "compositionStep",
  stepTool: "compositionStep",
  stepBranch: "compositionStep",
  stepParallel: "compositionParallel",
};

function edgeStyle(style?: WorkloadModel["edges"][number]["style"]): FlowEdge["style"] {
  if (style === "unresolved") return { strokeDasharray: "4 4", opacity: 0.5 };
  if (style === "loop") return { strokeDasharray: "6 3" };
  if (style === "provides") return { strokeDasharray: "2 4", opacity: 0.6 };
  if (style === "data") return { strokeDasharray: "3 3", stroke: "var(--category-6)", opacity: 0.8 };
  return undefined;
}

function baseNode(node: WorkloadNode, data: WorkloadNodeData): FlowNode<WorkloadNodeData> {
  const { width, height } = nodeSize(node.kind);
  const flow: FlowNode<WorkloadNodeData> = {
    id: node.id,
    type: KIND_TO_TYPE[node.kind],
    position: { x: 0, y: 0 }, // replaced by elk layout
    width,
    height,
    data,
  };
  if (node.parentId) {
    flow.parentId = node.parentId;
    flow.extent = "parent";
  }
  return flow;
}

function flowEdge(e: WorkloadModel["edges"][number]): FlowEdge {
  return {
    id: e.id,
    source: e.source,
    target: e.target,
    label: e.label,
    // default xyflow edge type = smooth bezier, connecting top/bottom handles
    animated: e.style === "loop",
    style: edgeStyle(e.style),
  };
}

export function modelToFlow(
  model: WorkloadModel,
  opts: ModelToFlowOptions = {},
): { nodes: FlowNode<WorkloadNodeData>[]; edges: FlowEdge[] } {
  const { onClick, onToggle, expanded } = opts;
  const nodes: FlowNode<WorkloadNodeData>[] = [];
  const edges: FlowEdge[] = model.edges.map(flowEdge);

  for (const node of model.nodes) {
    const sub = node.detail.composition;
    const isExpanded = Boolean(sub && expanded?.has(node.id));

    if (sub && isExpanded) {
      // container first (React Flow requires parent before children)
      const container: WorkloadNode = { ...node, kind: "composition", isContainer: true };
      nodes.push(baseNode(container, { node: container, onClick, onToggle, expanded: true }));
      for (const childNode of sub.nodes) {
        nodes.push(baseNode(childNode, { node: childNode, onClick, onToggle }));
      }
      for (const e of sub.edges) edges.push(flowEdge(e));
      continue;
    }

    const data: WorkloadNodeData = { node, onClick, onToggle, expandable: Boolean(sub), expanded: false };
    nodes.push(baseNode(node, data));
  }

  return { nodes, edges };
}
