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

package facade

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"

	"github.com/altairalabs/omnia/internal/session"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"

	"github.com/altairalabs/omnia/pkg/policy"
)

// RuntimeClient wraps the gRPC client for communicating with the runtime sidecar.
type RuntimeClient struct {
	conn   *grpc.ClientConn
	client runtimev1.RuntimeServiceClient
	addr   string
	log    logr.Logger
}

// RuntimeClientConfig contains configuration for the runtime client.
type RuntimeClientConfig struct {
	// Address is the runtime gRPC server address (e.g., "localhost:9000").
	Address string
	// DialTimeout is the timeout for establishing the connection.
	DialTimeout time.Duration
	// MaxMessageSize is the maximum message size in bytes (default 16MB).
	MaxMessageSize int
	// Log is an optional logger. If zero-value, a discard logger is used.
	Log logr.Logger
	// TracerProvider is an optional tracer provider for distributed tracing.
	TracerProvider trace.TracerProvider

	// SessionStore, RecordingPool and RecordingPolicy enable the bus recorder —
	// the protocol-agnostic recording of conversation messages off the bus. When
	// any is nil the recording interceptors are no-ops (e.g. doctor/tests).
	SessionStore    session.Recorder
	RecordingPool   *RecordingPool
	RecordingPolicy recordingPolicyGetter
}

// NewRuntimeClient creates a new RuntimeClient connected to the runtime sidecar.
func NewRuntimeClient(cfg RuntimeClientConfig) (*RuntimeClient, error) {
	// Use default max message size if not specified
	maxMsgSize := cfg.MaxMessageSize
	if maxMsgSize == 0 {
		maxMsgSize = 16 * 1024 * 1024 // 16MB default
	}

	log := cfg.Log
	if log.GetSink() == nil {
		log = logr.Discard()
	}

	// Chain the outbound policy-propagation interceptors with the bus recorder
	// (records conversation messages off the bus; no-op when recording deps are
	// not wired). Covers both Invoke (unary) and Converse (stream).
	rec := newBusRecorder(cfg.SessionStore, cfg.RecordingPool, cfg.RecordingPolicy, log)
	unaryInts := []grpc.UnaryClientInterceptor{policyUnaryClientInterceptor()}
	streamInts := []grpc.StreamClientInterceptor{policyStreamClientInterceptor()}
	if rec != nil {
		unaryInts = append(unaryInts, rec.recordingUnaryInterceptor())
		streamInts = append(streamInts, rec.recordingStreamInterceptor())
	}

	// Use insecure credentials for localhost sidecar communication.
	// In production, mTLS could be added for enhanced security.
	var dialOpts []grpc.DialOption
	dialOpts = append(dialOpts,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxMsgSize),
			grpc.MaxCallSendMsgSize(maxMsgSize),
		),
		grpc.WithChainUnaryInterceptor(unaryInts...),
		grpc.WithChainStreamInterceptor(streamInts...),
	)
	if cfg.TracerProvider != nil {
		dialOpts = append(dialOpts, grpc.WithStatsHandler(otelgrpc.NewClientHandler(
			otelgrpc.WithTracerProvider(cfg.TracerProvider),
			otelgrpc.WithFilter(isNotHealthCheckRPC),
		)))
	}
	conn, err := grpc.NewClient(cfg.Address, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create runtime client for %s: %w", cfg.Address, err)
	}

	client := &RuntimeClient{
		conn:   conn,
		client: runtimev1.NewRuntimeServiceClient(conn),
		addr:   cfg.Address,
		log:    log,
	}

	// Verify connection with a health check
	ctx, cancel := context.WithTimeout(context.Background(), cfg.DialTimeout)
	defer cancel()

	_, err = client.Health(ctx)
	if err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			log.Error(closeErr, "failed to close connection after health check failure")
		}
		return nil, fmt.Errorf("failed to connect to runtime at %s: %w", cfg.Address, err)
	}

	return client, nil
}

