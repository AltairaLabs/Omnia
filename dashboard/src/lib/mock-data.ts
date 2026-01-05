// Mock data for development and testing

import type {
  AgentRuntime,
  PromptPack,
  ToolRegistry,
  Session,
  SessionSummary,
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
      framework: { type: "promptkit", version: "0.4.0" },
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
      framework: { type: "langchain", version: "0.3.0" },
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
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ToolRegistry",
    metadata: {
      name: "mcp-tools",
      namespace: "production",
      creationTimestamp: daysAgo(5),
      uid: "registry-003",
    },
    spec: {
      handlers: [
        {
          name: "filesystem",
          type: "mcp",
          tool: {
            name: "read_file",
            description: "Read contents of a file from the filesystem",
            inputSchema: {
              type: "object",
              properties: {
                path: { type: "string", description: "Path to the file" },
              },
              required: ["path"],
            },
          },
          mcpConfig: {
            transport: "stdio",
            command: "/usr/local/bin/mcp-filesystem",
            args: ["--root", "/data"],
          },
        },
        {
          name: "web-search",
          type: "mcp",
          tool: {
            name: "search_web",
            description: "Search the web for information",
            inputSchema: {
              type: "object",
              properties: {
                query: { type: "string", description: "Search query" },
                limit: { type: "number", description: "Max results", default: 10 },
              },
              required: ["query"],
            },
          },
          mcpConfig: {
            transport: "sse",
            endpoint: "http://mcp-search:8080/sse",
          },
        },
      ],
    },
    status: {
      phase: "Ready",
      discoveredToolsCount: 2,
      discoveredTools: [
        {
          name: "read_file",
          handlerName: "filesystem",
          description: "Read contents of a file from the filesystem",
          endpoint: "stdio:///usr/local/bin/mcp-filesystem",
          status: "Available",
          lastChecked: hoursAgo(0.2),
          inputSchema: {
            type: "object",
            properties: {
              path: { type: "string", description: "Path to the file" },
            },
            required: ["path"],
          },
        },
        {
          name: "search_web",
          handlerName: "web-search",
          description: "Search the web for information",
          endpoint: "http://mcp-search:8080/sse",
          status: "Available",
          lastChecked: hoursAgo(0.2),
          inputSchema: {
            type: "object",
            properties: {
              query: { type: "string", description: "Search query" },
              limit: { type: "number", description: "Max results", default: 10 },
            },
            required: ["query"],
          },
        },
      ],
      lastDiscoveryTime: hoursAgo(0.2),
    },
  },
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ToolRegistry",
    metadata: {
      name: "grpc-services",
      namespace: "production",
      creationTimestamp: daysAgo(21),
      uid: "registry-004",
    },
    spec: {
      handlers: [
        {
          name: "user-service",
          type: "grpc",
          tool: {
            name: "get_user",
            description: "Retrieve user information by ID",
            inputSchema: {
              type: "object",
              properties: {
                user_id: { type: "string", description: "User ID" },
              },
              required: ["user_id"],
            },
          },
          grpcConfig: {
            endpoint: "user-service.default.svc.cluster.local:9090",
            tls: true,
          },
          timeout: "5s",
          retries: 3,
        },
        {
          name: "notification-service",
          type: "grpc",
          tool: {
            name: "send_notification",
            description: "Send a notification to a user",
            inputSchema: {
              type: "object",
              properties: {
                user_id: { type: "string" },
                message: { type: "string" },
                channel: { type: "string", enum: ["email", "sms", "push"] },
              },
              required: ["user_id", "message", "channel"],
            },
          },
          grpcConfig: {
            endpoint: "notification-service.default.svc.cluster.local:9090",
            tls: true,
          },
          timeout: "10s",
        },
      ],
    },
    status: {
      phase: "Ready",
      discoveredToolsCount: 2,
      discoveredTools: [
        {
          name: "get_user",
          handlerName: "user-service",
          description: "Retrieve user information by ID",
          endpoint: "grpc://user-service.default.svc.cluster.local:9090",
          status: "Available",
          lastChecked: hoursAgo(0.15),
          inputSchema: {
            type: "object",
            properties: {
              user_id: { type: "string", description: "User ID" },
            },
            required: ["user_id"],
          },
        },
        {
          name: "send_notification",
          handlerName: "notification-service",
          description: "Send a notification to a user",
          endpoint: "grpc://notification-service.default.svc.cluster.local:9090",
          status: "Available",
          lastChecked: hoursAgo(0.15),
          inputSchema: {
            type: "object",
            properties: {
              user_id: { type: "string" },
              message: { type: "string" },
              channel: { type: "string", enum: ["email", "sms", "push"] },
            },
            required: ["user_id", "message", "channel"],
          },
        },
      ],
      lastDiscoveryTime: hoursAgo(0.15),
    },
  },
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ToolRegistry",
    metadata: {
      name: "openapi-services",
      namespace: "staging",
      creationTimestamp: daysAgo(2),
      uid: "registry-005",
    },
    spec: {
      handlers: [
        {
          name: "petstore-api",
          type: "openapi",
          openAPIConfig: {
            specURL: "https://petstore.swagger.io/v2/swagger.json",
            baseURL: "https://petstore.swagger.io/v2",
            operationFilter: ["getPetById", "findPetsByStatus", "addPet"],
          },
        },
      ],
    },
    status: {
      phase: "Pending",
      discoveredToolsCount: 0,
      discoveredTools: [],
      conditions: [
        {
          type: "Discovered",
          status: "False",
          lastTransitionTime: hoursAgo(0.5),
          reason: "SpecFetchFailed",
          message: "Failed to fetch OpenAPI spec: connection timeout",
        },
      ],
    },
  },
];

