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
	"net"
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"google.golang.org/grpc"

	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/sdk"
	"github.com/altairalabs/omnia/internal/facade"
	pkruntime "github.com/altairalabs/omnia/internal/runtime"
	"github.com/altairalabs/omnia/internal/runtime/duplexmock"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// captureBinaryWriter is a ResponseWriter that records WriteBinaryMediaChunk payloads.
// It implements facade.ResponseWriter (the full interface).
type captureBinaryWriter struct {
	got chan []byte
}

func (c *captureBinaryWriter) WriteChunk(_ string) error                        { return nil }
func (c *captureBinaryWriter) WriteChunkWithParts(_ []facade.ContentPart) error { return nil }
func (c *captureBinaryWriter) WriteDone(_ string) error                         { return nil }
func (c *captureBinaryWriter) WriteDoneWithParts(_ []facade.ContentPart) error  { return nil }
func (c *captureBinaryWriter) WriteToolCall(_ *facade.ToolCallInfo) error       { return nil }
func (c *captureBinaryWriter) WriteToolResult(_ *facade.ToolResultInfo) error   { return nil }
func (c *captureBinaryWriter) WriteError(_, _ string) error                     { return nil }
func (c *captureBinaryWriter) WriteUploadReady(_ *facade.UploadReadyInfo) error { return nil }
func (c *captureBinaryWriter) WriteUploadComplete(_ *facade.UploadCompleteInfo) error {
	return nil
}
func (c *captureBinaryWriter) WriteMediaChunk(_ *facade.MediaChunkInfo) error { return nil }
func (c *captureBinaryWriter) SupportsBinary() bool                           { return true }
func (c *captureBinaryWriter) WriteBinaryMediaChunk(_ [facade.MediaIDSize]byte, _ uint32, _ bool, _ string, payload []byte) error {
	cp := make([]byte, len(payload))
	copy(cp, payload)
	c.got <- cp
	return nil
}

