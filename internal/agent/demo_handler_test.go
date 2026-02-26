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

package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/altairalabs/omnia/internal/facade"
)

func TestDemoHandler_Name(t *testing.T) {
	handler := &DemoHandler{}
	name := handler.Name()
	if name != string(HandlerModeDemo) {
		t.Errorf("Name() = %q, want %q", name, HandlerModeDemo)
	}
}

func TestDemoHandler_HandleMessage_Greeting(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "hello", content: "hello"},
		{name: "Hello uppercase", content: "Hello"},
		{name: "hi", content: "hi"},
		{name: "help", content: "help"},
		{name: "hello in sentence", content: "Can you say hello?"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &DemoHandler{}
			writer := &mockResponseWriter{}
			msg := &facade.ClientMessage{Content: tt.content}

			err := handler.HandleMessage(context.Background(), "test-session", msg, writer)
			if err != nil {
				t.Errorf("HandleMessage() error = %v", err)
				return
			}

			// Greeting should produce chunks (word-by-word streaming)
			if len(writer.chunks) == 0 {
				t.Error("HandleMessage() produced no chunks for greeting")
			}

			// Should contain greeting text
			allChunks := strings.Join(writer.chunks, "")
			if !strings.Contains(allChunks, "Omnia demo agent") {
				t.Errorf("HandleMessage() chunks = %q, should contain 'Omnia demo agent'", allChunks)
			}
		})
	}
}

func TestDemoHandler_HandleMessage_PasswordReset(t *testing.T) {
	handler := &DemoHandler{}
	writer := &mockResponseWriter{}
	msg := &facade.ClientMessage{Content: "How do I reset my password?"}

	err := handler.HandleMessage(context.Background(), "test-session", msg, writer)
	if err != nil {
		t.Errorf("HandleMessage() error = %v", err)
		return
	}

	// Password reset should produce chunks
	if len(writer.chunks) == 0 {
		t.Error("HandleMessage() produced no chunks")
	}

	// Should have a tool call
	if len(writer.toolCalls) != 1 {
		t.Errorf("HandleMessage() produced %d tool calls, want 1", len(writer.toolCalls))
	} else {
		if writer.toolCalls[0].Name != "lookup-user" {
			t.Errorf("HandleMessage() tool call name = %q, want %q", writer.toolCalls[0].Name, "lookup-user")
		}
	}

	// Should have a tool result
	if len(writer.toolResults) != 1 {
		t.Errorf("HandleMessage() produced %d tool results, want 1", len(writer.toolResults))
	}

	// Should have done message with instructions
	if !strings.Contains(writer.doneMsg, "reset your password") {
		t.Errorf("HandleMessage() doneMsg = %q, should contain reset instructions", writer.doneMsg)
	}
}

func TestDemoHandler_HandleMessage_Weather(t *testing.T) {
	handler := &DemoHandler{}
	writer := &mockResponseWriter{}
	msg := &facade.ClientMessage{Content: "What's the weather like?"}

	err := handler.HandleMessage(context.Background(), "test-session", msg, writer)
	if err != nil {
		t.Errorf("HandleMessage() error = %v", err)
		return
	}

	// Weather should produce chunks
	if len(writer.chunks) == 0 {
		t.Error("HandleMessage() produced no chunks")
	}

	// Should have a tool call for weather
	if len(writer.toolCalls) != 1 {
		t.Errorf("HandleMessage() produced %d tool calls, want 1", len(writer.toolCalls))
	} else {
		if writer.toolCalls[0].Name != "weather" {
			t.Errorf("HandleMessage() tool call name = %q, want %q", writer.toolCalls[0].Name, "weather")
		}
	}

	// Should have a tool result
	if len(writer.toolResults) != 1 {
		t.Errorf("HandleMessage() produced %d tool results, want 1", len(writer.toolResults))
	}

	// Should have done message with weather info
	if !strings.Contains(writer.doneMsg, "Denver") {
		t.Errorf("HandleMessage() doneMsg = %q, should contain 'Denver'", writer.doneMsg)
	}
}