// Helper for minutes ago
const minutesAgo = (minutes: number) =>
  new Date(Date.now() - minutes * 60 * 1000).toISOString();

// Mock Sessions
export const mockSessions: Session[] = [
  {
    id: "sess-001",
    agentName: "customer-support",
    agentNamespace: "production",
    status: "completed",
    startedAt: hoursAgo(2),
    endedAt: hoursAgo(1.5),
    messages: [
      {
        id: "msg-001-1",
        role: "user",
        content: "Hi, I need help with my recent order #12345. It hasn't arrived yet.",
        timestamp: hoursAgo(2),
      },
      {
        id: "msg-001-2",
        role: "assistant",
        content: "I'd be happy to help you track your order #12345. Let me look that up for you.",
        timestamp: hoursAgo(1.98),
        toolCalls: [
          {
            id: "tc-001-1",
            name: "lookup_order",
            arguments: { order_id: "12345" },
            result: {
              status: "shipped",
              carrier: "FedEx",
              tracking: "FX123456789",
              estimated_delivery: "2026-01-07",
            },
            status: "success",
            duration: 245,
          },
        ],
        tokens: { output: 42 },
      },
      {
        id: "msg-001-3",
        role: "tool",
        content: JSON.stringify({
          status: "shipped",
          carrier: "FedEx",
          tracking: "FX123456789",
          estimated_delivery: "2026-01-07",
        }),
        timestamp: hoursAgo(1.97),
        toolCallId: "tc-001-1",
      },
      {
        id: "msg-001-4",
        role: "assistant",
        content: "Great news! Your order #12345 has been shipped via FedEx. The tracking number is FX123456789 and it's estimated to arrive on January 7th, 2026. Would you like me to send you the tracking link?",
        timestamp: hoursAgo(1.95),
        tokens: { output: 58 },
      },
      {
        id: "msg-001-5",
        role: "user",
        content: "Yes please, and can you also update my email address?",
        timestamp: hoursAgo(1.8),
        tokens: { input: 14 },
      },
      {
        id: "msg-001-6",
        role: "assistant",
        content: "I'll send you the tracking link right away. For updating your email address, I'll need to verify your identity first. Could you please confirm the last 4 digits of the phone number on your account?",
        timestamp: hoursAgo(1.78),
        tokens: { output: 48 },
      },
      {
        id: "msg-001-7",
        role: "user",
        content: "It's 5678",
        timestamp: hoursAgo(1.7),
        tokens: { input: 4 },
      },
      {
        id: "msg-001-8",
        role: "assistant",
        content: "Thank you for verifying. What would you like your new email address to be?",
        timestamp: hoursAgo(1.68),
        tokens: { output: 18 },
      },
    ],
    metrics: {
      messageCount: 8,
      toolCallCount: 1,
      totalTokens: 312,
      inputTokens: 78,
      outputTokens: 234,
      estimatedCost: 0.0047,
      avgResponseTime: 890,
    },
  },
  {
    id: "sess-002",
    agentName: "code-assistant",
    agentNamespace: "production",
    status: "active",
    startedAt: minutesAgo(15),
    messages: [
      {
        id: "msg-002-1",
        role: "user",
        content: "Can you help me write a TypeScript function to validate email addresses?",
        timestamp: minutesAgo(15),
        tokens: { input: 16 },
      },
      {
        id: "msg-002-2",
        role: "assistant",
        content: "I'll help you create a robust email validation function in TypeScript. Here's a well-tested implementation:\n\n```typescript\nfunction isValidEmail(email: string): boolean {\n  const emailRegex = /^[^\\s@]+@[^\\s@]+\\.[^\\s@]+$/;\n  return emailRegex.test(email);\n}\n```\n\nThis regex checks for:\n- At least one character before the @\n- At least one character between @ and .\n- At least one character after the final .\n- No whitespace anywhere\n\nWould you like me to add more comprehensive validation like checking for valid TLDs?",
        timestamp: minutesAgo(14.5),
        tokens: { output: 124 },
      },
      {
        id: "msg-002-3",
        role: "user",
        content: "Yes, and can you also add a function to normalize the email to lowercase?",
        timestamp: minutesAgo(10),
        tokens: { input: 18 },
      },
      {
        id: "msg-002-4",
        role: "assistant",
        content: "Here's an enhanced version with normalization and more comprehensive validation:\n\n```typescript\ninterface EmailValidationResult {\n  isValid: boolean;\n  normalized: string | null;\n  error?: string;\n}\n\nfunction validateAndNormalizeEmail(email: string): EmailValidationResult {\n  // Normalize: trim and lowercase\n  const normalized = email.trim().toLowerCase();\n  \n  // Check for empty input\n  if (!normalized) {\n    return { isValid: false, normalized: null, error: 'Email is required' };\n  }\n  \n  // Comprehensive regex for email validation\n  const emailRegex = /^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$/;\n  \n  if (!emailRegex.test(normalized)) {\n    return { isValid: false, normalized: null, error: 'Invalid email format' };\n  }\n  \n  return { isValid: true, normalized };\n}\n```\n\nThis function returns an object with the validation status, normalized email, and any error message.",
        timestamp: minutesAgo(9),
        tokens: { output: 248 },
      },
    ],
    metrics: {
      messageCount: 4,
      toolCallCount: 0,
      totalTokens: 406,
      inputTokens: 34,
      outputTokens: 372,
      estimatedCost: 0.0061,
      avgResponseTime: 1250,
    },
  },
  {
    id: "sess-003",
    agentName: "data-analyst",
    agentNamespace: "production",
    status: "completed",
    startedAt: hoursAgo(5),
    endedAt: hoursAgo(4.5),
    messages: [
      {
        id: "msg-003-1",
        role: "user",
        content: "Show me the top 10 products by revenue this month",
        timestamp: hoursAgo(5),
        tokens: { input: 12 },
      },
      {
        id: "msg-003-2",
        role: "assistant",
        content: "I'll query the sales data to find the top 10 products by revenue for this month.",
        timestamp: hoursAgo(4.98),
        toolCalls: [
          {
            id: "tc-003-1",
            name: "execute_sql",
            arguments: {
              query: "SELECT product_name, SUM(revenue) as total_revenue FROM sales WHERE date >= DATE_TRUNC('month', CURRENT_DATE) GROUP BY product_name ORDER BY total_revenue DESC LIMIT 10",
            },
            result: {
              rows: [
                { product_name: "Enterprise License", total_revenue: 125000 },
                { product_name: "Pro Subscription", total_revenue: 89500 },
                { product_name: "Team Package", total_revenue: 67800 },
              ],
            },
            status: "success",
            duration: 1250,
          },
        ],
        tokens: { output: 28 },
      },
      {
        id: "msg-003-3",
        role: "tool",
        content: JSON.stringify({
          rows: [
            { product_name: "Enterprise License", total_revenue: 125000 },
            { product_name: "Pro Subscription", total_revenue: 89500 },
            { product_name: "Team Package", total_revenue: 67800 },
          ],
        }),
        timestamp: hoursAgo(4.95),
        toolCallId: "tc-003-1",
      },
      {
        id: "msg-003-4",
        role: "assistant",
        content: "Here are the top 10 products by revenue this month:\n\n| Rank | Product | Revenue |\n|------|---------|--------|\n| 1 | Enterprise License | $125,000 |\n| 2 | Pro Subscription | $89,500 |\n| 3 | Team Package | $67,800 |\n\nThe Enterprise License is leading with $125,000 in revenue. Would you like me to generate a chart or drill down into any specific product?",
        timestamp: hoursAgo(4.93),
        toolCalls: [
          {
            id: "tc-003-2",
            name: "generate_chart",
            arguments: {
              type: "bar",
              data: [
                { label: "Enterprise License", value: 125000 },
                { label: "Pro Subscription", value: 89500 },
                { label: "Team Package", value: 67800 },
              ],
              title: "Top Products by Revenue",
            },
            result: { chart_url: "https://charts.example.com/abc123" },
            status: "success",
            duration: 890,
          },
        ],
        tokens: { output: 112 },
      },
    ],
    metrics: {
      messageCount: 4,
      toolCallCount: 2,
      totalTokens: 245,
      inputTokens: 12,
      outputTokens: 233,
      estimatedCost: 0.0037,
      avgResponseTime: 1450,
    },
  },
  {
    id: "sess-004",
    agentName: "customer-support",
    agentNamespace: "production",
    status: "error",
    startedAt: hoursAgo(8),
    endedAt: hoursAgo(7.9),
    messages: [
      {
        id: "msg-004-1",
        role: "user",
        content: "I want to cancel my subscription immediately and get a full refund",
        timestamp: hoursAgo(8),
        tokens: { input: 14 },
      },
      {
        id: "msg-004-2",
        role: "assistant",
        content: "I understand you'd like to cancel your subscription. Let me look up your account details.",
        timestamp: hoursAgo(7.98),
        toolCalls: [
          {
            id: "tc-004-1",
            name: "lookup_subscription",
            arguments: { user_id: "user-789" },
            status: "error",
            duration: 5000,
          },
        ],
        tokens: { output: 22 },
      },
    ],
    metadata: {
      tags: ["escalation-needed", "refund-request"],
    },
    metrics: {
      messageCount: 2,
      toolCallCount: 1,
      totalTokens: 36,
      inputTokens: 14,
      outputTokens: 22,
      estimatedCost: 0.0005,
      avgResponseTime: 5200,
    },
  },
  {
    id: "sess-005",
    agentName: "sales-copilot",
    agentNamespace: "production",
    status: "completed",
    startedAt: daysAgo(1),
    endedAt: hoursAgo(23),
    messages: [
      {
        id: "msg-005-1",
        role: "user",
        content: "Find me all enterprise leads from the tech sector that haven't been contacted in 30 days",
        timestamp: daysAgo(1),
        tokens: { input: 20 },
      },
      {
        id: "msg-005-2",
        role: "assistant",
        content: "I'll search for enterprise leads in the tech sector with no recent contact.",
        timestamp: hoursAgo(23.98),
        toolCalls: [
          {
            id: "tc-005-1",
            name: "search_contacts",
            arguments: {
              segment: "enterprise",
              industry: "technology",
              last_contact_before: "30d",
            },
            result: {
              count: 15,
              contacts: [
                { name: "John Smith", company: "TechCorp", last_contact: "45 days ago" },
                { name: "Jane Doe", company: "DataSystems", last_contact: "38 days ago" },
              ],
            },
            status: "success",
            duration: 2100,
          },
        ],
        tokens: { output: 24 },
      },
      {
        id: "msg-005-3",
        role: "tool",
        content: JSON.stringify({
          count: 15,
          contacts: [
            { name: "John Smith", company: "TechCorp", last_contact: "45 days ago" },
            { name: "Jane Doe", company: "DataSystems", last_contact: "38 days ago" },
          ],
        }),
        timestamp: hoursAgo(23.95),
        toolCallId: "tc-005-1",
      },
      {
        id: "msg-005-4",
        role: "assistant",
        content: "I found 15 enterprise tech leads that haven't been contacted in over 30 days. Here are the top ones:\n\n1. **John Smith** - TechCorp (45 days)\n2. **Jane Doe** - DataSystems (38 days)\n\nWould you like me to schedule follow-up meetings with any of these contacts?",
        timestamp: hoursAgo(23.9),
        tokens: { output: 68 },
      },
      {
        id: "msg-005-5",
        role: "user",
        content: "Yes, schedule meetings with both for next week",
        timestamp: hoursAgo(23.5),
        tokens: { input: 10 },
      },
      {
        id: "msg-005-6",
        role: "assistant",
        content: "I'll schedule meetings with both contacts for next week.",
        timestamp: hoursAgo(23.48),
        toolCalls: [
          {
            id: "tc-005-2",
            name: "schedule_meeting",
            arguments: {
              contact: "John Smith",
              date: "2026-01-08",
              time: "10:00",
              duration: 30,
            },
            result: { meeting_id: "mtg-123", confirmed: true },
            status: "success",
            duration: 450,
          },
          {
            id: "tc-005-3",
            name: "schedule_meeting",
            arguments: {
              contact: "Jane Doe",
              date: "2026-01-08",
              time: "14:00",
              duration: 30,
            },
            result: { meeting_id: "mtg-124", confirmed: true },
            status: "success",
            duration: 480,
          },
        ],
        tokens: { output: 18 },
      },
    ],
    metrics: {
      messageCount: 6,
      toolCallCount: 3,
      totalTokens: 280,
      inputTokens: 30,
      outputTokens: 250,
      estimatedCost: 0.0042,
      avgResponseTime: 980,
    },
  },
  {
    id: "sess-006",
    agentName: "code-assistant",
    agentNamespace: "production",
    status: "expired",
    startedAt: daysAgo(3),
    endedAt: daysAgo(3) + "T01:00:00.000Z",
    messages: [
      {
        id: "msg-006-1",
        role: "user",
        content: "Help me debug this React component",
        timestamp: daysAgo(3),
        tokens: { input: 8 },
      },
    ],
    metrics: {
      messageCount: 1,
      toolCallCount: 0,
      totalTokens: 8,
      inputTokens: 8,
      outputTokens: 0,
      estimatedCost: 0.0001,
    },
  },
];

