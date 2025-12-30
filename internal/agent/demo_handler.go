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
	"time"

	"github.com/altairalabs/omnia/internal/facade"
)

// DemoHandler provides canned responses with streaming simulation.
// Useful for demos and screenshots.
type DemoHandler struct{}

// NewDemoHandler creates a new DemoHandler.
func NewDemoHandler() *DemoHandler {
	return &DemoHandler{}
}

// HandleMessage processes messages and returns demo responses with streaming.
func (h *DemoHandler) HandleMessage(
	ctx context.Context,
	sessionID string,
	msg *facade.ClientMessage,
	writer facade.ResponseWriter,
) error {
	content := strings.ToLower(msg.Content)

	// Simulate thinking delay
	time.Sleep(200 * time.Millisecond)

	// Password reset flow - demonstrates tool calls
	if strings.Contains(content, "password") {
		return h.handlePasswordReset(ctx, sessionID, writer)
	}

	// Weather query - demonstrates tool calls
	if strings.Contains(content, "weather") {
		return h.handleWeatherQuery(ctx, sessionID, writer)
	}

	// Help/greeting - demonstrates streaming
	if strings.Contains(content, "help") || strings.Contains(content, "hello") || strings.Contains(content, "hi") {
		return h.handleGreeting(ctx, sessionID, writer)
	}

	// Default response
	return h.handleDefault(ctx, sessionID, msg.Content, writer)
}

func (h *DemoHandler) handlePasswordReset(_ context.Context, sessionID string, writer facade.ResponseWriter) error {
	// Stream initial response
	chunks := []string{
		"I can help you ",
		"reset your password. ",
		"Let me look up ",
		"your account...",
	}
	for _, chunk := range chunks {
		if err := writer.WriteChunk(chunk); err != nil {
			return err
		}
		time.Sleep(80 * time.Millisecond)
	}

	// Simulate tool call
	if err := writer.WriteToolCall(&facade.ToolCallInfo{
		ID:   "call_001",
		Name: "lookup-user",
		Arguments: map[string]interface{}{
			"session_id": sessionID,
		},
	}); err != nil {
		return err
	}
	time.Sleep(400 * time.Millisecond)

	// Tool result
	if err := writer.WriteToolResult(&facade.ToolResultInfo{
		ID: "call_001",
		Result: map[string]interface{}{
			"status":       "found",
			"email":        "user@example.com",
			"account_type": "premium",
		},
	}); err != nil {
		return err
	}

	// Final response
	finalResponse := `

Great news! I found your account. Here's how to reset your password:

1. Go to the login page
2. Click "Forgot Password"
3. Enter your email: user@example.com
4. Check your inbox for the reset link
5. Create a new secure password

The reset link will expire in 24 hours. Let me know if you need any other help!`

	return writer.WriteDone(finalResponse)
}

func (h *DemoHandler) handleWeatherQuery(_ context.Context, _ string, writer facade.ResponseWriter) error {
	// Stream initial response
	if err := writer.WriteChunk("Checking the weather for you"); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)

	for i := 0; i < 3; i++ {
		if err := writer.WriteChunk("."); err != nil {
			return err
		}
		time.Sleep(150 * time.Millisecond)
	}

	// Simulate tool call
	if err := writer.WriteToolCall(&facade.ToolCallInfo{
		ID:   "call_002",
		Name: "weather",
		Arguments: map[string]interface{}{
			"location": "Denver, CO",
		},
	}); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)

	// Tool result
	if err := writer.WriteToolResult(&facade.ToolResultInfo{
		ID: "call_002",
		Result: map[string]interface{}{
			"temperature": "72°F",
			"condition":   "Sunny",
			"humidity":    "45%",
			"wind":        "5 mph NW",
		},
	}); err != nil {
		return err
	}

	finalResponse := `

Here's the current weather in Denver, CO:

🌡️ Temperature: 72°F
☀️ Condition: Sunny
💧 Humidity: 45%
💨 Wind: 5 mph NW

It's a beautiful day! Perfect for outdoor activities.`

	return writer.WriteDone(finalResponse)
}

func (h *DemoHandler) handleGreeting(_ context.Context, _ string, writer facade.ResponseWriter) error {
	response := `Hello! I'm the Omnia demo agent. I can help you with:

• **Password resets** - Just ask "How do I reset my password?"
• **Weather lookups** - Try "What's the weather?"
• **General questions** - I'll do my best to help!

This is a demo mode with simulated responses. In production, I would connect to an LLM provider for real AI-powered conversations.

How can I help you today?`

	// Stream word by word
	words := strings.Split(response, " ")
	for i, word := range words {
		if i > 0 {
			if err := writer.WriteChunk(" "); err != nil {
				return err
			}
		}
		if err := writer.WriteChunk(word); err != nil {
			return err
		}
		time.Sleep(30 * time.Millisecond)
	}

	return writer.WriteDone("")
}

func (h *DemoHandler) handleDefault(_ context.Context, _ string, input string, writer facade.ResponseWriter) error {
	response := "I understand you're asking about: \"" + input + "\"\n\n"
	response += "In demo mode, I have limited responses. Try asking about:\n"
	response += "• Password resets\n"
	response += "• Weather\n"
	response += "• Or just say \"hello\" for help!\n\n"
	response += "In production with LLM mode, I would provide a real AI response."

	// Stream the response
	words := strings.Split(response, " ")
	for i, word := range words {
		if i > 0 {
			if err := writer.WriteChunk(" "); err != nil {
				return err
			}
		}
		if err := writer.WriteChunk(word); err != nil {
			return err
		}
		time.Sleep(25 * time.Millisecond)
	}

	return writer.WriteDone("")
}
