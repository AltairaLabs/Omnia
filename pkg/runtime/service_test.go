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
