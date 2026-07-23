package runtime

import (
	"context"
	"io"

	"github.com/altairalabs/omnia/pkg/runtime/contract"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const statusReady = "ready"

// serviceAdapter drives the omnia.runtime.v1 wire protocol against a Handler.
type serviceAdapter struct {
	runtimev1.UnimplementedRuntimeServiceServer
	handler Handler
}

// Health advertises liveness, the contract version, and the capability set. The
// capabilities MUST match those in the per-stream RuntimeHello, so both come
// from the same Handler.Capabilities().
func (a *serviceAdapter) Health(_ context.Context, _ *runtimev1.HealthRequest) (*runtimev1.HealthResponse, error) {
	return &runtimev1.HealthResponse{
		Healthy:         true,
		Status:          statusReady,
		ContractVersion: contract.Version,
		Capabilities:    a.handler.Capabilities(),
	}, nil
}

const (
	codeInternal = "INTERNAL_ERROR"
	codeDuplex   = "DUPLEX_UNSUPPORTED"
)

// Converse drives one bidi stream: it sends RuntimeHello as the first frame,
// then routes each inbound turn through the Handler. Duplex sessions are refused
// gracefully (this SDK does not implement duplex audio).
func (a *serviceAdapter) Converse(stream runtimev1.RuntimeService_ConverseServer) error {
	ctx := stream.Context()
	id := parseIdentity(ctx)
	helloSent := false

	for {
		msg, err := stream.Recv()
		if err != nil {
			return converseRecvErr(err)
		}
		if msg.GetDuplexStart() != nil {
			_ = stream.Send(errorFrame(codeDuplex, "duplex audio is not supported by this runtime"))
			continue
		}
		if !helloSent {
			if err := a.sendHello(stream); err != nil {
				return status.Errorf(codes.Internal, "failed to send runtime hello: %v", err)
			}
			helloSent = true
		}
		if err := a.handleTurn(ctx, stream, msg, id); err != nil {
			_ = stream.Send(errorFrame(codeInternal, "an internal error occurred while processing the message"))
		}
	}
}

func converseRecvErr(err error) error {
	if err == io.EOF {
		return nil
	}
	return status.Errorf(codes.Internal, "failed to receive message: %v", err)
}

func (a *serviceAdapter) handleTurn(
	ctx context.Context,
	stream runtimev1.RuntimeService_ConverseServer,
	msg *runtimev1.ClientMessage,
	id Identity,
) error {
	emit := &streamEmitter{stream: stream}
	if err := a.handler.Converse(ctx, buildTurn(msg, id), emit); err != nil {
		return err
	}
	if !emit.doneSent {
		return emit.Done(Done{})
	}
	return nil
}

func (a *serviceAdapter) sendHello(stream runtimev1.RuntimeService_ConverseServer) error {
	return stream.Send(&runtimev1.ServerMessage{
		Message: &runtimev1.ServerMessage_RuntimeHello{
			RuntimeHello: &runtimev1.RuntimeHello{Capabilities: a.handler.Capabilities()},
		},
	})
}

func errorFrame(code, msg string) *runtimev1.ServerMessage {
	return &runtimev1.ServerMessage{
		Message: &runtimev1.ServerMessage_Error{Error: &runtimev1.Error{Code: code, Message: msg}},
	}
}

func buildTurn(msg *runtimev1.ClientMessage, id Identity) Turn {
	return Turn{
		SessionID:     msg.GetSessionId(),
		Content:       msg.GetContent(),
		Parts:         mapPartsFromProto(msg.GetParts()),
		Metadata:      msg.GetMetadata(),
		ConsentGrants: msg.GetConsentGrants(),
		Identity:      id,
	}
}

func mapPartsFromProto(parts []*runtimev1.ContentPart) []ContentPart {
	if len(parts) == 0 {
		return nil
	}
	out := make([]ContentPart, 0, len(parts))
	for _, p := range parts {
		cp := ContentPart{Type: p.GetType(), Text: p.GetText()}
		if m := p.GetMedia(); m != nil {
			cp.Data = m.GetData()
			cp.URL = m.GetUrl()
			cp.MimeType = m.GetMimeType()
			cp.StorageRef = m.GetStorageRef()
		}
		out = append(out, cp)
	}
	return out
}

// streamEmitter marshals Emitter calls to ServerMessage frames on one stream.
type streamEmitter struct {
	stream   runtimev1.RuntimeService_ConverseServer
	doneSent bool
}

func (e *streamEmitter) Chunk(text string) error {
	if text == "" {
		return nil
	}
	return e.stream.Send(&runtimev1.ServerMessage{
		Message: &runtimev1.ServerMessage_Chunk{Chunk: &runtimev1.Chunk{Content: text}},
	})
}

func (e *streamEmitter) Done(d Done) error {
	e.doneSent = true
	return e.stream.Send(&runtimev1.ServerMessage{
		Message: &runtimev1.ServerMessage_Done{Done: &runtimev1.Done{
			FinalContent: d.Final,
			Usage:        mapUsageToProto(d.Usage),
			Parts:        mapPartsToProto(d.Parts),
		}},
	})
}

// ToolCall and Media are wired in Task 4.
func (e *streamEmitter) ToolCall(_ ClientToolCall) (ClientToolResult, error) {
	return ClientToolResult{}, status.Error(codes.Unimplemented, "tool calls not yet wired")
}

func (e *streamEmitter) Media(_ MediaChunk) error {
	return status.Error(codes.Unimplemented, "media not yet wired")
}

func mapUsageToProto(u *Usage) *runtimev1.Usage {
	if u == nil {
		return nil
	}
	return &runtimev1.Usage{
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
		CostUsd:      u.CostUSD,
	}
}

func mapPartsToProto(parts []ContentPart) []*runtimev1.ContentPart {
	if len(parts) == 0 {
		return nil
	}
	out := make([]*runtimev1.ContentPart, 0, len(parts))
	for _, p := range parts {
		cp := &runtimev1.ContentPart{Type: p.Type, Text: p.Text}
		if p.Data != "" || p.URL != "" || p.MimeType != "" || p.StorageRef != "" {
			cp.Media = &runtimev1.MediaContent{
				Data:       p.Data,
				Url:        p.URL,
				MimeType:   p.MimeType,
				StorageRef: p.StorageRef,
			}
		}
		out = append(out, cp)
	}
	return out
}
