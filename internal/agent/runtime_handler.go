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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/altairalabs/omnia/internal/facade"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// defaultStreamInactivityTimeout is the maximum time to wait between gRPC messages
// from the runtime before cancelling the stream. This prevents hanging connections
// when the LLM provider stalls mid-response.
const defaultStreamInactivityTimeout = 120 * time.Second

// RuntimeHandler delegates message handling to the runtime sidecar via gRPC.
type RuntimeHandler struct {
	client *facade.RuntimeClient
}

// NewRuntimeHandler creates a new RuntimeHandler with the given client.
func NewRuntimeHandler(client *facade.RuntimeClient) *RuntimeHandler {
	return &RuntimeHandler{
		client: client,
	}
}

// Name returns the handler name for metrics.
func (h *RuntimeHandler) Name() string {
	return "runtime"
}

// HandleMessage sends the message to the runtime and streams responses back.
func (h *RuntimeHandler) HandleMessage(
	ctx context.Context,
	sessionID string,
	msg *facade.ClientMessage,
	writer facade.ResponseWriter,
) error {
	// Open bidirectional stream to runtime
	stream, err := h.client.Converse(ctx)
	if err != nil {
		return fmt.Errorf("failed to open stream to runtime: %w", err)
	}

	// Convert metadata to string map
	metadata := make(map[string]string)
	for k, v := range msg.Metadata {
		metadata[k] = v
	}

	// Send client message to runtime
	grpcMsg := &runtimev1.ClientMessage{
		SessionId: sessionID,
		Content:   msg.Content,
		Metadata:  metadata,
		Parts:     toGRPCContentParts(msg.Parts),
	}

	if err := stream.Send(grpcMsg); err != nil {
		return fmt.Errorf("failed to send message to runtime: %w", err)
	}

	// Close send side to signal we're done sending
	if err := stream.CloseSend(); err != nil {
		return fmt.Errorf("failed to close send: %w", err)
	}

	return h.receiveResponses(stream, writer)
}

// recvResult holds the result of a single gRPC Recv call.
type recvResult struct {
	resp *runtimev1.ServerMessage
	err  error
}

// receiveResponses reads from the gRPC stream with an inactivity timeout.
// If no message arrives within defaultStreamInactivityTimeout, it returns an error.
func (h *RuntimeHandler) receiveResponses(
	stream runtimev1.RuntimeService_ConverseClient,
	writer facade.ResponseWriter,
) error {
	inactivityTimer := time.NewTimer(defaultStreamInactivityTimeout)
	defer inactivityTimer.Stop()

	ch := make(chan recvResult, 1)
	done := make(chan struct{})
	defer close(done)

	// Start a single goroutine that continuously reads from the stream
	// and sends results to ch. The goroutine exits when the stream ends
	// or when done is closed (on timeout).
	go func() {
		for {
			resp, err := stream.Recv()
			select {
			case ch <- recvResult{resp, err}:
			case <-done:
				return
			}
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case result := <-ch:
			if result.err == io.EOF {
				return nil
			}
			if result.err != nil {
				return fmt.Errorf("error receiving from runtime: %w", result.err)
			}
			resetTimer(inactivityTimer, defaultStreamInactivityTimeout)
			if err := h.forwardResponse(result.resp, writer); err != nil {
				return fmt.Errorf("error forwarding response: %w", err)
			}
		case <-inactivityTimer.C:
			return fmt.Errorf("runtime stream inactivity timeout (%s)", defaultStreamInactivityTimeout)
		}
	}
}

// resetTimer safely resets a timer, draining the channel if needed.
func resetTimer(t *time.Timer, d time.Duration) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	t.Reset(d)
}

