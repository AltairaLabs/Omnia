package facade

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"google.golang.org/grpc"

	"github.com/altairalabs/omnia/internal/session"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
	"github.com/altairalabs/omnia/pkg/session/httpclient"
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
	store  session.Recorder
	pool   *RecordingPool
	policy recordingPolicyGetter
	log    logr.Logger
}

// newBusRecorder returns a recorder, or nil if recording isn't wired (store or
// pool absent) — in which case the interceptors are no-ops.
func newBusRecorder(store session.Recorder, pool *RecordingPool, policy recordingPolicyGetter, log logr.Logger) *busRecorder {
	if store == nil || pool == nil || policy == nil {
		return nil
	}
	return &busRecorder{store: store, pool: pool, policy: policy, log: log.WithName("bus-recorder")}
}

// recordUser records an outbound user message, gated by recording.facadeData
// (the facade is Omnia-controlled, so user turns are facade-sourced).
func (r *busRecorder) recordUser(ctx context.Context, sessionID, content string) {
	if r == nil || sessionID == "" || content == "" || !facadeAllowed(r.policy.Get(ctx)) {
		return
	}
	r.submit(ctx, sessionID, userMessage(content))
}

// recordAssistant records an inbound assistant message (with aggregate usage),
// gated by recording.runtimeData. The content is runtime-emitted, so it's
// opt-in.
func (r *busRecorder) recordAssistant(ctx context.Context, sessionID, content string, usage *runtimev1.Usage) {
	if r == nil || sessionID == "" || content == "" || !runtimeAllowed(r.policy.Get(ctx)) {
		return
	}
	r.submit(ctx, sessionID, assistantMessage(content, usage))
}

// recordExchange records a user turn then an assistant turn as a SINGLE ordered
// pool task, so the user message is always persisted before the assistant
// message. Used by the unary Invoke path, where both turns are observed at once
// — the streaming path is naturally ordered by the runtime round-trip, and the
// store assigns sequence numbers at write time.
func (r *busRecorder) recordExchange(ctx context.Context, sessionID, userContent, assistantContent string, usage *runtimev1.Usage) {
	if r == nil || sessionID == "" {
		return
	}
	p := r.policy.Get(ctx)
	var msgs []session.Message
	if userContent != "" && facadeAllowed(p) {
		msgs = append(msgs, userMessage(userContent))
	}
	if assistantContent != "" && runtimeAllowed(p) {
		msgs = append(msgs, assistantMessage(assistantContent, usage))
	}
	r.submit(ctx, sessionID, msgs...)
}

// submit records messages asynchronously via the pool, in order, within a single
// task (preserving their relative order). The request context is detached from
// cancellation (the stream may close before the write runs) while keeping values
// such as the propagated user identity for X-Omnia-User-ID.
func (r *busRecorder) submit(ctx context.Context, sessionID string, msgs ...session.Message) {
	if len(msgs) == 0 {
		return
	}
	recCtx := context.WithoutCancel(ctx)
	r.pool.Submit(func() {
		for i := range msgs {
			if err := r.store.AppendMessage(recCtx, sessionID, msgs[i]); err != nil {
				r.log.V(1).Info("bus message record failed",
					"sessionID", sessionID, "role", string(msgs[i].Role), "error", err.Error())
			}
		}
	})
}

// facadeAllowed reports whether facade-sourced content (user turns) may be
// recorded under the policy. A nil policy records (fail-open).
func facadeAllowed(p *httpclient.PrivacyPolicyResponse) bool {
	return p == nil || (p.Recording.Enabled && p.Recording.FacadeData)
}

// runtimeAllowed reports whether runtime-sourced content (assistant turns) may be
// recorded under the policy. A nil policy records (fail-open).
func runtimeAllowed(p *httpclient.PrivacyPolicyResponse) bool {
	return p == nil || (p.Recording.Enabled && p.Recording.RuntimeData)
}

func userMessage(content string) session.Message {
	return session.Message{ID: uuid.New().String(), Role: session.RoleUser, Content: content, Timestamp: time.Now()}
}

func assistantMessage(content string, usage *runtimev1.Usage) session.Message {
	msg := session.Message{ID: uuid.New().String(), Role: session.RoleAssistant, Content: content, Timestamp: time.Now()}
	if usage != nil {
		msg.InputTokens = usage.GetInputTokens()
		msg.OutputTokens = usage.GetOutputTokens()
		msg.CostUSD = float64(usage.GetCostUsd())
	}
	return msg
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
			r.recordExchange(ctx, ireq.GetInvocationId(), ireq.GetInputJson(), iresp.GetOutputJson(), iresp.GetUsage())
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
