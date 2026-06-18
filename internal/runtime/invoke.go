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
	"encoding/json"
	"strings"
	"time"

	"github.com/AltairaLabs/PromptKit/sdk"
	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/altairalabs/omnia/internal/tracing"
	"github.com/altairalabs/omnia/pkg/logctx"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// Invoke handles a single one-shot Function call. Unlike Converse, the
// conversation is ephemeral: it isn't tracked in s.conversations and is
// closed before the method returns. Schema validation lives in the facade
// (per #1103 PR-2 scope); the runtime binds the validated input_json to the
// PromptPack's template variables (see runInvocation, #1473) and also keeps
// it as the user turn, then returns the model's response verbatim.
//
// Client-side tools are not supported in function mode — there's no
// WebSocket peer to fulfil them. If the model emits a client tool call,
// Invoke returns codes.FailedPrecondition.
func (s *Server) Invoke(ctx context.Context, req *runtimev1.InvocationRequest) (*runtimev1.InvocationResponse, error) {
	invocationID := req.GetInvocationId()
	if invocationID == "" {
		return nil, status.Error(codes.InvalidArgument, "invocation_id is required")
	}
	if req.GetInputJson() == "" {
		return nil, status.Error(codes.InvalidArgument, "input_json is required")
	}

	ctx = logctx.WithInvocationID(ctx, invocationID)
	ctx, span := s.startInvocationSpan(ctx, invocationID)
	defer span.End()

	ctx = logctx.WithTraceID(ctx, span.SpanContext().TraceID().String())
	log := logctx.LoggerWithContext(s.log, ctx)
	log.V(1).Info("invoke starting",
		"invocationID", invocationID,
		"inputBytes", len(req.GetInputJson()))

	conv, err := s.openInvocationConversation(ctx, invocationID)
	if err != nil {
		tracing.RecordError(span, err)
		return nil, status.Errorf(codes.Internal, "failed to create conversation: %v", err)
	}
	defer s.closeInvocationConversation(invocationID, conv, log)

	start := time.Now()
	response, content, err := s.runInvocation(ctx, conv, req.GetInputJson(), log)
	// durationMs is captured for both success and failure paths but only
	// surfaced on the success-side gRPC response; logged on error.
	durationMs := int32(time.Since(start).Milliseconds())

	if err != nil {
		tracing.RecordError(span, err)
		log.Error(err, "invoke failed", "invocationID", invocationID, "durationMs", durationMs)
		return nil, err
	}

	usage := buildInvocationUsage(response)
	tracing.SetSuccess(span)

	log.V(1).Info("invoke complete",
		"invocationID", invocationID,
		"durationMs", durationMs,
		"outputBytes", len(content))

	return &runtimev1.InvocationResponse{
		OutputJson:   content,
		Usage:        usage,
		DurationMs:   durationMs,
		InvocationId: invocationID,
	}, nil
}

// openInvocationConversation acquires the conversation mutex and builds a
// fresh PromptKit conversation for the invocation. The mutex is held for
// the entire createConversation flow because subscribeToEventBusLogging
// writes to s.unsubscribeFns under the same lock the Converse path holds.
// The conversation is NOT added to s.conversations — Functions are
// stateless and the lifecycle is closeInvocationConversation's
// responsibility.
func (s *Server) openInvocationConversation(ctx context.Context, invocationID string) (*sdk.Conversation, error) {
	s.conversationMu.Lock()
	defer s.conversationMu.Unlock()
	return s.createConversation(ctx, invocationID)
}

// closeInvocationConversation runs the cleanup that removeConversation
// runs for stateful sessions, minus the s.conversations / s.turnIndices
// deletes (which aren't populated for invocations). Critically, it must
// drain s.unsubscribeFns[invocationID] before returning — createConversation
// subscribes ~11 event-bus listeners that would otherwise leak per call.
func (s *Server) closeInvocationConversation(invocationID string, conv *sdk.Conversation, log logr.Logger) {
	s.conversationMu.Lock()
	for _, unsub := range s.unsubscribeFns[invocationID] {
		unsub()
	}
	delete(s.unsubscribeFns, invocationID)
	s.conversationMu.Unlock()

	if err := conv.Close(); err != nil {
		log.Error(err, "failed to close ephemeral conversation",
			"invocationID", invocationID)
	}
}

