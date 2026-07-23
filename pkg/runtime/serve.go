package runtime

import (
	"net"

	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// defaultMaxMessageSize matches the first-party runtime: 16MB, to carry
// base64-encoded images.
const defaultMaxMessageSize = 16 * 1024 * 1024

type serveConfig struct {
	maxMessageSize int
	serverOptions  []grpc.ServerOption
}

// Option configures Serve.
type Option func(*serveConfig)

// WithMaxMessageSize overrides the max gRPC send/recv message size (bytes).
func WithMaxMessageSize(n int) Option {
	return func(c *serveConfig) { c.maxMessageSize = n }
}

// WithServerOptions appends extra grpc.ServerOptions (e.g. an otel stats handler
// or custom interceptors) to the server the SDK builds.
func WithServerOptions(o ...grpc.ServerOption) Option {
	return func(c *serveConfig) { c.serverOptions = append(c.serverOptions, o...) }
}

// Serve registers h as an omnia.runtime.v1 RuntimeService on a new gRPC server
// and serves it on lis until lis is closed or a fatal error occurs. It also
// registers the standard grpc health service in SERVING state. Serve blocks.
func Serve(lis net.Listener, h Handler, opts ...Option) error {
	cfg := serveConfig{maxMessageSize: defaultMaxMessageSize}
	for _, o := range opts {
		o(&cfg)
	}

	serverOpts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(cfg.maxMessageSize),
		grpc.MaxSendMsgSize(cfg.maxMessageSize),
	}
	serverOpts = append(serverOpts, cfg.serverOptions...)

	server := grpc.NewServer(serverOpts...)
	runtimev1.RegisterRuntimeServiceServer(server, &serviceAdapter{handler: h})

	hs := health.NewServer()
	healthpb.RegisterHealthServer(server, hs)
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	if err := server.Serve(lis); err != nil && err != grpc.ErrServerStopped {
		return err
	}
	return nil
}
