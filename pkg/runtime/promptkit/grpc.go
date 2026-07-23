/*
Copyright 2026 Altaira Labs.

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

package promptkit

import (
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/stats"

	pkruntime "github.com/altairalabs/omnia/internal/runtime"
	"github.com/altairalabs/omnia/internal/tracing"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// maxGRPCMsgSize is the runtime gRPC message ceiling — 16MB so base64-encoded
// images fit on the facade→runtime channel.
const maxGRPCMsgSize = 16 * 1024 * 1024

// buildGRPCServer constructs the runtime gRPC server with the policy interceptors
// and, optionally, the OpenTelemetry stats handler. Factored out so wiring tests
// can assert that the interceptors are installed on the real server (#714).
func buildGRPCServer(tracingProvider *tracing.Provider) *grpc.Server {
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(maxGRPCMsgSize),
		grpc.MaxSendMsgSize(maxGRPCMsgSize),
		grpc.ChainUnaryInterceptor(pkruntime.PolicyUnaryServerInterceptor()),
		grpc.ChainStreamInterceptor(pkruntime.PolicyStreamServerInterceptor()),
	}
	if tracingProvider != nil {
		opts = append(opts, grpc.StatsHandler(otelgrpc.NewServerHandler(
			otelgrpc.WithTracerProvider(tracingProvider.TracerProvider()),
			otelgrpc.WithFilter(isNotHealthCheck),
		)))
	}
	return grpc.NewServer(opts...)
}

// isNotHealthCheck filters out gRPC health check RPCs from tracing.
func isNotHealthCheck(info *stats.RPCTagInfo) bool {
	return info.FullMethodName != "/omnia.runtime.v1.RuntimeService/Health"
}

// newGRPCServer builds the gRPC server, registers the runtime's RuntimeService
// (with policy interceptors) and the gRPC health service, and marks it SERVING.
// It is the single construction path shared by Serve and the conformance test,
// so the test exercises the same interceptor wiring production uses.
func (r *Runtime) newGRPCServer() *grpc.Server {
	gs := buildGRPCServer(r.tracing)
	runtimev1.RegisterRuntimeServiceServer(gs, r.server)

	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(gs, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	return gs
}
