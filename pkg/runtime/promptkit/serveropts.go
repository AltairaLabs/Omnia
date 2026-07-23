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
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"

	"github.com/AltairaLabs/PromptKit/runtime/evals"
	"github.com/AltairaLabs/PromptKit/runtime/statestore"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	pkruntime "github.com/altairalabs/omnia/internal/runtime"
	"github.com/altairalabs/omnia/internal/runtime/tools"
)

// envPromptPackManifestPath points at the operator-emitted skill manifest. An
// empty value or missing file is a no-op — skills are optional.
const envPromptPackManifestPath = "OMNIA_PROMPTPACK_MANIFEST_PATH"

// configDerivedServerOpts returns the pkruntime.ServerOption slice derived
// solely from cfg fields (no logger, store, collector, etc.). Factored out so
// wiring tests can assert that cfg fields with real production impact —
// especially MediaBasePath — actually reach the runtime server. If you add a
// new cfg.Xxx field that the runtime needs, add the corresponding
// pkruntime.WithXxx here. See #728.
func configDerivedServerOpts(cfg *pkruntime.Config) []pkruntime.ServerOption {
	return []pkruntime.ServerOption{
		pkruntime.WithPackPath(cfg.PromptPackPath),
		pkruntime.WithPromptName(cfg.PromptName),
		pkruntime.WithAgentIdentity(cfg.AgentName, cfg.Namespace),
		pkruntime.WithAgentUID(cfg.AgentUID),
		pkruntime.WithPromptPackName(cfg.PromptPackName),
		pkruntime.WithModel(cfg.Model),
		pkruntime.WithMockProvider(cfg.MockProvider),
		pkruntime.WithMockConfigPath(cfg.MockConfigPath),
		pkruntime.WithToolsConfig(cfg.ToolsConfigPath),
		pkruntime.WithProviderInfo(cfg.ProviderType, cfg.Model),
		pkruntime.WithProviderAPIKey(cfg.ProviderAPIKey),
		pkruntime.WithProviderRefName(cfg.ProviderRefName),
		pkruntime.WithExtraProviders(cfg.ExtraProviders),
		pkruntime.WithBaseURL(cfg.BaseURL),
		pkruntime.WithHeaders(cfg.Headers),
		pkruntime.WithPlatform(pkruntime.PlatformConfig{
			Type:     cfg.PlatformType,
			Region:   cfg.PlatformRegion,
			Project:  cfg.PlatformProject,
			Endpoint: cfg.PlatformEndpoint,
		}),
		pkruntime.WithAuth(pkruntime.AuthConfig{
			Type:                       cfg.AuthType,
			RoleArn:                    cfg.AuthRoleArn,
			ServiceAccountEmail:        cfg.AuthServiceAccountEmail,
			CredentialsSecretName:      cfg.AuthCredentialsSecretName,
			CredentialsSecretKey:       cfg.AuthCredentialsSecretKey,
			CredentialsSecretNamespace: cfg.Namespace,
		}),
		pkruntime.WithProviderRequestTimeout(cfg.ProviderRequestTimeout),
		pkruntime.WithProviderStreamIdleTimeout(cfg.ProviderStreamIdleTimeout),
		pkruntime.WithPricing(cfg.InputCostPer1K, cfg.OutputCostPer1K),
		pkruntime.WithContextWindow(cfg.ContextWindow),
		pkruntime.WithTruncationStrategy(cfg.TruncationStrategy),
		pkruntime.WithMediaBasePath(cfg.MediaBasePath),
		pkruntime.WithMemoryRetrieval(cfg.MemoryStrategy, cfg.MemoryDenyCEL, cfg.MemoryLimit),
		pkruntime.WithFunctionOutputFormat(cfg.Mode, cfg.OutputFormat, cfg.OutputSchemaJSON),
		pkruntime.WithDuplexAudio(cfg.DuplexAudio),
	}
}

// newStateStore builds the conversation state store selected by cfg.ContextType.
// A "memory" type yields an in-process store; "redis" connects and pings the
// configured URL. Any other value (including empty) yields a nil store, matching
// the runtime's historical behaviour of leaving the server's state store unset.
func newStateStore(cfg *pkruntime.Config, log logr.Logger) (statestore.Store, error) {
	switch cfg.ContextType {
	case pkruntime.ContextTypeMemory:
		log.Info("using in-memory state store", "contextTTL", cfg.ContextTTL)
		return statestore.NewMemoryStore(memoryStoreOptions(cfg.ContextTTL)...), nil
	case pkruntime.ContextTypeRedis:
		return newRedisStore(cfg, log)
	default:
		return nil, nil
	}
}

