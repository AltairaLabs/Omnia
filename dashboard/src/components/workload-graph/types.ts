// Neutral, source- and altitude-agnostic workload model.
// Adapters (from-promptpack, from-agent, future from-arena) produce this;
// WorkloadGraph consumes it. It must not reference packs, agents, or Arena.

export type WorkloadTier = "single" | "workflow" | "multiagent";
export type WorkloadAltitude = "definition" | "deployment";
export type WorkloadNodeKind =
  | "agent" | "state" | "tool" | "skill" | "provider" | "scenario" | "judge";
export type ResolutionStatus = "resolved" | "unresolved" | "unavailable";
export type WorkloadEdgeStyle = "normal" | "loop" | "unresolved" | "provides";

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
}

export interface WorkloadEdge {
  id: string;
  source: string;
  target: string;
  label?: string;
  style?: WorkloadEdgeStyle;
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