// forwardResponse translates a gRPC ServerMessage to WebSocket response.
func (h *RuntimeHandler) forwardResponse(resp *runtimev1.ServerMessage, writer facade.ResponseWriter) error {
	switch msg := resp.Message.(type) {
	case *runtimev1.ServerMessage_Chunk:
		return writer.WriteChunk(msg.Chunk.Content)

	case *runtimev1.ServerMessage_Done:
		// Report usage if available
		if msg.Done.Usage != nil {
			if reporter, ok := writer.(facade.UsageReporter); ok {
				reporter.ReportUsage(&facade.UsageInfo{
					InputTokens:  msg.Done.Usage.InputTokens,
					OutputTokens: msg.Done.Usage.OutputTokens,
					CostUSD:      float64(msg.Done.Usage.CostUsd),
				})
			}
		}
		// If response has multimodal parts, forward them; otherwise use text
		if len(msg.Done.Parts) > 0 {
			return writer.WriteDoneWithParts(fromGRPCContentParts(msg.Done.Parts))
		}
		return writer.WriteDone(msg.Done.FinalContent)

	case *runtimev1.ServerMessage_ToolCall:
		// Parse arguments JSON to map
		var args map[string]interface{}
		if msg.ToolCall.ArgumentsJson != "" {
			if json.Unmarshal([]byte(msg.ToolCall.ArgumentsJson), &args) != nil {
				// If parsing fails, use raw JSON as single argument
				args = map[string]interface{}{"raw": msg.ToolCall.ArgumentsJson}
			}
		}
		return writer.WriteToolCall(&facade.ToolCallInfo{
			ID:        msg.ToolCall.Id,
			Name:      msg.ToolCall.Name,
			Arguments: args,
		})

	case *runtimev1.ServerMessage_ToolResult:
		// Parse result JSON
		var result interface{}
		if msg.ToolResult.ResultJson != "" {
			if json.Unmarshal([]byte(msg.ToolResult.ResultJson), &result) != nil {
				result = msg.ToolResult.ResultJson
			}
		}
		toolResult := &facade.ToolResultInfo{
			ID:     msg.ToolResult.Id,
			Result: result,
		}
		if msg.ToolResult.IsError {
			toolResult.Error = fmt.Sprintf("%v", result)
			toolResult.Result = nil
		}
		return writer.WriteToolResult(toolResult)

	case *runtimev1.ServerMessage_MediaChunk:
		mc := msg.MediaChunk
		return writer.WriteMediaChunk(&facade.MediaChunkInfo{
			MediaID:  mc.MediaId,
			Sequence: int(mc.Sequence),
			IsLast:   mc.IsLast,
			Data:     base64.StdEncoding.EncodeToString(mc.Data),
			MimeType: mc.MimeType,
		})

	case *runtimev1.ServerMessage_Error:
		return writer.WriteError(msg.Error.Code, msg.Error.Message)

	default:
		// Unknown message type, ignore
		return nil
	}
}

// Client returns the underlying runtime client for health checks.
func (h *RuntimeHandler) Client() *facade.RuntimeClient {
	return h.client
}

// toGRPCContentParts converts facade ContentParts to gRPC ContentParts.
func toGRPCContentParts(parts []facade.ContentPart) []*runtimev1.ContentPart {
	if len(parts) == 0 {
		return nil
	}

	grpcParts := make([]*runtimev1.ContentPart, len(parts))
	for i, part := range parts {
		grpcPart := &runtimev1.ContentPart{
			Type: string(part.Type),
			Text: part.Text,
		}
		if part.Media != nil {
			grpcPart.Media = &runtimev1.MediaContent{
				Data:     part.Media.Data,
				Url:      part.Media.URL,
				MimeType: part.Media.MimeType,
			}
		}
		grpcParts[i] = grpcPart
	}
	return grpcParts
}

// fromGRPCContentParts converts gRPC ContentParts to facade ContentParts.
func fromGRPCContentParts(parts []*runtimev1.ContentPart) []facade.ContentPart {
	if len(parts) == 0 {
		return nil
	}

	facadeParts := make([]facade.ContentPart, len(parts))
	for i, part := range parts {
		facadePart := facade.ContentPart{
			Type: facade.ContentPartType(part.Type),
			Text: part.Text,
		}
		if part.Media != nil {
			facadePart.Media = &facade.MediaContent{
				Data:     part.Media.Data,
				URL:      part.Media.Url,
				MimeType: part.Media.MimeType,
			}
		}
		facadeParts[i] = facadePart
	}
	return facadeParts
}
