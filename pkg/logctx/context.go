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

// Package logctx provides structured logging context management.
// It allows storing and extracting common logging fields from context.Context,
// enabling consistent logging across the facade and runtime components.
package logctx

import (
	"context"

	"github.com/go-logr/logr"
)

// contextKey is a private type for context keys to avoid collisions.
type contextKey string

// Context keys for common logging fields.
// These keys are used to store values in context.Context that will be
// automatically extracted and added to log entries.
const (
	// ContextKeySessionID identifies the user session.
	ContextKeySessionID contextKey = "session_id"

	// ContextKeyRequestID identifies the individual request.
	ContextKeyRequestID contextKey = "request_id"

	// ContextKeyCorrelationID is used for distributed tracing.
	ContextKeyCorrelationID contextKey = "correlation_id"

	// ContextKeyAgent identifies the agent name.
	ContextKeyAgent contextKey = "agent"

	// ContextKeyNamespace identifies the Kubernetes namespace.
	ContextKeyNamespace contextKey = "namespace"

	// ContextKeyProvider identifies the LLM provider (e.g., "openai", "anthropic").
	ContextKeyProvider contextKey = "provider"

	// ContextKeyModel identifies the specific model being used.
	ContextKeyModel contextKey = "model"

	// ContextKeyHandler identifies the message handler.
	ContextKeyHandler contextKey = "handler"

	// ContextKeyTool identifies a tool being called.
	ContextKeyTool contextKey = "tool"

	// ContextKeyStage identifies the processing stage.
	ContextKeyStage contextKey = "stage"
)

// allContextKeys lists all context keys that should be extracted for logging.
var allContextKeys = []contextKey{
	ContextKeySessionID,
	ContextKeyRequestID,
	ContextKeyCorrelationID,
	ContextKeyAgent,
	ContextKeyNamespace,
	ContextKeyProvider,
	ContextKeyModel,
	ContextKeyHandler,
	ContextKeyTool,
	ContextKeyStage,
}

// WithSessionID returns a new context with the session ID set.
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, ContextKeySessionID, sessionID)
}

// WithRequestID returns a new context with the request ID set.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, ContextKeyRequestID, requestID)
}

// WithCorrelationID returns a new context with the correlation ID set.
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, ContextKeyCorrelationID, correlationID)
}

// WithAgent returns a new context with the agent name set.
func WithAgent(ctx context.Context, agent string) context.Context {
	return context.WithValue(ctx, ContextKeyAgent, agent)
}

// WithNamespace returns a new context with the namespace set.
func WithNamespace(ctx context.Context, namespace string) context.Context {
	return context.WithValue(ctx, ContextKeyNamespace, namespace)
}

// WithProvider returns a new context with the provider name set.
func WithProvider(ctx context.Context, provider string) context.Context {
	return context.WithValue(ctx, ContextKeyProvider, provider)
}

// WithModel returns a new context with the model name set.
func WithModel(ctx context.Context, model string) context.Context {
	return context.WithValue(ctx, ContextKeyModel, model)
}

// WithHandler returns a new context with the handler name set.
func WithHandler(ctx context.Context, handler string) context.Context {
	return context.WithValue(ctx, ContextKeyHandler, handler)
}

// WithTool returns a new context with the tool name set.
func WithTool(ctx context.Context, tool string) context.Context {
	return context.WithValue(ctx, ContextKeyTool, tool)
}

// WithStage returns a new context with the processing stage set.
func WithStage(ctx context.Context, stage string) context.Context {
	return context.WithValue(ctx, ContextKeyStage, stage)
}

// LoggingFields holds all standard logging context fields.
// This struct is used with WithLoggingContext for bulk field setting.
type LoggingFields struct {
	SessionID     string
	RequestID     string
	CorrelationID string
	Agent         string
	Namespace     string
	Provider      string
	Model         string
	Handler       string
	Tool          string
	Stage         string
}

