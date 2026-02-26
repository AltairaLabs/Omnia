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

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// RuntimeClient wraps the gRPC client for communicating with the runtime sidecar.
type RuntimeClient struct {
	conn   *grpc.ClientConn
	client runtimev1.RuntimeServiceClient
	addr   string
}

// RuntimeClientConfig contains configuration for the runtime client.
type RuntimeClientConfig struct {
	// Address is the runtime gRPC server address (e.g., "localhost:9000").
	Address string
	// DialTimeout is the timeout for establishing the connection.
	DialTimeout time.Duration
	// MaxMessageSize is the maximum message size in bytes (default 16MB).
	MaxMessageSize int
}

// NewRuntimeClient creates a new RuntimeClient connected to the runtime sidecar.
func NewRuntimeClient(cfg RuntimeClientConfig) (*RuntimeClient, error) {
	// Use default max message size if not specified
	maxMsgSize := cfg.MaxMessageSize
	if maxMsgSize == 0 {
		maxMsgSize = 16 * 1024 * 1024 // 16MB default
	}

	// Use insecure credentials for localhost sidecar communication.
	// In production, mTLS could be added for enhanced security.
	conn, err := grpc.NewClient(cfg.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxMsgSize),
			grpc.MaxCallSendMsgSize(maxMsgSize),
		),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create runtime client for %s: %w", cfg.Address, err)
	}

	client := &RuntimeClient{
		conn:   conn,
		client: runtimev1.NewRuntimeServiceClient(conn),
		addr:   cfg.Address,
	}

	// Verify connection with a health check
	ctx, cancel := context.WithTimeout(context.Background(), cfg.DialTimeout)
	defer cancel()

	_, err = client.Health(ctx)
	if err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			// Log close error but return the primary connection error
			fmt.Printf("Warning: failed to close connection after health check failure: %v\n", closeErr)
		}
		return nil, fmt.Errorf("failed to connect to runtime at %s: %w", cfg.Address, err)
	}

	return client, nil
}

// Converse opens a bidirectional streaming RPC for conversation.
func (c *RuntimeClient) Converse(ctx context.Context) (runtimev1.RuntimeService_ConverseClient, error) {
	return c.client.Converse(ctx)
}

// Health checks the runtime's health status.
func (c *RuntimeClient) Health(ctx context.Context) (*runtimev1.HealthResponse, error) {
	return c.client.Health(ctx, &runtimev1.HealthRequest{})
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
