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
	"sync"
	"time"

	"github.com/altairalabs/omnia/internal/facade"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// defaultStreamInactivityTimeout is the maximum time to wait between gRPC messages
// from the runtime before cancelling the stream. This prevents hanging connections
// when the LLM provider stalls mid-response.
const defaultStreamInactivityTimeout = 120 * time.Second

// defaultClientToolTimeout is the maximum time to wait for a client tool response.
const defaultClientToolTimeout = 60 * time.Second

// RuntimeHandler delegates message handling to the runtime sidecar via gRPC.
// It implements facade.ClientToolRouter to support client-side tool execution.
type RuntimeHandler struct {
	client            *facade.RuntimeClient
	clientToolTimeout time.Duration

	// toolResultChannels maps sessionID → channel for receiving client tool results.
	toolResultChannels sync.Map
}

// NewRuntimeHandler creates a new RuntimeHandler with the given client.
func NewRuntimeHandler(client *facade.RuntimeClient) *RuntimeHandler {
	return &RuntimeHandler{
		client:            client,
		clientToolTimeout: defaultClientToolTimeout,
	}
}

// Name returns the handler name for metrics.
func (h *RuntimeHandler) Name() string {
	return "runtime"
}

// SetClientToolTimeout overrides the default timeout for client tool responses.
func (h *RuntimeHandler) SetClientToolTimeout(d time.Duration) {
	h.clientToolTimeout = d
}

// SendToolResult delivers a client tool result to the handler waiting for it.
// Returns true if the result was routed successfully, false if no handler is waiting.
func (h *RuntimeHandler) SendToolResult(sessionID string, result *facade.ClientToolResultInfo) bool {
	ch, ok := h.toolResultChannels.Load(sessionID)
	if !ok {
		return false
	}
	select {
	case ch.(chan *facade.ClientToolResultInfo) <- result:
		return true
	default:
		return false
	}
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

	// Register a tool result channel for this session
	toolResultCh := make(chan *facade.ClientToolResultInfo, 1)
	h.toolResultChannels.Store(sessionID, toolResultCh)
	defer h.toolResultChannels.Delete(sessionID)

	// Defer CloseSend — stream stays open for client tool results
	defer func() { _ = stream.CloseSend() }()

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

	return h.receiveResponses(ctx, stream, writer, toolResultCh)
}

// recvResult holds the result of a single gRPC Recv call.
type recvResult struct {
	resp *runtimev1.ServerMessage
	err  error
}

