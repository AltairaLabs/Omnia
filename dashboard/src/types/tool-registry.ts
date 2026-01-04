// ToolRegistry CRD types - matches api/v1alpha1/toolregistry_types.go

import { ObjectMeta, Condition, SecretKeyRef } from "./common";

// Enums
export type ToolRegistryPhase = "Pending" | "Ready" | "Degraded" | "Failed";
export type HandlerType = "http" | "openapi" | "grpc" | "mcp";
export type ToolStatus = "Available" | "Unavailable" | "Unknown";
export type MCPTransport = "sse" | "stdio";
export type HTTPAuthType = "none" | "bearer" | "basic";

// Nested types
export interface ServiceSelector {
  matchLabels?: Record<string, string>;
  namespace?: string;
  port?: string | number;
}

export interface ToolDefinition {
  name: string;
  description: string;
  inputSchema?: unknown;
  outputSchema?: unknown;
}

export interface HTTPConfig {
  endpoint: string;
  method?: string;
  headers?: Record<string, string>;
  contentType?: string;
  authType?: HTTPAuthType;
  authSecretRef?: SecretKeyRef;
}

export interface OpenAPIConfig {
  specURL: string;
  baseURL?: string;
  operationFilter?: string[];
}

export interface GRPCConfig {
  endpoint: string;
  tls?: boolean;
  tlsCertPath?: string;
  tlsKeyPath?: string;
  tlsCAPath?: string;
  tlsInsecureSkipVerify?: boolean;
}

export interface MCPConfig {
  transport: MCPTransport;
  endpoint?: string;
  command?: string;
  args?: string[];
  workDir?: string;
  env?: Record<string, string>;
}

export interface HandlerDefinition {
  name: string;
  type: HandlerType;
  selector?: ServiceSelector;
  tool?: ToolDefinition;
  httpConfig?: HTTPConfig;
  openAPIConfig?: OpenAPIConfig;
  grpcConfig?: GRPCConfig;
  mcpConfig?: MCPConfig;
  timeout?: string;
  retries?: number;
}

// Spec
export interface ToolRegistrySpec {
  handlers: HandlerDefinition[];
}

// Status
export interface DiscoveredTool {
  name: string;
  handlerName: string;
  description: string;
  inputSchema?: unknown;
  outputSchema?: unknown;
  endpoint: string;
  status: ToolStatus;
  lastChecked?: string;
  error?: string;
}

export interface ToolRegistryStatus {
  phase?: ToolRegistryPhase;
  discoveredToolsCount?: number;
  discoveredTools?: DiscoveredTool[];
  lastDiscoveryTime?: string;
  conditions?: Condition[];
}

// Full resource
export interface ToolRegistry {
  apiVersion: "omnia.altairalabs.ai/v1alpha1";
  kind: "ToolRegistry";
  metadata: ObjectMeta;
  spec: ToolRegistrySpec;
  status?: ToolRegistryStatus;
}
