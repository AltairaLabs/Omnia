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
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/AltairaLabs/PromptKit/runtime/logger"

	pkruntime "github.com/altairalabs/omnia/internal/runtime"
	"github.com/altairalabs/omnia/pkg/k8s"
)

// FromEnv loads the operator-injected OMNIA_* configuration and constructs a
// Runtime from it. It is the entry point a runtime binary's main() calls: the
// operator injects the same environment it does for the built-in runtime. It
// installs the global trace propagator, resolves the pack entry point, and
// best-effort self-reports pack validation + capabilities to the AgentRuntime
// status.
//
// FromEnv depends on the cluster (LoadConfigWithContext reads the AgentRuntime
// CRD via an in-cluster/kubeconfig client) and installs process-global state, so
// it is exercised in-cluster (E2E / wiring) rather than by unit tests; it is
// coverage-excluded like the binaries' main() for the same reason.
func FromEnv(opts ...Option) (*Runtime, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	b := applyOptions(opts)
	logCleanup, err := b.ensureLogger()
	if err != nil {
		return nil, fmt.Errorf("build logger: %w", err)
	}
	if b.sdkLogger != nil {
		logger.SetLogger(b.sdkLogger)
	}

	cfg, err := pkruntime.LoadConfigWithContext(context.Background())
	if err != nil {
		b.log.Error(err, "failed to load configuration")
		runCleanup(logCleanup)
		return nil, err
	}
	cfg.PromptName = pkruntime.ResolvePackEntry(cfg.PromptPackPath, cfg.PromptName, b.log)
	logStartup(b.log, cfg)

	rt, err := newFromBuilder(cfg, b, logCleanup)
	if err != nil {
		runCleanup(logCleanup)
		return nil, err
	}
	rt.reportStartup(context.Background())
	return rt, nil
}

// reportStartup validates pack content and, when the runtime is operator-managed
// (agent name + namespace known), self-reports pack-validation and capabilities
// to the AgentRuntime status. It is part of the FromEnv (in-cluster) entry point
// — it constructs a Kubernetes client — so it lives here with FromEnv and is
// coverage-excluded for the same reason. All failures are best-effort: they log
// and return without affecting serving.
func (r *Runtime) reportStartup(ctx context.Context) {
	warnings := validatePackContent(r.cfg.PromptPackPath, r.evalDefs, r.log)
	if r.cfg.AgentName == "" || r.cfg.Namespace == "" {
		return
	}
	patchCtx, cancel := context.WithTimeout(ctx, statusReportTimeout)
	defer cancel()
	k8sClient, err := k8s.NewClient()
	if err != nil {
		r.log.Error(err, "failed to create k8s client for status reporting")
		return
	}
	reportStartupStatus(patchCtx, r.log, k8sClient, r.cfg.AgentName, r.cfg.Namespace, warnings)
}

// logStartup emits the single structured startup line describing the resolved
// runtime configuration.
func logStartup(log logr.Logger, cfg *pkruntime.Config) {
	log.Info("starting runtime",
		"agent", cfg.AgentName,
		"namespace", cfg.Namespace,
		"grpcPort", cfg.GRPCPort,
		"healthPort", cfg.HealthPort,
		"packPath", cfg.PromptPackPath,
		"promptName", cfg.PromptName,
		"providerType", cfg.ProviderType,
		"model", cfg.Model,
		"baseURL", cfg.BaseURL,
		"mockProvider", cfg.MockProvider,
		"toolsConfigPath", cfg.ToolsConfigPath,
		"tracingEnabled", cfg.TracingEnabled,
		"evalEnabled", cfg.EvalEnabled,
		"sessionAPIURL", cfg.SessionAPIURL,
		"memoryEnabled", cfg.MemoryEnabled)
}
