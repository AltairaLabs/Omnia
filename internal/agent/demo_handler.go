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
	"github.com/altairalabs/omnia/pkg/metrics"
)

// Simulated pricing (per 1M tokens)
const (
	demoProvider         = "anthropic"
	demoModel            = "claude-sonnet-4"
	demoInputPricePer1M  = 3.00  // $3 per 1M input tokens
	demoOutputPricePer1M = 15.00 // $15 per 1M output tokens
)

// demoDurationBuckets are histogram buckets for demo mode (shorter than production).
var demoDurationBuckets = []float64{0.5, 1, 2, 5, 10, 30, 60}

// newDemoLLMMetrics creates LLM metrics for demo mode using the shared metrics package.
func newDemoLLMMetrics(agentName, namespace string) *metrics.LLMMetrics {
	return metrics.NewLLMMetrics(metrics.LLMMetricsConfig{
		AgentName:       agentName,
		Namespace:       namespace,
		DurationBuckets: demoDurationBuckets,
	})
}

// recordDemoRequest records metrics for a simulated LLM request.
func recordDemoRequest(m *metrics.LLMMetrics, inputTokens, outputTokens int, durationSeconds float64) {
	// Calculate cost
	inputCost := (float64(inputTokens) / 1_000_000) * demoInputPricePer1M
	outputCost := (float64(outputTokens) / 1_000_000) * demoOutputPricePer1M

	m.RecordRequest(metrics.LLMRequestMetrics{
		Provider:        demoProvider,
		Model:           demoModel,
		InputTokens:     inputTokens,
		OutputTokens:    outputTokens,
		CacheHits:       0,
		CostUSD:         inputCost + outputCost,
		DurationSeconds: durationSeconds,
		Success:         true,
	})
}

// estimateTokens estimates token count from text (roughly 4 chars per token).
func estimateTokens(text string) int {
	return len(text) / 4
}

// DemoHandler provides canned responses with streaming simulation.
// Useful for demos and screenshots.
type DemoHandler struct {
	metrics *metrics.LLMMetrics
}

// NewDemoHandler creates a new DemoHandler.
func NewDemoHandler() *DemoHandler {
	return &DemoHandler{}
}

// NewDemoHandlerWithMetrics creates a DemoHandler with LLM metrics.
func NewDemoHandlerWithMetrics(agentName, namespace string) *DemoHandler {
	return &DemoHandler{
		metrics: newDemoLLMMetrics(agentName, namespace),
	}
}

// Name returns the handler name for metrics.
func (h *DemoHandler) Name() string {
	return "demo"
}

// HandleMessage processes messages and returns demo responses with streaming.
func (h *DemoHandler) HandleMessage(
	ctx context.Context,
	sessionID string,
	msg *facade.ClientMessage,
	writer facade.ResponseWriter,
) error {
	content := strings.ToLower(msg.Content)
	startTime := time.Now()

	// Simulate thinking delay
	time.Sleep(200 * time.Millisecond)

	var response string
	var err error

	// Password reset flow - demonstrates tool calls
	if strings.Contains(content, "password") {
		response, err = h.handlePasswordReset(ctx, sessionID, writer)
	} else if strings.Contains(content, "weather") {
		// Weather query - demonstrates tool calls
		response, err = h.handleWeatherQuery(ctx, sessionID, writer)
	} else if strings.Contains(content, "help") || strings.Contains(content, "hello") || strings.Contains(content, "hi") {
		// Help/greeting - demonstrates streaming
		response, err = h.handleGreeting(ctx, sessionID, writer)
	} else {
		// Default response
		response, err = h.handleDefault(ctx, sessionID, msg.Content, writer)
	}

	// Record metrics if enabled
	if h.metrics != nil {
		inputTokens := estimateTokens(msg.Content)
		outputTokens := estimateTokens(response)
		duration := time.Since(startTime).Seconds()
		recordDemoRequest(h.metrics, inputTokens, outputTokens, duration)
	}

	return err
}

func (h *DemoHandler) handlePasswordReset(_ context.Context, sessionID string, writer facade.ResponseWriter) (string, error) {
	// Stream initial response
	chunks := []string{
		"I can help you ",
		"reset your password. ",
		"Let me look up ",
		"your account...",
	}
	var fullResponse string
	for _, chunk := range chunks {
		if err := writer.WriteChunk(chunk); err != nil {
			return "", err
		}
		fullResponse += chunk
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
		return "", err
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
		return "", err
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

	fullResponse += finalResponse
	return fullResponse, writer.WriteDone(finalResponse)
}

func (h *DemoHandler) handleWeatherQuery(_ context.Context, _ string, writer facade.ResponseWriter) (string, error) {
	// Stream initial response
	fullResponse := "Checking the weather for you"
	if err := writer.WriteChunk(fullResponse); err != nil {
		return "", err
	}
	time.Sleep(100 * time.Millisecond)

	for i := 0; i < 3; i++ {
		if err := writer.WriteChunk("."); err != nil {
			return "", err
		}
		fullResponse += "."
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
		return "", err
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
		return "", err
	}

	finalResponse := `

Here's the current weather in Denver, CO:

🌡️ Temperature: 72°F
☀️ Condition: Sunny
💧 Humidity: 45%
💨 Wind: 5 mph NW

It's a beautiful day! Perfect for outdoor activities.`

	fullResponse += finalResponse
	return fullResponse, writer.WriteDone(finalResponse)
}

func (h *DemoHandler) handleGreeting(_ context.Context, _ string, writer facade.ResponseWriter) (string, error) {
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
				return "", err
			}
		}
		if err := writer.WriteChunk(word); err != nil {
			return "", err
		}
		time.Sleep(30 * time.Millisecond)
	}

	return response, writer.WriteDone("")
}

func (h *DemoHandler) handleDefault(_ context.Context, _ string, input string, writer facade.ResponseWriter) (string, error) {
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
				return "", err
			}
		}
		if err := writer.WriteChunk(word); err != nil {
			return "", err
		}
		time.Sleep(25 * time.Millisecond)
	}

	return response, writer.WriteDone("")
}
