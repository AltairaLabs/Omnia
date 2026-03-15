/**
 * WebSocket protocol types for agent communication.
 *
 * Protocol types (ClientMessage, ServerMessage, ToolCallInfo, etc.) are
 * auto-generated from Go structs in internal/facade/protocol.go via tygo.
 * Run 'make generate-websocket-types' to regenerate.
 *
 * This file re-exports those generated types and adds dashboard-specific
 * UI types that don't exist in the Go protocol layer.
 */

// Re-export all generated protocol types
export type {
  ContentPart,
  ContentPartType,
  ClientMessage,
  ClientToolResultInfo,
  ServerMessage,
  ToolCallInfo,
  ToolResultInfo,
  ErrorInfo,
  UploadRequestInfo,
  UploadReadyInfo,
  UploadCompleteInfo,
  MediaChunkInfo,
  ConnectionCapabilities,
  ConnectedInfo,
} from "./generated/websocket";

export {
  ContentPartTypeText,
  ContentPartTypeImage,
  ContentPartTypeAudio,
  ContentPartTypeVideo,
  ContentPartTypeFile,
  MessageTypeMessage,
  MessageTypeUploadRequest,
  MessageTypeToolResult,
  MessageTypeChunk,
  MessageTypeDone,
  MessageTypeToolCall,
  MessageTypeError,
  MessageTypeConnected,
  MessageTypeUploadReady,
  MessageTypeUploadComplete,
  MessageTypeMediaChunk,
  ErrorCodeInvalidMessage,
  ErrorCodeSessionNotFound,
  ErrorCodeSessionExpired,
  ErrorCodeInternalError,
  ErrorCodeAgentUnavailable,
  ErrorCodeToolFailed,
  ErrorCodeUploadFailed,
  ErrorCodeMediaNotEnabled,
  ErrorCodeRateLimited,
} from "./generated/websocket";

export type { MediaContent } from "./generated/websocket";

// Re-export MessageType — generated as `string`, but we keep the union alias
// for backward compatibility with code that uses the literal union.
export type { MessageType } from "./generated/websocket";

// ─── Dashboard-specific UI types (not part of the WebSocket protocol) ────────

// File attachment for messages
export interface FileAttachment {
  id: string;
  name: string;
  type: string;
  size: number;
  dataUrl: string; // base64 data URL for preview and sending
}

// Console message types for UI rendering
export type ConsoleMessageRole = "user" | "assistant" | "system";

export interface ConsoleMessage {
  id: string;
  role: ConsoleMessageRole;
  content: string;
  timestamp: Date;
  toolCalls?: ToolCallWithResult[];
  attachments?: FileAttachment[];
  isStreaming?: boolean;
}

export interface ToolCallWithResult {
  id: string;
  name: string;
  arguments?: Record<string, unknown>;
  result?: unknown;
  error?: string;
  status: "pending" | "awaiting_consent" | "success" | "error";
  consent_message?: string;
  categories?: string[];
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
