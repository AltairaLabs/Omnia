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
	"log/slog"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"

	"github.com/AltairaLabs/PromptKit/sdk"

	"github.com/altairalabs/omnia/pkg/logging"
)

// Option configures a Runtime built by New or FromEnv.
type Option func(*builder)

// builder accumulates the caller-facing configuration for a Runtime before it
// is constructed. It is intentionally private: callers mutate it only through
// Option values.
type builder struct {
	log       logr.Logger
	sdkLogger *slog.Logger
	sdkOpts   []sdk.Option
}

// WithLogger sets the logr.Logger the runtime logs through. When omitted, New
// and FromEnv construct a production Zap-backed logger (respecting LOG_LEVEL)
// and derive the PromptKit SDK's slog.Logger from the same Zap core. Supplying
// a logger here (e.g. logr.Discard() in tests) sets only the logr side; the SDK
// slog logger is left at the server default.
func WithLogger(log logr.Logger) Option {
	return func(b *builder) { b.log = log }
}

// WithSDKOptions threads opaque PromptKit sdk.Option values through to the
// underlying runtime server. This is the extension seam a downstream,
// separate-repo PromptKit runtime uses to bring its own SDK behaviour (for
// example sdk.WithIngestion for A/V pipelines) without Omnia's own source ever
// referencing that option — so Omnia CI stays on the published SDK while the
// downstream repo compiles this package against whatever newer PromptKit it
// depends on.
func WithSDKOptions(o ...sdk.Option) Option {
	return func(b *builder) { b.sdkOpts = append(b.sdkOpts, o...) }
}

// applyOptions folds opts into a fresh builder.
func applyOptions(opts []Option) *builder {
	b := &builder{}
	for _, o := range opts {
		o(b)
	}
	return b
}

// ensureLogger populates the builder's loggers when the caller supplied none.
// It returns a sync cleanup func (nil when the caller provided the logger, since
// the caller then owns its lifecycle) that flushes buffered log entries on
// shutdown. A non-nil error means the Zap logger could not be constructed.
func (b *builder) ensureLogger() (func(), error) {
	if b.log.GetSink() != nil {
		return nil, nil
	}
	zapLog, err := logging.NewZapLogger()
	if err != nil {
		return nil, err
	}
	b.log = zapr.NewLogger(zapLog)
	b.sdkLogger = logging.SlogFromZap(zapLog)
	return func() { _ = zapLog.Sync() }, nil
}
