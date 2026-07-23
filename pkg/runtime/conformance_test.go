package runtime_test

import (
	"context"
	"net"
	"testing"

	rt "github.com/altairalabs/omnia/pkg/runtime"
	"github.com/altairalabs/omnia/pkg/runtime/conformance"
	"github.com/altairalabs/omnia/pkg/runtime/contract"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type conformantHandler struct{}

func (conformantHandler) Capabilities() []string {
	return []string{contract.CapabilityClientTools, contract.CapabilityInvoke}
}

func (conformantHandler) Converse(_ context.Context, turn rt.Turn, emit rt.Emitter) error {
	if err := emit.Chunk("ack: " + turn.Content); err != nil {
		return err
	}
	return emit.Done(rt.Done{Final: "ack: " + turn.Content})
}

func (conformantHandler) Invoke(_ context.Context, _ rt.InvocationRequest) (rt.InvocationResponse, error) {
	return rt.InvocationResponse{OutputJSON: "{}"}, nil
}

func TestSDKRuntime_IsConformant(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	go func() { _ = rt.Serve(lis, conformantHandler{}) }()
	t.Cleanup(func() { _ = lis.Close() })

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	res := conformance.Run(context.Background(), conformance.Config{Conn: conn})
	for _, c := range res.Checks {
		t.Logf("%-26s %-5s %s", c.Name, c.Status, c.Detail)
	}
	require.True(t, res.Passed, "SDK-backed runtime must pass conformance")
}