// receiveResponses reads from the gRPC stream with an inactivity timeout.
// When a client-side tool call arrives, it forwards it to the WebSocket client,
// waits for the result, and sends it back on the gRPC stream.
func (h *RuntimeHandler) receiveResponses(
	ctx context.Context,
	stream runtimev1.RuntimeService_ConverseClient,
	writer facade.ResponseWriter,
	toolResultCh <-chan *facade.ClientToolResultInfo,
) error {
	inactivityTimer := time.NewTimer(defaultStreamInactivityTimeout)
	defer inactivityTimer.Stop()

	ch := make(chan recvResult, 1)
	done := make(chan struct{})
	defer close(done)

	go h.startRecvLoop(stream, ch, done)

	for {
		select {
		case result := <-ch:
			err := h.handleRecvResult(ctx, stream, writer, result, toolResultCh, inactivityTimer)
			if err == errStreamDone {
				return nil
			}
			if err != nil {
				return err
			}
		case <-inactivityTimer.C:
			return fmt.Errorf("runtime stream inactivity timeout (%s)", defaultStreamInactivityTimeout)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// errStreamDone is a sentinel indicating the gRPC stream ended normally.
var errStreamDone = fmt.Errorf("stream done")

// startRecvLoop continuously reads from the gRPC stream and sends results to ch.
func (h *RuntimeHandler) startRecvLoop(stream runtimev1.RuntimeService_ConverseClient, ch chan<- recvResult, done <-chan struct{}) {
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
}

// handleRecvResult processes a single received gRPC message.
// Returns errStreamDone on EOF, nil to continue, or an error to abort.
func (h *RuntimeHandler) handleRecvResult(
	ctx context.Context,
	stream runtimev1.RuntimeService_ConverseClient,
	writer facade.ResponseWriter,
	result recvResult,
	toolResultCh <-chan *facade.ClientToolResultInfo,
	inactivityTimer *time.Timer,
) error {
	if result.err == io.EOF {
		return errStreamDone
	}
	if result.err != nil {
		return fmt.Errorf("error receiving from runtime: %w", result.err)
	}
	resetTimer(inactivityTimer, defaultStreamInactivityTimeout)

	if isClientToolCall(result.resp) {
		return h.handleClientToolCall(ctx, stream, writer, result.resp, toolResultCh)
	}

	// Check if this is a Done message — the conversation turn is complete.
	// Without this, both sides block on Recv() (deadlock): the runtime loops
	// back to read the next client message while the facade waits for more
	// server messages. The old code avoided this by calling CloseSend()
	// before reading, but the client-tool flow needs the stream open.
	_, isDone := result.resp.Message.(*runtimev1.ServerMessage_Done)

	if err := h.forwardResponse(result.resp, writer); err != nil {
		return fmt.Errorf("error forwarding response: %w", err)
	}

	if isDone {
		return errStreamDone
	}
	return nil
}

// isClientToolCall returns true if the message is a ToolCall with CLIENT execution.
func isClientToolCall(resp *runtimev1.ServerMessage) bool {
	tc, ok := resp.Message.(*runtimev1.ServerMessage_ToolCall)
	return ok && tc.ToolCall.Execution == runtimev1.ToolExecution_TOOL_EXECUTION_CLIENT
}

// handleClientToolCall forwards a client tool call to the WebSocket client,
// waits for the result, and sends it back on the gRPC stream.
func (h *RuntimeHandler) handleClientToolCall(
	ctx context.Context,
	stream runtimev1.RuntimeService_ConverseClient,
	writer facade.ResponseWriter,
	resp *runtimev1.ServerMessage,
	toolResultCh <-chan *facade.ClientToolResultInfo,
) error {
	// Forward the tool call to the WebSocket client with execution=client
	if err := h.forwardResponse(resp, writer); err != nil {
		return fmt.Errorf("error forwarding client tool call: %w", err)
	}

	// Wait for the client to send back a result
	result, err := h.waitForToolResult(ctx, toolResultCh)
	if err != nil {
		return fmt.Errorf("error waiting for client tool result: %w", err)
	}

	// Send the result back to the runtime on the gRPC stream
	return h.sendToolResultToRuntime(stream, result)
}

// waitForToolResult waits for a client tool result with a configurable timeout.
func (h *RuntimeHandler) waitForToolResult(
	ctx context.Context,
	toolResultCh <-chan *facade.ClientToolResultInfo,
) (*facade.ClientToolResultInfo, error) {
	timer := time.NewTimer(h.clientToolTimeout)
	defer timer.Stop()

	select {
	case result := <-toolResultCh:
		return result, nil
	case <-timer.C:
		return nil, fmt.Errorf("client tool timeout (%s)", h.clientToolTimeout)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// sendToolResultToRuntime sends a ClientToolResult back on the gRPC stream.
func (h *RuntimeHandler) sendToolResultToRuntime(
	stream runtimev1.RuntimeService_ConverseClient,
	result *facade.ClientToolResultInfo,
) error {
	grpcResult := &runtimev1.ClientToolResult{
		CallId: result.CallID,
	}

	if result.Error != "" {
		grpcResult.IsRejected = true
		grpcResult.RejectionReason = result.Error
	} else {
		resultJSON, err := json.Marshal(result.Result)
		if err != nil {
			return fmt.Errorf("failed to marshal tool result: %w", err)
		}
		grpcResult.ResultJson = string(resultJSON)
	}

	return stream.Send(&runtimev1.ClientMessage{
		ClientToolResult: grpcResult,
	})
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
		return h.forwardToolCall(msg.ToolCall, writer)

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

// forwardToolCall translates a gRPC ToolCall to a facade ToolCallInfo.
func (h *RuntimeHandler) forwardToolCall(tc *runtimev1.ToolCall, writer facade.ResponseWriter) error {
	var args map[string]interface{}
	if tc.ArgumentsJson != "" {
		if json.Unmarshal([]byte(tc.ArgumentsJson), &args) != nil {
			args = map[string]interface{}{"raw": tc.ArgumentsJson}
		}
	}

	info := &facade.ToolCallInfo{
		ID:        tc.Id,
		Name:      tc.Name,
		Arguments: args,
	}

	// Add client tool fields — copy Categories to avoid sharing the proto slice.
	if tc.Execution == runtimev1.ToolExecution_TOOL_EXECUTION_CLIENT {
		info.Execution = "client"
		info.ConsentMessage = tc.ConsentMessage
		if len(tc.Categories) > 0 {
			info.Categories = make([]string, len(tc.Categories))
			copy(info.Categories, tc.Categories)
		}
	}

	return writer.WriteToolCall(info)
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