// newRedisStore parses cfg.ContextURL, connects, instruments tracing, and pings
// before returning a Redis-backed state store.
func newRedisStore(cfg *pkruntime.Config, log logr.Logger) (statestore.Store, error) {
	opts, err := redis.ParseURL(cfg.ContextURL)
	if err != nil {
		return nil, err
	}
	client := redis.NewClient(opts)
	if err := redisotel.InstrumentTracing(client); err != nil {
		log.Error(err, "failed to instrument redis tracing")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	log.Info("using Redis state store", "url", cfg.ContextURL, "contextTTL", cfg.ContextTTL)
	return statestore.NewRedisStore(client, redisStoreOptions(cfg.ContextTTL)...), nil
}

// memoryStoreOptions and redisStoreOptions translate spec.context.ttl — how
// long a conversation's working context survives between messages — into store
// construction options. A non-positive TTL is left to the store default rather
// than forwarded, because zero means "never expire" to both stores; silently
// unbounded retention is a worse failure than falling back to the default.
func memoryStoreOptions(ttl time.Duration) []statestore.MemoryStoreOption {
	if ttl <= 0 {
		return nil
	}
	return []statestore.MemoryStoreOption{statestore.WithMemoryTTL(ttl)}
}

func redisStoreOptions(ttl time.Duration) []statestore.RedisOption {
	if ttl <= 0 {
		return nil
	}
	return []statestore.RedisOption{statestore.WithTTL(ttl)}
}

// loadEvalDefs loads pack-level and prompt-level eval definitions when evals are
// enabled, logging (but not failing) on load errors and surfacing eval types
// with no registered handler. Returns nil when evals are disabled.
func loadEvalDefs(cfg *pkruntime.Config, log logr.Logger) []evals.EvalDef {
	if !cfg.EvalEnabled {
		return nil
	}
	defs, err := pkruntime.LoadAllEvalDefs(cfg.PromptPackPath)
	if err != nil {
		log.Error(err, "failed to load eval definitions from pack, continuing without evals")
		return nil
	}
	log.Info("evals enabled", "evalCount", len(defs))
	if missing := pkruntime.ValidateEvalDefs(defs); len(missing) > 0 {
		log.Error(errUnregisteredEvalTypes,
			"some eval types in the pack have no registered handler and will fail at runtime",
			"missingTypes", missing, "evalCount", len(defs))
	}
	return defs
}

// warnIfCustomTruncation flags the one combination where truncationStrategy
// silently does nothing: "custom" tells the runtime to implement truncation
// itself, but this PromptKit runtime has no custom implementation, so no
// truncation is applied at all and context grows until the provider rejects the
// request. The field is valid for custom runtimes (spec.framework.type: custom),
// which is why this warns rather than failing.
func warnIfCustomTruncation(log logr.Logger, strategy string) {
	if strategy != string(omniav1alpha1.TruncationStrategyCustom) {
		return
	}
	log.Info("truncation disabled",
		"reason", "customStrategyOnPromptKitRuntime",
		"truncationStrategy", strategy,
		"impact", "no truncation applied; context may exceed the provider limit",
		"remedy", "use sliding or summarize, or run a custom runtime that implements truncation")
}

// enrichToolRegistryMeta records ToolRegistry provenance on the tool manager so
// tool spans, metrics, and ToolPolicy `registry:` selectors carry the registry
// name rather than the handler name.
//
// The registry name and namespace come from the AgentRuntime spec via Config —
// NOT from a ToolRegistry API read, which would 403 cross-namespace and silently
// fall back to matching on the handler name (#1874). Handler metadata only feeds
// tool spans/labels; policy enforcement needs just the registry name, so a
// tools-config reload failure still records the registry and stays fail-closed.
func enrichToolRegistryMeta(cfg *pkruntime.Config, server *pkruntime.Server, log logr.Logger) {
	toolsCfg, err := tools.LoadConfig(cfg.ToolsConfigPath)
	if err != nil {
		log.Error(err, "tools config reload failed; recording registry provenance without handler metadata",
			"registryName", cfg.ToolRegistryName, "reason", "policy enforcement stays fail-closed")
		server.SetToolRegistryInfo(cfg.ToolRegistryName, cfg.ToolRegistryNamespace, nil)
		return
	}
	server.SetToolRegistryInfo(cfg.ToolRegistryName, cfg.ToolRegistryNamespace, toolsCfg.Handlers)
	log.Info("tool registry metadata enriched",
		"registryName", cfg.ToolRegistryName, "registryNamespace", cfg.ToolRegistryNamespace)
}

// initTools loads and connects tool backends when a tools config path is set,
// then best-effort enriches ToolRegistry provenance. Tool init failures are
// logged and swallowed — tools are optional and must not stop the runtime.
func initTools(cfg *pkruntime.Config, server *pkruntime.Server, log logr.Logger) {
	if cfg.ToolsConfigPath == "" {
		log.V(1).Info("tools disabled (no config path specified)")
		return
	}
	initCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.InitializeTools(initCtx); err != nil {
		log.Error(err, "failed to initialize tools", "configPath", cfg.ToolsConfigPath)
		return
	}
	log.Info("tools initialized", "configPath", cfg.ToolsConfigPath)
	if cfg.ToolRegistryName != "" {
		enrichToolRegistryMeta(cfg, server, log)
	}
}

// getEnvOrDefault returns the env var value or def when unset/empty.
func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
