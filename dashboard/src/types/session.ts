// Session types for conversation history

export interface ToolCall {
  id: string;
  name: string;
  arguments: Record<string, unknown>;
  result?: unknown;
  status: "pending" | "success" | "error";
  duration?: number; // ms
}

export interface Message {
  id: string;
  role: "user" | "assistant" | "system" | "tool";
  content: string;
  timestamp: string; // ISO date string
  toolCalls?: ToolCall[];
  toolCallId?: string; // for tool response messages
  tokens?: {
    input?: number;
    output?: number;
  };
}

export interface Session {
  id: string;
  agentName: string;
  agentNamespace: string;
  status: "active" | "completed" | "error" | "expired";
  startedAt: string; // ISO date string
  endedAt?: string; // ISO date string
  messages: Message[];
  metadata?: {
    userAgent?: string;
    clientIp?: string;
    tags?: string[];
  };
  metrics: {
    messageCount: number;
    toolCallCount: number;
    totalTokens: number;
    inputTokens: number;
    outputTokens: number;
    estimatedCost?: number; // USD
    avgResponseTime?: number; // ms
  };
}

export interface SessionSummary {
  id: string;
  agentName: string;
  agentNamespace: string;
  status: Session["status"];
  startedAt: string;
  endedAt?: string;
  messageCount: number;
  toolCallCount: number;
  totalTokens: number;
  lastMessage?: string;
}
