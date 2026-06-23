/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	pkproviders "github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/trace"

	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
	"github.com/altairalabs/omnia/internal/session"
)

// Status constants for execution results.
const (
	statusPass = "pass"
	statusFail = "fail"
)

// workItemToTraceID derives a deterministic trace ID from a job + item ID.
// Each work item gets its own trace so load tests don't produce a single massive trace.
func workItemToTraceID(jobName, itemID string) trace.TraceID {
	h := sha256.Sum256([]byte(jobName + ":" + itemID))
	var tid trace.TraceID
	copy(tid[:], h[:16])
	return tid
}

// workItemToSpanID derives a deterministic span ID from an item ID.
func workItemToSpanID(itemID string) trace.SpanID {
	h := sha256.Sum256([]byte(itemID))
	var sid trace.SpanID
	copy(sid[:], h[16:24])
	return sid
}

// sessionIDToTraceID converts a UUID session ID to an OpenTelemetry trace ID.
// This mirrors the facade's logic so the arena worker can create span links
// that point to the session-derived trace.
func sessionIDToTraceID(sessionID string) trace.TraceID {
	cleaned := strings.ReplaceAll(sessionID, "-", "")
	var tid trace.TraceID
	_, _ = hex.Decode(tid[:], []byte(cleaned))
	return tid
}

// maxItemTimeout is the maximum time allowed for a single work item execution.
const maxItemTimeout = 10 * time.Minute

const defaultScenarioID = "default"

// Config holds the worker configuration from environment variables.
type Config struct {
	// Job identification
	JobName      string
	JobNamespace string
	ConfigName   string
	JobType      string

	// Filesystem content configuration
	// ContentPath is the mount point for the job's content (e.g., /workspace-content)
	// The content is isolated via subPath to only show the job's root folder
	ContentPath    string
	ContentVersion string // Content-addressable version hash
	ConfigFile     string // Arena config filename within the content path

	// Redis configuration. URL form (redis:// or rediss://); host/port/
	// auth/TLS/db-index all encoded per RFC 7595. Comes from REDIS_URL
	// env which the operator's ArenaJobReconciler sets on the worker
	// pod (literal value or secretKeyRef).
	RedisURL string

	// Session recording
	SessionAPIURL string // Optional session-api URL for recording arena sessions
	WorkspaceName string // Workspace name (resolved from namespace label)

	// MgmtPlaneTokenURL is the dashboard's /api/auth/service-token endpoint.
	// When set, the worker fetches a mgmt-plane JWT (presenting its own SA
	// token) and attaches it as a Bearer credential on fleet-mode WS dials, so
	// it can authenticate to agent facades that enforce mgmt-plane auth. Empty
	// → fleet dials are unauthenticated (installs without enforcement).
	MgmtPlaneTokenURL string

	// Worker configuration
	WorkDir       string
	PollInterval  time.Duration
	ShutdownDelay time.Duration
	Verbose       bool // Enable verbose/debug output from promptarena

	// VU pool configuration
	VUsPerWorker int           // Number of virtual users (goroutines) per worker, default 1
	Concurrency  int           // Global concurrency limit (0 = unlimited)
	RampUp       time.Duration // Ramp-up duration (0 = no ramp-up)
	RampDown     time.Duration // Ramp-down duration (0 = no ramp-down)

	// Output configuration
	// OutputConfig is parsed from the ARENA_OUTPUT_CONFIG env var (JSON-encoded OutputConfig).
	// When nil, output is written to /tmp/arena-output and discarded when the pod exits.
	OutputConfig *omniav1alpha1.OutputConfig
	// OutputDir is the mount path injected by the controller for PVC output.
	// Populated from the ARENA_OUTPUT_DIR env var.
	OutputDir string

	// Override configurations (resolved from CRDs)
	ToolOverrides map[string]ToolOverrideConfig // Tool name -> override config

	// K8sClient is an optional pre-configured k8s client for testing.
	// When nil, the worker creates one via k8s.NewClient() (in-cluster config).
	K8sClient client.Client
}

// ToolOverrideConfig contains the configuration for a tool override from ToolRegistry CRD.
type ToolOverrideConfig struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Endpoint     string `json:"endpoint,omitempty"`
	HandlerName  string `json:"handlerName"`
	RegistryName string `json:"registryName"`
	HandlerType  string `json:"handlerType,omitempty"`
}

// ExecutionResult represents the result of running a scenario.
type ExecutionResult struct {
	Status     string             `json:"status"`
	DurationMs float64            `json:"durationMs"`
	Error      string             `json:"error,omitempty"`
	Metrics    map[string]float64 `json:"metrics,omitempty"`
	Assertions []AssertionResult  `json:"assertions,omitempty"`
	SessionID  string             `json:"sessionId,omitempty"`
}

// AssertionResult represents a single assertion result.
type AssertionResult struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
}

