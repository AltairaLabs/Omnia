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
	"github.com/AltairaLabs/PromptKit/runtime/types"
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

// duplexMediaParams holds the per-session audio parameters negotiated from DuplexStart.
type duplexMediaParams struct {
	codec      string
	mimeType   string
	sampleRate int
	channels   int
}

// defaultAudioCodec is the fallback codec when DuplexStart omits one.
const defaultAudioCodec = "pcm"

// buildStreamingConfig resolves the effective duplex audio format and converts
// it into a providers.StreamingInputConfig. The client's DuplexStart proposal
// (with pcm / 16 kHz / mono defaults for omitted fields) is the baseline; any
// non-zero field in required (spec.duplex.audio) overrides it — the bounded
// counter-offer. required may be nil, meaning "accept the client's proposal".
func buildStreamingConfig(ds *runtimev1.DuplexStart, required *DuplexAudioParams) (providers.StreamingInputConfig, duplexMediaParams) {
	codec := ds.GetCodec()
	if codec == "" {
		codec = defaultAudioCodec
	}
	sampleRate := int(ds.GetSampleRate())
	if sampleRate == 0 {
		sampleRate = 16000
	}
	channels := int(ds.GetChannels())
	if channels == 0 {
		channels = 1
	}

	// Apply the runtime's required format (counter-offer): each non-zero field
	// overrides the client's proposal.
	if required != nil {
		if required.Codec != "" {
			codec = required.Codec
		}
		if required.SampleRate != 0 {
			sampleRate = required.SampleRate
		}
		if required.Channels != 0 {
			channels = required.Channels
		}
	}

	cfg := providers.StreamingInputConfig{
		Config: types.StreamingMediaConfig{
			Type:       types.ContentTypeAudio,
			Encoding:   codec,
			SampleRate: sampleRate,
			Channels:   channels,
		},
		SystemInstruction: ds.GetSystemInstruction(),
	}
	params := duplexMediaParams{
		codec:      codec,
		mimeType:   "audio/" + codec,
		sampleRate: sampleRate,
		channels:   channels,
	}
	return cfg, params
}

// mediaNegotiationFromParams builds the RuntimeHello media counter-offer from
// the resolved per-session audio params. Video fields stay zero (carried, not
// yet enforced).
func mediaNegotiationFromParams(p duplexMediaParams) *runtimev1.MediaNegotiation {
	return &runtimev1.MediaNegotiation{
		Codec:      p.codec,
		SampleRate: int32(p.sampleRate), //nolint:gosec // sample rate is a small positive constant
		Channels:   int32(p.channels),   //nolint:gosec // channel count is a small positive constant
	}
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

	// Override the static WithStreamingConfig added by sdkOptions with the
	// per-session params: the client's DuplexStart proposal reconciled against
	// the runtime's required format (spec.duplex.audio) — the bounded counter-offer.
	ds := start.GetDuplexStart()
	streamCfg, mediaParams := buildStreamingConfig(ds, s.duplexAudio)
	opts = append(opts, sdk.WithStreamingConfig(&streamCfg))

	// RuntimeHello is the runtime's first ServerMessage on the stream: the
	// session's authoritative capabilities plus the media counter-offer the
	// facade relays to the client. Sent synchronously before the forward
	// goroutine starts so it always precedes any audio chunk.
	if helloErr := s.sendRuntimeHello(stream, mediaNegotiationFromParams(mediaParams)); helloErr != nil {
		return helloErr
	}

	// If a per-session system instruction was supplied, inject it as a template
	// variable so packs using {{system_instruction}} compile it into the system
	// prompt.  The SDK pipeline always populates StreamingInputConfig.SystemInstruction
	// from the compiled TurnState.SystemPrompt, so the provider receives it correctly.
	if si := ds.GetSystemInstruction(); si != "" {
		opts = append(opts, sdk.WithVariables(map[string]string{"system_instruction": si}))
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
	pumpErr := s.pumpDuplexInput(ctx, stream, conv, mediaParams)

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
// an is_last chunk, EOF, or a stream receive error. The media params are the
// negotiated codec / sample rate / channels from the session's DuplexStart.
func (s *Server) pumpDuplexInput(ctx context.Context, stream runtimev1.RuntimeService_ConverseServer, conv *sdk.Conversation, params duplexMediaParams) error {
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
					MIMEType:   params.mimeType,
					SampleRate: params.sampleRate,
					Channels:   params.channels,
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
// and writes it to the stream. It handles audio (MediaChunk), text (Chunk) output,
// and interruption (barge-in) signals (Interruption).
//
// When chunk.Interrupted is true an Interruption message is forwarded to the client,
// which signals it to discard any buffered audio and stop playback immediately.
//
// MediaChunk.IsLast is intentionally always false here: duplex audio output is a
// continuous stream and no consumer (grpcDuplexSink.relayOut) relies on it for
// session teardown — the facade closes the session via the inbound is_last frame
// / context cancellation path instead.
func (s *Server) forwardDuplexChunk(stream runtimev1.RuntimeService_ConverseServer, chunk providers.StreamChunk) error {
	if chunk.Interrupted {
		return stream.Send(&runtimev1.ServerMessage{
			Message: &runtimev1.ServerMessage_Interruption{Interruption: &runtimev1.Interruption{}},
		})
	}
	if chunk.MediaData != nil && len(chunk.MediaData.Data) > 0 {
		return stream.Send(&runtimev1.ServerMessage{
			Message: &runtimev1.ServerMessage_MediaChunk{
				MediaChunk: &runtimev1.MediaChunk{
					Data:     chunk.MediaData.Data,
					MimeType: chunk.MediaData.MIMEType,
					Sequence: int32(chunk.MediaData.FrameNum), //nolint:gosec // FrameNum is bounded by audio frame count
					// IsLast is intentionally false — see function comment above.
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
