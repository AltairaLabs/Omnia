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
	"time"

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

// defaultChunkDurationMs is the audio-batching chunk duration used when
// spec.duplex.audio does not set chunkDurationMs. 100ms matches PromptKit's
// Gemini Live default; it is a latency/throughput knob (smaller = lower
// interruption latency, more provider messages).
const defaultChunkDurationMs = 100

// Response-modality request. A duplex session is a spoken voice call, so the
// model must reply with AUDIO — native-audio Live models (e.g. Gemini 3 Flash
// Live) reject a TEXT-only modality.
//
// NOTE (provider-specific bridge): "response_modalities" is Gemini's wire key.
// The intent ("audio out") is provider-agnostic, but PromptKit exposes no
// first-class response-modality field on StreamingInputConfig — only its
// provider-specific Metadata escape hatch — so this is the only lever to
// override Gemini's TEXT default. It is safe cross-provider (adapters ignore
// unknown metadata keys). TODO: replace with an agnostic StreamingInputConfig
// field once PromptKit adds one (issue to file); then the per-provider spelling
// moves into PromptKit where it belongs.
const (
	metaKeyResponseModalities = "response_modalities"
	modalityAudio             = "AUDIO"

	// metaKeyAssistantTurnComplete marks the StreamChunk that signals the end of
	// an assistant turn on the duplex stream (PromptKit DuplexProviderStage).
	metaKeyAssistantTurnComplete = "assistant_turn_complete"
	// metaKeyInputTranscription carries the user's transcribed speech on the
	// duplex stream (PromptKit DuplexProviderStage); the chunk has an empty Delta.
	metaKeyInputTranscription = "input_transcription"

	// chunkRoleUser tags a duplex text chunk as the caller's transcribed speech.
	chunkRoleUser = "user"
)

// pcmBytesPerSample returns the bytes per sample for a PCM codec. Realtime
// providers (Gemini Live, OpenAI Realtime) use 16-bit PCM (2 bytes); codecs
// carrying an explicit bit-depth are honored so a future non-16-bit format
// "just works" without touching the chunk-size math.
func pcmBytesPerSample(codec string) int {
	switch codec {
	case "pcm24", "pcm-24":
		return 3
	case "pcm8", "pcm-8":
		return 1
	default:
		return 2 // pcm / pcm16 and unknown → 16-bit PCM
	}
}

