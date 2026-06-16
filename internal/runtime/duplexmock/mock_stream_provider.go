// Package duplexmock provides an in-memory StreamInputSupport provider for
// testing the duplex audio path without a real provider (Gemini/OpenAI).
// It echoes input audio chunks back as output and emits text on SendText.
package duplexmock

import (
	"context"
	"sync"

	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/runtime/providers/base"
	"github.com/AltairaLabs/PromptKit/runtime/types"
)

// Provider is a minimal in-memory StreamInputSupport that echoes audio chunks.
// All base.Provider / providers.Provider methods are stubbed to safe no-ops
// so that the struct satisfies the full interface without panicking on nil.
type Provider struct{}

// New returns a *Provider that implements providers.StreamInputSupport.
func New() *Provider { return &Provider{} }

// --- base.Provider stubs ---

// Name returns the provider name.
func (p *Provider) Name() string { return "duplex-mock" }

// Type returns the provider type.
func (p *Provider) Type() base.ProviderType { return base.ProviderTypeInference }

// Pricing returns nil (no cost tracking in the mock).
func (p *Provider) Pricing() *base.PricingDescriptor { return nil }

// Validate always returns nil.
func (p *Provider) Validate() error { return nil }

// Init always returns nil.
func (p *Provider) Init(_ context.Context) error { return nil }

// HealthCheck always returns nil.
func (p *Provider) HealthCheck(_ context.Context) error { return nil }

// Close always returns nil.
func (p *Provider) Close() error { return nil }

// --- providers.Provider stubs (beyond base.Provider) ---

// ID returns the same value as Name.
func (p *Provider) ID() string { return p.Name() }

// Model returns the mock model identifier.
func (p *Provider) Model() string { return "duplex-mock-v0" }

// Predict is not implemented for the mock.
func (p *Provider) Predict(_ context.Context, _ providers.PredictionRequest) (providers.PredictionResponse, error) {
	return providers.PredictionResponse{}, nil
}

// PredictStream is not implemented for the mock.
func (p *Provider) PredictStream(_ context.Context, _ providers.PredictionRequest) (<-chan providers.StreamChunk, error) {
	ch := make(chan providers.StreamChunk)
	close(ch)
	return ch, nil
}

// SupportsStreaming returns false (the mock only supports duplex, not predict streaming).
func (p *Provider) SupportsStreaming() bool { return false }

// ShouldIncludeRawOutput returns false.
func (p *Provider) ShouldIncludeRawOutput() bool { return false }

// CalculateCost returns a zero CostInfo.
func (p *Provider) CalculateCost(_, _, _ int) types.CostInfo { return types.CostInfo{} }

// --- providers.StreamInputSupport methods ---

// SupportsStreamInput returns the media types this mock accepts (audio).
func (p *Provider) SupportsStreamInput() []string {
	return []string{types.ContentTypeAudio}
}

// GetStreamingCapabilities returns capabilities for the mock provider.
func (p *Provider) GetStreamingCapabilities() providers.StreamingCapabilities {
	return providers.StreamingCapabilities{
		SupportedMediaTypes:  []string{types.ContentTypeAudio},
		BidirectionalSupport: true,
	}
}

// CreateStreamSession returns a new in-memory session that echoes input.
func (p *Provider) CreateStreamSession(_ context.Context, _ *providers.StreamingInputConfig) (providers.StreamInputSession, error) {
	return &session{
		resp: make(chan providers.StreamChunk, 16),
		done: make(chan struct{}),
	}, nil
}

// session is an in-memory bidirectional streaming session.
type session struct {
	mu     sync.Mutex
	closed bool
	resp   chan providers.StreamChunk
	done   chan struct{}
}

// SendChunk echoes the incoming audio chunk back onto the response channel.
func (s *session) SendChunk(_ context.Context, chunk *types.MediaChunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.resp <- providers.StreamChunk{
		MediaData: &providers.StreamMediaData{
			Data:       chunk.Data,
			MIMEType:   "audio/pcm",
			SampleRate: 16000,
			Channels:   1,
		},
	}
	return nil
}

// SendText echoes the text as a Delta chunk on the response channel.
func (s *session) SendText(_ context.Context, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.resp <- providers.StreamChunk{Delta: text}
	return nil
}

// SendSystemContext is a no-op in the mock.
func (s *session) SendSystemContext(_ context.Context, _ string) error { return nil }

// Response returns the read-only channel of streamed output chunks.
func (s *session) Response() <-chan providers.StreamChunk { return s.resp }

// Done returns a channel that is closed when the session ends.
func (s *session) Done() <-chan struct{} { return s.done }

// Error always returns nil for the mock.
func (s *session) Error() error { return nil }

// Close ends the session and closes both internal channels. Safe to call multiple times.
func (s *session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	close(s.resp)
	close(s.done)
	return nil
}
