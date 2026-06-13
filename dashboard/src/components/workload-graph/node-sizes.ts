import type { WorkloadNodeKind } from "./types";

export interface NodeSize {
  width: number;
  height: number;
}

const SIZES: Partial<Record<WorkloadNodeKind, NodeSize>> = {
  initial: { width: 24, height: 24 },
  final: { width: 24, height: 24 },
  variable: { width: 120, height: 30 },
  artifact: { width: 150, height: 44 },
  provider: { width: 200, height: 68 },
};

const DEFAULT: NodeSize = { width: 200, height: 68 };

export function nodeSize(kind: WorkloadNodeKind): NodeSize {
  return SIZES[kind] ?? DEFAULT;
}