// Get session summaries for list view
export function getMockSessionSummaries(): SessionSummary[] {
  return mockSessions.map((session) => ({
    id: session.id,
    agentName: session.agentName,
    agentNamespace: session.agentNamespace,
    status: session.status,
    startedAt: session.startedAt,
    endedAt: session.endedAt,
    messageCount: session.metrics.messageCount,
    toolCallCount: session.metrics.toolCallCount,
    totalTokens: session.metrics.totalTokens,
    lastMessage: session.messages[session.messages.length - 1]?.content.slice(0, 100),
  }));
}

// Get session by ID
export function getMockSession(id: string): Session | undefined {
  return mockSessions.find((s) => s.id === id);
}

// Mock usage data per agent (for cost tracking)
export interface AgentUsageData {
  agentName: string;
  agentNamespace: string;
  model: string;
  period: "24h" | "7d" | "30d";
  inputTokens: number;
  outputTokens: number;
  cacheHits: number;
  requestCount: number;
  errorCount: number;
  avgLatencyMs: number;
  // Time series data for charts
  timeSeries: {
    timestamp: string;
    inputTokens: number;
    outputTokens: number;
    requests: number;
  }[];
}

// Generate time series data for the last N hours
function generateTimeSeries(hours: number, baseInput: number, baseOutput: number) {
  const series = [];
  for (let i = hours; i > 0; i--) {
    const variance = 0.5 + Math.random(); // 50% to 150% of base
    series.push({
      timestamp: hoursAgo(i),
      inputTokens: Math.floor(baseInput * variance),
      outputTokens: Math.floor(baseOutput * variance),
      requests: Math.floor(10 + Math.random() * 50),
    });
  }
  return series;
}

