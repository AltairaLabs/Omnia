package facade

import (
	"context"

	"github.com/go-logr/logr"
	"google.golang.org/grpc"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/httpclient"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// recordingPolicyGetter supplies the effective recording policy for the facade's
// agent. The facade is single-agent, so no per-request key is needed.
type recordingPolicyGetter interface {
	Get(ctx context.Context) *httpclient.PrivacyPolicyResponse
}

// busRecorder records conversation MESSAGES (user + assistant) observed on the
// facade<->runtime gRPC bus. It lives on the RuntimeClient, so recording is
// protocol-agnostic (WS/A2A/Functions all use this client) and runtime-agnostic
// (any runtime that speaks the Converse/Invoke contract). Tool calls, provider
// calls, and pipeline events are the runtime's responsibility (event_store) and
// are deliberately NOT recorded here.
type busRecorder struct {
	store  session.Store
	pool   *RecordingPool
	policy recordingPolicyGetter
	log    logr.Logger
}

// newBusRecorder returns a recorder, or nil if recording isn't wired (store or
// pool absent) — in which case the interceptors are no-ops.
func newBusRecorder(store session.Store, pool *RecordingPool, policy recordingPolicyGetter, log logr.Logger) *busRecorder {
	if store == nil || pool == nil || policy == nil {
		return nil
	}
	return &busRecorder{store: store, pool: pool, policy: policy, log: log.WithName("bus-recorder")}
}

// recordUser records an outbound user message, gated by recording.facadeData
// (the facade is Omnia-controlled, so user turns are facade-sourced).
func (r *busRecorder) recordUser(ctx context.Context, sessionID, content string) {
	if r == nil || sessionID == "" || content == "" {
		return
	}
	p := r.policy.Get(ctx)
	if p != nil && (!p.Recording.Enabled || !p.Recording.FacadeData) {
		return
	}
	r.submit(ctx, sessionID, session.Message{Role: session.RoleUser, Content: content})
}

// recordAssistant records an inbound assistant message (with aggregate usage),
// gated by recording.runtimeData (richData kept as deprecated alias). The
// content is runtime-emitted, so it's opt-in.
func (r *busRecorder) recordAssistant(ctx context.Context, sessionID, content string, usage *runtimev1.Usage) {
	if r == nil || sessionID == "" || content == "" {
		return
	}
	p := r.policy.Get(ctx)
	if p != nil && (!p.Recording.Enabled || (!p.Recording.RuntimeData && !p.Recording.RichData)) {
		return
	}
	msg := session.Message{Role: session.RoleAssistant, Content: content}
	if usage != nil {
		msg.InputTokens = usage.GetInputTokens()
		msg.OutputTokens = usage.GetOutputTokens()
	}
	r.submit(ctx, sessionID, msg)
}

// submit records asynchronously via the pool. The request context is detached
// from cancellation (the stream may close before the write runs) while keeping
// values such as the propagated user identity for X-Omnia-User-ID.
func (r *busRecorder) submit(ctx context.Context, sessionID string, msg session.Message) {
	recCtx := context.WithoutCancel(ctx)
	r.pool.Submit(func() {
		if err := r.store.AppendMessage(recCtx, sessionID, msg); err != nil {
			r.log.V(1).Info("bus message record failed",
				"sessionID", sessionID, "role", string(msg.Role), "error", err.Error())
		}
	})
}

// recordingStreamInterceptor wraps the Converse bidi stream to record the user
// turn (SendMsg) and the assistant turn on Done (RecvMsg).
func (r *busRecorder) recordingStreamInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn,
		method string, streamer grpc.Streamer, opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		cs, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil || r == nil || method != runtimev1.RuntimeService_Converse_FullMethodName {
			return cs, err
		}
		return &recordingClientStream{ClientStream: cs, rec: r}, nil
	}
}

// recordingUnaryInterceptor records the user input + assistant output of a
// one-shot Invoke (Functions facade). For Functions the session id is the
// invocation id (the facade creates the session under that id).
func (r *busRecorder) recordingUnaryInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context, method string, req, reply any,
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption,
	) error {
		err := invoker(ctx, method, req, reply, cc, opts...)
		if err != nil || r == nil || method != runtimev1.RuntimeService_Invoke_FullMethodName {
			return err
		}
		ireq, _ := req.(*runtimev1.InvocationRequest)
		iresp, _ := reply.(*runtimev1.InvocationResponse)
		if ireq != nil && iresp != nil {
			sid := ireq.GetInvocationId()
			r.recordUser(ctx, sid, ireq.GetInputJson())
			r.recordAssistant(ctx, sid, iresp.GetOutputJson(), iresp.GetUsage())
		}
		return nil
	}
}

// recordingClientStream observes a Converse stream. SessionID is taken from the
// first ClientMessage (every client message carries it).
type recordingClientStream struct {
	grpc.ClientStream
	rec       *busRecorder
	sessionID string
}

func (s *recordingClientStream) SendMsg(m any) error {
	err := s.ClientStream.SendMsg(m)
	if err != nil {
		return err
	}
	if cm, ok := m.(*runtimev1.ClientMessage); ok {
		if s.sessionID == "" {
			s.sessionID = cm.GetSessionId()
		}
		// Only plain user turns — not client tool results or audio frames.
		if cm.GetClientToolResult() == nil && cm.GetAudioInput() == nil {
			s.rec.recordUser(s.Context(), s.sessionID, cm.GetContent())
		}
	}
	return nil
}

func (s *recordingClientStream) RecvMsg(m any) error {
	err := s.ClientStream.RecvMsg(m)
	if err != nil {
		return err
	}
	if sm, ok := m.(*runtimev1.ServerMessage); ok {
		if done := sm.GetDone(); done != nil {
			s.rec.recordAssistant(s.Context(), s.sessionID, done.GetFinalContent(), done.GetUsage())
		}
	}
	return nil
}
