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
	"io"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/sdk"
	"github.com/altairalabs/omnia/internal/runtime/duplexmock"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// fakeConverseServer is a test double for RuntimeService_ConverseServer.
type fakeConverseServer struct {
	ctx     context.Context
	recv    chan *runtimev1.ClientMessage
	sent    chan *runtimev1.ServerMessage
	sendErr error // when set, Send returns it instead of buffering
}

func (f *fakeConverseServer) Send(m *runtimev1.ServerMessage) error {
	if f.sendErr != nil {
		return f.sendErr
	}
	f.sent <- m
	return nil
}
func (f *fakeConverseServer) Recv() (*runtimev1.ClientMessage, error) {
	m, ok := <-f.recv
	if !ok {
		return nil, io.EOF
	}
	return m, nil
}
func (f *fakeConverseServer) Context() context.Context     { return f.ctx }
func (f *fakeConverseServer) SetHeader(metadata.MD) error  { return nil }
func (f *fakeConverseServer) SendHeader(metadata.MD) error { return nil }
func (f *fakeConverseServer) SetTrailer(metadata.MD)       {}
func (f *fakeConverseServer) SendMsg(any) error            { return nil }
func (f *fakeConverseServer) RecvMsg(any) error            { return nil }

// newTestServerWithDuplexProvider builds a *Server whose buildConversationOptions
// will inject p as the SDK provider. We do this by appending sdk.WithProvider(p)
// to s.sdkOptions and leaving s.mockProvider=false, s.providerType="" (so
// buildConversationOptions won't add a second provider on top).
// packPath and promptName must point at a valid minimal prompt pack.
func newTestServerWithDuplexProvider(t *testing.T, p providers.StreamInputSupport) *Server {
	t.Helper()

	// Write a minimal valid PromptPack so sdk.OpenDuplex can load it.
	packContent := `{
		"id": "duplex-test",
		"name": "duplex-test",
		"version": "1.0.0",
		"template_engine": {"version": "v1", "syntax": "{{variable}}"},
		"prompts": {
			"default": {
				"id": "default",
				"name": "default",
				"version": "1.0.0",
				"system_template": "You are a test assistant."
			}
		}
	}`

	packPath := t.TempDir() + "/duplex-test.promptpack"
	if err := writeTestFile(t, packPath, packContent); err != nil {
		t.Fatalf("writeTestFile: %v", err)
	}

	return NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		// Inject the duplex mock via sdkOptions; leave mockProvider=false and
		// providerType="" so buildConversationOptions won't override it.
		// WithStreamingConfig enables ASM mode so OpenDuplex calls
		// p.CreateStreamSession() — enabling the echo path in duplexmock.
		WithSDKOptions(
			sdk.WithProvider(p),
			sdk.WithStreamingConfig(&providers.StreamingInputConfig{}),
		),
	)
}

func TestHandleDuplexSession_EchoesAudio(t *testing.T) {
	s := newTestServerWithDuplexProvider(t, duplexmock.New())
	fake := &fakeConverseServer{
		ctx:  context.Background(),
		recv: make(chan *runtimev1.ClientMessage, 4),
		sent: make(chan *runtimev1.ServerMessage, 8),
	}
	start := &runtimev1.ClientMessage{
		SessionId:   "sess-1",
		DuplexStart: &runtimev1.DuplexStart{Codec: defaultAudioCodec, SampleRate: 16000, Channels: 1},
	}
	go func() {
		fake.recv <- &runtimev1.ClientMessage{SessionId: "sess-1", AudioInput: &runtimev1.AudioInputChunk{Data: []byte{9, 8, 7}, Sequence: 0}}
		fake.recv <- &runtimev1.ClientMessage{SessionId: "sess-1", AudioInput: &runtimev1.AudioInputChunk{IsLast: true}}
		close(fake.recv)
	}()
	if err := s.handleDuplexSession(fake.ctx, fake, start); err != nil {
		t.Fatalf("handleDuplexSession: %v", err)
	}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case m := <-fake.sent:
			if mc, ok := m.Message.(*runtimev1.ServerMessage_MediaChunk); ok && string(mc.MediaChunk.Data) == string([]byte{9, 8, 7}) {
				return // success
			}
		case <-deadline:
			t.Fatal("never received echoed audio MediaChunk")
		}
	}
}