// Mock usage data for each agent
export const mockAgentUsage: Record<string, AgentUsageData> = {
  "production/customer-support": {
    agentName: "customer-support",
    agentNamespace: "production",
    model: "claude-sonnet-4-20250514",
    period: "24h",
    inputTokens: 2_450_000,
    outputTokens: 1_890_000,
    cacheHits: 450_000,
    requestCount: 3_247,
    errorCount: 12,
    avgLatencyMs: 890,
    timeSeries: generateTimeSeries(24, 100_000, 78_000),
  },
  "production/code-assistant": {
    agentName: "code-assistant",
    agentNamespace: "production",
    model: "claude-sonnet-4-20250514",
    period: "24h",
    inputTokens: 1_780_000,
    outputTokens: 2_340_000,
    cacheHits: 320_000,
    requestCount: 1_892,
    errorCount: 5,
    avgLatencyMs: 1250,
    timeSeries: generateTimeSeries(24, 74_000, 97_000),
  },
  "production/data-analyst": {
    agentName: "data-analyst",
    agentNamespace: "production",
    model: "gpt-4-turbo",
    period: "24h",
    inputTokens: 890_000,
    outputTokens: 1_120_000,
    cacheHits: 0,
    requestCount: 856,
    errorCount: 23,
    avgLatencyMs: 1450,
    timeSeries: generateTimeSeries(24, 37_000, 46_000),
  },
  "production/onboarding-bot": {
    agentName: "onboarding-bot",
    agentNamespace: "production",
    model: "claude-sonnet-4-20250514",
    period: "24h",
    inputTokens: 0,
    outputTokens: 0,
    cacheHits: 0,
    requestCount: 0,
    errorCount: 0,
    avgLatencyMs: 0,
    timeSeries: [],
  },
  "production/sales-copilot": {
    agentName: "sales-copilot",
    agentNamespace: "production",
    model: "gpt-4-turbo",
    period: "24h",
    inputTokens: 1_230_000,
    outputTokens: 1_560_000,
    cacheHits: 0,
    requestCount: 1_124,
    errorCount: 8,
    avgLatencyMs: 980,
    timeSeries: generateTimeSeries(24, 51_000, 65_000),
  },
  "staging/legacy-agent": {
    agentName: "legacy-agent",
    agentNamespace: "staging",
    model: "gpt-3.5-turbo",
    period: "24h",
    inputTokens: 0,
    outputTokens: 0,
    cacheHits: 0,
    requestCount: 0,
    errorCount: 47,
    avgLatencyMs: 0,
    timeSeries: [],
  },
};

