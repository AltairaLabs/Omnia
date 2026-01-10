/**
 * WebSocket protocol types for agent communication.
 * These mirror the Go types in internal/facade/protocol.go
 */

// Message types
export type MessageType =
  | "message"         // Client → Server: user message
  | "upload_request"  // Client → Server: file upload request
  | "chunk"           // Server → Client: streaming text chunk
  | "done"            // Server → Client: response complete
  | "tool_call"       // Server → Client: agent is calling a tool
  | "tool_result"     // Server → Client: tool execution result
  | "error"           // Server → Client: error occurred
  | "connected"       // Server → Client: connection established
  | "upload_ready"    // Server → Client: upload URL ready
  | "upload_complete" // Server → Client: upload complete
  | "media_chunk";    // Server → Client: streaming media chunk

// Client → Server message
export interface ClientMessage {
  type: "message";
  session_id?: string;
  content: string;
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

// Error codes
export const ErrorCodes = {
  INVALID_MESSAGE: "INVALID_MESSAGE",
  SESSION_NOT_FOUND: "SESSION_NOT_FOUND",
  SESSION_EXPIRED: "SESSION_EXPIRED",
  INTERNAL_ERROR: "INTERNAL_ERROR",
  AGENT_UNAVAILABLE: "AGENT_UNAVAILABLE",
  TOOL_FAILED: "TOOL_FAILED",
  // Connection errors from WebSocket proxy
  CONNECTION_ERROR: "CONNECTION_ERROR",
  CONNECTION_TIMEOUT: "CONNECTION_TIMEOUT",
  CONNECTION_REFUSED: "CONNECTION_REFUSED",
  AGENT_NOT_FOUND: "AGENT_NOT_FOUND",
} as const;

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

// Binary WebSocket frame support
export const BINARY_MAGIC = "OMNI";
export const BINARY_VERSION = 1;
export const BINARY_HEADER_SIZE = 32;
export const MEDIA_ID_SIZE = 12;

// Binary message types
export const BinaryMessageType = {
  MEDIA_CHUNK: 1,
  UPLOAD: 2,
} as const;

// Binary frame header
export interface BinaryFrameHeader {
  magic: string;
  version: number;
  flags: number;
  messageType: number;
  metadataLen: number;
  payloadLen: number;
  sequence: number;
  mediaId: string;
}

// Binary frame flags
export const BinaryFlags = {
  COMPRESSED: 0x01,
  CHUNKED: 0x02,
  IS_LAST: 0x04,
} as const;

/**
 * Decode a binary WebSocket frame.
 * Returns parsed header, metadata, and payload.
 */
export function decodeBinaryFrame(data: ArrayBuffer): {
  header: BinaryFrameHeader;
  metadata: Record<string, unknown>;
  payload: ArrayBuffer;
} {
  const view = new DataView(data);
  const decoder = new TextDecoder();

  // Read header
  const magic = decoder.decode(new Uint8Array(data, 0, 4));
  if (magic !== BINARY_MAGIC) {
    throw new Error(`Invalid magic bytes: ${magic}`);
  }

  const version = view.getUint8(4);
  if (version !== BINARY_VERSION) {
    throw new Error(`Unsupported protocol version: ${version}`);
  }

  const flags = view.getUint8(5);
  const messageType = view.getUint16(6, false); // big-endian
  const metadataLen = view.getUint32(8, false);
  const payloadLen = view.getUint32(12, false);
  const sequence = view.getUint32(16, false);

  // Read media ID, trimming null bytes
  const mediaIdBytes = new Uint8Array(data, 20, MEDIA_ID_SIZE);
  let mediaIdLen = MEDIA_ID_SIZE;
  for (let i = 0; i < MEDIA_ID_SIZE; i++) {
    if (mediaIdBytes[i] === 0) {
      mediaIdLen = i;
      break;
    }
  }
  const mediaId = decoder.decode(mediaIdBytes.slice(0, mediaIdLen));

  const header: BinaryFrameHeader = {
    magic,
    version,
    flags,
    messageType,
    metadataLen,
    payloadLen,
    sequence,
    mediaId,
  };

  // Read metadata
  let metadata: Record<string, unknown> = {};
  if (metadataLen > 0) {
    const metadataBytes = new Uint8Array(data, BINARY_HEADER_SIZE, metadataLen);
    const metadataJson = decoder.decode(metadataBytes);
    metadata = JSON.parse(metadataJson);
  }

  // Read payload
  const payload = data.slice(BINARY_HEADER_SIZE + metadataLen);

  return { header, metadata, payload };
}

/**
 * Check if a binary frame has the is_last flag set.
 */
export function isBinaryFrameLast(flags: number): boolean {
  return (flags & BinaryFlags.IS_LAST) !== 0;
}