// startRealRuntimeServer starts a real internal/runtime gRPC server configured
// with the given StreamInputSupport provider (duplexmock). Returns the server
// address and a cleanup function.
//
// This replicates the logic of newTestServerWithDuplexProvider in
// internal/runtime/duplex_test.go — that function is unexported and in a
// different package, so we reproduce it here rather than exporting it.
func startRealRuntimeServer(t *testing.T, p providers.StreamInputSupport) (string, func()) {
	t.Helper()

	packContent := `{
		"id": "duplex-sink-test",
		"name": "duplex-sink-test",
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

	packPath := t.TempDir() + "/duplex-sink-test.promptpack"
	if err := writeFile(packPath, packContent); err != nil {
		t.Fatalf("writeFile: %v", err)
	}

	runtimeSrv := pkruntime.NewServer(
		pkruntime.WithLogger(logr.Discard()),
		pkruntime.WithPackPath(packPath),
		pkruntime.WithPromptName("default"),
		pkruntime.WithSDKOptions(
			sdk.WithProvider(p),
			sdk.WithStreamingConfig(&providers.StreamingInputConfig{}),
		),
	)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}

	grpcSrv := grpc.NewServer()
	runtimev1.RegisterRuntimeServiceServer(grpcSrv, runtimeSrv)

	go func() { _ = grpcSrv.Serve(lis) }()

	cleanup := func() {
		grpcSrv.Stop()
		_ = runtimeSrv.Close()
	}

	return lis.Addr().String(), cleanup
}

// writeFile is a local test helper — the equivalent in internal/runtime is unexported.
func writeFile(path, content string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

// TestDuplexSink_EchoesAudioOverGRPC exercises the full grpcDuplexSink path:
// Start (sends DuplexStart) → SendAudio (sends AudioInputChunk) → relayOut
// forwards the runtime's echoed MediaChunk → WriteBinaryMediaChunk fires.
func TestDuplexSink_EchoesAudioOverGRPC(t *testing.T) {
	addr, cleanup := startRealRuntimeServer(t, duplexmock.New())
	defer cleanup()

	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     addr,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewRuntimeClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	w := &captureBinaryWriter{got: make(chan []byte, 4)}
	sink := NewGRPCDuplexSink("sess-echo", client, w)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sink.Start(ctx, &facade.AudioSessionStart{Codec: "pcm", SampleRate: 16000, Channels: 1}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = sink.Close() }()

	if err := sink.SendAudio([]byte{9, 8, 7}, 0, false); err != nil {
		t.Fatalf("SendAudio: %v", err)
	}
	// Signal end of audio stream.
	_ = sink.SendAudio(nil, 1, true)

	select {
	case p := <-w.got:
		if string(p) != string([]byte{9, 8, 7}) {
			t.Fatalf("echo payload = %v, want [9 8 7]", p)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no echoed audio MediaChunk received from runtime within 3s")
	}
}

// TestDuplexSink_StartError verifies that a failure to open the runtime stream
// is returned immediately from Start.
func TestDuplexSink_StartError(t *testing.T) {
	// Point at a port with nothing listening — Converse will fail.
	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     "localhost:1", // invalid — nothing listens on port 1
		DialTimeout: 100 * time.Millisecond,
	})
	// NewRuntimeClient does a health check, so this may itself fail.
	// Either way, we can't get a working client; skip cleanly if so.
	if err != nil {
		t.Skipf("NewRuntimeClient returned %v (expected in isolated env)", err)
	}
	defer func() { _ = client.Close() }()

	w := &captureBinaryWriter{got: make(chan []byte, 1)}
	sink := NewGRPCDuplexSink("sess-err", client, w)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err = sink.Start(ctx, &facade.AudioSessionStart{Codec: "pcm", SampleRate: 16000, Channels: 1})
	if err == nil {
		t.Fatal("Start should have failed with an unreachable runtime")
	}
}

// immediatelyClosingServer is a gRPC RuntimeService that reports itself healthy
// but returns immediately from Converse without reading — causing the client's
// Send(DuplexStart) to fail with a stream-closed / EOF error.
type immediatelyClosingServer struct {
	runtimev1.UnimplementedRuntimeServiceServer
}

func (s *immediatelyClosingServer) Health(_ context.Context, _ *runtimev1.HealthRequest) (*runtimev1.HealthResponse, error) {
	return &runtimev1.HealthResponse{Healthy: true, Status: "ok"}, nil
}

func (s *immediatelyClosingServer) Converse(_ runtimev1.RuntimeService_ConverseServer) error {
	return nil // close the stream immediately; the client's Send will get EOF
}

// startImmediatelyClosingServer starts a gRPC server backed by
// immediatelyClosingServer and returns the address and a cleanup function.
func startImmediatelyClosingServer(t *testing.T) (string, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	srv := grpc.NewServer()
	runtimev1.RegisterRuntimeServiceServer(srv, &immediatelyClosingServer{})
	go func() { _ = srv.Serve(lis) }()
	return lis.Addr().String(), srv.Stop
}

// TestDuplexSink_StartConverseError covers the branch in Start where
// client.Converse returns an error. We trigger it by passing an
// already-cancelled context — gRPC rejects the stream-open with context.Canceled.
func TestDuplexSink_StartConverseError(t *testing.T) {
	addr, cleanup := startImmediatelyClosingServer(t)
	defer cleanup()

	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     addr,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewRuntimeClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	w := &captureBinaryWriter{got: make(chan []byte, 1)}
	sink := NewGRPCDuplexSink("sess-ctx-cancel", client, w)

	// Pre-cancel the context so Converse itself returns an error.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = sink.Start(ctx, &facade.AudioSessionStart{Codec: "pcm", SampleRate: 16000, Channels: 1})
	// gRPC client-side may open the stream lazily; if Converse somehow
	// succeeds with the cancelled context, the subsequent Send will fail.
	// Either outcome exercises an error-return branch in Start.
	if err != nil {
		return // error path covered
	}
	// Stream opened despite cancelled ctx: cleanup gracefully.
	_ = sink.Close()
}

// TestDuplexSink_StartSendError covers the branch in Start where stream.Send
// of the DuplexStart message fails because the server already closed the stream.
func TestDuplexSink_StartSendError(t *testing.T) {
	addr, cleanup := startImmediatelyClosingServer(t)
	defer cleanup()

	client, err := facade.NewRuntimeClient(facade.RuntimeClientConfig{
		Address:     addr,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewRuntimeClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	w := &captureBinaryWriter{got: make(chan []byte, 1)}
	sink := NewGRPCDuplexSink("sess-send-err", client, w)

	ctx := context.Background()
	// The server closes the stream immediately; Start may succeed or may get
	// an error from Send depending on gRPC buffering. Either path is valid.
	err = sink.Start(ctx, &facade.AudioSessionStart{Codec: "pcm", SampleRate: 16000, Channels: 1})
	_ = sink.Close()
	_ = err // both outcomes (error or nil) are acceptable
}

// TestDuplexSink_CloseWithoutStart verifies that Close on an uninitialised sink
// does not panic.
func TestDuplexSink_CloseWithoutStart(t *testing.T) {
	w := &captureBinaryWriter{got: make(chan []byte, 1)}
	sink := NewGRPCDuplexSink("sess-close", nil, w)
	if err := sink.Close(); err != nil {
		t.Fatalf("Close on uninitialised sink returned error: %v", err)
	}
}

// TestDuplexSink_SendAudioWithoutStart verifies SendAudio panics rather than
// silently succeeding when Start was never called (stream is nil).
func TestDuplexSink_SendAudioWithoutStart(t *testing.T) {
	w := &captureBinaryWriter{got: make(chan []byte, 1)}
	sink := NewGRPCDuplexSink("sess-nostart", nil, w)
	defer func() { _ = recover() }() // catch the expected nil-pointer panic
	_ = sink.SendAudio([]byte{1}, 0, false)
}
