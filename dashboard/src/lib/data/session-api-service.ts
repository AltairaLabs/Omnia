/**
 * Session API service for live session data.
 *
 * Calls the workspace-scoped session API proxy routes:
 *   /api/workspaces/{name}/sessions
 *   /api/workspaces/{name}/sessions/{sessionId}
 *   /api/workspaces/{name}/sessions/{sessionId}/messages
 *
 * This service is used by LiveDataService when fetching session data.
 * It transforms the Go session API response shape to match the dashboard TS types.
 */

import type {
  Session,
  SessionSummary,
  Message,
  SessionListOptions,
  SessionSearchOptions,
  SessionMessageOptions,
  SessionListResponse,
  SessionMessagesResponse,
} from "@/types/session";

const SESSION_API_BASE = "/api/workspaces";

/**
 * Raw session shape from the Go session API.
 * Field names match the Go JSON tags.
 */
interface ApiSession {
  id: string;
  agentName: string;
  namespace: string;
  createdAt: string;
  updatedAt: string;
  expiresAt?: string;
  endedAt?: string;
  status?: Session["status"];
  messages?: ApiMessage[];
  state?: Record<string, string>;
  workspaceName?: string;
  messageCount?: number;
  toolCallCount?: number;
  totalInputTokens?: number;
  totalOutputTokens?: number;
  estimatedCostUSD?: number;
  tags?: string[];
  lastMessagePreview?: string;
}

interface ApiMessage {
  id: string;
  role: Message["role"];
  content: string;
  timestamp: string;
  metadata?: Record<string, string>;
  inputTokens?: number;
  outputTokens?: number;
  toolCallId?: string;
  sequenceNum?: number;
}

/**
 * Transform an API session to a SessionSummary for list views.
 */
function transformApiSessionSummary(api: ApiSession): SessionSummary {
  const inputTokens = api.totalInputTokens || 0;
  const outputTokens = api.totalOutputTokens || 0;
  return {
    id: api.id,
    agentName: api.agentName,
    agentNamespace: api.namespace,
    status: api.status || "active",
    startedAt: api.createdAt,
    endedAt: api.endedAt,
    messageCount: api.messageCount || 0,
    toolCallCount: api.toolCallCount || 0,
    totalTokens: inputTokens + outputTokens,
    lastMessage: api.lastMessagePreview,
  };
}

/**
 * Transform an API message to the dashboard Message type.
 */
function transformApiMessage(api: ApiMessage): Message {
  return {
    id: api.id,
    role: api.role,
    content: api.content,
    timestamp: api.timestamp,
    toolCallId: api.toolCallId,
    tokens:
      api.inputTokens || api.outputTokens
        ? { input: api.inputTokens, output: api.outputTokens }
        : undefined,
  };
}

/**
 * Transform an API session to a full Session object.
 */
function transformApiSession(api: ApiSession): Session {
  const inputTokens = api.totalInputTokens || 0;
  const outputTokens = api.totalOutputTokens || 0;
  const messages = (api.messages || []).map(transformApiMessage);

  return {
    id: api.id,
    agentName: api.agentName,
    agentNamespace: api.namespace,
    status: api.status || "active",
    startedAt: api.createdAt,
    endedAt: api.endedAt,
    messages,
    metadata: {
      tags: api.tags,
    },
    metrics: {
      messageCount: api.messageCount || messages.length,
      toolCallCount: api.toolCallCount || 0,
      totalTokens: inputTokens + outputTokens,
      inputTokens,
      outputTokens,
      estimatedCost: api.estimatedCostUSD,
    },
  };
}

/**
 * Session API service that calls workspace-scoped session endpoints.
 */
export class SessionApiService {
  readonly name = "SessionApiService";

  async getSessions(
    workspace: string,
    options?: SessionListOptions
  ): Promise<SessionListResponse> {
    const params = new URLSearchParams();
    if (options?.agent) params.set("agent", options.agent);
    if (options?.status) params.set("status", options.status);
    if (options?.from) params.set("from", options.from);
    if (options?.to) params.set("to", options.to);
    if (options?.limit) params.set("limit", String(options.limit));
    if (options?.offset) params.set("offset", String(options.offset));

    const queryString = params.toString();
    const suffix = queryString ? `?${queryString}` : "";

    const response = await fetch(
      `${SESSION_API_BASE}/${encodeURIComponent(workspace)}/sessions${suffix}`
    );
    if (!response.ok) {
      if (response.status === 401 || response.status === 403 || response.status === 404) {
        return { sessions: [], total: 0, hasMore: false };
      }
      throw new Error(`Failed to fetch sessions: ${response.statusText}`);
    }

    const data = await response.json();
    return {
      sessions: (data.sessions || []).map(transformApiSessionSummary),
      total: data.total || 0,
      hasMore: data.hasMore || false,
    };
  }

  async getSessionById(
    workspace: string,
    sessionId: string
  ): Promise<Session | undefined> {
    const response = await fetch(
      `${SESSION_API_BASE}/${encodeURIComponent(workspace)}/sessions/${encodeURIComponent(sessionId)}`
    );
    if (!response.ok) {
      if (response.status === 404) {
        return undefined;
      }
      throw new Error(`Failed to fetch session: ${response.statusText}`);
    }

    const data = await response.json();
    // The Go API wraps the session in a { session, messages } envelope
    const apiSession: ApiSession = data.session || data;
    if (data.messages) {
      apiSession.messages = data.messages;
    }
    return transformApiSession(apiSession);
  }

  async searchSessions(
    workspace: string,
    options: SessionSearchOptions
  ): Promise<SessionListResponse> {
    const params = new URLSearchParams();
    params.set("q", options.q);
    if (options.agent) params.set("agent", options.agent);
    if (options.status) params.set("status", options.status);
    if (options.from) params.set("from", options.from);
    if (options.to) params.set("to", options.to);
    if (options.limit) params.set("limit", String(options.limit));
    if (options.offset) params.set("offset", String(options.offset));

    const response = await fetch(
      `${SESSION_API_BASE}/${encodeURIComponent(workspace)}/sessions?${params.toString()}`
    );
    if (!response.ok) {
      if (response.status === 401 || response.status === 403 || response.status === 404) {
        return { sessions: [], total: 0, hasMore: false };
      }
      throw new Error(`Failed to search sessions: ${response.statusText}`);
    }

    const data = await response.json();
    return {
      sessions: (data.sessions || []).map(transformApiSessionSummary),
      total: data.total || 0,
      hasMore: data.hasMore || false,
    };
  }

  async getSessionMessages(
    workspace: string,
    sessionId: string,
    options?: SessionMessageOptions
  ): Promise<SessionMessagesResponse> {
    const params = new URLSearchParams();
    if (options?.limit) params.set("limit", String(options.limit));
    if (options?.before !== undefined) params.set("before", String(options.before));
    if (options?.after !== undefined) params.set("after", String(options.after));

    const queryString = params.toString();
    const suffix = queryString ? `?${queryString}` : "";

    const response = await fetch(
      `${SESSION_API_BASE}/${encodeURIComponent(workspace)}/sessions/${encodeURIComponent(sessionId)}/messages${suffix}`
    );
    if (!response.ok) {
      if (response.status === 404) {
        return { messages: [], hasMore: false };
      }
      throw new Error(`Failed to fetch session messages: ${response.statusText}`);
    }

    const data = await response.json();
    return {
      messages: (data.messages || []).map(transformApiMessage),
      hasMore: data.hasMore || false,
    };
  }
}