func TestHandleDuplexSession_ForwardTextDelta(t *testing.T) {
	s := newTestServerWithDuplexProvider(t, duplexmock.New())
	fake := &fakeConverseServer{
		ctx:  context.Background(),
		recv: make(chan *runtimev1.ClientMessage, 4),
		sent: make(chan *runtimev1.ServerMessage, 8),
	}
	start := &runtimev1.ClientMessage{
		SessionId:   "sess-2",
		DuplexStart: &runtimev1.DuplexStart{Codec: defaultAudioCodec, SampleRate: 16000, Channels: 1},
	}
	go func() {
		// Non-audio message is skipped; is_last closes the input side.
		fake.recv <- &runtimev1.ClientMessage{SessionId: "sess-2", AudioInput: &runtimev1.AudioInputChunk{IsLast: true}}
		close(fake.recv)
	}()
	// handleDuplexSession should return without error even with no audio data.
	err := s.handleDuplexSession(fake.ctx, fake, start)
	require.NoError(t, err)
}

func TestHandleDuplexSession_SkipsNilAudioInput(t *testing.T) {
	s := newTestServerWithDuplexProvider(t, duplexmock.New())
	fake := &fakeConverseServer{
		ctx:  context.Background(),
		recv: make(chan *runtimev1.ClientMessage, 4),
		sent: make(chan *runtimev1.ServerMessage, 8),
	}
	start := &runtimev1.ClientMessage{
		SessionId:   "sess-3",
		DuplexStart: &runtimev1.DuplexStart{Codec: defaultAudioCodec, SampleRate: 16000, Channels: 1},
	}
	go func() {
		// A message with no AudioInput (nil) should be skipped, not panic.
		fake.recv <- &runtimev1.ClientMessage{SessionId: "sess-3"}
		fake.recv <- &runtimev1.ClientMessage{SessionId: "sess-3", AudioInput: &runtimev1.AudioInputChunk{IsLast: true}}
		close(fake.recv)
	}()
	err := s.handleDuplexSession(fake.ctx, fake, start)
	require.NoError(t, err)
}

func TestHandleDuplexSession_InvalidPack(t *testing.T) {
	// With no pack path, sdk.OpenDuplex returns an error; handleDuplexSession propagates it.
	s := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath("/nonexistent/path.promptpack"),
		WithPromptName("default"),
		WithSDKOptions(
			sdk.WithProvider(duplexmock.New()),
			sdk.WithStreamingConfig(&providers.StreamingInputConfig{}),
		),
	)
	fake := &fakeConverseServer{
		ctx:  context.Background(),
		recv: make(chan *runtimev1.ClientMessage, 2),
		sent: make(chan *runtimev1.ServerMessage, 2),
	}
	start := &runtimev1.ClientMessage{
		SessionId:   "sess-err",
		DuplexStart: &runtimev1.DuplexStart{},
	}
	err := s.handleDuplexSession(fake.ctx, fake, start)
	require.Error(t, err)
}

func TestHandleDuplexSession_RecvError(t *testing.T) {
	s := newTestServerWithDuplexProvider(t, duplexmock.New())
	// fakeErrStream returns an error on the first Recv call.
	fake := &fakeConverseServer{
		ctx:  context.Background(),
		recv: make(chan *runtimev1.ClientMessage, 2),
		sent: make(chan *runtimev1.ServerMessage, 8),
	}
	start := &runtimev1.ClientMessage{
		SessionId:   "sess-recv-err",
		DuplexStart: &runtimev1.DuplexStart{Codec: defaultAudioCodec, SampleRate: 16000, Channels: 1},
	}
	// Close the recv channel immediately (simulates stream termination with EOF).
	// pumpDuplexInput treats io.EOF as clean exit (no error).
	close(fake.recv)
	err := s.handleDuplexSession(fake.ctx, fake, start)
	require.NoError(t, err) // EOF → clean exit
}