func loadConfig() (*Config, error) {
	cfg := &Config{
		JobName:           os.Getenv("ARENA_JOB_NAME"),
		JobNamespace:      os.Getenv("ARENA_JOB_NAMESPACE"),
		ConfigName:        os.Getenv("ARENA_CONFIG_NAME"),
		JobType:           os.Getenv("ARENA_JOB_TYPE"),
		ContentPath:       os.Getenv("ARENA_CONTENT_PATH"),
		ContentVersion:    os.Getenv("ARENA_CONTENT_VERSION"),
		ConfigFile:        os.Getenv("ARENA_CONFIG_FILE"), // Config file name in content path
		SessionAPIURL:     os.Getenv("SESSION_API_URL"),
		WorkspaceName:     os.Getenv("ARENA_WORKSPACE_NAME"),
		MgmtPlaneTokenURL: os.Getenv("OMNIA_MGMT_PLANE_SERVICE_TOKEN_URL"),
		RedisURL:          os.Getenv("REDIS_URL"),
		WorkDir:           getEnvOrDefault("ARENA_WORK_DIR", "/tmp/arena"),
		PollInterval:      getDurationEnv("ARENA_POLL_INTERVAL", 100*time.Millisecond),
		ShutdownDelay:     getDurationEnv("ARENA_SHUTDOWN_DELAY", 65*time.Second),
		Verbose:           os.Getenv("ARENA_VERBOSE") == "true",
	}

	cfg.VUsPerWorker = getIntEnvOrDefault("ARENA_VUS_PER_WORKER", 1)
	cfg.Concurrency = getIntEnvOrDefault("ARENA_CONCURRENCY", 0)
	cfg.RampUp = getDurationEnv("ARENA_RAMP_UP", 0)
	cfg.RampDown = getDurationEnv("ARENA_RAMP_DOWN", 0)

	// Output configuration — optional; defaults to /tmp/arena-output (lost on pod exit).
	cfg.OutputDir = os.Getenv("ARENA_OUTPUT_DIR")
	if raw := os.Getenv("ARENA_OUTPUT_CONFIG"); raw != "" {
		var outCfg omniav1alpha1.OutputConfig
		if err := json.Unmarshal([]byte(raw), &outCfg); err != nil {
			return nil, fmt.Errorf("failed to parse ARENA_OUTPUT_CONFIG: %w", err)
		}
		cfg.OutputConfig = &outCfg
	}

	if cfg.JobName == "" {
		return nil, errors.New("ARENA_JOB_NAME is required")
	}
	if cfg.ContentPath == "" {
		return nil, errors.New("ARENA_CONTENT_PATH is required")
	}

	return cfg, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getIntEnvOrDefault(key string, defaultValue int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultValue
}

// workerLoopContext bundles the plumbing (ctx, logger, config, queue, job id)
// threaded through the work-loop helpers. Extracted to keep executeAndReport
// and handlePopError under Sonar's 7-param threshold (go:S107).
type workerLoopContext struct {
	ctx   context.Context
	log   logr.Logger
	cfg   *Config
	queue queue.WorkQueue
	jobID string
}

type workItemExecutor func(context.Context, *queue.WorkItem) (*ExecutionResult, error)

type workItemReporter func(context.Context, *queue.WorkItem, *ExecutionResult, error)

// fleetSessionInputs bundles the inputs for resolving/finalizing the session of a
// work item, keeping the call site and helper signatures readable.
type fleetSessionInputs struct {
	loadTestFleet bool
	meta          arenaSessionMetadata
	personaIDs    []string
	selfPlayCalls []session.ProviderCall
	sessionMgr    *arenaSessionManager
	registry      *pkproviders.Registry
	fleet         []*resolvedFleetProvider
}

// runAggregator collects and aggregates run results.
type runAggregator struct {
	passCount     int
	failCount     int
	errors        []string
	totalDuration time.Duration
	assertions    []AssertionResult
	inputTokens   int
	outputTokens  int
	log           logr.Logger
}

// Token metric keys extracted from ExecutionResult.Metrics.
const (
	metricKeyInputTokens  = "totalInputTokens"
	metricKeyOutputTokens = "totalOutputTokens"
	metricKeyCost         = "totalCost"
	metricKeyTTFT         = "ttftSeconds"
)

// providerPricing holds parsed pricing from a Provider CRD.
type providerPricing struct {
	inputCostPer1K  float64
	outputCostPer1K float64
}

// toolConfigWrapper wraps tool configuration for YAML parsing/serialization.
type toolConfigWrapper struct {
	APIVersion string         `yaml:"apiVersion"`
	Kind       string         `yaml:"kind"`
	Metadata   toolMetadata   `yaml:"metadata"`
	Spec       toolSpecConfig `yaml:"spec"`
}

type toolMetadata struct {
	Name string `yaml:"name"`
}

type toolSpecConfig struct {
	Name         string                 `yaml:"name,omitempty"`
	Description  string                 `yaml:"description,omitempty"`
	InputSchema  map[string]interface{} `yaml:"input_schema,omitempty"`
	OutputSchema map[string]interface{} `yaml:"output_schema,omitempty"`
	Mode         string                 `yaml:"mode,omitempty"`
	TimeoutMs    int                    `yaml:"timeout_ms,omitempty"`
	MockResult   interface{}            `yaml:"mock_result,omitempty"`
	MockTemplate string                 `yaml:"mock_template,omitempty"`
	HTTP         *toolHTTPConfig        `yaml:"http,omitempty"`
}

type toolHTTPConfig struct {
	URL            string            `yaml:"url"`
	Method         string            `yaml:"method,omitempty"`
	Headers        map[string]string `yaml:"headers,omitempty"`
	HeadersFromEnv []string          `yaml:"headers_from_env,omitempty"`
	TimeoutMs      int               `yaml:"timeout_ms,omitempty"`
}
