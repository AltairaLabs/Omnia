/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package facade provides the WebSocket facade for agent communication.
package facade

import "time"

// ContentPartType represents the type of content in a message part.
type ContentPartType string

const (
	// ContentPartTypeText represents text content.
	ContentPartTypeText ContentPartType = "text"
	// ContentPartTypeImage represents an image.
	ContentPartTypeImage ContentPartType = "image"
	// ContentPartTypeAudio represents audio content.
	ContentPartTypeAudio ContentPartType = "audio"
	// ContentPartTypeVideo represents video content.
	ContentPartTypeVideo ContentPartType = "video"
	// ContentPartTypeFile represents a generic file.
	ContentPartTypeFile ContentPartType = "file"
)

// ContentPart represents a part of a multi-modal message.
// A message can contain multiple parts of different types (text, images, audio, etc.).
type ContentPart struct {
	// Type is the content part type.
	Type ContentPartType `json:"type"`
	// Text is the text content (for type "text").
	Text string `json:"text,omitempty"`
	// Media contains media content details (for image, audio, video, file types).
	Media *MediaContent `json:"media,omitempty"`
}

// MediaContent contains the data or reference to media content.
// Exactly one of Data, URL, or StorageRef should be set.
type MediaContent struct {
	// Data is base64-encoded content for small files (< 256KB recommended).
	Data string `json:"data,omitempty"`
	// URL is an HTTP/HTTPS URL for externalized files.
	URL string `json:"url,omitempty"`
	// StorageRef is a backend storage reference (e.g., S3 key).
	StorageRef string `json:"storage_ref,omitempty"`

	// MimeType is the MIME type of the content (required).
	MimeType string `json:"mime_type"`

	// Filename is the original filename (optional).
	Filename string `json:"filename,omitempty"`
	// SizeBytes is the file size in bytes (optional).
	SizeBytes int64 `json:"size_bytes,omitempty"`

	// Image-specific fields
	// Width is the image width in pixels.
	Width int `json:"width,omitempty"`
	// Height is the image height in pixels.
	Height int `json:"height,omitempty"`
	// Detail is a processing hint for vision models ("low", "high", "auto").
	Detail string `json:"detail,omitempty"`

	// Audio/Video-specific fields
	// DurationMs is the duration in milliseconds.
	DurationMs int64 `json:"duration_ms,omitempty"`
	// SampleRate is the audio sample rate in Hz.
	SampleRate int `json:"sample_rate,omitempty"`
	// Channels is the number of audio channels (1=mono, 2=stereo).
	Channels int `json:"channels,omitempty"`
}

// NewTextPart creates a new text content part.
func NewTextPart(text string) ContentPart {
	return ContentPart{
		Type: ContentPartTypeText,
		Text: text,
	}
}

// NewImagePart creates a new image content part with base64 data.
func NewImagePart(data, mimeType string) ContentPart {
	return ContentPart{
		Type: ContentPartTypeImage,
		Media: &MediaContent{
			Data:     data,
			MimeType: mimeType,
		},
	}
}

// NewImagePartFromURL creates a new image content part from a URL.
func NewImagePartFromURL(url, mimeType string) ContentPart {
	return ContentPart{
		Type: ContentPartTypeImage,
		Media: &MediaContent{
			URL:      url,
			MimeType: mimeType,
		},
	}
}

// NewAudioPart creates a new audio content part with base64 data.
func NewAudioPart(data, mimeType string) ContentPart {
	return ContentPart{
		Type: ContentPartTypeAudio,
		Media: &MediaContent{
			Data:     data,
			MimeType: mimeType,
		},
	}
}

// NewAudioPartFromURL creates a new audio content part from a URL.
func NewAudioPartFromURL(url, mimeType string) ContentPart {
	return ContentPart{
		Type: ContentPartTypeAudio,
		Media: &MediaContent{
			URL:      url,
			MimeType: mimeType,
		},
	}
}

