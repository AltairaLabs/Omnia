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
	"testing"

	"github.com/altairalabs/omnia/internal/facade"
)

// mockResponseWriter implements facade.ResponseWriter for testing.
type mockResponseWriter struct {
	chunks      []string
	chunkParts  [][]facade.ContentPart
	doneMsg     string
	doneParts   []facade.ContentPart
	toolCalls   []*facade.ToolCallInfo
	toolResults []*facade.ToolResultInfo
	errors      []struct{ code, message string }
	err         error
}

func (m *mockResponseWriter) WriteChunk(content string) error {
	if m.err != nil {
		return m.err
	}
	m.chunks = append(m.chunks, content)
	return nil
}

func (m *mockResponseWriter) WriteChunkWithParts(parts []facade.ContentPart) error {
	if m.err != nil {
		return m.err
	}
	m.chunkParts = append(m.chunkParts, parts)
	return nil
}

func (m *mockResponseWriter) WriteDone(content string) error {
	if m.err != nil {
		return m.err
	}
	m.doneMsg = content
	return nil
}

func (m *mockResponseWriter) WriteDoneWithParts(parts []facade.ContentPart) error {
	if m.err != nil {
		return m.err
	}
	m.doneParts = parts
	return nil
}

func (m *mockResponseWriter) WriteToolCall(info *facade.ToolCallInfo) error {
	if m.err != nil {
		return m.err
	}
	m.toolCalls = append(m.toolCalls, info)
	return nil
}

func (m *mockResponseWriter) WriteToolResult(info *facade.ToolResultInfo) error {
	if m.err != nil {
		return m.err
	}
	m.toolResults = append(m.toolResults, info)
	return nil
}

func (m *mockResponseWriter) WriteError(code, message string) error {
	if m.err != nil {
		return m.err
	}
	m.errors = append(m.errors, struct{ code, message string }{code, message})
	return nil
}

func TestNewEchoHandler(t *testing.T) {
	handler := NewEchoHandler()
	if handler == nil {
		t.Fatal("NewEchoHandler() returned nil")
	}
}

func TestEchoHandler_Name(t *testing.T) {
	handler := NewEchoHandler()
	name := handler.Name()
	if name != "echo" {
		t.Errorf("Name() = %q, want %q", name, "echo")
	}
}

func TestEchoHandler_HandleMessage(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantDoneMsg string
	}{
		{
			name:        "simple message",
			content:     "Hello",
			wantDoneMsg: "Echo: Hello",
		},
		{
			name:        "empty message",
			content:     "",
			wantDoneMsg: "Echo: ",
		},
		{
			name:        "message with special characters",
			content:     "Hello, World! 🌍",
			wantDoneMsg: "Echo: Hello, World! 🌍",
		},
		{
			name:        "multiline message",
			content:     "line1\nline2\nline3",
			wantDoneMsg: "Echo: line1\nline2\nline3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewEchoHandler()
			writer := &mockResponseWriter{}
			msg := &facade.ClientMessage{Content: tt.content}

			err := handler.HandleMessage(context.Background(), "test-session", msg, writer)
			if err != nil {
				t.Errorf("HandleMessage() error = %v", err)
				return
			}

			if writer.doneMsg != tt.wantDoneMsg {
				t.Errorf("HandleMessage() doneMsg = %q, want %q", writer.doneMsg, tt.wantDoneMsg)
			}

			if len(writer.chunks) != 0 {
				t.Errorf("HandleMessage() wrote %d chunks, want 0", len(writer.chunks))
			}
		})
	}
}
