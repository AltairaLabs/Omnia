// Mock data for development and testing

import type {
  AgentRuntime,
  PromptPack,
  ToolRegistry,
} from "@/types";

// Helper to generate timestamps
const hoursAgo = (hours: number) =>
  new Date(Date.now() - hours * 60 * 60 * 1000).toISOString();

const daysAgo = (days: number) =>
  new Date(Date.now() - days * 24 * 60 * 60 * 1000).toISOString();

// Mock AgentRuntimes
export const mockAgentRuntimes: AgentRuntime[] = [
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "AgentRuntime",
    metadata: {
      name: "customer-support",
      namespace: "production",
      creationTimestamp: daysAgo(14),
      uid: "agent-001",
      labels: {
        "app.kubernetes.io/name": "customer-support",
        team: "support",
      },
    },
    spec: {
      promptPackRef: { name: "support-prompts", version: "1.2.0" },
      facade: { type: "websocket", port: 8080, handler: "runtime" },
      provider: { type: "claude", model: "claude-sonnet-4-20250514" },
      session: { type: "redis", ttl: "24h" },
      runtime: { replicas: 3 },
    },
    status: {
      phase: "Running",
      replicas: { desired: 3, ready: 3, available: 3 },
      activeVersion: "1.2.0",
      conditions: [
        {
          type: "Available",
          status: "True",
          lastTransitionTime: hoursAgo(2),
          reason: "MinimumReplicasAvailable",
        },
      ],
    },
  },
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "AgentRuntime",
    metadata: {
      name: "code-assistant",
      namespace: "production",
      creationTimestamp: daysAgo(7),
      uid: "agent-002",
      labels: {
        "app.kubernetes.io/name": "code-assistant",
        team: "engineering",
      },
    },
    spec: {
      promptPackRef: { name: "code-prompts", track: "stable" },
      facade: { type: "websocket", port: 8080, handler: "runtime" },
      provider: { type: "claude", model: "claude-sonnet-4-20250514" },
      session: { type: "redis", ttl: "1h" },
      runtime: { replicas: 2 },
    },
    status: {
      phase: "Running",
      replicas: { desired: 2, ready: 2, available: 2 },
      activeVersion: "2.0.1",
      conditions: [
        {
          type: "Available",
          status: "True",
          lastTransitionTime: hoursAgo(6),
          reason: "MinimumReplicasAvailable",
        },
      ],
    },
  },
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "AgentRuntime",
    metadata: {
      name: "data-analyst",
      namespace: "production",
      creationTimestamp: daysAgo(3),
      uid: "agent-003",
      labels: {
        "app.kubernetes.io/name": "data-analyst",
        team: "data",
      },
    },
    spec: {
      promptPackRef: { name: "analyst-prompts", version: "1.0.0" },
      facade: { type: "websocket", port: 8080, handler: "runtime" },
      provider: { type: "openai", model: "gpt-4-turbo" },
      toolRegistryRef: { name: "data-tools" },
      session: { type: "redis", ttl: "2h" },
      runtime: { replicas: 2 },
    },
    status: {
      phase: "Running",
      replicas: { desired: 2, ready: 2, available: 2 },
      activeVersion: "1.0.0",
      conditions: [
        {
          type: "Available",
          status: "True",
          lastTransitionTime: hoursAgo(1),
          reason: "MinimumReplicasAvailable",
        },
      ],
    },
  },
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "AgentRuntime",
    metadata: {
      name: "onboarding-bot",
      namespace: "production",
      creationTimestamp: hoursAgo(2),
      uid: "agent-004",
      labels: {
        "app.kubernetes.io/name": "onboarding-bot",
        team: "hr",
      },
    },
    spec: {
      promptPackRef: { name: "onboarding-prompts", version: "0.9.0" },
      facade: { type: "websocket", port: 8080, handler: "runtime" },
      provider: { type: "claude", model: "claude-sonnet-4-20250514" },
      session: { type: "memory", ttl: "30m" },
      runtime: { replicas: 1 },
    },
    status: {
      phase: "Pending",
      replicas: { desired: 1, ready: 0, available: 0 },
      conditions: [
        {
          type: "Available",
          status: "False",
          lastTransitionTime: hoursAgo(0.5),
          reason: "MinimumReplicasUnavailable",
          message: "Waiting for pod to be scheduled",
        },
      ],
    },
  },
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "AgentRuntime",
    metadata: {
      name: "sales-copilot",
      namespace: "production",
      creationTimestamp: daysAgo(5),
      uid: "agent-005",
      labels: {
        "app.kubernetes.io/name": "sales-copilot",
        team: "sales",
      },
    },
    spec: {
      promptPackRef: { name: "sales-prompts", version: "1.1.0" },
      facade: { type: "grpc", port: 9090, handler: "runtime" },
      provider: { type: "openai", model: "gpt-4-turbo" },
      toolRegistryRef: { name: "crm-tools" },
      session: { type: "redis", ttl: "4h" },
      runtime: { replicas: 2 },
    },
    status: {
      phase: "Running",
      replicas: { desired: 2, ready: 1, available: 1 },
      activeVersion: "1.1.0",
      conditions: [
        {
          type: "Available",
          status: "True",
          lastTransitionTime: hoursAgo(4),
          reason: "MinimumReplicasAvailable",
        },
        {
          type: "Progressing",
          status: "True",
          lastTransitionTime: hoursAgo(0.25),
          reason: "ReplicaSetUpdated",
          message: "Scaling up to 2 replicas",
        },
      ],
    },
  },
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "AgentRuntime",
    metadata: {
      name: "legacy-agent",
      namespace: "staging",
      creationTimestamp: daysAgo(30),
      uid: "agent-006",
      labels: {
        "app.kubernetes.io/name": "legacy-agent",
        team: "platform",
      },
    },
    spec: {
      promptPackRef: { name: "legacy-prompts", version: "0.5.0" },
      facade: { type: "websocket", port: 8080, handler: "demo" },
      provider: { type: "openai", model: "gpt-3.5-turbo" },
      session: { type: "memory", ttl: "1h" },
      runtime: { replicas: 1 },
    },
    status: {
      phase: "Failed",
      replicas: { desired: 1, ready: 0, available: 0 },
      conditions: [
        {
          type: "Available",
          status: "False",
          lastTransitionTime: hoursAgo(12),
          reason: "ProviderError",
          message: "Failed to authenticate with provider: invalid API key",
        },
      ],
    },
  },
];

