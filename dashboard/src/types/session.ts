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

// Options for listing sessions
export interface SessionListOptions {
  agent?: string;
  status?: Session["status"];
  from?: string; // ISO date string
  to?: string; // ISO date string
  limit?: number;
  offset?: number;
}

// Options for searching sessions (extends list options with query)
export interface SessionSearchOptions extends SessionListOptions {
  q: string;
}

// Options for fetching session messages
export interface SessionMessageOptions {
  limit?: number;
  before?: number; // sequence number
  after?: number; // sequence number
}

// Response shape for session list/search endpoints
export interface SessionListResponse {
  sessions: SessionSummary[];
  total: number;
  hasMore: boolean;
}

// Response shape for session messages endpoint
export interface SessionMessagesResponse {
  messages: Message[];
  hasMore: boolean;
}
