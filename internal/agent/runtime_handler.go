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
	"encoding/json"
	"fmt"
	"io"

	"github.com/altairalabs/omnia/internal/facade"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

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

	// Receive and forward responses
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			// Stream completed normally
			return nil
		}
		if err != nil {
			return fmt.Errorf("error receiving from runtime: %w", err)
		}

		// Forward response to client via ResponseWriter
		if err := h.forwardResponse(resp, writer); err != nil {
			return fmt.Errorf("error forwarding response: %w", err)
		}
	}
}

// forwardResponse translates a gRPC ServerMessage to WebSocket response.
func (h *RuntimeHandler) forwardResponse(resp *runtimev1.ServerMessage, writer facade.ResponseWriter) error {
	switch msg := resp.Message.(type) {
	case *runtimev1.ServerMessage_Chunk:
		return writer.WriteChunk(msg.Chunk.Content)

	case *runtimev1.ServerMessage_Done:
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