// NewFilePart creates a new file content part from a URL.
func NewFilePart(url, mimeType, filename string) ContentPart {
	return ContentPart{
		Type: ContentPartTypeFile,
		Media: &MediaContent{
			URL:      url,
			MimeType: mimeType,
			Filename: filename,
		},
	}
}

// MessageType represents the type of WebSocket message.
type MessageType string

const (
	// Client to Server message types
	MessageTypeMessage       MessageType = "message"
	MessageTypeUploadRequest MessageType = "upload_request"

	// Server to Client message types
	MessageTypeChunk          MessageType = "chunk"
	MessageTypeDone           MessageType = "done"
	MessageTypeToolCall       MessageType = "tool_call"
	MessageTypeToolResult     MessageType = "tool_result"
	MessageTypeError          MessageType = "error"
	MessageTypeConnected      MessageType = "connected"
	MessageTypeUploadReady    MessageType = "upload_ready"
	MessageTypeUploadComplete MessageType = "upload_complete"
	MessageTypeMediaChunk     MessageType = "media_chunk"
)

// ClientMessage represents a message sent from client to server.
type ClientMessage struct {
	// Type is the message type ("message" or "upload_request").
	Type MessageType `json:"type"`
	// SessionID is the optional session ID for resuming a session.
	SessionID string `json:"session_id,omitempty"`
	// Content is the message content (text-only, for backward compatibility).
	// If Parts is provided, it takes precedence over Content.
	Content string `json:"content,omitempty"`
	// Parts contains multi-modal content parts (text, images, audio, etc.).
	// When provided, this takes precedence over the Content field.
	Parts []ContentPart `json:"parts,omitempty"`
	// Metadata contains optional additional data.
	Metadata map[string]string `json:"metadata,omitempty"`
	// UploadRequest contains upload request details (for type "upload_request").
	UploadRequest *UploadRequestInfo `json:"upload_request,omitempty"`
}

// ServerMessage represents a message sent from server to client.
type ServerMessage struct {
	// Type is the message type.
	Type MessageType `json:"type"`
	// SessionID is the session identifier.
	SessionID string `json:"session_id,omitempty"`
	// Content is the message content (for chunk, done, error types).
	// For text-only responses. If Parts is provided, it takes precedence.
	Content string `json:"content,omitempty"`
	// Parts contains multi-modal content parts (text, images, audio, etc.).
	// Used for responses that include media content.
	Parts []ContentPart `json:"parts,omitempty"`
	// ToolCall contains tool call details (for tool_call type).
	ToolCall *ToolCallInfo `json:"tool_call,omitempty"`
	// ToolResult contains tool result details (for tool_result type).
	ToolResult *ToolResultInfo `json:"tool_result,omitempty"`
	// Error contains error details (for error type).
	Error *ErrorInfo `json:"error,omitempty"`
	// UploadReady contains upload URL details (for upload_ready type).
	UploadReady *UploadReadyInfo `json:"upload_ready,omitempty"`
	// UploadComplete contains upload completion details (for upload_complete type).
	UploadComplete *UploadCompleteInfo `json:"upload_complete,omitempty"`
	// MediaChunk contains streaming media chunk details (for media_chunk type).
	MediaChunk *MediaChunkInfo `json:"media_chunk,omitempty"`
	// Timestamp is when the message was created.
	Timestamp time.Time `json:"timestamp"`
}

