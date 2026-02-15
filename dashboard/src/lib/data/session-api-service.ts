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
  ToolCall,
  SessionListOptions,
  SessionSearchOptions,
  SessionMessageOptions,
  SessionListResponse,
  SessionMessagesResponse,
} from "@/types/session";
import type { EvalResult } from "@/types/eval";

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
 * Index tool_result messages by their toolCallId for fast lookup.
 */
function indexToolResults(apiMessages: ApiMessage[]): {
  resultsByToolCallId: Map<string, { content: string; isError: boolean }>;
  toolResultIds: Set<string>;
} {
  const resultsByToolCallId = new Map<string, { content: string; isError: boolean }>();
  const toolResultIds = new Set<string>();

  for (const api of apiMessages) {
    if (api.metadata?.type === "tool_result" && api.toolCallId) {
      resultsByToolCallId.set(api.toolCallId, {
        content: api.content,
        isError: api.metadata?.is_error === "true",
      });
      toolResultIds.add(api.id);
    }
  }

  return { resultsByToolCallId, toolResultIds };
}

/**
 * Parse a tool_call API message into a ToolCall object, pairing it with
 * its result if available.
 */
function buildToolCall(
  api: ApiMessage,
  resultsByToolCallId: Map<string, { content: string; isError: boolean }>,
): ToolCall {
  let name = "unknown";
  let args: Record<string, unknown> = {};
  try {
    const parsed = JSON.parse(api.content);
    name = parsed.name || name;
    args = parsed.arguments || args;
  } catch {
    // Content is not valid JSON
  }

  const result = resultsByToolCallId.get(api.toolCallId!);
  let parsedResult: unknown;
  if (result) {
    try {
      parsedResult = JSON.parse(result.content);
    } catch {
      parsedResult = result.content;
    }
  }

  let status: "pending" | "success" | "error" = "pending";
  if (result) {
    status = result.isError ? "error" : "success";
  }

  return {
    id: api.toolCallId!,
    name,
    arguments: args,
    result: result ? parsedResult : undefined,
    status,
  };
}

/**
 * Attach leftover tool calls to the last assistant message, or the last
 * message if no assistant message exists.
 */
function attachLeftoverToolCalls(output: Message[], toolCalls: ToolCall[]): void {
  if (toolCalls.length === 0) return;

  const lastAssistant = output.findLast((m) => m.role === "assistant");
  const target = lastAssistant ?? output.at(-1);
  if (target) {
    target.toolCalls = [...(target.toolCalls || []), ...toolCalls];
  }
}

/**
 * Pair tool_call and tool_result API messages into ToolCall objects attached
 * to the assistant "done" messages, then transform the result to Message[].
 *
 * The recording writer stores three separate messages per tool-use cycle:
 *   1. role=assistant, metadata.type=tool_call, toolCallId=X  (content = JSON {name, arguments})
 *   2. role=system,    metadata.type=tool_result, toolCallId=X (content = result data)
 *   3. role=assistant  (no metadata.type — the final "done" response)
 *
 * This function operates on raw ApiMessage objects (which have metadata) to
 * build the pairing, then transforms to Message[] for the UI.
 */
function transformAndPairMessages(apiMessages: ApiMessage[]): Message[] {
  const { resultsByToolCallId, toolResultIds } = indexToolResults(apiMessages);

  // Build ToolCall objects from tool_call messages and collect their IDs
  const toolCallIds = new Set<string>();
  const pendingToolCalls: ToolCall[] = [];
  for (const api of apiMessages) {
    if (api.metadata?.type === "tool_call" && api.toolCallId) {
      toolCallIds.add(api.id);
      pendingToolCalls.push(buildToolCall(api, resultsByToolCallId));
    }
  }

  // Transform non-tool messages and attach collected ToolCalls to
  // the next assistant "done" message
  const output: Message[] = [];
  let toolCallsToAttach = [...pendingToolCalls];

  for (const api of apiMessages) {
    if (toolCallIds.has(api.id) || toolResultIds.has(api.id)) continue;

    const msg = transformApiMessage(api);
    if (msg.role === "assistant" && toolCallsToAttach.length > 0) {
      msg.toolCalls = toolCallsToAttach;
      toolCallsToAttach = [];
    }
    output.push(msg);
  }

  attachLeftoverToolCalls(output, toolCallsToAttach);
  return output;
}

/**
 * Transform an API session to a full Session object.
 */
function transformApiSession(api: ApiSession): Session {
  const inputTokens = api.totalInputTokens || 0;
  const outputTokens = api.totalOutputTokens || 0;
  const messages = transformAndPairMessages(api.messages || []);

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
      messages: transformAndPairMessages(data.messages || []),
      hasMore: data.hasMore || false,
    };
  }

  async getSessionEvalResults(
    workspace: string,
    sessionId: string
  ): Promise<EvalResult[]> {
    const response = await fetch(
      `${SESSION_API_BASE}/${encodeURIComponent(workspace)}/sessions/${encodeURIComponent(sessionId)}/eval-results`
    );
    if (!response.ok) {
      if (response.status === 401 || response.status === 403 || response.status === 404) {
        return [];
      }
      throw new Error(`Failed to fetch eval results: ${response.statusText}`);
    }

    const data = await response.json();
    return data.evalResults || [];
  }
}