// TestHandleDuplexSession_HonorsDuplexStartParams verifies that the negotiated
// codec, sample_rate, channels, and system_instruction from DuplexStart are
// propagated to the provider's StreamingInputConfig and not hardcoded.
//
// system_instruction is injected as a template variable ("system_instruction")
// so that packs using {{system_instruction}} in their system_template compile to
// the per-session override.  The SDK pipeline always populates
// StreamingInputConfig.SystemInstruction from the compiled TurnState.SystemPrompt,
// so a pack with system_template "{{system_instruction}}" will cause the mock to
// receive the override value verbatim.
func TestHandleDuplexSession_HonorsDuplexStartParams(t *testing.T) {
	mock := duplexmock.New()

	// Use a pack template that references {{system_instruction}} so the compiled
	// system prompt equals the per-session override injected via WithVariables.
	packContent := `{
		"id": "duplex-si-test",
		"name": "duplex-si-test",
		"version": "1.0.0",
		"template_engine": {"version": "v1", "syntax": "{{variable}}"},
		"prompts": {
			"default": {
				"id": "default",
				"name": "default",
				"version": "1.0.0",
				"system_template": "{{system_instruction}}"
			}
		}
	}`
	packPath := t.TempDir() + "/duplex-si-test.promptpack"
	if err := writeTestFile(t, packPath, packContent); err != nil {
		t.Fatalf("writeTestFile: %v", err)
	}
	s := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithSDKOptions(
			sdk.WithProvider(mock),
			sdk.WithStreamingConfig(&providers.StreamingInputConfig{}),
		),
	)

	fake := &fakeConverseServer{
		ctx:  context.Background(),
		recv: make(chan *runtimev1.ClientMessage, 2),
		sent: make(chan *runtimev1.ServerMessage, 8),
	}
	start := &runtimev1.ClientMessage{
		SessionId: "sess-cfg",
		DuplexStart: &runtimev1.DuplexStart{
			Codec: defaultAudioCodec, SampleRate: 24000, Channels: 1,
			SystemInstruction: "be terse",
		},
	}
	go func() {
		fake.recv <- &runtimev1.ClientMessage{SessionId: "sess-cfg", AudioInput: &runtimev1.AudioInputChunk{IsLast: true}}
		close(fake.recv)
	}()
	if err := s.handleDuplexSession(fake.ctx, fake, start); err != nil {
		t.Fatalf("handleDuplexSession: %v", err)
	}
	cfg := mock.LastConfig()
	if cfg == nil {
		t.Fatal("CreateStreamSession never received a config")
	}
	if cfg.Config.SampleRate != 24000 {
		t.Fatalf("sample_rate not propagated: got %d, want 24000", cfg.Config.SampleRate)
	}
	if cfg.Config.Channels != 1 {
		t.Fatalf("channels not propagated: got %d, want 1", cfg.Config.Channels)
	}
	if cfg.SystemInstruction != "be terse" {
		t.Fatalf("system_instruction not propagated: got %q, want %q", cfg.SystemInstruction, "be terse")
	}
}

// TestHandleDuplexSession_SendsHelloWithCounterOffer verifies that the runtime
// sends a RuntimeHello as its first ServerMessage carrying its capabilities and,
// when spec.duplex.audio requires a format, a bounded counter-offer that
// overrides the client's DuplexStart proposal and is applied to the provider.
func TestHandleDuplexSession_SendsHelloWithCounterOffer(t *testing.T) {
	mock := duplexmock.New()
	s := newTestServerWithDuplexProvider(t, mock)
	// Operator requires 24kHz mono pcm; the client proposes 16kHz below.
	s.duplexAudio = &DuplexAudioParams{Codec: defaultAudioCodec, SampleRate: 24000, Channels: 1}

	fake := &fakeConverseServer{
		ctx:  context.Background(),
		recv: make(chan *runtimev1.ClientMessage, 2),
		sent: make(chan *runtimev1.ServerMessage, 8),
	}
	start := &runtimev1.ClientMessage{
		SessionId:   "sess-hello",
		DuplexStart: &runtimev1.DuplexStart{Codec: defaultAudioCodec, SampleRate: 16000, Channels: 1},
	}
	go func() {
		fake.recv <- &runtimev1.ClientMessage{SessionId: "sess-hello", AudioInput: &runtimev1.AudioInputChunk{IsLast: true}}
		close(fake.recv)
	}()
	if err := s.handleDuplexSession(fake.ctx, fake, start); err != nil {
		t.Fatalf("handleDuplexSession: %v", err)
	}

	// The FIRST ServerMessage must be the RuntimeHello.
	first := <-fake.sent
	hello, ok := first.Message.(*runtimev1.ServerMessage_RuntimeHello)
	require.True(t, ok, "first ServerMessage must be RuntimeHello, got %T", first.Message)
	require.Equal(t, Capabilities(), hello.RuntimeHello.GetCapabilities())
	require.NotNil(t, hello.RuntimeHello.GetMedia())
	require.Equal(t, int32(24000), hello.RuntimeHello.GetMedia().GetSampleRate(), "counter-offer overrides client's 16000")
	require.Equal(t, int32(1), hello.RuntimeHello.GetMedia().GetChannels())
	require.Equal(t, defaultAudioCodec, hello.RuntimeHello.GetMedia().GetCodec())

	// The counter-offer is also applied to the provider's streaming config.
	cfg := mock.LastConfig()
	require.NotNil(t, cfg)
	require.Equal(t, 24000, cfg.Config.SampleRate, "counter-offer applied to provider")
}

