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
	"strings"
	"sync/atomic"

	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/AltairaLabs/PromptKit/sdk"
	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/trace"

	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"

	"github.com/altairalabs/omnia/internal/tracing"
	"github.com/altairalabs/omnia/pkg/logctx"
)

// mediaIDCounter generates unique media IDs across the process lifetime.
var mediaIDCounter atomic.Uint64

func (s *Server) processMessage(ctx context.Context, stream runtimev1.RuntimeService_ConverseServer, msg *runtimev1.ClientMessage) error {
	sessionID := msg.GetSessionId()
	content := msg.GetContent()
	metadata := msg.GetMetadata()

	// Check if trace context exists in incoming gRPC context
	incomingSpan := trace.SpanFromContext(ctx)
	if incomingSpan.SpanContext().IsValid() {
		s.log.V(1).Info("received context with trace",
			"traceID", incomingSpan.SpanContext().TraceID(),
			"spanID", incomingSpan.SpanContext().SpanID())
	} else {
		s.log.V(1).Info("received context WITHOUT trace - spans will be orphaned")
	}

	// Enrich context with session ID and start tracing span
	ctx = logctx.WithSessionID(ctx, sessionID)
	ctx, span := s.startTracingSpan(ctx, sessionID)
	defer span.End()

	// Add trace ID to log context for log↔trace correlation in Grafana
	ctx = logctx.WithTraceID(ctx, span.SpanContext().TraceID().String())
	log := logctx.LoggerWithContext(s.log, ctx)

	// Extract mock scenario and prepare message content
	scenario := s.extractScenario(metadata, content, log)
	log.V(1).Info("processing message", "contentLength", len(content), "scenario", scenario)

	log.V(1).Info("message eval config",
		"hasEvalCollector", s.evalCollector != nil,
		"evalDefCount", len(s.evalDefs))

	// Get or create conversation for this session
	conv, err := s.getOrCreateConversation(ctx, sessionID)
	if err != nil {
		err = fmt.Errorf("failed to get conversation: %w", err)
		tracing.RecordError(span, err)
		return err
	}

	// Prepare message content with scenario if needed
	messageContent := s.prepareMessageContent(content, scenario, log)

	// Build send options for multimodal content (images, audio, etc.)
	sendOpts := buildSendOptions(msg.GetParts(), log)

	// Stream response and collect results
	finalResponse, accumulatedContent, err := s.streamResponse(ctx, stream, conv, messageContent, sendOpts)
	if err != nil {
		tracing.RecordError(span, err)
		return err
	}

	// Build and send the done message
	if err := s.sendDoneMessage(ctx, stream, log, finalResponse, accumulatedContent, content); err != nil {
		tracing.RecordError(span, err)
		return err
	}

	tracing.SetSuccess(span)
	return nil
}

// startTracingSpan starts a conversation span if tracing is enabled, returning the enriched context and span.
func (s *Server) startTracingSpan(ctx context.Context, sessionID string) (context.Context, trace.Span) {
	if s.tracingProvider != nil {
		// Get and increment turn index for this session
		s.conversationMu.Lock()
		turnIndex := s.turnIndices[sessionID]
		s.turnIndices[sessionID] = turnIndex + 1
		s.conversationMu.Unlock()

		return s.tracingProvider.StartConversationSpan(ctx, sessionID, s.promptPackName, s.promptPackVersion, turnIndex)
	}
	// Return a no-op span from the context (may have been set by otelgrpc server handler)
	return ctx, trace.SpanFromContext(ctx)
}

// extractScenario extracts the mock scenario from metadata/content if mock provider is enabled.
func (s *Server) extractScenario(metadata map[string]string, content string, log logr.Logger) string {
	if !s.mockProvider {
		return ScenarioDefault
	}
	scenario := extractMockScenario(metadata, content)
	log.V(1).Info("mock scenario detected", "scenario", scenario)
	return scenario
}

// prepareMessageContent prepends scenario context to the message if using mock provider.
func (s *Server) prepareMessageContent(content string, scenario string, log logr.Logger) string {
	if s.mockProvider && scenario != ScenarioDefault {
		log.V(2).Info("enriched message with scenario", "scenario", scenario)
		return fmt.Sprintf("[scenario:%s] %s", scenario, content)
	}
	return content
}