// Get usage data for a specific agent
export function getMockAgentUsage(namespace: string, name: string): AgentUsageData | undefined {
  return mockAgentUsage[`${namespace}/${name}`];
}

// Get aggregated usage stats for all agents
export function getMockAggregatedUsage() {
  const allUsage = Object.values(mockAgentUsage);

  const totalInputTokens = allUsage.reduce((sum, u) => sum + u.inputTokens, 0);
  const totalOutputTokens = allUsage.reduce((sum, u) => sum + u.outputTokens, 0);
  const totalRequests = allUsage.reduce((sum, u) => sum + u.requestCount, 0);
  const totalErrors = allUsage.reduce((sum, u) => sum + u.errorCount, 0);

  // Group by model
  const byModel: Record<string, { inputTokens: number; outputTokens: number; requests: number }> = {};
  for (const usage of allUsage) {
    if (!byModel[usage.model]) {
      byModel[usage.model] = { inputTokens: 0, outputTokens: 0, requests: 0 };
    }
    byModel[usage.model].inputTokens += usage.inputTokens;
    byModel[usage.model].outputTokens += usage.outputTokens;
    byModel[usage.model].requests += usage.requestCount;
  }

  return {
    totalInputTokens,
    totalOutputTokens,
    totalTokens: totalInputTokens + totalOutputTokens,
    totalRequests,
    totalErrors,
    errorRate: totalRequests > 0 ? (totalErrors / totalRequests) * 100 : 0,
    byModel,
  };
}