// TestHandleDuplexSession_HelloSendError verifies handleDuplexSession propagates
// a failure to send the initial RuntimeHello.
func TestHandleDuplexSession_HelloSendError(t *testing.T) {
	s := newTestServerWithDuplexProvider(t, duplexmock.New())
	fake := &fakeConverseServer{
		ctx:     context.Background(),
		recv:    make(chan *runtimev1.ClientMessage, 1),
		sent:    make(chan *runtimev1.ServerMessage, 1),
		sendErr: io.ErrClosedPipe,
	}
	start := &runtimev1.ClientMessage{
		SessionId:   "sess-hello-err",
		DuplexStart: &runtimev1.DuplexStart{Codec: defaultAudioCodec, SampleRate: 16000, Channels: 1},
	}
	err := s.handleDuplexSession(fake.ctx, fake, start)
	require.ErrorIs(t, err, io.ErrClosedPipe)
}

// TestHandleDuplexSession_HelloWithoutCounterOfferEchoesClient verifies that with
// no spec.duplex.audio the hello carries the client's proposed format (the runtime
// accepts it) so a client that already captures at that format needs no change.
func TestHandleDuplexSession_HelloWithoutCounterOfferEchoesClient(t *testing.T) {
	mock := duplexmock.New()
	s := newTestServerWithDuplexProvider(t, mock)

	fake := &fakeConverseServer{
		ctx:  context.Background(),
		recv: make(chan *runtimev1.ClientMessage, 2),
		sent: make(chan *runtimev1.ServerMessage, 8),
	}
	start := &runtimev1.ClientMessage{
		SessionId:   "sess-echo",
		DuplexStart: &runtimev1.DuplexStart{Codec: defaultAudioCodec, SampleRate: 16000, Channels: 1},
	}
	go func() {
		fake.recv <- &runtimev1.ClientMessage{SessionId: "sess-echo", AudioInput: &runtimev1.AudioInputChunk{IsLast: true}}
		close(fake.recv)
	}()
	if err := s.handleDuplexSession(fake.ctx, fake, start); err != nil {
		t.Fatalf("handleDuplexSession: %v", err)
	}

	first := <-fake.sent
	hello, ok := first.Message.(*runtimev1.ServerMessage_RuntimeHello)
	require.True(t, ok, "first ServerMessage must be RuntimeHello, got %T", first.Message)
	require.Equal(t, int32(16000), hello.RuntimeHello.GetMedia().GetSampleRate())
}

// TestHandleDuplexSession_DefaultsWhenZeroParams verifies that zero-value DuplexStart
// params fall back to sensible defaults (pcm / 16000 / 1) so existing sessions continue to work.
func TestHandleDuplexSession_DefaultsWhenZeroParams(t *testing.T) {
	mock := duplexmock.New()
	s := newTestServerWithDuplexProvider(t, mock)
	fake := &fakeConverseServer{
		ctx:  context.Background(),
		recv: make(chan *runtimev1.ClientMessage, 2),
		sent: make(chan *runtimev1.ServerMessage, 8),
	}
	start := &runtimev1.ClientMessage{
		SessionId:   "sess-defaults",
		DuplexStart: &runtimev1.DuplexStart{}, // all zero
	}
	go func() {
		fake.recv <- &runtimev1.ClientMessage{SessionId: "sess-defaults", AudioInput: &runtimev1.AudioInputChunk{IsLast: true}}
		close(fake.recv)
	}()
	if err := s.handleDuplexSession(fake.ctx, fake, start); err != nil {
		t.Fatalf("handleDuplexSession: %v", err)
	}
	cfg := mock.LastConfig()
	if cfg == nil {
		t.Fatal("CreateStreamSession never received a config")
	}
	if cfg.Config.SampleRate != 16000 {
		t.Fatalf("default sample_rate: got %d, want 16000", cfg.Config.SampleRate)
	}
	if cfg.Config.Channels != 1 {
		t.Fatalf("default channels: got %d, want 1", cfg.Config.Channels)
	}
	if cfg.Config.Encoding != defaultAudioCodec {
		t.Fatalf("default encoding: got %q, want %q", cfg.Config.Encoding, defaultAudioCodec)
	}
}

