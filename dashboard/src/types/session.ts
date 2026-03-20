// Session types for conversation history

export interface ToolCall {
  id: string;
  callId: string;
  sessionId: string;
  name: string;
  arguments: Record<string, unknown>;
  result?: unknown;
  status: "pending" | "success" | "error";
  durationMs?: number;
  errorMessage?: string;
  labels?: Record<string, string>;
  createdAt: string;
}

export interface ProviderCall {
  id: string;
  sessionId: string;
  provider: string;
  model: string;
  status: "pending" | "completed" | "failed";
  inputTokens?: number;
  outputTokens?: number;
  cachedTokens?: number;
  costUsd?: number;
  durationMs?: number;
  finishReason?: string;
  toolCallCount?: number;
  errorMessage?: string;
  labels?: Record<string, string>;
  createdAt: string;
}

export interface RuntimeEvent {
  id: string;
  sessionId: string;
  eventType: string;
  data?: Record<string, unknown>;
  durationMs?: number;
  errorMessage?: string;
  timestamp: string;
}

/** Inline tool call embedded in a message (live console WebSocket path). */
export interface MessageToolCall {
  id: string;
  name: string;
  arguments: Record<string, unknown>;
  result?: unknown;
  status: string;
  duration?: number; // ms
  error?: string;
}

export interface Message {
  id: string;
  role: "user" | "assistant" | "system" | "tool";
  content: string;
  timestamp: string; // ISO date string
  toolCalls?: MessageToolCall[];
  toolCallId?: string; // for tool response messages
  metadata?: Record<string, string>;
  tokens?: {
    input?: number;
    output?: number;
  };
  sequenceNum?: number; // ordering within session, used for pagination cursors
  hasMedia?: boolean;
  mediaTypes?: string[];
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
  /** Request a total count from the server (triggers a separate COUNT query). */
  count?: boolean;
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