func TestDemoHandler_HandleMessage_Default(t *testing.T) {
	handler := &DemoHandler{}
	writer := &mockResponseWriter{}
	msg := &facade.ClientMessage{Content: "Tell me about quantum computing"}

	err := handler.HandleMessage(context.Background(), "test-session", msg, writer)
	if err != nil {
		t.Errorf("HandleMessage() error = %v", err)
		return
	}

	// Default response should produce chunks
	if len(writer.chunks) == 0 {
		t.Error("HandleMessage() produced no chunks")
	}

	// Should not have tool calls
	if len(writer.toolCalls) != 0 {
		t.Errorf("HandleMessage() produced %d tool calls, want 0", len(writer.toolCalls))
	}

	// Chunks should contain the input text
	allChunks := strings.Join(writer.chunks, "")
	if !strings.Contains(allChunks, "quantum computing") {
		t.Errorf("HandleMessage() chunks = %q, should contain input text", allChunks)
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{name: "empty string", text: "", expected: 0},
		{name: "short text", text: "hello", expected: 1},                                           // 5 / 4 = 1
		{name: "medium text", text: "hello world", expected: 2},                                    // 11 / 4 = 2
		{name: "longer text", text: "This is a longer sentence for testing tokens.", expected: 11}, // 46 / 4 = 11
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := estimateTokens(tt.text)
			if result != tt.expected {
				t.Errorf("estimateTokens(%q) = %d, want %d", tt.text, result, tt.expected)
			}
		})
	}
}

func TestNewDemoHandlerWithMetrics(t *testing.T) {
	// Note: NewDemoLLMMetrics and Record are tested indirectly through this test
	// because promauto registers metrics globally and re-registration would panic.
	handler := NewDemoHandlerWithMetrics(DemoMetricsConfig{
		AgentName: "test-agent",
		Namespace: "test-namespace",
	})
	if handler == nil {
		t.Fatal("NewDemoHandlerWithMetrics() returned nil")
	}
	if handler.metrics == nil {
		t.Error("NewDemoHandlerWithMetrics() should set metrics")
	}
	// Verify the name still works
	if handler.Name() != "demo" {
		t.Errorf("Name() = %q, want %q", handler.Name(), "demo")
	}

	// Test HandleMessage with metrics enabled - this exercises Record()
	writer := &mockResponseWriter{}
	msg := &facade.ClientMessage{Content: "hello"}

	err := handler.HandleMessage(context.Background(), "test-session", msg, writer)
	if err != nil {
		t.Errorf("HandleMessage() error = %v", err)
		return
	}

	// Should produce response even with metrics enabled
	if len(writer.chunks) == 0 {
		t.Error("HandleMessage() produced no chunks")
	}
}

// Multi-modal response tests for E2E testing support

func TestDemoHandler_HandleMessage_ImageResponse(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "show image", content: "show image"},
		{name: "send image", content: "Can you send image please?"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &DemoHandler{}
			writer := &mockResponseWriter{}
			msg := &facade.ClientMessage{Content: tt.content}

			err := handler.HandleMessage(context.Background(), "test-session", msg, writer)
			if err != nil {
				t.Errorf("HandleMessage() error = %v", err)
				return
			}

			// Should have chunks (text streamed before done)
			if len(writer.chunks) == 0 {
				t.Error("HandleMessage() produced no chunks")
			}

			// Should have doneParts with image
			if len(writer.doneParts) < 2 {
				t.Errorf("HandleMessage() produced %d doneParts, want at least 2", len(writer.doneParts))
				return
			}

			// First part should be text
			if writer.doneParts[0].Type != facade.ContentPartTypeText {
				t.Errorf("doneParts[0].Type = %q, want %q", writer.doneParts[0].Type, facade.ContentPartTypeText)
			}

			// Second part should be image
			if writer.doneParts[1].Type != facade.ContentPartTypeImage {
				t.Errorf("doneParts[1].Type = %q, want %q", writer.doneParts[1].Type, facade.ContentPartTypeImage)
			}
			if writer.doneParts[1].Media == nil {
				t.Error("doneParts[1].Media is nil")
			} else {
				if writer.doneParts[1].Media.MimeType != "image/png" {
					t.Errorf("doneParts[1].Media.MimeType = %q, want %q", writer.doneParts[1].Media.MimeType, "image/png")
				}
				if writer.doneParts[1].Media.Data == "" {
					t.Error("doneParts[1].Media.Data is empty")
				}
			}
		})
	}
}

func TestDemoHandler_HandleMessage_AudioResponse(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "play audio", content: "play audio"},
		{name: "send audio", content: "please send audio"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &DemoHandler{}
			writer := &mockResponseWriter{}
			msg := &facade.ClientMessage{Content: tt.content}

			err := handler.HandleMessage(context.Background(), "test-session", msg, writer)
			if err != nil {
				t.Errorf("HandleMessage() error = %v", err)
				return
			}

			// Should have doneParts with audio
			if len(writer.doneParts) < 2 {
				t.Errorf("HandleMessage() produced %d doneParts, want at least 2", len(writer.doneParts))
				return
			}

			// Second part should be audio
			if writer.doneParts[1].Type != facade.ContentPartTypeAudio {
				t.Errorf("doneParts[1].Type = %q, want %q", writer.doneParts[1].Type, facade.ContentPartTypeAudio)
			}
			if writer.doneParts[1].Media == nil {
				t.Error("doneParts[1].Media is nil")
			} else {
				if writer.doneParts[1].Media.MimeType != "audio/mpeg" {
					t.Errorf("doneParts[1].Media.MimeType = %q, want %q", writer.doneParts[1].Media.MimeType, "audio/mpeg")
				}
				if writer.doneParts[1].Media.DurationMs != 1000 {
					t.Errorf("doneParts[1].Media.DurationMs = %d, want %d", writer.doneParts[1].Media.DurationMs, 1000)
				}
			}
		})
	}
}

