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
	"errors"
	"io"

	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/sdk"
	"github.com/altairalabs/omnia/pkg/logctx"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// eosChunk is a StreamChunk with end_of_stream metadata, signalling the
// SDK's DuplexProviderStage to call EndInput() on the provider session.
// This makes the duplexmock (and real providers) close their response channel
// promptly so the pipeline drains and streamOutput closes without waiting for
// the 30-second finalResponseTimeout.
var eosChunk = &providers.StreamChunk{
	Metadata: map[string]any{"end_of_stream": true},
}

// handleDuplexSession bridges a Converse stream into a PromptKit duplex conversation.
// It is called when the first ClientMessage carries a DuplexStart. The function:
//  1. Opens an sdk.OpenDuplex conversation using the same options as a normal turn.
//  2. Starts a goroutine that forwards provider response chunks → gRPC ServerMessages.
//  3. Pumps inbound AudioInputChunk messages → conv.SendChunk until is_last or EOF.
//  4. Sends an end_of_stream signal to the pipeline (triggering EndInput on the
//     provider session, which closes the session's response channel).
//  5. Waits for the response goroutine to drain the now-closing respCh, then closes conv.
//
// Ordering avoids the Close/respDone deadlock: the pipeline closes streamOutput
// (via EndInput → session.Close) BEFORE we call conv.Close(), so the response goroutine
// can drain naturally. conv.Close() / Drain then has nothing left to wait for.
func (s *Server) handleDuplexSession(ctx context.Context, stream runtimev1.RuntimeService_ConverseServer, start *runtimev1.ClientMessage) error {
	log := logctx.LoggerWithContext(s.log, ctx)
	sessionID := start.GetSessionId()

	opts, err := s.buildConversationOptions(ctx, sessionID)
	if err != nil {
		return err
	}

	conv, err := sdk.OpenDuplex(s.packPath, s.promptName, opts...)
	if err != nil {
		return err
	}
	defer func() { _ = conv.Close() }()

	respCh, err := conv.Response()
	if err != nil {
		return err
	}

	// Drain response chunks → gRPC messages in a background goroutine.
	// respDone is closed when the goroutine exits (when respCh closes or on error).
	respDone := make(chan struct{})
	go func() {
		defer close(respDone)
		for chunk := range respCh {
			if fwdErr := s.forwardDuplexChunk(stream, chunk); fwdErr != nil {
				log.Error(fwdErr, "forward duplex chunk failed", "sessionID", sessionID)
				return
			}
		}
	}()

	// Pump inbound audio until is_last or stream EOF/error.
	pumpErr := s.pumpDuplexInput(ctx, stream, conv)

	// Send end_of_stream to the pipeline. The DuplexProviderStage converts this
	// to an EndOfStream element and calls EndInput() on the provider session, which
	// closes the session's response channel. The pipeline then drains, closes
	// streamOutput, and respCh closes — unblocking the response goroutine above.
	// If the pipeline never started (no chunks were ever sent), SendChunk will
	// start it now; the empty EOS element causes it to exit quickly.
	if eosErr := conv.SendChunk(ctx, eosChunk); eosErr != nil {
		log.Error(eosErr, "duplex EOS send failed", "sessionID", sessionID)
	}

	// Wait for the response goroutine to drain all echoed chunks and exit.
	<-respDone

	return pumpErr
}

// pumpDuplexInput reads AudioInputChunk messages from the stream and forwards
// them to the conversation as provider.StreamChunk. Returns when it encounters
// an is_last chunk, EOF, or a stream receive error.
func (s *Server) pumpDuplexInput(ctx context.Context, stream runtimev1.RuntimeService_ConverseServer, conv *sdk.Conversation) error {
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		ai := msg.GetAudioInput()
		if ai == nil {
			// Non-audio message; skip silently.
			continue
		}

		if len(ai.GetData()) > 0 {
			chunk := &providers.StreamChunk{
				MediaData: &providers.StreamMediaData{
					Data:       ai.GetData(),
					MIMEType:   "audio/pcm",
					SampleRate: 16000,
					Channels:   1,
				},
			}
			if sendErr := conv.SendChunk(ctx, chunk); sendErr != nil {
				return sendErr
			}
		}

		if ai.GetIsLast() {
			return nil
		}
	}
}

// forwardDuplexChunk converts a providers.StreamChunk into a gRPC ServerMessage
// and writes it to the stream. It handles both audio (MediaChunk) and text (Chunk) output.
func (s *Server) forwardDuplexChunk(stream runtimev1.RuntimeService_ConverseServer, chunk providers.StreamChunk) error {
	if chunk.MediaData != nil && len(chunk.MediaData.Data) > 0 {
		return stream.Send(&runtimev1.ServerMessage{
			Message: &runtimev1.ServerMessage_MediaChunk{
				MediaChunk: &runtimev1.MediaChunk{
					Data:     chunk.MediaData.Data,
					MimeType: chunk.MediaData.MIMEType,
					Sequence: int32(chunk.MediaData.FrameNum), //nolint:gosec // FrameNum is bounded by audio frame count
				},
			},
		})
	}
	if chunk.Delta != "" {
		return stream.Send(&runtimev1.ServerMessage{
			Message: &runtimev1.ServerMessage_Chunk{
				Chunk: &runtimev1.Chunk{Content: chunk.Delta},
			},
		})
	}
	return nil
}
