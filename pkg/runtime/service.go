package runtime

import (
	"context"

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

// Converse is implemented in Task 3.
func (a *serviceAdapter) Converse(_ runtimev1.RuntimeService_ConverseServer) error {
	return status.Error(codes.Unimplemented, "converse not yet implemented")
}