// startInvocationSpan wraps the invocation in a tracing span if a
// provider is configured; otherwise returns a no-op span from context.
// Uses a dedicated span name + omnia.mode="function" attribute so
// downstream queries can tell stateful turns apart from stateless calls.
func (s *Server) startInvocationSpan(ctx context.Context, invocationID string) (context.Context, trace.Span) {
	if s.tracingProvider != nil {
		return s.tracingProvider.StartInvocationSpan(ctx, invocationID, s.promptPackName, s.promptPackVersion)
	}
	return ctx, trace.SpanFromContext(ctx)
}

// runInvocation drives the SDK stream to completion and returns the
// final Response plus accumulated text.
func (s *Server) runInvocation(ctx context.Context, conv *sdk.Conversation, input string, log logr.Logger) (*sdk.Response, string, error) {
	log.V(1).Info("calling PromptKit SDK for one-shot invoke")
	var llmSpan trace.Span
	if s.tracingProvider != nil {
		ctx, llmSpan = s.tracingProvider.StartLLMSpan(ctx, s.model, s.providerType)
		defer llmSpan.End()
	}

	// Bind the validated input JSON to the PromptPack's template variables:
	// the whole object resolves {{input}}, and each top-level field resolves
	// {{field}} (e.g. {{topic}}). Passing an empty user message keeps the raw
	// JSON as the user turn (preserving prior behavior) while making
	// spec.inputSchema ↔ pack-variables a real binding rather than convention.
	// See #1473.
	streamCh := conv.Stream(ctx, "", sdk.WithJSONInput(json.RawMessage(input)))
	response, content, err := consumeInvocationStream(streamCh, llmSpan)
	if err != nil {
		return nil, "", err
	}

	if llmSpan != nil && response.TokensUsed() > 0 {
		tracing.AddLLMMetrics(llmSpan, response.InputTokens(), response.OutputTokens(), response.Cost())
		tracing.AddFinishReason(llmSpan, "stop")
		tracing.SetSuccess(llmSpan)
	}

	return response, content, nil
}

// consumeInvocationStream drains an SDK stream channel for a one-shot
// Function invocation. Client-tool chunks are treated as an authoring
// error — function mode has no WebSocket peer to fulfil them. Media
// chunks are dropped (PR 5 may add structured media to the response).
//
// Extracted so tests can drive every Chunk branch without standing up
// a full SDK Conversation.
func consumeInvocationStream(streamCh <-chan sdk.StreamChunk, llmSpan trace.Span) (*sdk.Response, string, error) {
	var finalResponse *sdk.Response
	var accumulated strings.Builder

	for chunk := range streamCh {
		if chunk.Error != nil {
			if llmSpan != nil {
				tracing.RecordError(llmSpan, chunk.Error)
			}
			return nil, "", status.Errorf(codes.Internal,
				"provider stream failed: %v", chunk.Error)
		}
		switch chunk.Type {
		case sdk.ChunkText:
			accumulated.WriteString(chunk.Text)
		case sdk.ChunkClientTool:
			toolName := ""
			if chunk.ClientTool != nil {
				toolName = chunk.ClientTool.ToolName
			}
			return nil, "", status.Errorf(codes.FailedPrecondition,
				"function emitted a client-side tool call (%q); client tools are only supported in agent mode",
				toolName)
		case sdk.ChunkDone:
			finalResponse = chunk.Message
		case sdk.ChunkMedia:
			// Media chunks are dropped for function-mode invocations.
		}
	}

	if finalResponse == nil {
		return nil, "", status.Error(codes.Internal,
			"provider stream closed without a done chunk")
	}

	return finalResponse, accumulated.String(), nil
}

// buildInvocationUsage extracts the protobuf Usage shape from the SDK
// Response, returning nil if no token accounting is available.
func buildInvocationUsage(r *sdk.Response) *runtimev1.Usage {
	if r == nil || r.TokensUsed() == 0 {
		return nil
	}
	return &runtimev1.Usage{
		InputTokens:  int32(r.InputTokens()),
		OutputTokens: int32(r.OutputTokens()),
		CostUsd:      float32(r.Cost()),
	}
}