func TestDemoHandler_HandleMessage_VideoResponse(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "play video", content: "play video"},
		{name: "send video", content: "please send video"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &DemoHandler{}
			writer := &mockResponseWriter{}
			msg := &facade.ClientMessage{Content: tt.content}

			err := handler.HandleMessage(context.Background(), "test-session", msg, writer)
			if err != nil {
				t.Errorf("HandleMessage() error = %v", err)
				return
			}

			// Should have doneParts with video
			if len(writer.doneParts) < 2 {
				t.Errorf("HandleMessage() produced %d doneParts, want at least 2", len(writer.doneParts))
				return
			}

			// Second part should be video
			if writer.doneParts[1].Type != facade.ContentPartTypeVideo {
				t.Errorf("doneParts[1].Type = %q, want %q", writer.doneParts[1].Type, facade.ContentPartTypeVideo)
			}
			if writer.doneParts[1].Media == nil {
				t.Error("doneParts[1].Media is nil")
			} else {
				if writer.doneParts[1].Media.MimeType != "video/mp4" {
					t.Errorf("doneParts[1].Media.MimeType = %q, want %q", writer.doneParts[1].Media.MimeType, "video/mp4")
				}
				if writer.doneParts[1].Media.DurationMs != 2000 {
					t.Errorf("doneParts[1].Media.DurationMs = %d, want %d", writer.doneParts[1].Media.DurationMs, 2000)
				}
			}
		})
	}
}

func TestDemoHandler_HandleMessage_DocumentResponse(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "show document", content: "show document"},
		{name: "send document", content: "send document please"},
		{name: "send pdf", content: "can you send pdf?"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &DemoHandler{}
			writer := &mockResponseWriter{}
			msg := &facade.ClientMessage{Content: tt.content}

			err := handler.HandleMessage(context.Background(), "test-session", msg, writer)
			if err != nil {
				t.Errorf("HandleMessage() error = %v", err)
				return
			}

			// Should have doneParts with file
			if len(writer.doneParts) < 2 {
				t.Errorf("HandleMessage() produced %d doneParts, want at least 2", len(writer.doneParts))
				return
			}

			// Second part should be file
			if writer.doneParts[1].Type != facade.ContentPartTypeFile {
				t.Errorf("doneParts[1].Type = %q, want %q", writer.doneParts[1].Type, facade.ContentPartTypeFile)
			}
			if writer.doneParts[1].Media == nil {
				t.Error("doneParts[1].Media is nil")
			} else {
				if writer.doneParts[1].Media.MimeType != "application/pdf" {
					t.Errorf("doneParts[1].Media.MimeType = %q, want %q", writer.doneParts[1].Media.MimeType, "application/pdf")
				}
				if writer.doneParts[1].Media.Filename != "test-document.pdf" {
					t.Errorf("doneParts[1].Media.Filename = %q, want %q", writer.doneParts[1].Media.Filename, "test-document.pdf")
				}
			}
		})
	}
}

func TestDemoHandler_HandleMessage_MultiModalResponse(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "multimodal", content: "give me multimodal content"},
		{name: "mixed content", content: "mixed content please"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &DemoHandler{}
			writer := &mockResponseWriter{}
			msg := &facade.ClientMessage{Content: tt.content}

			err := handler.HandleMessage(context.Background(), "test-session", msg, writer)
			if err != nil {
				t.Errorf("HandleMessage() error = %v", err)
				return
			}

			// Should have doneParts with text, image, text, and audio
			if len(writer.doneParts) < 4 {
				t.Errorf("HandleMessage() produced %d doneParts, want at least 4", len(writer.doneParts))
				return
			}

			// Verify types
			expectedTypes := []facade.ContentPartType{
				facade.ContentPartTypeText,
				facade.ContentPartTypeImage,
				facade.ContentPartTypeText,
				facade.ContentPartTypeAudio,
			}

			for i, expectedType := range expectedTypes {
				if writer.doneParts[i].Type != expectedType {
					t.Errorf("doneParts[%d].Type = %q, want %q", i, writer.doneParts[i].Type, expectedType)
				}
			}
		})
	}
}