// streamResponse streams the LLM response and sends chunks to the client.
func (s *Server) streamResponse(ctx context.Context, stream runtimev1.RuntimeService_ConverseServer, conv *sdk.Conversation, content string, opts []sdk.SendOption) (*sdk.Response, string, error) {
	log := logctx.LoggerWithContext(s.log, ctx)
	log.V(1).Info("stream starting",
		"hasEvalCollector", s.evalCollector != nil,
		"contentLength", len(content))

	// Start LLM span around the streaming call
	var llmSpan trace.Span
	if s.tracingProvider != nil {
		ctx, llmSpan = s.tracingProvider.StartLLMSpan(ctx, s.model, s.providerType)
		defer llmSpan.End()
		log.V(1).Info("created LLM span",
			"traceID", llmSpan.SpanContext().TraceID(),
			"spanID", llmSpan.SpanContext().SpanID(),
			"hasParent", llmSpan.SpanContext().HasTraceID())
	}

	log.V(1).Info("calling PromptKit SDK with context",
		"hasTraceContext", trace.SpanFromContext(ctx).SpanContext().IsValid(),
		"traceID", trace.SpanFromContext(ctx).SpanContext().TraceID().String(),
		"spanID", trace.SpanFromContext(ctx).SpanContext().SpanID().String())
	streamCh := conv.Stream(ctx, content, opts...)
	var finalResponse *sdk.Response
	var accumulatedContent strings.Builder

	for chunk := range streamCh {
		if chunk.Error != nil {
			log.Error(chunk.Error, "provider stream failed",
				"accumulatedLength", accumulatedContent.Len())
			if llmSpan != nil {
				tracing.RecordError(llmSpan, chunk.Error)
			}
			return nil, "", fmt.Errorf("failed to send message: provider stream failed: %w", chunk.Error)
		}

		switch chunk.Type {
		case sdk.ChunkText:
			if chunk.Text != "" {
				accumulatedContent.WriteString(chunk.Text)
				if err := stream.Send(&runtimev1.ServerMessage{
					Message: &runtimev1.ServerMessage_Chunk{
						Chunk: &runtimev1.Chunk{Content: chunk.Text},
					},
				}); err != nil {
					return nil, "", fmt.Errorf("failed to send chunk: %w", err)
				}
			}
		case sdk.ChunkMedia:
			if chunk.Media != nil {
				mediaChunk, err := buildMediaChunk(ctx, s, chunk.Media)
				if err != nil {
					log.Error(err, "failed to build media chunk")
					continue
				}
				if err := stream.Send(&runtimev1.ServerMessage{
					Message: &runtimev1.ServerMessage_MediaChunk{
						MediaChunk: mediaChunk,
					},
				}); err != nil {
					return nil, "", fmt.Errorf("failed to send media chunk: %w", err)
				}
			}
		case sdk.ChunkDone:
			finalResponse = chunk.Message
		}
	}

	// Add GenAI metrics to the LLM span before it ends
	if llmSpan != nil && finalResponse != nil && finalResponse.TokensUsed() > 0 {
		tracing.AddLLMMetrics(llmSpan, finalResponse.InputTokens(), finalResponse.OutputTokens(), finalResponse.Cost())
		tracing.AddFinishReason(llmSpan, "stop")
		tracing.SetSuccess(llmSpan)
	}

	log.V(1).Info("stream complete",
		"hasResponse", finalResponse != nil,
		"responseLength", accumulatedContent.Len())

	return finalResponse, accumulatedContent.String(), nil
}