// audioChunkSizeBytes computes the provider audio-batching chunk size in bytes:
// chunkDurationMs of PCM audio at the given sample rate and channel count.
// PromptKit's StreamingMediaConfig.Validate requires a positive, sample-aligned
// value, so the result is always > 0 and a multiple of the sample size.
func audioChunkSizeBytes(sampleRate, channels, chunkDurationMs, bytesPerSample int) int {
	if chunkDurationMs <= 0 {
		chunkDurationMs = defaultChunkDurationMs
	}
	framesPerChunk := sampleRate * chunkDurationMs / 1000
	if framesPerChunk <= 0 {
		framesPerChunk = 1
	}
	return framesPerChunk * bytesPerSample * channels
}

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

	// Resolve the audio-batching chunk size. PromptKit's StreamingMediaConfig
	// requires a positive, sample-aligned ChunkSize for audio; without it the
	// provider session fails to open ("chunk size must be positive"). The
	// duration is a per-agent knob (spec.duplex.audio.chunkDurationMs), defaulting
	// to 100ms; the byte width comes from the codec, not a hardcoded 16-bit.
	chunkDurationMs := defaultChunkDurationMs
	if required != nil && required.ChunkDurationMs != 0 {
		chunkDurationMs = required.ChunkDurationMs
	}
	bytesPerSample := pcmBytesPerSample(codec)
	chunkSize := audioChunkSizeBytes(sampleRate, channels, chunkDurationMs, bytesPerSample)

	cfg := providers.StreamingInputConfig{
		Config: types.StreamingMediaConfig{
			Type:       types.ContentTypeAudio,
			Encoding:   codec,
			SampleRate: sampleRate,
			Channels:   channels,
			ChunkSize:  chunkSize,
			// BitDepth is required by realtime providers (Gemini Live rejects a
			// zero/mismatched depth). Derived from the codec so it stays in lockstep
			// with the chunk-size math: 16-bit PCM → 16, etc.
			BitDepth: bytesPerSample * 8,
		},
		SystemInstruction: ds.GetSystemInstruction(),
		// Request spoken (AUDIO) responses — this is a voice call.
		Metadata: map[string]interface{}{
			metaKeyResponseModalities: []string{modalityAudio},
		},
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

	// Rate-reconciliation telemetry: the effective rate feeds BOTH the provider
	// (what OpenAI/Gemini is told the PCM is) and the RuntimeHello counter-offer
	// (what the client is asked to capture at). Any divergence between the client's
	// proposal, the CRD-required override, and the effective rate lands here.
	requiredRate := 0
	if s.duplexAudio != nil {
		requiredRate = s.duplexAudio.SampleRate
	}
	log.Info("duplex rate reconciliation",
		"duplexStartRate", ds.GetSampleRate(),
		"duplexStartCodec", ds.GetCodec(),
		"duplexStartChannels", ds.GetChannels(),
		"requiredRate", requiredRate,
		"effectiveRate", mediaParams.sampleRate,
		"effectiveCodec", mediaParams.codec,
		"chunkSize", streamCfg.Config.ChunkSize,
		"bitDepth", streamCfg.Config.BitDepth)

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
	log := logctx.LoggerWithContext(s.log, ctx)

	// Input-rate audit: measure the TRUE data rate of the inbound stream
	// (bytes ÷ wall-clock, independent of any declared label) and compare it to
	// params.sampleRate — the rate we tell the provider the PCM is. A live mic
	// streams at 1×, so bytes/frameBytes/elapsed == the client's real capture
	// rate. If measuredTrueRate diverges from effectiveRate, the client is
	// sending PCM at one rate but it is being played back to the provider as
	// another (e.g. 16 kHz data labelled 24 kHz → pitch-shifted → wrong language).
	bytesPerSample := pcmBytesPerSample(params.codec)
	var totalBytes, chunkCount int
	var firstChunkAt, lastChunkAt time.Time
	defer func() {
		if totalBytes == 0 {
			return
		}
		// elapsed spans first→last audio chunk (NOT function return), so trailing
		// idle before is_last/EOF does not dilute the measured rate.
		elapsed := lastChunkAt.Sub(firstChunkAt).Seconds()
		frameBytes := bytesPerSample * params.channels
		measuredTrueRate := 0
		if elapsed > 0 && frameBytes > 0 {
			measuredTrueRate = int(float64(totalBytes) / float64(frameBytes) / elapsed)
		}
		log.Info("duplex input rate audit",
			"effectiveRate", params.sampleRate,
			"measuredTrueRate", measuredTrueRate,
			"totalBytes", totalBytes,
			"chunkCount", chunkCount,
			"activeElapsedSec", elapsed,
			"channels", params.channels,
			"bytesPerSample", bytesPerSample)
	}()

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
			now := time.Now()
			if firstChunkAt.IsZero() {
				firstChunkAt = now
			}
			lastChunkAt = now
			totalBytes += len(ai.GetData())
			chunkCount++
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
	// User transcript: the caller's transcribed speech arrives with an empty
	// Delta and the text in Metadata[input_transcription]. Forward it as a
	// user-role chunk so the client renders it as a user message (the assistant
	// path uses the default empty role above).
	if v, ok := chunk.Metadata[metaKeyInputTranscription]; ok {
		if transcript, isStr := v.(string); isStr && transcript != "" {
			return stream.Send(&runtimev1.ServerMessage{
				Message: &runtimev1.ServerMessage_Chunk{
					Chunk: &runtimev1.Chunk{Content: transcript, Role: chunkRoleUser},
				},
			})
		}
	}
	// End of an assistant turn: seal the streamed transcript so the client
	// finalizes the current assistant message and the next turn starts a fresh
	// one. Done carries no content — the client keeps the text it streamed.
	if _, ok := chunk.Metadata[metaKeyAssistantTurnComplete]; ok {
		return stream.Send(&runtimev1.ServerMessage{
			Message: &runtimev1.ServerMessage_Done{Done: &runtimev1.Done{}},
		})
	}
	return nil
}
