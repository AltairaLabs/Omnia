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

package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/AltairaLabs/PromptKit/sdk"
	"go.opentelemetry.io/otel/trace"

	"github.com/altairalabs/omnia/internal/tracing"
	"github.com/altairalabs/omnia/pkg/logctx"
)

// registerToolsWithConversation registers all available tools with a conversation.
func (s *Server) registerToolsWithConversation(ctx context.Context, conv *sdk.Conversation) error {
	log := logctx.LoggerWithContext(s.log, ctx)

	// Get all tool descriptors from the executor
	descriptors, err := s.toolExecutor.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("failed to list tools: %w", err)
	}

	// Register each tool with the conversation using a context-aware handler
	for _, desc := range descriptors {
		toolName := desc.Name
		log.V(1).Info("registering tool with conversation", "tool", toolName)

		// Create a closure that captures the executor, descriptor, and context info
		conv.OnToolCtx(toolName, func(toolCtx context.Context, args map[string]any) (any, error) {
			// Enrich with tool name
			toolCtx = logctx.WithTool(toolCtx, toolName)
			return s.executeToolForConversation(toolCtx, toolName, args)
		})
	}

	log.Info("registered tools with conversation", "count", len(descriptors))
	return nil
}

// executeToolForConversation executes a tool call for a conversation.
func (s *Server) executeToolForConversation(ctx context.Context, toolName string, args map[string]any) (any, error) {
	log := logctx.LoggerWithContext(s.log, ctx)

	// Start tool span if tracing is enabled
	var span trace.Span
	if s.tracingProvider != nil {
		ctx, span = s.tracingProvider.StartToolSpan(ctx, toolName)
		defer span.End()
	}

	log.V(1).Info("executing tool for conversation")

	// Call the tool through the manager
	start := time.Now()
	result, err := s.toolManager.Call(ctx, toolName, args)
	durationMs := int(time.Since(start).Milliseconds())
	if err != nil {
		if span != nil {
			tracing.RecordError(span, err)
		}
		return nil, fmt.Errorf("tool execution failed: %w", err)
	}

	// Add tool result metrics to span
	if span != nil {
		tracing.AddToolResult(span, result.IsError, durationMs)
		if result.IsError {
			tracing.RecordError(span, fmt.Errorf("tool error: %v", result.Content))
		} else {
			tracing.SetSuccess(span)
		}
	}

	if result.IsError {
		return nil, fmt.Errorf("tool error: %v", result.Content)
	}

	return result.Content, nil
}