// ToolCallInfo contains information about a tool call.
type ToolCallInfo struct {
	// ID is the unique identifier for this tool call.
	ID string `json:"id"`
	// Name is the name of the tool being called.
	Name string `json:"name"`
	// Arguments are the arguments passed to the tool.
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// ToolResultInfo contains information about a tool result.
type ToolResultInfo struct {
	// ID is the tool call ID this result is for.
	ID string `json:"id"`
	// Result is the tool execution result.
	Result interface{} `json:"result,omitempty"`
	// Error is the error message if the tool failed.
	Error string `json:"error,omitempty"`
}

// ErrorInfo contains error details.
type ErrorInfo struct {
	// Code is the error code.
	Code string `json:"code"`
	// Message is the error message.
	Message string `json:"message"`
	// Details contains additional error details.
	Details map[string]interface{} `json:"details,omitempty"`
}

// UploadRequestInfo contains information about a file upload request from the client.
type UploadRequestInfo struct {
	// Filename is the original filename.
	Filename string `json:"filename"`
	// MimeType is the MIME type of the file.
	MimeType string `json:"mime_type"`
	// SizeBytes is the file size in bytes.
	SizeBytes int64 `json:"size_bytes"`
}

// UploadReadyInfo contains information for the client to perform the upload.
type UploadReadyInfo struct {
	// UploadID is the unique identifier for this upload.
	UploadID string `json:"upload_id"`
	// UploadURL is the URL where the client should PUT the file content.
	UploadURL string `json:"upload_url"`
	// StorageRef is the storage reference to use in subsequent messages.
	StorageRef string `json:"storage_ref"`
	// ExpiresAt is when the upload URL expires.
	ExpiresAt time.Time `json:"expires_at"`
}

// UploadCompleteInfo contains information about a completed upload.
type UploadCompleteInfo struct {
	// UploadID is the upload identifier.
	UploadID string `json:"upload_id"`
	// StorageRef is the storage reference for the uploaded file.
	StorageRef string `json:"storage_ref"`
	// SizeBytes is the actual size of the uploaded file.
	SizeBytes int64 `json:"size_bytes"`
}

// MediaChunkInfo contains information about a streaming media chunk.
// This is used for streaming audio/video responses where playback can begin
// before the entire media is generated.
type MediaChunkInfo struct {
	// MediaID is the unique identifier for this media stream.
	// All chunks belonging to the same media share this ID.
	MediaID string `json:"media_id"`
	// Sequence is the sequence number for ordering chunks (0-indexed).
	Sequence int `json:"sequence"`
	// IsLast indicates whether this is the final chunk.
	IsLast bool `json:"is_last"`
	// Data is the base64-encoded chunk data.
	Data string `json:"data"`
	// MimeType is the MIME type of the media (e.g., "audio/mp3", "video/mp4").
	MimeType string `json:"mime_type"`
}

// Error codes.
const (
	ErrorCodeInvalidMessage   = "INVALID_MESSAGE"
	ErrorCodeSessionNotFound  = "SESSION_NOT_FOUND"
	ErrorCodeSessionExpired   = "SESSION_EXPIRED"
	ErrorCodeInternalError    = "INTERNAL_ERROR"
	ErrorCodeAgentUnavailable = "AGENT_UNAVAILABLE"
	ErrorCodeToolFailed       = "TOOL_FAILED"
	ErrorCodeUploadFailed     = "UPLOAD_FAILED"
	ErrorCodeMediaNotEnabled  = "MEDIA_NOT_ENABLED"
)

// NewChunkMessage creates a new chunk message.
func NewChunkMessage(sessionID, content string) *ServerMessage {
	return &ServerMessage{
		Type:      MessageTypeChunk,
		SessionID: sessionID,
		Content:   content,
		Timestamp: time.Now(),
	}
}

// NewDoneMessage creates a new done message.
func NewDoneMessage(sessionID, content string) *ServerMessage {
	return &ServerMessage{
		Type:      MessageTypeDone,
		SessionID: sessionID,
		Content:   content,
		Timestamp: time.Now(),
	}
}

// NewToolCallMessage creates a new tool call message.
func NewToolCallMessage(sessionID string, toolCall *ToolCallInfo) *ServerMessage {
	return &ServerMessage{
		Type:      MessageTypeToolCall,
		SessionID: sessionID,
		ToolCall:  toolCall,
		Timestamp: time.Now(),
	}
}

// NewToolResultMessage creates a new tool result message.
func NewToolResultMessage(sessionID string, result *ToolResultInfo) *ServerMessage {
	return &ServerMessage{
		Type:       MessageTypeToolResult,
		SessionID:  sessionID,
		ToolResult: result,
		Timestamp:  time.Now(),
	}
}

// NewErrorMessage creates a new error message.
func NewErrorMessage(sessionID, code, message string) *ServerMessage {
	return &ServerMessage{
		Type:      MessageTypeError,
		SessionID: sessionID,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
		},
		Timestamp: time.Now(),
	}
}

