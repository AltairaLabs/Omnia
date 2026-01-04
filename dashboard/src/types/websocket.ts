/**
 * WebSocket protocol types for agent communication.
 * These mirror the Go types in internal/facade/protocol.go
 */

// Message types
export type MessageType =
  | "message"      // Client → Server: user message
  | "chunk"        // Server → Client: streaming text chunk
  | "done"         // Server → Client: response complete
  | "tool_call"    // Server → Client: agent is calling a tool
  | "tool_result"  // Server → Client: tool execution result
  | "error"        // Server → Client: error occurred
  | "connected";   // Server → Client: connection established

// Client → Server message
export interface ClientMessage {
  type: "message";
  session_id?: string;
  content: string;
  metadata?: Record<string, string>;
}

// Server → Client message
export interface ServerMessage {
  type: MessageType;
  session_id?: string;
  content?: string;
  tool_call?: ToolCallInfo;
  tool_result?: ToolResultInfo;
  error?: ErrorInfo;
  timestamp: string;
}

export interface ToolCallInfo {
  id: string;
  name: string;
  arguments?: Record<string, unknown>;
}

export interface ToolResultInfo {
  id: string;
  result?: unknown;
  error?: string;
}

export interface ErrorInfo {
  code: string;
  message: string;
  details?: Record<string, unknown>;
}

// Error codes
export const ErrorCodes = {
  INVALID_MESSAGE: "INVALID_MESSAGE",
  SESSION_NOT_FOUND: "SESSION_NOT_FOUND",
  SESSION_EXPIRED: "SESSION_EXPIRED",
  INTERNAL_ERROR: "INTERNAL_ERROR",
  AGENT_UNAVAILABLE: "AGENT_UNAVAILABLE",
  TOOL_FAILED: "TOOL_FAILED",
} as const;

// Console message types for UI rendering
export type ConsoleMessageRole = "user" | "assistant" | "system";

export interface ConsoleMessage {
  id: string;
  role: ConsoleMessageRole;
  content: string;
  timestamp: Date;
  toolCalls?: ToolCallWithResult[];
  isStreaming?: boolean;
}

export interface ToolCallWithResult {
  id: string;
  name: string;
  arguments?: Record<string, unknown>;
  result?: unknown;
  error?: string;
  status: "pending" | "success" | "error";
}

// Connection status
export type ConnectionStatus = "disconnected" | "connecting" | "connected" | "error";

// Console state
export interface ConsoleState {
  sessionId: string | null;
  status: ConnectionStatus;
  messages: ConsoleMessage[];
  error: string | null;
}
