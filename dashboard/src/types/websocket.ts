/**
 * WebSocket protocol types for agent communication.
 * These mirror the Go types in internal/facade/protocol.go
 *
 * NOTE: The proto types (api/proto/runtime/v1/runtime.proto) define the internal
 * gRPC protocol between facade and runtime. These WebSocket types define the
 * external protocol between dashboard and facade, which includes additional
 * metadata fields for rich client experiences.
 *
 * To regenerate proto types: npm run generate:proto
 */

// Message types
export type MessageType =
  | "message"         // Client → Server: user message
  | "chat"            // Client → Server: dev console chat message
  | "upload_request"  // Client → Server: file upload request
  | "chunk"           // Server → Client: streaming text chunk
  | "done"            // Server → Client: response complete
  | "tool_call"       // Server → Client: agent is calling a tool
  | "tool_result"     // Server → Client: tool execution result
  | "error"           // Server → Client: error occurred
  | "connected"       // Server → Client: connection established
  | "reloaded"        // Server → Client: configuration reloaded (dev console)
  | "upload_ready"    // Server → Client: upload URL ready
  | "upload_complete" // Server → Client: upload complete
  | "media_chunk";    // Server → Client: streaming media chunk

// Client → Server message
export interface ClientMessage {
  type: "message";
  session_id?: string;
  content: string;
  /** Multi-modal content parts (images, audio, etc.). Takes precedence over content. */
  parts?: ContentPart[];
  metadata?: Record<string, string>;
}

// Connection capabilities for binary frame support
export interface ConnectionCapabilities {
  binary_frames: boolean;
  max_payload_size?: number;
  protocol_version?: number;
}

// Connected message info with capabilities
export interface ConnectedInfo {
  capabilities?: ConnectionCapabilities;
}

// Content part types for multi-modal messages
export type ContentPartType = "text" | "image" | "audio" | "video" | "file";

// Media content for non-text parts
export interface MediaContent {
  data?: string;      // base64-encoded content
  url?: string;       // HTTP/HTTPS URL
  storage_ref?: string; // backend storage reference
  mime_type: string;
  filename?: string;
  size_bytes?: number;
  // Image-specific fields
  width?: number;
  height?: number;
  detail?: string;
  // Audio/Video-specific fields
  duration_ms?: number;
  sample_rate?: number;
  channels?: number;
}

// Content part for multi-modal messages
export interface ContentPart {
  type: ContentPartType;
  text?: string;      // for type "text"
  media?: MediaContent; // for image, audio, video, file types
}

// Media chunk info for streaming responses
export interface MediaChunkInfo {
  media_id: string;
  sequence: number;
  is_last: boolean;
  data: string; // base64 for JSON frames
  mime_type: string;
}

// Upload ready info
export interface UploadReadyInfo {
  upload_id: string;
  upload_url: string;
  storage_ref: string;
  expires_at: string;
}

// Upload complete info
export interface UploadCompleteInfo {
  upload_id: string;
  storage_ref: string;
  size_bytes: number;
}

// Server → Client message
export interface ServerMessage {
  type: MessageType;
  session_id?: string;
  content?: string;
  parts?: ContentPart[]; // multi-modal content parts
  tool_call?: ToolCallInfo;
  tool_result?: ToolResultInfo;
  error?: ErrorInfo;
  connected?: ConnectedInfo;
  media_chunk?: MediaChunkInfo;
  upload_ready?: UploadReadyInfo;
  upload_complete?: UploadCompleteInfo;
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

