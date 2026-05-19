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
// (per #1103 PR-2 scope); the runtime treats input_json as opaque user
// content and returns the model's response verbatim.
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

	ctx = logctx.WithSessionID(ctx, invocationID)
	ctx, span := s.startInvocationSpan(ctx, invocationID)
	defer span.End()

	ctx = logctx.WithTraceID(ctx, span.SpanContext().TraceID().String())
	log := logctx.LoggerWithContext(s.log, ctx)
	log.V(1).Info("invoke starting",
		"invocationID", invocationID,
		"inputBytes", len(req.GetInputJson()))

	conv, err := s.createConversation(ctx, invocationID)
	if err != nil {
		tracing.RecordError(span, err)
		return nil, status.Errorf(codes.Internal, "failed to create conversation: %v", err)
	}
	defer func() {
		// Functions are stateless — close the conversation as soon as the
		// invocation completes. We deliberately do NOT add the conversation
		// to s.conversations.
		if closeErr := conv.Close(); closeErr != nil {
			log.Error(closeErr, "failed to close ephemeral conversation",
				"invocationID", invocationID)
		}
	}()

	start := time.Now()
	response, content, err := s.runInvocation(ctx, conv, req.GetInputJson(), log)
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

// startInvocationSpan wraps the invocation in a tracing span if a
// provider is configured; otherwise returns a no-op span from context.
func (s *Server) startInvocationSpan(ctx context.Context, invocationID string) (context.Context, trace.Span) {
	if s.tracingProvider != nil {
		// Reuse the conversation-span shape: the SDK + LLM children will
		// nest under it. Turn index is always 0 for one-shot Functions.
		return s.tracingProvider.StartConversationSpan(ctx, invocationID, s.promptPackName, s.promptPackVersion, 0)
	}
	return ctx, trace.SpanFromContext(ctx)
}

// runInvocation drives the SDK stream to completion and returns the
// final Response plus accumulated text. Client-tool chunks are treated
// as an authoring error — function mode has no WebSocket to fulfil them.
func (s *Server) runInvocation(ctx context.Context, conv *sdk.Conversation, input string, log logr.Logger) (*sdk.Response, string, error) {
	log.V(1).Info("calling PromptKit SDK for one-shot invoke")
	var llmSpan trace.Span
	if s.tracingProvider != nil {
		ctx, llmSpan = s.tracingProvider.StartLLMSpan(ctx, s.model, s.providerType)
		defer llmSpan.End()
	}

	streamCh := conv.Stream(ctx, input)

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
			// Function mode has no peer to fulfil client tools.
			return nil, "", status.Errorf(codes.FailedPrecondition,
				"function emitted a client-side tool call (%q); client tools are only supported in agent mode",
				chunk.ClientTool.ToolName)
		case sdk.ChunkDone:
			finalResponse = chunk.Message
		case sdk.ChunkMedia:
			// Media chunks are dropped for function-mode invocations.
			// PR 5 may add structured media to the response if needed.
		}
	}

	if llmSpan != nil && finalResponse != nil && finalResponse.TokensUsed() > 0 {
		tracing.AddLLMMetrics(llmSpan, finalResponse.InputTokens(), finalResponse.OutputTokens(), finalResponse.Cost())
		tracing.AddFinishReason(llmSpan, "stop")
		tracing.SetSuccess(llmSpan)
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
