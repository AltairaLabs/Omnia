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

package tools

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"

	pktools "github.com/AltairaLabs/PromptKit/runtime/tools"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	toolsv1 "github.com/altairalabs/omnia/pkg/tools/v1"
)

// buildGRPCDescriptor populates the descriptor from discovered gRPC tools.
func (e *OmniaExecutor) buildGRPCDescriptor(desc *pktools.ToolDescriptor, toolName, handlerName string) {
	tools, ok := e.grpcTools[handlerName]
	if !ok {
		return
	}
	tool, ok := tools[toolName]
	if !ok {
		return
	}
	desc.Description = tool.Description
	if tool.InputSchema != "" {
		desc.InputSchema = json.RawMessage(tool.InputSchema)
	}
}

func (e *OmniaExecutor) initGRPCHandler(ctx context.Context, name string, h *HandlerEntry) error {
	if h.GRPCConfig == nil {
		e.log.Info("skipping gRPC handler without config", "handler", name)
		return nil
	}

	opts, err := buildGRPCDialOptions(h.GRPCConfig)
	if err != nil {
		return err
	}

	dialCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	conn, err := grpc.DialContext(dialCtx, h.GRPCConfig.Endpoint, opts...) //nolint:staticcheck // SA1019: DialContext preserves eager-dial behaviour; pure move, not changing semantics
	if err != nil {
		return fmt.Errorf("failed to connect gRPC: %w", err)
	}

	client := toolsv1.NewToolServiceClient(conn)
	e.grpcConns[name] = conn
	e.grpcClients[name] = client
	e.grpcTools[name] = make(map[string]*toolsv1.ToolInfo)

	// If tool definition is in the handler config, use it directly
	if h.Tool != nil {
		e.grpcTools[name][h.Tool.Name] = &toolsv1.ToolInfo{
			Name:        h.Tool.Name,
			Description: h.Tool.Description,
		}
		e.toolHandlers[h.Tool.Name] = name
		e.log.V(1).Info("registered gRPC tool", "tool", h.Tool.Name, "handler", name)
		return nil
	}

	// Otherwise discover via ListTools RPC
	resp, err := client.ListTools(ctx, &toolsv1.ListToolsRequest{})
	if err != nil {
		e.log.V(1).Info("gRPC ListTools unavailable", "handler", name, "error", err)
		return nil
	}
	for _, tool := range resp.Tools {
		e.grpcTools[name][tool.Name] = tool
		e.toolHandlers[tool.Name] = name
		e.log.V(1).Info("registered gRPC tool", "tool", tool.Name, "handler", name)
	}
	return nil
}

func buildGRPCDialOptions(cfg *GRPCCfg) ([]grpc.DialOption, error) {
	if !cfg.TLS {
		return []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}, nil
	}
	tlsCfg, err := buildGRPCTLSConfig(cfg)
	if err != nil {
		return nil, err
	}
	return []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))}, nil
}

func buildGRPCTLSConfig(cfg *GRPCCfg) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: cfg.TLSInsecureSkipVerify, //nolint:gosec // user-configured
	}
	if cfg.TLSCAPath != "" {
		caCert, err := os.ReadFile(cfg.TLSCAPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA cert")
		}
		tlsConfig.RootCAs = pool
	}
	if cfg.TLSCertPath != "" && cfg.TLSKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertPath, cfg.TLSKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load client cert: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	return tlsConfig, nil
}

func (e *OmniaExecutor) executeGRPC(
	ctx context.Context,
	toolName, handlerName string,
	args json.RawMessage,
) (json.RawMessage, error) {
	e.mu.RLock()
	client := e.grpcClients[handlerName]
	handler := e.handlers[handlerName]
	e.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("gRPC handler %q not connected", handlerName)
	}

	policy, classify := grpcRetryParams(handler.GRPCConfig)

	return retryWithBackoff(ctx, e.log, e.currentSpan(ctx), policy, handler.Timeout.Get(), classify,
		func(attemptCtx context.Context) (json.RawMessage, error) {
			// Inject policy metadata.
			md := PolicyGRPCMetadata(attemptCtx, toolName, handlerName, nil)
			if len(md) > 0 {
				pairs := make([]string, 0, len(md)*2)
				for k, v := range md {
					pairs = append(pairs, k, v)
				}
				attemptCtx = metadata.AppendToOutgoingContext(attemptCtx, pairs...)
			}

			// Execute through circuit breaker.
			var resp *toolsv1.ToolResponse
			_, cbErr := e.breakers.Execute(toolName, func() ([]byte, error) {
				var execErr error
				resp, execErr = client.Execute(attemptCtx, &toolsv1.ToolRequest{
					ToolName:      toolName,
					ArgumentsJson: string(args),
				})
				return nil, execErr
			})
			if cbErr != nil {
				return nil, fmt.Errorf("gRPC tool execution failed: %w", cbErr)
			}

			return marshalGRPCResponse(resp)
		},
	)
}

func marshalGRPCResponse(resp *toolsv1.ToolResponse) (json.RawMessage, error) {
	if resp.IsError {
		return nil, fmt.Errorf("tool error: %s", resp.ErrorMessage)
	}
	if resp.ResultJson != "" {
		return json.RawMessage(resp.ResultJson), nil
	}
	return json.RawMessage("null"), nil
}
