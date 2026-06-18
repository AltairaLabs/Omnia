// Neutral, source- and altitude-agnostic workload model.
// Adapters (from-promptpack, from-agent, future from-arena) produce this;
// WorkloadGraph consumes it. It must not reference packs, agents, or Arena.

export type WorkloadTier = "single" | "workflow" | "multiagent";
export type WorkloadAltitude = "definition" | "deployment" | "test";
export type WorkloadNodeKind =
  | "agent" | "state" | "tool" | "skill" | "provider" | "scenario" | "judge"
  | "initial" | "final" | "variable" | "artifact" | "persona"
  | "composition" | "stepPrompt" | "stepAgent" | "stepTool" | "stepBranch" | "stepParallel";
export type ResolutionStatus = "resolved" | "unresolved" | "unavailable";
export type WorkloadEdgeStyle = "normal" | "loop" | "unresolved" | "provides" | "data";

export type WorkloadBadgeIcon = "tool" | "skill" | "entry" | "terminal" | "loop";

export interface WorkloadBadge {
  icon?: WorkloadBadgeIcon;
  label: string;
}

export interface WorkloadToolDetail {
  name: string;
  description?: string;
  endpoint?: string;     // deployment altitude only
  handlerType?: string;  // deployment altitude only
  status?: ResolutionStatus;
}

export interface WorkloadNodeDetail {
  description?: string;
  systemTemplatePreview?: string;
  tools?: WorkloadToolDetail[];
  skills?: string[];
  parameters?: Record<string, unknown>;
  ioModes?: { input?: string[]; output?: string[] };
  // provider node fields (deployment only):
  model?: string;
  providerType?: string;
  baseURL?: string;
  role?: string;
  // skill node fields (skill kind): a SkillSource the pack binds to.
  skillSource?: string;        // SkillSource CRD name
  include?: string[];          // include filter from the SkillRef
  mountAs?: string;            // mount path rename from the SkillRef
  skillCount?: number;         // SkillSource.status.skillCount
  skillPhase?: string;         // SkillSource.status.phase, or "missing"
  // variable node fields:
  varType?: string;
  required?: boolean;
  example?: string;
  values?: string[];
  // artifact node fields:
  artifactMode?: string;       // "replace" | "append"
  artifactType?: string;
  producers?: string[];        // state ids that declare/write it
  consumers?: string[];        // state ids whose template reads {{artifacts.<name>}}
  // arena harness node fields:
  pricing?: { inputPer1kTokens?: number; outputPer1kTokens?: number }; // provider node
  scenarios?: Array<{                                                  // scenario group node
    id: string;
    description?: string;
    turnCount?: number;
    tags?: string[];
    assertions?: string[];
  }>;
  judgeProvider?: string;                                              // judge node
  persona?: { id: string; role?: string; provider?: string };         // persona node
  // composition / step node fields:
  stepKind?: string;             // CompositionStep.kind
  promptTask?: string;           // prompt/agent step
  toolRef?: string;              // tool step
  args?: Record<string, unknown>; // tool step
  predicateText?: string;        // branch step, rendered predicate
  reducer?: string;              // parallel step, e.g. "barrier → metadata"
  termination?: string;          // agent step, e.g. "≤10 steps"
  evals?: string[];              // step modifiers.eval
  compositionName?: string;      // composition container
  stepCount?: number;            // composition container
  composition?: CompositionSubgraph; // attached to a composition-orchestrated state node
}

export interface WorkloadNode {
  id: string;
  kind: WorkloadNodeKind;
  label: string;
  badges: WorkloadBadge[];
  detail: WorkloadNodeDetail;
  isEntry?: boolean;
  isTerminal?: boolean;
  resolution?: ResolutionStatus;
  parentId?: string;     // React Flow parent (set on composition step children)
  isContainer?: boolean; // composition / parallel group box
}

export interface WorkloadEdge {
  id: string;
  source: string;
  target: string;
  label?: string;
  style?: WorkloadEdgeStyle;
}

export interface CompositionSubgraph {
  name: string;
  nodes: WorkloadNode[];
  edges: WorkloadEdge[];
}

export interface WorkloadBudget {
  maxTotalVisits?: number;
  maxToolCalls?: number;
  maxWallTimeSec?: number;
}

export interface WorkloadBinding {
  providers: Array<{ name: string; model?: string; role?: string }>;
  toolRegistry?: string;
}

export interface WorkloadCounts {
  agents: number;
  tools: number;
  skills: number;
  states: number;
}

export interface WorkloadMeta {
  budget?: WorkloadBudget;
  counts: WorkloadCounts;
  binding?: WorkloadBinding;
}

export interface WorkloadModel {
  tier: WorkloadTier;
  altitude: WorkloadAltitude;
  nodes: WorkloadNode[];
  edges: WorkloadEdge[];
  meta: WorkloadMeta;
}
