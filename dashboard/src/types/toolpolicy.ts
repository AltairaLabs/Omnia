/**
 * TypeScript types for the ToolPolicy CRD.
 *
 * ToolPolicy is an Enterprise CRD for CEL-based tool parameter validation.
 * It enables platform operators to enforce constraints on tool calls at runtime
 * (e.g., deny queries containing sensitive patterns, inject audit headers).
 */

export type PolicyMode = "enforce" | "audit";
export type ToolPolicyOnFailureAction = "deny" | "allow";
export type ToolPolicyPhase = "Active" | "Error";

export interface ToolPolicySelector {
  registry: string;
  tools?: string[];
}

export interface PolicyRuleDeny {
  cel: string;
  message: string;
}

export interface PolicyRule {
  name: string;
  description?: string;
  deny: PolicyRuleDeny;
}

export interface RequiredClaim {
  claim: string;
  message: string;
}

export interface HeaderInjectionRule {
  header: string;
  value?: string;
  cel?: string;
}

export interface ToolPolicyAuditConfig {
  logDecisions?: boolean;
  redactFields?: string[];
}

export interface ToolPolicySpec {
  selector: ToolPolicySelector;
  rules: PolicyRule[];
  requiredClaims?: RequiredClaim[];
  mode?: PolicyMode;
  onFailure?: ToolPolicyOnFailureAction;
  headerInjection?: HeaderInjectionRule[];
  audit?: ToolPolicyAuditConfig;
}

export interface ToolPolicyCondition {
  type: string;
  status: "True" | "False" | "Unknown";
  lastTransitionTime?: string;
  reason?: string;
  message?: string;
}

export interface ToolPolicyStatus {
  phase?: ToolPolicyPhase;
  conditions?: ToolPolicyCondition[];
  observedGeneration?: number;
  ruleCount?: number;
}

export interface ToolPolicy {
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
  spec: ToolPolicySpec;
  status?: ToolPolicyStatus;
}