// buildMediaChunk converts a PromptKit MediaContent into a gRPC MediaChunk.
// It resolves base64 data, file paths, and URLs to raw bytes for efficient gRPC transport.
func buildMediaChunk(ctx context.Context, s *Server, media *types.MediaContent) (*runtimev1.MediaChunk, error) {
	mediaID := fmt.Sprintf("media-%d", mediaIDCounter.Add(1))

	chunk := &runtimev1.MediaChunk{
		MediaId:  mediaID,
		Sequence: 0,
		IsLast:   true, // SDK emits one ChunkMedia per media item
		MimeType: media.MIMEType,
	}

	// Resolve media data to raw bytes
	if media.Data != nil && *media.Data != "" {
		decoded, err := decodeMediaData(*media.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to decode media data: %w", err)
		}
		chunk.Data = decoded
		return chunk, nil
	}

	if media.URL != nil && *media.URL != "" {
		url := *media.URL
		if IsResolvableURL(url) && s.mediaResolver != nil {
			base64Data, mimeType, _, err := s.mediaResolver.ResolveURL(url)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve media URL %s: %w", url, err)
			}
			decoded, err := decodeMediaData(base64Data)
			if err != nil {
				return nil, fmt.Errorf("failed to decode resolved media: %w", err)
			}
			chunk.Data = decoded
			chunk.MimeType = mimeType
			return chunk, nil
		}
		// For HTTP/HTTPS URLs, send zero-data chunk — facade/browser fetches directly
		_ = ctx // ctx available for future use
		return chunk, nil
	}

	if media.FilePath != nil && *media.FilePath != "" {
		if s.mediaResolver != nil {
			base64Data, mimeType, _, err := s.mediaResolver.ResolveURL("file://" + *media.FilePath)
			if err != nil {
				return nil, fmt.Errorf("failed to read media file %s: %w", *media.FilePath, err)
			}
			decoded, err := decodeMediaData(base64Data)
			if err != nil {
				return nil, fmt.Errorf("failed to decode file media: %w", err)
			}
			chunk.Data = decoded
			chunk.MimeType = mimeType
			return chunk, nil
		}
		return nil, fmt.Errorf("media resolver not configured, cannot resolve file: %s", *media.FilePath)
	}

	return nil, fmt.Errorf("media content has no data source")
}

// sendDoneMessage builds usage info and sends the done message to the client.
func (s *Server) sendDoneMessage(ctx context.Context, stream runtimev1.RuntimeService_ConverseServer, log logr.Logger, finalResponse *sdk.Response, accumulatedContent string, originalContent string) error {
	responseText, usage := s.buildUsageInfo(ctx, finalResponse, accumulatedContent, originalContent)

	// Build multimodal parts if response contains media
	var parts []*runtimev1.ContentPart
	if finalResponse != nil && finalResponse.HasMedia() && s.mediaResolver != nil {
		var err error
		parts, err = s.resolveResponseParts(ctx, finalResponse.Parts())
		if err != nil {
			log.Error(err, "failed to resolve media parts, falling back to text-only")
		}
	}

	// Send done message
	if err := stream.Send(&runtimev1.ServerMessage{
		Message: &runtimev1.ServerMessage_Done{
			Done: &runtimev1.Done{
				FinalContent: responseText,
				Usage:        usage,
				Parts:        parts,
			},
		},
	}); err != nil {
		return fmt.Errorf("failed to send done: %w", err)
	}

	return nil
}

// buildUsageInfo extracts usage info from the final response and records tracing metrics.
func (s *Server) buildUsageInfo(ctx context.Context, finalResponse *sdk.Response, accumulatedContent string, originalContent string) (string, *runtimev1.Usage) {
	if finalResponse == nil {
		return accumulatedContent, nil
	}

	responseText := finalResponse.Text()
	if finalResponse.TokensUsed() == 0 {
		return responseText, nil
	}

	inputTokens := finalResponse.InputTokens()
	outputTokens := finalResponse.OutputTokens()
	costUSD := finalResponse.Cost()

	usage := &runtimev1.Usage{
		InputTokens:  int32(inputTokens),
		OutputTokens: int32(outputTokens),
		CostUsd:      float32(costUSD),
	}

	// Add LLM metrics to the conversation span
	if s.tracingProvider != nil {
		span := trace.SpanFromContext(ctx)
		tracing.AddLLMMetrics(span, inputTokens, outputTokens, costUSD)
		tracing.AddConversationMetrics(span, len(originalContent), len(responseText))
	}

	return responseText, usage
}
