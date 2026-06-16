package duplexmock

import (
	"context"
	"testing"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/runtime/types"
)

func TestMockStreamSession_EchoesAudio(t *testing.T) {
	p := New()
	sess, err := p.CreateStreamSession(context.Background(), &providers.StreamingInputConfig{})
	if err != nil {
		t.Fatalf("CreateStreamSession: %v", err)
	}
	defer func() { _ = sess.Close() }()

	in := []byte{0x01, 0x02, 0x03, 0x04}
	if err := sess.SendChunk(context.Background(), &types.MediaChunk{Data: in, SequenceNum: 0}); err != nil {
		t.Fatalf("SendChunk: %v", err)
	}

	select {
	case out := <-sess.Response():
		if out.MediaData == nil || string(out.MediaData.Data) != string(in) {
			t.Fatalf("expected echoed audio %v, got %+v", in, out.MediaData)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for echoed chunk")
	}
}

func TestMockProvider_ImplementsStreamInputSupport(t *testing.T) {
	var _ providers.StreamInputSupport = New()
}

func TestMockStreamSession_SendText(t *testing.T) {
	p := New()
	sess, err := p.CreateStreamSession(context.Background(), &providers.StreamingInputConfig{})
	if err != nil {
		t.Fatalf("CreateStreamSession: %v", err)
	}
	defer func() { _ = sess.Close() }()

	if err := sess.SendText(context.Background(), "hello"); err != nil {
		t.Fatalf("SendText: %v", err)
	}

	select {
	case out := <-sess.Response():
		if out.Delta != "hello" {
			t.Fatalf("expected delta %q, got %q", "hello", out.Delta)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for text echo")
	}
}

func TestMockStreamSession_CloseIdempotent(t *testing.T) {
	p := New()
	sess, err := p.CreateStreamSession(context.Background(), &providers.StreamingInputConfig{})
	if err != nil {
		t.Fatalf("CreateStreamSession: %v", err)
	}

	if err := sess.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestMockStreamSession_SendAfterClose(t *testing.T) {
	p := New()
	sess, err := p.CreateStreamSession(context.Background(), &providers.StreamingInputConfig{})
	if err != nil {
		t.Fatalf("CreateStreamSession: %v", err)
	}

	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// SendChunk/SendText after close must not panic or block
	if err := sess.SendChunk(context.Background(), &types.MediaChunk{Data: []byte{0x01}}); err != nil {
		t.Fatalf("SendChunk after close returned unexpected error: %v", err)
	}
	if err := sess.SendText(context.Background(), "late"); err != nil {
		t.Fatalf("SendText after close returned unexpected error: %v", err)
	}
}

func TestMockStreamSession_DoneAndError(t *testing.T) {
	p := New()
	sess, err := p.CreateStreamSession(context.Background(), &providers.StreamingInputConfig{})
	if err != nil {
		t.Fatalf("CreateStreamSession: %v", err)
	}

	if sess.Error() != nil {
		t.Fatalf("Error() before close should be nil, got %v", sess.Error())
	}

	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case <-sess.Done():
		// expected
	case <-time.After(time.Second):
		t.Fatal("Done() not closed after Close()")
	}
}

func TestMockStreamSession_SendSystemContext(t *testing.T) {
	p := New()
	sess, err := p.CreateStreamSession(context.Background(), &providers.StreamingInputConfig{})
	if err != nil {
		t.Fatalf("CreateStreamSession: %v", err)
	}
	defer func() { _ = sess.Close() }()

	// SendSystemContext is a no-op on the mock; just verify it does not error
	if err := sess.SendSystemContext(context.Background(), "system context"); err != nil {
		t.Fatalf("SendSystemContext: %v", err)
	}
}

func TestMockProvider_Capabilities(t *testing.T) {
	p := New()

	types_ := p.SupportsStreamInput()
	if len(types_) == 0 {
		t.Fatal("SupportsStreamInput should return at least one type")
	}
	foundAudio := false
	for _, mt := range types_ {
		if mt == types.ContentTypeAudio {
			foundAudio = true
		}
	}
	if !foundAudio {
		t.Fatalf("SupportsStreamInput missing audio, got %v", types_)
	}

	caps := p.GetStreamingCapabilities()
	if !caps.BidirectionalSupport {
		t.Fatal("GetStreamingCapabilities should report BidirectionalSupport=true")
	}
}

func TestMockProvider_BaseProviderStubs(t *testing.T) {
	p := New()

	if p.Name() == "" {
		t.Fatal("Name() should return non-empty string")
	}
	if p.Type() == "" {
		t.Fatal("Type() should return non-empty ProviderType")
	}
	// Pricing returns nil for the mock
	if p.Pricing() != nil {
		t.Fatal("Pricing() should return nil for mock")
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate() should return nil, got %v", err)
	}
	if err := p.Init(context.Background()); err != nil {
		t.Fatalf("Init() should return nil, got %v", err)
	}
	if err := p.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck() should return nil, got %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close() should return nil, got %v", err)
	}
}

func TestMockProvider_InferenceStubs(t *testing.T) {
	p := New()

	if p.ID() == "" {
		t.Fatal("ID() should return non-empty string")
	}
	if p.Model() == "" {
		t.Fatal("Model() should return non-empty string")
	}

	_, err := p.Predict(context.Background(), providers.PredictionRequest{})
	if err != nil {
		t.Fatalf("Predict() should return nil error, got %v", err)
	}

	ch, err := p.PredictStream(context.Background(), providers.PredictionRequest{})
	if err != nil {
		t.Fatalf("PredictStream() should return nil error, got %v", err)
	}
	// channel should be closed immediately
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("PredictStream channel should be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("PredictStream channel not closed promptly")
	}

	if p.SupportsStreaming() {
		t.Fatal("SupportsStreaming() should return false for mock")
	}
	if p.ShouldIncludeRawOutput() {
		t.Fatal("ShouldIncludeRawOutput() should return false for mock")
	}

	cost := p.CalculateCost(10, 20, 5)
	_ = cost // zero value is expected
}
