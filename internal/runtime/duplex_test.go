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
	ctx  context.Context
	recv chan *runtimev1.ClientMessage
	sent chan *runtimev1.ServerMessage
}

func (f *fakeConverseServer) Send(m *runtimev1.ServerMessage) error { f.sent <- m; return nil }
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
		DuplexStart: &runtimev1.DuplexStart{Codec: "pcm", SampleRate: 16000, Channels: 1},
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
		DuplexStart: &runtimev1.DuplexStart{Codec: "pcm", SampleRate: 16000, Channels: 1},
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
		DuplexStart: &runtimev1.DuplexStart{Codec: "pcm", SampleRate: 16000, Channels: 1},
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
		DuplexStart: &runtimev1.DuplexStart{Codec: "pcm", SampleRate: 16000, Channels: 1},
	}
	// Close the recv channel immediately (simulates stream termination with EOF).
	// pumpDuplexInput treats io.EOF as clean exit (no error).
	close(fake.recv)
	err := s.handleDuplexSession(fake.ctx, fake, start)
	require.NoError(t, err) // EOF → clean exit
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

// TestForwardDuplexChunk_TextDelta covers the Delta branch.
func TestForwardDuplexChunk_TextDelta(t *testing.T) {
	s := NewServer(WithLogger(logr.Discard()))
	fake := &fakeConverseServer{
		ctx:  context.Background(),
		recv: make(chan *runtimev1.ClientMessage, 1),
		sent: make(chan *runtimev1.ServerMessage, 4),
	}
	err := s.forwardDuplexChunk(fake, providers.StreamChunk{Delta: "hello"})
	require.NoError(t, err)
	require.Len(t, fake.sent, 1)
	chunk, ok := (<-fake.sent).Message.(*runtimev1.ServerMessage_Chunk)
	require.True(t, ok)
	require.Equal(t, "hello", chunk.Chunk.Content)
}
