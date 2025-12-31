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

func TestNewDemoHandler(t *testing.T) {
	handler := NewDemoHandler()
	if handler == nil {
		t.Fatal("NewDemoHandler() returned nil")
	}
}

func TestDemoHandler_Name(t *testing.T) {
	handler := NewDemoHandler()
	name := handler.Name()
	if name != "demo" {
		t.Errorf("Name() = %q, want %q", name, "demo")
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
			handler := NewDemoHandler()
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
	handler := NewDemoHandler()
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
	handler := NewDemoHandler()
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
	handler := NewDemoHandler()
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