// ============================================================================
// Cost Allocation Data (for /costs page)
// ============================================================================

export interface CostAllocationItem {
  agent: string;
  namespace: string;
  provider: "anthropic" | "openai";
  model: string;
  team?: string;
  inputTokens: number;
  outputTokens: number;
  cacheHits: number;
  requests: number;
  inputCost: number;
  outputCost: number;
  cacheSavings: number;
  totalCost: number;
}

export interface CostTimeSeriesPoint {
  timestamp: string;
  anthropic: number;
  openai: number;
  total: number;
}

// Detailed cost allocation per agent (derived from mockAgentUsage with pricing applied)
export const mockCostAllocation: CostAllocationItem[] = [
  {
    agent: "customer-support",
    namespace: "production",
    provider: "anthropic",
    model: "claude-sonnet-4-20250514",
    team: "support",
    inputTokens: 2_450_000,
    outputTokens: 1_890_000,
    cacheHits: 450_000,
    requests: 3_247,
    inputCost: 7.35, // $3/1M * 2.45M
    outputCost: 28.35, // $15/1M * 1.89M
    cacheSavings: 1.22, // ($3 - $0.30)/1M * 0.45M
    totalCost: 35.70,
  },
  {
    agent: "code-assistant",
    namespace: "production",
    provider: "anthropic",
    model: "claude-sonnet-4-20250514",
    team: "engineering",
    inputTokens: 1_780_000,
    outputTokens: 2_340_000,
    cacheHits: 320_000,
    requests: 1_892,
    inputCost: 5.34,
    outputCost: 35.10,
    cacheSavings: 0.86,
    totalCost: 40.44,
  },
  {
    agent: "data-analyst",
    namespace: "production",
    provider: "openai",
    model: "gpt-4-turbo",
    team: "data",
    inputTokens: 890_000,
    outputTokens: 1_120_000,
    cacheHits: 0,
    requests: 856,
    inputCost: 8.90, // $10/1M
    outputCost: 33.60, // $30/1M
    cacheSavings: 0,
    totalCost: 42.50,
  },
  {
    agent: "sales-copilot",
    namespace: "production",
    provider: "openai",
    model: "gpt-4-turbo",
    team: "sales",
    inputTokens: 1_230_000,
    outputTokens: 1_560_000,
    cacheHits: 0,
    requests: 1_124,
    inputCost: 12.30,
    outputCost: 46.80,
    cacheSavings: 0,
    totalCost: 59.10,
  },
  {
    agent: "onboarding-bot",
    namespace: "production",
    provider: "anthropic",
    model: "claude-sonnet-4-20250514",
    team: "hr",
    inputTokens: 0,
    outputTokens: 0,
    cacheHits: 0,
    requests: 0,
    inputCost: 0,
    outputCost: 0,
    cacheSavings: 0,
    totalCost: 0,
  },
  {
    agent: "legacy-agent",
    namespace: "staging",
    provider: "openai",
    model: "gpt-3.5-turbo",
    team: "platform",
    inputTokens: 0,
    outputTokens: 0,
    cacheHits: 0,
    requests: 0,
    inputCost: 0,
    outputCost: 0,
    cacheSavings: 0,
    totalCost: 0,
  },
];

