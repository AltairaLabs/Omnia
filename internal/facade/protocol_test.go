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

package facade

import (
	"encoding/json"
	"testing"
	"time"
)

const testSessionID = "session-1"

func TestMessageTypes(t *testing.T) {
	tests := []struct {
		msgType  MessageType
		expected string
	}{
		{MessageTypeMessage, "message"},
		{MessageTypeChunk, "chunk"},
		{MessageTypeDone, "done"},
		{MessageTypeToolCall, "tool_call"},
		{MessageTypeToolResult, "tool_result"},
		{MessageTypeError, "error"},
		{MessageTypeConnected, "connected"},
	}

	for _, tt := range tests {
		if string(tt.msgType) != tt.expected {
			t.Errorf("MessageType = %v, want %v", tt.msgType, tt.expected)
		}
	}
}

func TestClientMessageJSON(t *testing.T) {
	msg := ClientMessage{
		Type:      MessageTypeMessage,
		SessionID: "test-session",
		Content:   "Hello, world!",
		Metadata: map[string]string{
			"key": "value",
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded ClientMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Type != msg.Type {
		t.Errorf("Type = %v, want %v", decoded.Type, msg.Type)
	}
	if decoded.SessionID != msg.SessionID {
		t.Errorf("SessionID = %v, want %v", decoded.SessionID, msg.SessionID)
	}
	if decoded.Content != msg.Content {
		t.Errorf("Content = %v, want %v", decoded.Content, msg.Content)
	}
	if decoded.Metadata["key"] != msg.Metadata["key"] {
		t.Errorf("Metadata[key] = %v, want %v", decoded.Metadata["key"], msg.Metadata["key"])
	}
}

func TestServerMessageJSON(t *testing.T) {
	msg := ServerMessage{
		Type:      MessageTypeChunk,
		SessionID: "test-session",
		Content:   "chunk content",
		Timestamp: time.Now(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded ServerMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Type != msg.Type {
		t.Errorf("Type = %v, want %v", decoded.Type, msg.Type)
	}
	if decoded.SessionID != msg.SessionID {
		t.Errorf("SessionID = %v, want %v", decoded.SessionID, msg.SessionID)
	}
	if decoded.Content != msg.Content {
		t.Errorf("Content = %v, want %v", decoded.Content, msg.Content)
	}
}

func TestNewChunkMessage(t *testing.T) {
	msg := NewChunkMessage(testSessionID, "test content")

	if msg.Type != MessageTypeChunk {
		t.Errorf("Type = %v, want %v", msg.Type, MessageTypeChunk)
	}
	if msg.SessionID != testSessionID {
		t.Errorf("SessionID = %v, want %v", msg.SessionID, testSessionID)
	}
	if msg.Content != "test content" {
		t.Errorf("Content = %v, want 'test content'", msg.Content)
	}
	if msg.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestNewDoneMessage(t *testing.T) {
	msg := NewDoneMessage(testSessionID, "final content")

	if msg.Type != MessageTypeDone {
		t.Errorf("Type = %v, want %v", msg.Type, MessageTypeDone)
	}
	if msg.SessionID != testSessionID {
		t.Errorf("SessionID = %v, want %v", msg.SessionID, testSessionID)
	}
	if msg.Content != "final content" {
		t.Errorf("Content = %v, want 'final content'", msg.Content)
	}
}

func TestNewToolCallMessage(t *testing.T) {
	toolCall := &ToolCallInfo{
		ID:   "tool-1",
		Name: "search",
		Arguments: map[string]interface{}{
			"query": "test query",
		},
	}

	msg := NewToolCallMessage(testSessionID, toolCall)

	if msg.Type != MessageTypeToolCall {
		t.Errorf("Type = %v, want %v", msg.Type, MessageTypeToolCall)
	}
	if msg.SessionID != testSessionID {
		t.Errorf("SessionID = %v, want %v", msg.SessionID, testSessionID)
	}
	if msg.ToolCall == nil {
		t.Fatal("ToolCall should not be nil")
	}
	if msg.ToolCall.ID != toolCall.ID {
		t.Errorf("ToolCall.ID = %v, want %v", msg.ToolCall.ID, toolCall.ID)
	}
	if msg.ToolCall.Name != toolCall.Name {
		t.Errorf("ToolCall.Name = %v, want %v", msg.ToolCall.Name, toolCall.Name)
	}
}

func TestNewToolResultMessage(t *testing.T) {
	result := &ToolResultInfo{
		ID:     "tool-1",
		Result: "search results",
	}

	msg := NewToolResultMessage(testSessionID, result)

	if msg.Type != MessageTypeToolResult {
		t.Errorf("Type = %v, want %v", msg.Type, MessageTypeToolResult)
	}
	if msg.SessionID != testSessionID {
		t.Errorf("SessionID = %v, want %v", msg.SessionID, testSessionID)
	}
	if msg.ToolResult == nil {
		t.Fatal("ToolResult should not be nil")
	}
	if msg.ToolResult.ID != result.ID {
		t.Errorf("ToolResult.ID = %v, want %v", msg.ToolResult.ID, result.ID)
	}
}

func TestNewToolResultMessageWithError(t *testing.T) {
	result := &ToolResultInfo{
		ID:    "tool-1",
		Error: "tool execution failed",
	}

	msg := NewToolResultMessage(testSessionID, result)

	if msg.ToolResult.Error != result.Error {
		t.Errorf("ToolResult.Error = %v, want %v", msg.ToolResult.Error, result.Error)
	}
}

func TestNewErrorMessage(t *testing.T) {
	msg := NewErrorMessage(testSessionID, ErrorCodeInvalidMessage, "invalid format")

	if msg.Type != MessageTypeError {
		t.Errorf("Type = %v, want %v", msg.Type, MessageTypeError)
	}
	if msg.SessionID != testSessionID {
		t.Errorf("SessionID = %v, want %v", msg.SessionID, testSessionID)
	}
	if msg.Error == nil {
		t.Fatal("Error should not be nil")
	}
	if msg.Error.Code != ErrorCodeInvalidMessage {
		t.Errorf("Error.Code = %v, want %v", msg.Error.Code, ErrorCodeInvalidMessage)
	}
	if msg.Error.Message != "invalid format" {
		t.Errorf("Error.Message = %v, want 'invalid format'", msg.Error.Message)
	}
}

func TestNewConnectedMessage(t *testing.T) {
	msg := NewConnectedMessage(testSessionID)

	if msg.Type != MessageTypeConnected {
		t.Errorf("Type = %v, want %v", msg.Type, MessageTypeConnected)
	}
	if msg.SessionID != testSessionID {
		t.Errorf("SessionID = %v, want %v", msg.SessionID, testSessionID)
	}
	if msg.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestErrorCodes(t *testing.T) {
	codes := []string{
		ErrorCodeInvalidMessage,
		ErrorCodeSessionNotFound,
		ErrorCodeSessionExpired,
		ErrorCodeInternalError,
		ErrorCodeAgentUnavailable,
		ErrorCodeToolFailed,
	}

	for _, code := range codes {
		if code == "" {
			t.Error("Error code should not be empty")
		}
	}
}

func TestToolCallInfoJSON(t *testing.T) {
	info := ToolCallInfo{
		ID:   "call-123",
		Name: "calculate",
		Arguments: map[string]interface{}{
			"a": 10,
			"b": 20,
		},
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded ToolCallInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.ID != info.ID {
		t.Errorf("ID = %v, want %v", decoded.ID, info.ID)
	}
	if decoded.Name != info.Name {
		t.Errorf("Name = %v, want %v", decoded.Name, info.Name)
	}
}

func TestErrorInfoJSON(t *testing.T) {
	info := ErrorInfo{
		Code:    ErrorCodeInternalError,
		Message: "something went wrong",
		Details: map[string]interface{}{
			"trace_id": "abc123",
		},
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded ErrorInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Code != info.Code {
		t.Errorf("Code = %v, want %v", decoded.Code, info.Code)
	}
	if decoded.Message != info.Message {
		t.Errorf("Message = %v, want %v", decoded.Message, info.Message)
	}
}

// Tests for ContentPart and multi-modal messaging

func TestContentPartTypes(t *testing.T) {
	tests := []struct {
		partType ContentPartType
		expected string
	}{
		{ContentPartTypeText, "text"},
		{ContentPartTypeImage, "image"},
		{ContentPartTypeAudio, "audio"},
		{ContentPartTypeVideo, "video"},
		{ContentPartTypeFile, "file"},
	}

	for _, tt := range tests {
		if string(tt.partType) != tt.expected {
			t.Errorf("ContentPartType = %v, want %v", tt.partType, tt.expected)
		}
	}
}

func TestNewTextPart(t *testing.T) {
	part := NewTextPart("Hello, world!")

	if part.Type != ContentPartTypeText {
		t.Errorf("Type = %v, want %v", part.Type, ContentPartTypeText)
	}
	if part.Text != "Hello, world!" {
		t.Errorf("Text = %v, want 'Hello, world!'", part.Text)
	}
	if part.Media != nil {
		t.Error("Media should be nil for text parts")
	}
}

func TestNewImagePart(t *testing.T) {
	part := NewImagePart("iVBORw0KGgo=", "image/png")

	if part.Type != ContentPartTypeImage {
		t.Errorf("Type = %v, want %v", part.Type, ContentPartTypeImage)
	}
	if part.Media == nil {
		t.Fatal("Media should not be nil")
	}
	if part.Media.Data != "iVBORw0KGgo=" {
		t.Errorf("Media.Data = %v, want 'iVBORw0KGgo='", part.Media.Data)
	}
	if part.Media.MimeType != "image/png" {
		t.Errorf("Media.MimeType = %v, want 'image/png'", part.Media.MimeType)
	}
}

func TestNewImagePartFromURL(t *testing.T) {
	part := NewImagePartFromURL("https://example.com/image.png", "image/png")

	if part.Type != ContentPartTypeImage {
		t.Errorf("Type = %v, want %v", part.Type, ContentPartTypeImage)
	}
	if part.Media == nil {
		t.Fatal("Media should not be nil")
	}
	if part.Media.URL != "https://example.com/image.png" {
		t.Errorf("Media.URL = %v, want 'https://example.com/image.png'", part.Media.URL)
	}
	if part.Media.MimeType != "image/png" {
		t.Errorf("Media.MimeType = %v, want 'image/png'", part.Media.MimeType)
	}
}

func TestNewAudioPart(t *testing.T) {
	part := NewAudioPart("//uQxAAAAAAA", "audio/mp3")

	if part.Type != ContentPartTypeAudio {
		t.Errorf("Type = %v, want %v", part.Type, ContentPartTypeAudio)
	}
	if part.Media == nil {
		t.Fatal("Media should not be nil")
	}
	if part.Media.Data != "//uQxAAAAAAA" {
		t.Errorf("Media.Data = %v, want '//uQxAAAAAAA'", part.Media.Data)
	}
	if part.Media.MimeType != "audio/mp3" {
		t.Errorf("Media.MimeType = %v, want 'audio/mp3'", part.Media.MimeType)
	}
}

func TestNewAudioPartFromURL(t *testing.T) {
	part := NewAudioPartFromURL("https://example.com/audio.mp3", "audio/mpeg")

	if part.Type != ContentPartTypeAudio {
		t.Errorf("Type = %v, want %v", part.Type, ContentPartTypeAudio)
	}
	if part.Media == nil {
		t.Fatal("Media should not be nil")
	}
	if part.Media.URL != "https://example.com/audio.mp3" {
		t.Errorf("Media.URL = %v, want 'https://example.com/audio.mp3'", part.Media.URL)
	}
}

func TestNewFilePart(t *testing.T) {
	part := NewFilePart("https://example.com/doc.pdf", "application/pdf", "document.pdf")

	if part.Type != ContentPartTypeFile {
		t.Errorf("Type = %v, want %v", part.Type, ContentPartTypeFile)
	}
	if part.Media == nil {
		t.Fatal("Media should not be nil")
	}
	if part.Media.URL != "https://example.com/doc.pdf" {
		t.Errorf("Media.URL = %v, want 'https://example.com/doc.pdf'", part.Media.URL)
	}
	if part.Media.Filename != "document.pdf" {
		t.Errorf("Media.Filename = %v, want 'document.pdf'", part.Media.Filename)
	}
}

func TestContentPartJSON(t *testing.T) {
	part := ContentPart{
		Type: ContentPartTypeImage,
		Media: &MediaContent{
			URL:       "https://example.com/image.jpg",
			MimeType:  "image/jpeg",
			Width:     1920,
			Height:    1080,
			SizeBytes: 102400,
		},
	}

	data, err := json.Marshal(part)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded ContentPart
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Type != part.Type {
		t.Errorf("Type = %v, want %v", decoded.Type, part.Type)
	}
	if decoded.Media.URL != part.Media.URL {
		t.Errorf("Media.URL = %v, want %v", decoded.Media.URL, part.Media.URL)
	}
	if decoded.Media.Width != part.Media.Width {
		t.Errorf("Media.Width = %v, want %v", decoded.Media.Width, part.Media.Width)
	}
	if decoded.Media.Height != part.Media.Height {
		t.Errorf("Media.Height = %v, want %v", decoded.Media.Height, part.Media.Height)
	}
}

func TestMediaContentWithAudioFields(t *testing.T) {
	media := MediaContent{
		URL:        "https://example.com/audio.wav",
		MimeType:   "audio/wav",
		DurationMs: 30000,
		SampleRate: 44100,
		Channels:   2,
	}

	data, err := json.Marshal(media)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded MediaContent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.DurationMs != media.DurationMs {
		t.Errorf("DurationMs = %v, want %v", decoded.DurationMs, media.DurationMs)
	}
	if decoded.SampleRate != media.SampleRate {
		t.Errorf("SampleRate = %v, want %v", decoded.SampleRate, media.SampleRate)
	}
	if decoded.Channels != media.Channels {
		t.Errorf("Channels = %v, want %v", decoded.Channels, media.Channels)
	}
}

func TestClientMessageWithParts(t *testing.T) {
	msg := ClientMessage{
		Type:      MessageTypeMessage,
		SessionID: "test-session",
		Parts: []ContentPart{
			NewTextPart("What's in this image?"),
			NewImagePartFromURL("https://example.com/photo.jpg", "image/jpeg"),
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded ClientMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(decoded.Parts) != 2 {
		t.Fatalf("Parts length = %v, want 2", len(decoded.Parts))
	}
	if decoded.Parts[0].Type != ContentPartTypeText {
		t.Errorf("Parts[0].Type = %v, want %v", decoded.Parts[0].Type, ContentPartTypeText)
	}
	if decoded.Parts[1].Type != ContentPartTypeImage {
		t.Errorf("Parts[1].Type = %v, want %v", decoded.Parts[1].Type, ContentPartTypeImage)
	}
}

func TestServerMessageWithParts(t *testing.T) {
	msg := ServerMessage{
		Type:      MessageTypeDone,
		SessionID: "test-session",
		Parts: []ContentPart{
			NewTextPart("Here's the analysis:"),
			NewImagePartFromURL("https://example.com/annotated.png", "image/png"),
		},
		Timestamp: time.Now(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded ServerMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(decoded.Parts) != 2 {
		t.Fatalf("Parts length = %v, want 2", len(decoded.Parts))
	}
}

func TestNewDoneMessageWithParts(t *testing.T) {
	parts := []ContentPart{
		NewTextPart("Response with image"),
		NewImagePartFromURL("https://example.com/result.png", "image/png"),
	}

	msg := NewDoneMessageWithParts(testSessionID, parts)

	if msg.Type != MessageTypeDone {
		t.Errorf("Type = %v, want %v", msg.Type, MessageTypeDone)
	}
	if msg.SessionID != testSessionID {
		t.Errorf("SessionID = %v, want %v", msg.SessionID, testSessionID)
	}
	if len(msg.Parts) != 2 {
		t.Fatalf("Parts length = %v, want 2", len(msg.Parts))
	}
	if msg.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestNewChunkMessageWithParts(t *testing.T) {
	parts := []ContentPart{
		NewTextPart("Streaming chunk"),
	}

	msg := NewChunkMessageWithParts(testSessionID, parts)

	if msg.Type != MessageTypeChunk {
		t.Errorf("Type = %v, want %v", msg.Type, MessageTypeChunk)
	}
	if len(msg.Parts) != 1 {
		t.Fatalf("Parts length = %v, want 1", len(msg.Parts))
	}
}

func TestClientMessageGetTextContent(t *testing.T) {
	tests := []struct {
		name     string
		msg      ClientMessage
		expected string
	}{
		{
			name: "text from Content field",
			msg: ClientMessage{
				Content: "Hello from content",
			},
			expected: "Hello from content",
		},
		{
			name: "text from Parts",
			msg: ClientMessage{
				Parts: []ContentPart{
					NewTextPart("Hello from parts"),
				},
			},
			expected: "Hello from parts",
		},
		{
			name: "Parts takes precedence over Content",
			msg: ClientMessage{
				Content: "Should be ignored",
				Parts: []ContentPart{
					NewTextPart("Parts wins"),
				},
			},
			expected: "Parts wins",
		},
		{
			name: "text from mixed parts",
			msg: ClientMessage{
				Parts: []ContentPart{
					NewImagePartFromURL("https://example.com/img.png", "image/png"),
					NewTextPart("Text after image"),
				},
			},
			expected: "Text after image",
		},
		{
			name: "empty when no text",
			msg: ClientMessage{
				Parts: []ContentPart{
					NewImagePartFromURL("https://example.com/img.png", "image/png"),
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.msg.GetTextContent()
			if result != tt.expected {
				t.Errorf("GetTextContent() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestClientMessageHasMediaContent(t *testing.T) {
	tests := []struct {
		name     string
		msg      ClientMessage
		expected bool
	}{
		{
			name:     "text only - no media",
			msg:      ClientMessage{Content: "Hello"},
			expected: false,
		},
		{
			name: "text part only - no media",
			msg: ClientMessage{
				Parts: []ContentPart{NewTextPart("Hello")},
			},
			expected: false,
		},
		{
			name: "has image - has media",
			msg: ClientMessage{
				Parts: []ContentPart{
					NewTextPart("Check this"),
					NewImagePartFromURL("https://example.com/img.png", "image/png"),
				},
			},
			expected: true,
		},
		{
			name: "has audio - has media",
			msg: ClientMessage{
				Parts: []ContentPart{
					NewAudioPartFromURL("https://example.com/audio.mp3", "audio/mpeg"),
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.msg.HasMediaContent()
			if result != tt.expected {
				t.Errorf("HasMediaContent() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestClientMessageGetMediaParts(t *testing.T) {
	msg := ClientMessage{
		Parts: []ContentPart{
			NewTextPart("Analyze these:"),
			NewImagePartFromURL("https://example.com/img1.png", "image/png"),
			NewTextPart("And this:"),
			NewImagePartFromURL("https://example.com/img2.png", "image/png"),
			NewAudioPartFromURL("https://example.com/audio.mp3", "audio/mpeg"),
		},
	}

	mediaParts := msg.GetMediaParts()

	if len(mediaParts) != 3 {
		t.Fatalf("GetMediaParts() returned %d parts, want 3", len(mediaParts))
	}
	if mediaParts[0].Type != ContentPartTypeImage {
		t.Errorf("mediaParts[0].Type = %v, want %v", mediaParts[0].Type, ContentPartTypeImage)
	}
	if mediaParts[1].Type != ContentPartTypeImage {
		t.Errorf("mediaParts[1].Type = %v, want %v", mediaParts[1].Type, ContentPartTypeImage)
	}
	if mediaParts[2].Type != ContentPartTypeAudio {
		t.Errorf("mediaParts[2].Type = %v, want %v", mediaParts[2].Type, ContentPartTypeAudio)
	}
}

func TestServerMessageGetTextContent(t *testing.T) {
	tests := []struct {
		name     string
		msg      ServerMessage
		expected string
	}{
		{
			name: "text from Content field",
			msg: ServerMessage{
				Content: "Response content",
			},
			expected: "Response content",
		},
		{
			name: "text from Parts",
			msg: ServerMessage{
				Parts: []ContentPart{
					NewTextPart("Response from parts"),
				},
			},
			expected: "Response from parts",
		},
		{
			name: "Parts takes precedence",
			msg: ServerMessage{
				Content: "Ignored",
				Parts: []ContentPart{
					NewTextPart("Parts wins"),
				},
			},
			expected: "Parts wins",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.msg.GetTextContent()
			if result != tt.expected {
				t.Errorf("GetTextContent() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestServerMessageHasMediaContent(t *testing.T) {
	tests := []struct {
		name     string
		msg      ServerMessage
		expected bool
	}{
		{
			name:     "text only",
			msg:      ServerMessage{Content: "Hello"},
			expected: false,
		},
		{
			name: "has image",
			msg: ServerMessage{
				Parts: []ContentPart{
					NewTextPart("Here's the result"),
					NewImagePartFromURL("https://example.com/result.png", "image/png"),
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.msg.HasMediaContent()
			if result != tt.expected {
				t.Errorf("HasMediaContent() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBackwardCompatibility(t *testing.T) {
	// Test that old-style text-only messages still work
	oldStyleJSON := `{"type":"message","session_id":"sess-1","content":"Hello"}`

	var msg ClientMessage
	if err := json.Unmarshal([]byte(oldStyleJSON), &msg); err != nil {
		t.Fatalf("Failed to unmarshal old-style message: %v", err)
	}

	if msg.Content != "Hello" {
		t.Errorf("Content = %v, want 'Hello'", msg.Content)
	}
	if len(msg.Parts) != 0 {
		t.Errorf("Parts should be empty for old-style messages")
	}
	if msg.GetTextContent() != "Hello" {
		t.Errorf("GetTextContent() = %v, want 'Hello'", msg.GetTextContent())
	}
}