// Mock PromptPacks
export const mockPromptPacks: PromptPack[] = [
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "PromptPack",
    metadata: {
      name: "support-prompts",
      namespace: "production",
      creationTimestamp: daysAgo(30),
      uid: "pack-001",
    },
    spec: {
      source: { type: "configmap", configMapRef: { name: "support-prompts-v1.2.0" } },
      version: "1.2.0",
      rollout: { type: "immediate" },
    },
    status: {
      phase: "Active",
      activeVersion: "1.2.0",
      lastUpdated: daysAgo(7),
    },
  },
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "PromptPack",
    metadata: {
      name: "code-prompts",
      namespace: "production",
      creationTimestamp: daysAgo(21),
      uid: "pack-002",
    },
    spec: {
      source: { type: "configmap", configMapRef: { name: "code-prompts-v2.1.0" } },
      version: "2.1.0",
      rollout: {
        type: "canary",
        canary: { weight: 30, stepWeight: 10, interval: "5m" },
      },
    },
    status: {
      phase: "Canary",
      activeVersion: "2.0.1",
      canaryVersion: "2.1.0",
      canaryWeight: 30,
      lastUpdated: hoursAgo(2),
    },
  },
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "PromptPack",
    metadata: {
      name: "analyst-prompts",
      namespace: "production",
      creationTimestamp: daysAgo(10),
      uid: "pack-003",
    },
    spec: {
      source: { type: "configmap", configMapRef: { name: "analyst-prompts-v1.0.0" } },
      version: "1.0.0",
      rollout: { type: "immediate" },
    },
    status: {
      phase: "Active",
      activeVersion: "1.0.0",
      lastUpdated: daysAgo(3),
    },
  },
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "PromptPack",
    metadata: {
      name: "sales-prompts",
      namespace: "production",
      creationTimestamp: daysAgo(14),
      uid: "pack-004",
    },
    spec: {
      source: { type: "configmap", configMapRef: { name: "sales-prompts-v1.2.0" } },
      version: "1.2.0",
      rollout: {
        type: "canary",
        canary: { weight: 75, stepWeight: 25, interval: "10m" },
      },
    },
    status: {
      phase: "Canary",
      activeVersion: "1.1.0",
      canaryVersion: "1.2.0",
      canaryWeight: 75,
      lastUpdated: hoursAgo(1),
    },
  },
];

