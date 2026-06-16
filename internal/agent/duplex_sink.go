/*
Copyright 2025-2026.

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

	"github.com/altairalabs/omnia/internal/facade"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// grpcDuplexSink bridges the facade audio session to the runtime gRPC Converse
// stream. It satisfies facade.DuplexSink and is injected into the WebSocket
// facade via facade.WithDuplexSinkFactory in cmd/agent.
type grpcDuplexSink struct {
	sessionID string
	client    *facade.RuntimeClient
	writer    facade.ResponseWriter
	stream    runtimev1.RuntimeService_ConverseClient
	cancel    context.CancelFunc
}

// NewGRPCDuplexSink constructs a grpcDuplexSink. Start must be called before
// SendAudio to open the runtime stream.
// It is exported so cmd/agent can inject it into the facade via
// facade.WithDuplexSinkFactory without the facade importing internal/agent.
func NewGRPCDuplexSink(sessionID string, client *facade.RuntimeClient, w facade.ResponseWriter) *grpcDuplexSink {
	return &grpcDuplexSink{sessionID: sessionID, client: client, writer: w}
}

// Start opens the Converse stream toward the runtime, sends the DuplexStart
// negotiation message, and launches a background goroutine to relay outbound
// audio chunks back to the WebSocket client via the ResponseWriter.
// Audio frames must NOT use gzip compression; no grpc.CallOption is passed.
func (g *grpcDuplexSink) Start(ctx context.Context, s *facade.AudioSessionStart) error {
	streamCtx, cancel := context.WithCancel(ctx)
	g.cancel = cancel

	// Converse with no compressor — audio frames must not be gzip-compressed.
	stream, err := g.client.Converse(streamCtx)
	if err != nil {
		cancel()
		return err
	}
	g.stream = stream

	if err := stream.Send(&runtimev1.ClientMessage{
		SessionId: g.sessionID,
		DuplexStart: &runtimev1.DuplexStart{
			Codec:      s.Codec,
			SampleRate: s.SampleRate,
			Channels:   s.Channels,
		},
	}); err != nil {
		cancel()
		return err
	}

	go g.relayOut()
	return nil
}

// relayOut reads ServerMessages from the runtime stream and forwards any
// MediaChunk payloads to the WebSocket client via WriteBinaryMediaChunk.
// It runs until the stream ends or the context is cancelled.
func (g *grpcDuplexSink) relayOut() {
	for {
		resp, err := g.stream.Recv()
		if err != nil {
			return
		}
		if mc, ok := resp.Message.(*runtimev1.ServerMessage_MediaChunk); ok {
			var mediaID [facade.MediaIDSize]byte
			_ = g.writer.WriteBinaryMediaChunk(
				mediaID,
				uint32(mc.MediaChunk.Sequence), //nolint:gosec // sequence is non-negative protocol value
				mc.MediaChunk.IsLast,
				mc.MediaChunk.MimeType,
				mc.MediaChunk.Data,
			)
		}
	}
}

// SendAudio forwards a raw audio chunk to the runtime over the open stream.
func (g *grpcDuplexSink) SendAudio(data []byte, seq uint32, isLast bool) error {
	return g.stream.Send(&runtimev1.ClientMessage{
		SessionId: g.sessionID,
		AudioInput: &runtimev1.AudioInputChunk{
			Data:     data,
			Sequence: seq,
			IsLast:   isLast,
		},
	})
}

// Close cancels the stream context and signals the runtime that no more client
// messages will be sent.
func (g *grpcDuplexSink) Close() error {
	if g.cancel != nil {
		g.cancel()
	}
	if g.stream != nil {
		return g.stream.CloseSend()
	}
	return nil
}

// compile-time assertion: grpcDuplexSink satisfies the facade.DuplexSink interface.
var _ facade.DuplexSink = (*grpcDuplexSink)(nil)
