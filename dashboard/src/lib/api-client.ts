// API client configuration for the Omnia dashboard.
// In production, calls the operator's REST API.
// In demo mode, returns mock data.

import {
  mockAgentRuntimes,
  mockPromptPacks,
  mockToolRegistries,
  getMockStats,
} from "./mock-data";
import type { AgentRuntime, PromptPack, ToolRegistry } from "@/types";

// Operator API base URL - can be configured via environment variable
const OPERATOR_API_URL = process.env.OPERATOR_API_URL || "http://localhost:8082";

// Demo mode flag - when true, uses mock data instead of operator API
export const isDemoMode = process.env.DEMO_MODE === "true";

/**
 * Fetch wrapper that handles demo mode fallback
 */
async function apiFetch<T>(
  path: string,
  mockData: T
): Promise<T> {
  if (isDemoMode) {
    // Simulate network delay
    await new Promise((resolve) => setTimeout(resolve, 100));
    return mockData;
  }

  const response = await fetch(`${OPERATOR_API_URL}${path}`, {
    headers: {
      "Content-Type": "application/json",
    },
  });

  if (!response.ok) {
    throw new Error(`API error: ${response.status} ${response.statusText}`);
  }

  return response.json();
}

// ============================================================================
// AgentRuntime API
// ============================================================================

export async function fetchAgents(namespace?: string): Promise<AgentRuntime[]> {
  const query = namespace ? `?namespace=${namespace}` : "";
  return apiFetch(`/api/v1/agents${query}`, mockAgentRuntimes);
}

export async function fetchAgent(
  namespace: string,
  name: string
): Promise<AgentRuntime | undefined> {
  if (isDemoMode) {
    await new Promise((resolve) => setTimeout(resolve, 100));
    return mockAgentRuntimes.find(
      (a) => a.metadata.name === name && a.metadata.namespace === namespace
    );
  }

  const response = await fetch(
    `${OPERATOR_API_URL}/api/v1/agents/${namespace}/${name}`
  );

  if (response.status === 404) {
    return undefined;
  }

  if (!response.ok) {
    throw new Error(`API error: ${response.status}`);
  }

  return response.json();
}

// ============================================================================
// PromptPack API
// ============================================================================

export async function fetchPromptPacks(namespace?: string): Promise<PromptPack[]> {
  const query = namespace ? `?namespace=${namespace}` : "";
  return apiFetch(`/api/v1/promptpacks${query}`, mockPromptPacks);
}

export async function fetchPromptPack(
  namespace: string,
  name: string
): Promise<PromptPack | undefined> {
  if (isDemoMode) {
    await new Promise((resolve) => setTimeout(resolve, 100));
    return mockPromptPacks.find(
      (p) => p.metadata.name === name && p.metadata.namespace === namespace
    );
  }

  const response = await fetch(
    `${OPERATOR_API_URL}/api/v1/promptpacks/${namespace}/${name}`
  );

  if (response.status === 404) {
    return undefined;
  }

  if (!response.ok) {
    throw new Error(`API error: ${response.status}`);
  }

  return response.json();
}

// ============================================================================
// ToolRegistry API
// ============================================================================

export async function fetchToolRegistries(namespace?: string): Promise<ToolRegistry[]> {
  const query = namespace ? `?namespace=${namespace}` : "";
  return apiFetch(`/api/v1/toolregistries${query}`, mockToolRegistries);
}

export async function fetchToolRegistry(
  namespace: string,
  name: string
): Promise<ToolRegistry | undefined> {
  if (isDemoMode) {
    await new Promise((resolve) => setTimeout(resolve, 100));
    return mockToolRegistries.find(
      (t) => t.metadata.name === name && t.metadata.namespace === namespace
    );
  }

  const response = await fetch(
    `${OPERATOR_API_URL}/api/v1/toolregistries/${namespace}/${name}`
  );

  if (response.status === 404) {
    return undefined;
  }

  if (!response.ok) {
    throw new Error(`API error: ${response.status}`);
  }

  return response.json();
}

// ============================================================================
// Stats API
// ============================================================================

export interface Stats {
  agents: {
    total: number;
    running: number;
    pending: number;
    failed: number;
  };
  promptPacks: {
    total: number;
    active: number;
    canary: number;
  };
  tools: {
    total: number;
    available: number;
    degraded: number;
  };
}

export async function fetchStats(): Promise<Stats> {
  return apiFetch("/api/v1/stats", getMockStats());
}
