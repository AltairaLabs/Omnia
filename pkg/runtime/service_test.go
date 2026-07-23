package runtime

import (
	"context"
	"net"
	"testing"

	"github.com/altairalabs/omnia/pkg/runtime/contract"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// stubHandler is a minimal Handler for wire tests. converse is a hook so each
// test can drive the emitter however it needs.
type stubHandler struct {
	caps     []string
	converse func(ctx context.Context, turn Turn, emit Emitter) error
}

func (s *stubHandler) Capabilities() []string { return s.caps }
func (s *stubHandler) Converse(ctx context.Context, turn Turn, emit Emitter) error {
	if s.converse != nil {
		return s.converse(ctx, turn, emit)
	}
	return emit.Done(Done{})
}

// newTestConn starts Serve on an in-memory bufconn listener and returns a
// dialed client connection. The server and connection are torn down on cleanup.
func newTestConn(t *testing.T, h Handler) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	go func() { _ = Serve(lis, h) }()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
		_ = lis.Close()
	})
	return conn
}

func TestHealth_ReportsContractAndCapabilities(t *testing.T) {
	h := &stubHandler{caps: contract.KnownCapabilities()}
	conn := newTestConn(t, h)
	client := runtimev1.NewRuntimeServiceClient(conn)

	resp, err := client.Health(context.Background(), &runtimev1.HealthRequest{})
	require.NoError(t, err)
	assert.True(t, resp.GetHealthy())
	assert.Equal(t, "ready", resp.GetStatus())
	assert.Equal(t, contract.Version, resp.GetContractVersion())
	assert.Equal(t, contract.KnownCapabilities(), resp.GetCapabilities())
}

func drainConverse(t *testing.T, client runtimev1.RuntimeServiceClient, content string) []*runtimev1.ServerMessage {
	t.Helper()
	stream, err := client.Converse(context.Background())
	require.NoError(t, err)
	require.NoError(t, stream.Send(&runtimev1.ClientMessage{SessionId: "s1", Content: content}))
	require.NoError(t, stream.CloseSend())

	var frames []*runtimev1.ServerMessage
	for {
		msg, recvErr := stream.Recv()
		if recvErr != nil {
			break
		}
		frames = append(frames, msg)
	}
	return frames
}

func TestConverse_HelloFirstThenChunkThenDone(t *testing.T) {
	h := &stubHandler{
		caps: []string{contract.CapabilityClientTools},
		converse: func(_ context.Context, turn Turn, emit Emitter) error {
			assert.Equal(t, "hi", turn.Content)
			assert.Equal(t, "s1", turn.SessionID)
			if err := emit.Chunk("hello "); err != nil {
				return err
			}
			if err := emit.Chunk("world"); err != nil {
				return err
			}
			return emit.Done(Done{Final: "hello world", Usage: &Usage{InputTokens: 3, OutputTokens: 2}})
		},
	}
	client := runtimev1.NewRuntimeServiceClient(newTestConn(t, h))
	frames := drainConverse(t, client, "hi")

	require.GreaterOrEqual(t, len(frames), 4)
	// First frame is RuntimeHello with caps matching Health.
	hello := frames[0].GetRuntimeHello()
	require.NotNil(t, hello)
	assert.Equal(t, []string{contract.CapabilityClientTools}, hello.GetCapabilities())
	// Chunks in order.
	assert.Equal(t, "hello ", frames[1].GetChunk().GetContent())
	assert.Equal(t, "world", frames[2].GetChunk().GetContent())
	// Done last, with usage.
	done := frames[3].GetDone()
	require.NotNil(t, done)
	assert.Equal(t, "hello world", done.GetFinalContent())
	assert.Equal(t, int32(3), done.GetUsage().GetInputTokens())
}

func TestConverse_AutoDoneWhenHandlerReturnsWithoutDone(t *testing.T) {
	h := &stubHandler{
		caps: []string{contract.CapabilityClientTools},
		converse: func(_ context.Context, _ Turn, emit Emitter) error {
			return emit.Chunk("no explicit done")
		},
	}
	client := runtimev1.NewRuntimeServiceClient(newTestConn(t, h))
	frames := drainConverse(t, client, "hi")

	var sawDone bool
	for _, f := range frames {
		if f.GetDone() != nil {
			sawDone = true
		}
	}
	assert.True(t, sawDone, "SDK must emit Done when the handler returns without one")
}

func TestConverse_EmptyInputDoesNotCrash(t *testing.T) {
	h := &stubHandler{caps: []string{contract.CapabilityClientTools}}
	client := runtimev1.NewRuntimeServiceClient(newTestConn(t, h))

	stream, err := client.Converse(context.Background())
	require.NoError(t, err)
	require.NoError(t, stream.Send(&runtimev1.ClientMessage{SessionId: "s1"}))
	require.NoError(t, stream.CloseSend())
	for {
		if _, recvErr := stream.Recv(); recvErr != nil {
			// Clean EOF or a non-crash close is acceptable; a transport crash is not.
			assert.NotContains(t, recvErr.Error(), "transport")
			break
		}
	}
}

func TestConverse_DuplexStartRejectedGracefully(t *testing.T) {
	h := &stubHandler{caps: []string{contract.CapabilityClientTools}} // duplex_audio NOT advertised
	client := runtimev1.NewRuntimeServiceClient(newTestConn(t, h))

	stream, err := client.Converse(context.Background())
	require.NoError(t, err)
	require.NoError(t, stream.Send(&runtimev1.ClientMessage{
		SessionId:   "s1",
		DuplexStart: &runtimev1.DuplexStart{Codec: "pcm", SampleRate: 16000, Channels: 1},
	}))
	require.NoError(t, stream.CloseSend())

	var sawError bool
	for {
		msg, recvErr := stream.Recv()
		if recvErr != nil {
			break
		}
		if e := msg.GetError(); e != nil {
			sawError = true
			assert.Equal(t, "DUPLEX_UNSUPPORTED", e.GetCode())
		}
	}
	assert.True(t, sawError, "duplex must be refused with an Error frame, not a crash")
}