// NewConnectedMessage creates a new connected message.
func NewConnectedMessage(sessionID string) *ServerMessage {
	return &ServerMessage{
		Type:      MessageTypeConnected,
		SessionID: sessionID,
		Timestamp: time.Now(),
	}
}

// NewDoneMessageWithParts creates a new done message with multi-modal parts.
func NewDoneMessageWithParts(sessionID string, parts []ContentPart) *ServerMessage {
	return &ServerMessage{
		Type:      MessageTypeDone,
		SessionID: sessionID,
		Parts:     parts,
		Timestamp: time.Now(),
	}
}

// NewChunkMessageWithParts creates a new chunk message with multi-modal parts.
// This is useful for streaming responses that include media chunks.
func NewChunkMessageWithParts(sessionID string, parts []ContentPart) *ServerMessage {
	return &ServerMessage{
		Type:      MessageTypeChunk,
		SessionID: sessionID,
		Parts:     parts,
		Timestamp: time.Now(),
	}
}

// NewUploadReadyMessage creates a new upload ready message.
func NewUploadReadyMessage(sessionID string, uploadReady *UploadReadyInfo) *ServerMessage {
	return &ServerMessage{
		Type:        MessageTypeUploadReady,
		SessionID:   sessionID,
		UploadReady: uploadReady,
		Timestamp:   time.Now(),
	}
}

// NewUploadCompleteMessage creates a new upload complete message.
func NewUploadCompleteMessage(sessionID string, uploadComplete *UploadCompleteInfo) *ServerMessage {
	return &ServerMessage{
		Type:           MessageTypeUploadComplete,
		SessionID:      sessionID,
		UploadComplete: uploadComplete,
		Timestamp:      time.Now(),
	}
}

// NewMediaChunkMessage creates a new media chunk message for streaming media responses.
func NewMediaChunkMessage(sessionID string, mediaChunk *MediaChunkInfo) *ServerMessage {
	return &ServerMessage{
		Type:       MessageTypeMediaChunk,
		SessionID:  sessionID,
		MediaChunk: mediaChunk,
		Timestamp:  time.Now(),
	}
}

// GetTextContent returns the text content from a ClientMessage.
// It checks Parts first, then falls back to Content for backward compatibility.
func (m *ClientMessage) GetTextContent() string {
	if len(m.Parts) > 0 {
		for _, part := range m.Parts {
			if part.Type == ContentPartTypeText && part.Text != "" {
				return part.Text
			}
		}
	}
	return m.Content
}

// HasMediaContent returns true if the message contains any media parts.
func (m *ClientMessage) HasMediaContent() bool {
	for _, part := range m.Parts {
		if part.Type != ContentPartTypeText && part.Media != nil {
			return true
		}
	}
	return false
}

// GetMediaParts returns all non-text parts from the message.
func (m *ClientMessage) GetMediaParts() []ContentPart {
	var media []ContentPart
	for _, part := range m.Parts {
		if part.Type != ContentPartTypeText {
			media = append(media, part)
		}
	}
	return media
}

// GetTextContent returns the text content from a ServerMessage.
// It checks Parts first, then falls back to Content.
func (m *ServerMessage) GetTextContent() string {
	if len(m.Parts) > 0 {
		for _, part := range m.Parts {
			if part.Type == ContentPartTypeText && part.Text != "" {
				return part.Text
			}
		}
	}
	return m.Content
}

// HasMediaContent returns true if the message contains any media parts.
func (m *ServerMessage) HasMediaContent() bool {
	for _, part := range m.Parts {
		if part.Type != ContentPartTypeText && part.Media != nil {
			return true
		}
	}
	return false
}