// Generate cost time series for the last 24 hours
function generateCostTimeSeries(hours: number): CostTimeSeriesPoint[] {
  const series: CostTimeSeriesPoint[] = [];
  for (let i = hours; i > 0; i--) {
    const hourVariance = 0.7 + Math.random() * 0.6; // 70% to 130%
    const anthropic = 3.2 * hourVariance; // ~$3.2/hour base for Anthropic
    const openai = 4.2 * hourVariance; // ~$4.2/hour base for OpenAI
    series.push({
      timestamp: hoursAgo(i),
      anthropic: Number(anthropic.toFixed(2)),
      openai: Number(openai.toFixed(2)),
      total: Number((anthropic + openai).toFixed(2)),
    });
  }
  return series;
}

export const mockCostTimeSeries = generateCostTimeSeries(24);

// Aggregate cost data by different dimensions
export function getMockCostByProvider() {
  const byProvider: Record<string, { cost: number; requests: number; tokens: number }> = {
    anthropic: { cost: 0, requests: 0, tokens: 0 },
    openai: { cost: 0, requests: 0, tokens: 0 },
  };

  for (const item of mockCostAllocation) {
    byProvider[item.provider].cost += item.totalCost;
    byProvider[item.provider].requests += item.requests;
    byProvider[item.provider].tokens += item.inputTokens + item.outputTokens;
  }

  return Object.entries(byProvider).map(([provider, data]) => ({
    name: provider === "anthropic" ? "Anthropic" : "OpenAI",
    provider,
    ...data,
  }));
}