// Mock ToolRegistries
export const mockToolRegistries: ToolRegistry[] = [
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ToolRegistry",
    metadata: {
      name: "data-tools",
      namespace: "production",
      creationTimestamp: daysAgo(14),
      uid: "registry-001",
    },
    spec: {
      handlers: [
        {
          name: "sql-query",
          type: "http",
          tool: {
            name: "execute_sql",
            description: "Execute SQL queries against the data warehouse",
          },
          httpConfig: {
            endpoint: "http://sql-service:8080/query",
            method: "POST",
          },
        },
        {
          name: "chart-generator",
          type: "http",
          tool: {
            name: "generate_chart",
            description: "Generate charts from data",
          },
          httpConfig: {
            endpoint: "http://chart-service:8080/generate",
            method: "POST",
          },
        },
      ],
    },
    status: {
      phase: "Ready",
      discoveredToolsCount: 2,
      discoveredTools: [
        {
          name: "execute_sql",
          handlerName: "sql-query",
          description: "Execute SQL queries against the data warehouse",
          endpoint: "http://sql-service:8080/query",
          status: "Available",
          lastChecked: hoursAgo(0.1),
        },
        {
          name: "generate_chart",
          handlerName: "chart-generator",
          description: "Generate charts from data",
          endpoint: "http://chart-service:8080/generate",
          status: "Available",
          lastChecked: hoursAgo(0.1),
        },
      ],
      lastDiscoveryTime: hoursAgo(0.1),
    },
  },
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ToolRegistry",
    metadata: {
      name: "crm-tools",
      namespace: "production",
      creationTimestamp: daysAgo(10),
      uid: "registry-002",
    },
    spec: {
      handlers: [
        {
          name: "salesforce",
          type: "http",
          tool: {
            name: "search_contacts",
            description: "Search CRM contacts",
          },
          httpConfig: {
            endpoint: "http://crm-adapter:8080/contacts/search",
            method: "POST",
            authType: "bearer",
          },
        },
        {
          name: "calendar",
          type: "http",
          tool: {
            name: "schedule_meeting",
            description: "Schedule a meeting",
          },
          httpConfig: {
            endpoint: "http://calendar-service:8080/meetings",
            method: "POST",
          },
        },
      ],
    },
    status: {
      phase: "Degraded",
      discoveredToolsCount: 2,
      discoveredTools: [
        {
          name: "search_contacts",
          handlerName: "salesforce",
          description: "Search CRM contacts",
          endpoint: "http://crm-adapter:8080/contacts/search",
          status: "Unavailable",
          lastChecked: hoursAgo(0.05),
          error: "Connection refused: service unreachable",
        },
        {
          name: "schedule_meeting",
          handlerName: "calendar",
          description: "Schedule a meeting",
          endpoint: "http://calendar-service:8080/meetings",
          status: "Available",
          lastChecked: hoursAgo(0.05),
        },
      ],
      lastDiscoveryTime: hoursAgo(0.05),
    },
  },
];

// Summary stats
export function getMockStats() {
  const agents = mockAgentRuntimes;
  const running = agents.filter((a) => a.status?.phase === "Running").length;
  const pending = agents.filter((a) => a.status?.phase === "Pending").length;
  const failed = agents.filter((a) => a.status?.phase === "Failed").length;

  const packs = mockPromptPacks;
  const activePacks = packs.filter((p) => p.status?.phase === "Active").length;
  const canaryPacks = packs.filter((p) => p.status?.phase === "Canary").length;

  const registries = mockToolRegistries;
  const totalTools = registries.reduce(
    (sum, r) => sum + (r.status?.discoveredToolsCount || 0),
    0
  );
  const availableTools = registries.reduce(
    (sum, r) =>
      sum +
      (r.status?.discoveredTools?.filter((t) => t.status === "Available").length || 0),
    0
  );

  return {
    agents: { total: agents.length, running, pending, failed },
    promptPacks: { total: packs.length, active: activePacks, canary: canaryPacks },
    tools: { total: totalTools, available: availableTools, degraded: totalTools - availableTools },
    sessions: { active: 1847 }, // Mock value
  };
}
