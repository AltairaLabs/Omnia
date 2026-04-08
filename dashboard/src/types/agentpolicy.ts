/**
 * TypeScript types for the AgentPolicy CRD.
 *
 * Hand-written based on api/v1alpha1/agentpolicy_types.go.
 */

export type AgentPolicyMode = "enforce" | "permissive";
export type ToolAccessMode = "allowlist" | "denylist";
export type OnFailureAction = "deny" | "allow";
export type AgentPolicyPhase = "Active" | "Error";

export interface AgentPolicySelector {
  agents?: string[];
}

export interface ClaimMappingEntry {
  claim: string;
  header: string;
}

export interface ClaimMapping {
  forwardClaims?: ClaimMappingEntry[];
}

export interface ToolAccessRule {
  registry: string;
  tools: string[];
}

export interface ToolAccessConfig {
  mode: ToolAccessMode;
  rules: ToolAccessRule[];
}

export interface AgentPolicySpec {
  selector?: AgentPolicySelector;
  claimMapping?: ClaimMapping;
  toolAccess?: ToolAccessConfig;
  mode?: AgentPolicyMode;
  onFailure?: OnFailureAction;
}

export interface AgentPolicyCondition {
  type: string;
  status: "True" | "False" | "Unknown";
  lastTransitionTime?: string;
  reason?: string;
  message?: string;
}

export interface AgentPolicyStatus {
  phase?: AgentPolicyPhase;
  matchedAgents?: number;
  conditions?: AgentPolicyCondition[];
  observedGeneration?: number;
}

export interface AgentPolicy {
  apiVersion: string;
  kind: string;
  metadata: {
    name: string;
    namespace?: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
    uid?: string;
    resourceVersion?: string;
    creationTimestamp?: string;
  };
  spec: AgentPolicySpec;
  status?: AgentPolicyStatus;
}