export function getMockCostByModel() {
  const byModel: Record<string, { cost: number; requests: number; tokens: number; provider: string }> = {};

  for (const item of mockCostAllocation) {
    if (!byModel[item.model]) {
      byModel[item.model] = { cost: 0, requests: 0, tokens: 0, provider: item.provider };
    }
    byModel[item.model].cost += item.totalCost;
    byModel[item.model].requests += item.requests;
    byModel[item.model].tokens += item.inputTokens + item.outputTokens;
  }

  return Object.entries(byModel)
    .map(([model, data]) => ({
      model,
      displayName: getModelDisplayName(model),
      ...data,
    }))
    .sort((a, b) => b.cost - a.cost);
}

function getModelDisplayName(model: string): string {
  const names: Record<string, string> = {
    "claude-sonnet-4-20250514": "Claude Sonnet 4",
    "gpt-4-turbo": "GPT-4 Turbo",
    "gpt-3.5-turbo": "GPT-3.5 Turbo",
    "claude-opus-4-20250514": "Claude Opus 4",
  };
  return names[model] || model;
}

export function getMockCostByTeam() {
  const byTeam: Record<string, { cost: number; requests: number; agents: string[] }> = {};

  for (const item of mockCostAllocation) {
    const team = item.team || "unassigned";
    if (!byTeam[team]) {
      byTeam[team] = { cost: 0, requests: 0, agents: [] };
    }
    byTeam[team].cost += item.totalCost;
    byTeam[team].requests += item.requests;
    if (!byTeam[team].agents.includes(item.agent)) {
      byTeam[team].agents.push(item.agent);
    }
  }

  return Object.entries(byTeam)
    .map(([team, data]) => ({
      team,
      ...data,
      agentCount: data.agents.length,
    }))
    .sort((a, b) => b.cost - a.cost);
}

export function getMockCostSummary() {
  const totalCost = mockCostAllocation.reduce((sum, item) => sum + item.totalCost, 0);
  const totalInputCost = mockCostAllocation.reduce((sum, item) => sum + item.inputCost, 0);
  const totalOutputCost = mockCostAllocation.reduce((sum, item) => sum + item.outputCost, 0);
  const totalCacheSavings = mockCostAllocation.reduce((sum, item) => sum + item.cacheSavings, 0);
  const totalRequests = mockCostAllocation.reduce((sum, item) => sum + item.requests, 0);
  const totalTokens = mockCostAllocation.reduce(
    (sum, item) => sum + item.inputTokens + item.outputTokens,
    0
  );

  // Calculate costs by provider
  const byProvider = getMockCostByProvider();
  const anthropicCost = byProvider.find((p) => p.provider === "anthropic")?.cost || 0;
  const openaiCost = byProvider.find((p) => p.provider === "openai")?.cost || 0;

  // Calculate projected monthly cost (24h * 30 days)
  const projectedMonthlyCost = totalCost * 30;

  return {
    totalCost,
    totalInputCost,
    totalOutputCost,
    totalCacheSavings,
    totalRequests,
    totalTokens,
    anthropicCost,
    openaiCost,
    projectedMonthlyCost,
    // Percentage breakdowns
    anthropicPercent: totalCost > 0 ? (anthropicCost / totalCost) * 100 : 0,
    openaiPercent: totalCost > 0 ? (openaiCost / totalCost) * 100 : 0,
    inputPercent: totalCost > 0 ? (totalInputCost / (totalInputCost + totalOutputCost)) * 100 : 0,
    outputPercent: totalCost > 0 ? (totalOutputCost / (totalInputCost + totalOutputCost)) * 100 : 0,
  };
}

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