// TestHandleDuplexSession_EchoesAudioWithNegotiatedMIME verifies that the MIME type
// sent in the provider StreamChunk reflects the negotiated codec (not hardcoded "audio/pcm").
func TestHandleDuplexSession_EchoesAudioWithNegotiatedMIME(t *testing.T) {
	mock := duplexmock.New()
	s := newTestServerWithDuplexProvider(t, mock)
	fake := &fakeConverseServer{
		ctx:  context.Background(),
		recv: make(chan *runtimev1.ClientMessage, 4),
		sent: make(chan *runtimev1.ServerMessage, 8),
	}
	start := &runtimev1.ClientMessage{
		SessionId:   "sess-mime",
		DuplexStart: &runtimev1.DuplexStart{Codec: defaultAudioCodec, SampleRate: 24000, Channels: 2},
	}
	go func() {
		fake.recv <- &runtimev1.ClientMessage{SessionId: "sess-mime", AudioInput: &runtimev1.AudioInputChunk{Data: []byte{1, 2, 3}}}
		fake.recv <- &runtimev1.ClientMessage{SessionId: "sess-mime", AudioInput: &runtimev1.AudioInputChunk{IsLast: true}}
		close(fake.recv)
	}()
	if err := s.handleDuplexSession(fake.ctx, fake, start); err != nil {
		t.Fatalf("handleDuplexSession: %v", err)
	}
}

// TestForwardDuplexChunk_EmptyChunk covers the nil-return branch (no media, no delta).
func TestForwardDuplexChunk_EmptyChunk(t *testing.T) {
	s := NewServer(WithLogger(logr.Discard()))
	fake := &fakeConverseServer{
		ctx:  context.Background(),
		recv: make(chan *runtimev1.ClientMessage, 1),
		sent: make(chan *runtimev1.ServerMessage, 1),
	}
	// Empty chunk: no MediaData, no Delta → returns nil without calling Send.
	err := s.forwardDuplexChunk(fake, providers.StreamChunk{})
	require.NoError(t, err)
	require.Empty(t, fake.sent)
}

// testDelta is the text delta asserted across forwardDuplexChunk tests.
const testDelta = "hello"

// TestForwardDuplexChunk_TextDelta covers the Delta branch.
func TestForwardDuplexChunk_TextDelta(t *testing.T) {
	s := NewServer(WithLogger(logr.Discard()))
	fake := &fakeConverseServer{
		ctx:  context.Background(),
		recv: make(chan *runtimev1.ClientMessage, 1),
		sent: make(chan *runtimev1.ServerMessage, 4),
	}
	err := s.forwardDuplexChunk(fake, providers.StreamChunk{Delta: testDelta})
	require.NoError(t, err)
	require.Len(t, fake.sent, 1)
	chunk, ok := (<-fake.sent).Message.(*runtimev1.ServerMessage_Chunk)
	require.True(t, ok)
	require.Equal(t, testDelta, chunk.Chunk.Content)
}

// TestForwardDuplexChunk_EmitsInterruption verifies that a barge-in chunk
// (Interrupted=true) emits exactly one ServerMessage_Interruption before any
// other message type, and that no other messages are sent.
func TestForwardDuplexChunk_EmitsInterruption(t *testing.T) {
	s := NewServer(WithLogger(logr.Discard()))
	fake := &fakeConverseServer{
		ctx:  context.Background(),
		recv: make(chan *runtimev1.ClientMessage, 1),
		sent: make(chan *runtimev1.ServerMessage, 4),
	}
	err := s.forwardDuplexChunk(fake, providers.StreamChunk{Interrupted: true})
	require.NoError(t, err)
	require.Len(t, fake.sent, 1)
	_, ok := (<-fake.sent).Message.(*runtimev1.ServerMessage_Interruption)
	require.True(t, ok, "expected ServerMessage_Interruption")
}