// WithLoggingContext returns a new context with multiple logging fields set at once.
// Only non-empty values are set.
func WithLoggingContext(ctx context.Context, fields *LoggingFields) context.Context {
	if fields == nil {
		return ctx
	}
	if fields.SessionID != "" {
		ctx = WithSessionID(ctx, fields.SessionID)
	}
	if fields.RequestID != "" {
		ctx = WithRequestID(ctx, fields.RequestID)
	}
	if fields.CorrelationID != "" {
		ctx = WithCorrelationID(ctx, fields.CorrelationID)
	}
	if fields.Agent != "" {
		ctx = WithAgent(ctx, fields.Agent)
	}
	if fields.Namespace != "" {
		ctx = WithNamespace(ctx, fields.Namespace)
	}
	if fields.Provider != "" {
		ctx = WithProvider(ctx, fields.Provider)
	}
	if fields.Model != "" {
		ctx = WithModel(ctx, fields.Model)
	}
	if fields.Handler != "" {
		ctx = WithHandler(ctx, fields.Handler)
	}
	if fields.Tool != "" {
		ctx = WithTool(ctx, fields.Tool)
	}
	if fields.Stage != "" {
		ctx = WithStage(ctx, fields.Stage)
	}
	return ctx
}

// ExtractLoggingFields extracts all logging fields from a context.
func ExtractLoggingFields(ctx context.Context) LoggingFields {
	fields := LoggingFields{}
	if v := ctx.Value(ContextKeySessionID); v != nil {
		fields.SessionID, _ = v.(string)
	}
	if v := ctx.Value(ContextKeyRequestID); v != nil {
		fields.RequestID, _ = v.(string)
	}
	if v := ctx.Value(ContextKeyCorrelationID); v != nil {
		fields.CorrelationID, _ = v.(string)
	}
	if v := ctx.Value(ContextKeyAgent); v != nil {
		fields.Agent, _ = v.(string)
	}
	if v := ctx.Value(ContextKeyNamespace); v != nil {
		fields.Namespace, _ = v.(string)
	}
	if v := ctx.Value(ContextKeyProvider); v != nil {
		fields.Provider, _ = v.(string)
	}
	if v := ctx.Value(ContextKeyModel); v != nil {
		fields.Model, _ = v.(string)
	}
	if v := ctx.Value(ContextKeyHandler); v != nil {
		fields.Handler, _ = v.(string)
	}
	if v := ctx.Value(ContextKeyTool); v != nil {
		fields.Tool, _ = v.(string)
	}
	if v := ctx.Value(ContextKeyStage); v != nil {
		fields.Stage, _ = v.(string)
	}
	return fields
}

// LogrValues extracts context values and returns them as key-value pairs
// suitable for use with logr.Logger.WithValues().
// Only non-empty values are included.
func LogrValues(ctx context.Context) []interface{} {
	var values []interface{}
	for _, key := range allContextKeys {
		if v := ctx.Value(key); v != nil {
			if s, ok := v.(string); ok && s != "" {
				values = append(values, string(key), s)
			}
		}
	}
	return values
}

// LoggerWithContext returns a logger enriched with all context values.
// This is a convenience function for logr.Logger.
func LoggerWithContext(log logr.Logger, ctx context.Context) logr.Logger {
	values := LogrValues(ctx)
	if len(values) == 0 {
		return log
	}
	return log.WithValues(values...)
}

// SessionID extracts the session ID from the context.
func SessionID(ctx context.Context) string {
	if v := ctx.Value(ContextKeySessionID); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// RequestID extracts the request ID from the context.
func RequestID(ctx context.Context) string {
	if v := ctx.Value(ContextKeyRequestID); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// Agent extracts the agent name from the context.
func Agent(ctx context.Context) string {
	if v := ctx.Value(ContextKeyAgent); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// Namespace extracts the namespace from the context.
func Namespace(ctx context.Context) string {
	if v := ctx.Value(ContextKeyNamespace); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