// Converse opens a bidirectional streaming RPC for conversation.
// Callers supply per-call options (e.g. grpc.UseCompressor) to control
// compression; text/function paths should pass grpc.UseCompressor(gzip.Name)
// while audio duplex callers omit it to avoid PCM recompression overhead.
func (c *RuntimeClient) Converse(ctx context.Context, opts ...grpc.CallOption) (runtimev1.RuntimeService_ConverseClient, error) {
	return c.client.Converse(ctx, opts...)
}

// Invoke runs a one-shot Function call through the runtime. The facade's
// FunctionsHandler is the sole consumer; agent-mode flows continue to
// use Converse.
// Callers supply per-call options (e.g. grpc.UseCompressor) to control
// compression; function invocation paths should pass grpc.UseCompressor(gzip.Name).
func (c *RuntimeClient) Invoke(ctx context.Context, req *runtimev1.InvocationRequest, opts ...grpc.CallOption) (*runtimev1.InvocationResponse, error) {
	return c.client.Invoke(ctx, req, opts...)
}

// Health checks the runtime's health status.
func (c *RuntimeClient) Health(ctx context.Context) (*runtimev1.HealthResponse, error) {
	return c.client.Health(ctx, &runtimev1.HealthRequest{})
}

// HasConversation reports whether a session's working context can still be
// resumed, translating the wire enum into the facade's ResumeState.
//
// An unrecognised wire value maps to ResumeStateUnavailable rather than to a
// resumable or expired verdict: an older or newer runtime that answers with a
// state this build does not know must not have that silently read as "the
// user's conversation is gone".
func (c *RuntimeClient) HasConversation(ctx context.Context, sessionID string) (ResumeState, error) {
	resp, err := c.client.HasConversation(ctx, &runtimev1.HasConversationRequest{SessionId: sessionID})
	if err != nil {
		// A runtime built against an older contract version does not serve this
		// method. It cannot answer, which is not the same as answering "gone" —
		// report it distinctly so the facade degrades to letting the session
		// through rather than failing every resume against such a runtime.
		if status.Code(err) == codes.Unimplemented {
			return ResumeStateUnknown, ErrProbeUnsupported
		}
		return ResumeStateUnknown, err
	}
	switch resp.GetState() {
	case runtimev1.ResumeState_RESUME_STATE_RESUMABLE:
		return ResumeStateResumable, nil
	case runtimev1.ResumeState_RESUME_STATE_NOT_FOUND:
		return ResumeStateNotFound, nil
	default:
		return ResumeStateUnavailable, nil
	}
}

// Close closes the gRPC connection.
func (c *RuntimeClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Address returns the runtime address.
func (c *RuntimeClient) Address() string {
	return c.addr
}

// policyUnaryClientInterceptor returns a gRPC unary client interceptor that
// injects policy propagation fields from the Go context into outgoing gRPC metadata.
func policyUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		ctx = injectPolicyMetadata(ctx)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// policyStreamClientInterceptor returns a gRPC stream client interceptor that
// injects policy propagation fields from the Go context into outgoing gRPC metadata.
func policyStreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		ctx = injectPolicyMetadata(ctx)
		return streamer(ctx, desc, cc, method, opts...)
	}
}

// injectPolicyMetadata reads policy propagation fields from the Go context and
// appends them as gRPC outgoing metadata.
func injectPolicyMetadata(ctx context.Context) context.Context {
	md := policy.ToGRPCMetadata(ctx)
	if len(md) == 0 {
		return ctx
	}
	pairs := make([]string, 0, len(md)*2)
	for k, v := range md {
		pairs = append(pairs, k, v)
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...)
}

// isNotHealthCheckRPC filters out gRPC health check RPCs from tracing.
func isNotHealthCheckRPC(info *stats.RPCTagInfo) bool {
	return info.FullMethodName != "/omnia.runtime.v1.RuntimeService/Health"
}
