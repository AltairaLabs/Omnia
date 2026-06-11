package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	goredis "github.com/redis/go-redis/v9"

	"github.com/altairalabs/omnia/internal/doctor"
	"github.com/altairalabs/omnia/internal/doctor/checks"
	memoryhttpclient "github.com/altairalabs/omnia/internal/memory/httpclient"
	"github.com/altairalabs/omnia/internal/mgmtplane"
	"github.com/altairalabs/omnia/internal/session/httpclient"
	"github.com/altairalabs/omnia/pkg/k8s"
	"github.com/altairalabs/omnia/pkg/logging"
	"github.com/altairalabs/omnia/pkg/servicediscovery"
)

const (
	defaultNamespace      = "omnia-system"
	defaultAgentNamespace = "omnia-demo"
	defaultAgentName      = "tools-demo"
	defaultAPIPort        = 8080

	serviceSessionAPI      = "omnia-session-api"
	serviceMemoryAPI       = "omnia-memory-api"
	serviceOllama          = "ollama"
	serviceOperator        = "omnia-operator"
	serviceDashboard       = "omnia-dashboard"
	serviceArenaController = "omnia-arena-controller"
	defaultOllamaPort      = 11434
	defaultOperatorPort    = 8083
	defaultDashboardPort   = 3000
	defaultArenaPort       = 8082
)

func discoverServiceURL(namespace, service string, port int) string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", service, namespace, port)
}

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	runOnce := flag.Bool("run-once", false, "run all checks once, print JSON results, and exit")
	exitCode := flag.Bool("exit-code", false, "when combined with --run-once, exit 1 if any check fails")
	namespace := flag.String("namespace", defaultNamespace, "Omnia system namespace")
	agentNamespace := flag.String("agent-namespace", defaultAgentNamespace, "agent namespace to test")
	agentName := flag.String("agent-name", defaultAgentName, "agent name to test")
	sessionAPIURLFlag := flag.String("session-api-url", "", "override session-api URL")
	memoryAPIURLFlag := flag.String("memory-api-url", "", "override memory-api URL")
	ollamaURLFlag := flag.String("ollama-url", "", "override Ollama URL")
	operatorURLFlag := flag.String("operator-url", "", "override operator API URL")
	dashboardURLFlag := flag.String("dashboard-url", "", "override dashboard URL")
	redisURLFlag := flag.String("redis-url", os.Getenv("REDIS_URL"),
		"Redis URL (redis:// or rediss://). When unset, the Redis reachability "+
			"check is skipped — installs without Redis (no memory cache, no session "+
			"store, no Arena queue) are a valid OSS configuration. Chart wires this "+
			"from omnia.redis.url when a Redis target resolves; operators can also "+
			"set it explicitly. The TCPCheck dials host:port extracted from the URL "+
			"via goredis.ParseURL; the Doctor doesn't authenticate.")
	arenaURLFlag := flag.String("arena-url", "", "override arena controller URL")
	workspaceFlag := flag.String("workspace", "", "workspace name for per-workspace service discovery (optional)")
	serviceGroupFlag := flag.String("service-group", "default", "service group to resolve within the workspace")
	mgmtPlaneTokenURL := flag.String(
		"mgmt-plane-token-url",
		os.Getenv("OMNIA_MGMT_PLANE_TOKEN_URL"),
		"URL of the dashboard's service-token endpoint, e.g. "+
			"http://omnia-dashboard.omnia-system.svc.cluster.local:3000/api/auth/service-token. "+
			"When set, Doctor exchanges its k8s service-account token for a "+
			"mgmt-plane JWT and attaches it to every WebSocket dial. "+
			"Defaults to OMNIA_MGMT_PLANE_TOKEN_URL env var; empty "+
			"disables the Authorization header (anonymous installs).")
	flag.Parse()

	log, sync, err := logging.NewLogger()
	if err != nil {
		os.Exit(1)
	}
	defer sync()

	sessionAPIURL := *sessionAPIURLFlag
	if sessionAPIURL == "" {
		sessionAPIURL = discoverServiceURL(*namespace, serviceSessionAPI, defaultAPIPort)
	}

	memoryAPIURL := *memoryAPIURLFlag
	if memoryAPIURL == "" {
		memoryAPIURL = discoverServiceURL(*namespace, serviceMemoryAPI, defaultAPIPort)
	}

	ollamaURL := *ollamaURLFlag
	if ollamaURL == "" {
		ollamaURL = discoverServiceURL(*agentNamespace, serviceOllama, defaultOllamaPort)
	}

	operatorURL := *operatorURLFlag
	if operatorURL == "" {
		operatorURL = discoverServiceURL(*namespace, serviceOperator, defaultOperatorPort)
	}

	dashboardURL := *dashboardURLFlag
	if dashboardURL == "" {
		dashboardURL = discoverServiceURL(*namespace, serviceDashboard, defaultDashboardPort)
	}

	// No auto-derivation — Redis is consumer-optional. The check is
	// registered only when a non-empty URL arrives via the flag, so a
	// fresh OSS install (no Redis) doesn't surface a fake "Redis
	// unreachable" failure. URL → host:port via ParseURL because the
	// TCPCheck takes a dial address (auth is irrelevant for a TCP
	// reachability probe).
	redisAddr := ""
	if *redisURLFlag != "" {
		opts, parseErr := goredis.ParseURL(*redisURLFlag)
		if parseErr != nil {
			log.Error(parseErr, "invalid --redis-url", "value", *redisURLFlag)
			os.Exit(1)
		}
		redisAddr = opts.Addr
	}

	arenaURL := *arenaURLFlag
	if arenaURL == "" {
		arenaURL = discoverServiceURL(*namespace, serviceArenaController, defaultArenaPort)
	}

	// Build the mgmt-plane fetcher once at startup. The fetcher itself
	// makes no network calls until first use — failing here would
	// conflate "no dashboard service-token endpoint configured" (which
	// is OK for installs without service principals) with "Doctor
	// misconfigured" (which is the case we want loud at first dial).
	// Token retrieval / SA-token read happens inside Token().
	var mgmtPlaneFetcher *mgmtplane.TokenFetcher
	if *mgmtPlaneTokenURL != "" {
		f, fetchErr := mgmtplane.NewTokenFetcher(mgmtplane.FetcherOptions{
			Endpoint: *mgmtPlaneTokenURL,
		})
		if fetchErr != nil {
			log.Error(fetchErr, "mgmt-plane fetcher init failed")
			os.Exit(1)
		}
		mgmtPlaneFetcher = f
		log.Info("mgmt-plane fetcher enabled", "endpoint", *mgmtPlaneTokenURL)
	} else {
		log.V(1).Info("mgmt-plane fetcher disabled — WebSocket checks will dial without Authorization",
			"reason", "no token endpoint configured",
			"hint", "set --mgmt-plane-token-url or "+
				"OMNIA_MGMT_PLANE_TOKEN_URL to the dashboard's "+
				"/api/auth/service-token endpoint for clusters whose "+
				"facade enforces mgmt-plane JWTs")
	}

	cfg := runnerConfig{
		log:               log,
		namespace:         *namespace,
		agentNamespace:    *agentNamespace,
		agentName:         *agentName,
		workspace:         *workspaceFlag,
		serviceGroup:      *serviceGroupFlag,
		sessionAPIBaseURL: sessionAPIURL,
		memoryAPIBaseURL:  memoryAPIURL,
		ollamaURL:         ollamaURL,
		operatorURL:       operatorURL,
		dashboardURL:      dashboardURL,
		redisAddr:         redisAddr,
		arenaURL:          arenaURL,
		mgmtPlaneFetcher:  mgmtPlaneFetcher,
	}

	// build is invoked per /api/v1/run request — see issue #1040. A
	// startup-only build means a Doctor pod that came up before its
	// Workspace existed permanently uses the global fallback URLs and
	// every Memory / Sessions / Privacy check returns "no such host".
	build := func(_ context.Context) (*doctor.Runner, error) {
		return buildRunner(cfg)
	}

	if *runOnce {
		runner, err := buildRunner(cfg)
		if err != nil {
			log.Error(err, "build runner failed")
			os.Exit(1)
		}
		runOnceMode(runner, log, *exitCode)
		return
	}

	srv := doctor.NewServer(build, *addr, log)
	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("doctor starting", "addr", *addr)
		if srvErr := httpSrv.ListenAndServe(); srvErr != nil && !errors.Is(srvErr, http.ErrServerClosed) {
			log.Error(srvErr, "server failed")
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Error(err, "shutdown failed")
	}
}

// runnerConfig captures the static inputs needed to assemble a fresh
// doctor.Runner on each call. Workspace + service-discovery state is
// resolved INSIDE buildRunner so a startup-time race against a not-
// yet-existing Workspace doesn't permanently cripple the pod
// (issue #1040).
//
// `sessionAPIBaseURL` / `memoryAPIBaseURL` are the flag-derived
// fallback URLs used when service discovery fails or `--workspace`
// is not set. Workspace-resolved URLs override these per call.
type runnerConfig struct {
	log               logr.Logger
	namespace         string
	agentNamespace    string
	agentName         string
	workspace         string
	serviceGroup      string
	sessionAPIBaseURL string
	memoryAPIBaseURL  string
	ollamaURL         string
	operatorURL       string
	dashboardURL      string
	redisAddr         string
	arenaURL          string
	// mgmtPlaneFetcher is shared across runs. nil means "no token" —
	// the WS dial proceeds without an Authorization header, which is
	// fine for installs that don't enforce mgmt-plane validation
	// (Arena E2E, anonymous test setups).
	mgmtPlaneFetcher *mgmtplane.TokenFetcher
}

// buildRunner constructs a fresh doctor.Runner with all checks
// registered. Called per /api/v1/run request so workspace service
// discovery happens at run time, not pod start. Each call:
//   - re-resolves workspace URLs (handles startup-race recovery)
//   - opens a fresh session store HTTP client (so a stale URL doesn't
//     stick across calls)
//   - re-fetches the workspace UID (handles Workspace creation after
//     pod start)
//
// The session store is intentionally NOT closed here — the caller
// gets the runner and the embedded store; the store closes when its
// owning runner is GC'd. Per-run leak is bounded by the run handler's
// context cancellation.
func buildRunner(cfg runnerConfig) (*doctor.Runner, error) {
	sessionAPIURL := cfg.sessionAPIBaseURL
	memoryAPIURL := cfg.memoryAPIBaseURL

	if cfg.workspace != "" {
		resolveWorkspaceURLs(cfg.log, cfg.workspace, cfg.serviceGroup, &sessionAPIURL, &memoryAPIURL)
	}

	agentFacadeURL := discoverServiceURL(cfg.agentNamespace, cfg.agentName, defaultAPIPort)
	sessionStore := httpclient.NewStore(sessionAPIURL, cfg.log, httpclient.WithBufferCapacity(0))

	runner := doctor.NewRunner()

	runner.Register(checks.InfrastructureChecks(map[string]string{
		"SessionAPI": sessionAPIURL,
		"MemoryAPI":  memoryAPIURL,
	})...)
	runner.Register(checks.OllamaCheck(cfg.ollamaURL))
	runner.Register(checks.OperatorAPICheck(cfg.operatorURL))
	runner.Register(checks.DashboardCheck(cfg.dashboardURL))
	// Redis is optional. Skip registration when no addr was provided —
	// otherwise OSS installs (no Redis configured) would surface a
	// noisy false-positive "RedisReachable: unreachable" on every run.
	if cfg.redisAddr != "" {
		runner.Register(checks.TCPCheck("Redis", cfg.redisAddr))
	}
	runner.Register(checks.ArenaControllerCheck(cfg.arenaURL))

	k8sClient, k8sErr := k8s.NewClient()
	if k8sErr != nil {
		cfg.log.Info("k8s client unavailable, CRD checks will be skipped", "error", k8sErr.Error())
	}
	if k8sClient != nil {
		crdChecker := checks.NewCRDChecker(k8sClient)
		runner.Register(crdChecker.Checks()...)
	}

	// agentChecker.MgmtPlaneTokenSource accepts the *MgmtPlaneTokenMinter
	// agentChecker.MgmtPlaneTokenSource accepts the *MgmtPlaneTokenFetcher
	// directly (nil-safe via the interface — a typed nil pointer behaves
	// the same as nil because the Token method short-circuits on nil
	// receiver). When unset, the dialer skips the Authorization header.
	var tokenSource checks.MgmtPlaneTokenSource
	if cfg.mgmtPlaneFetcher != nil {
		tokenSource = cfg.mgmtPlaneFetcher
	}
	agentChecker := checks.NewAgentChecker(checks.AgentConfig{
		FacadeURL:            agentFacadeURL,
		AgentName:            cfg.agentName,
		Namespace:            cfg.agentNamespace,
		SessionAPIURL:        sessionAPIURL,
		SessionStore:         sessionStore,
		MgmtPlaneTokenSource: tokenSource,
	})
	runner.Register(agentChecker.Checks()...)

	sessionChecker := checks.NewSessionChecker(sessionAPIURL, cfg.agentNamespace, sessionStore, func() string {
		return agentChecker.LastSessionID
	})
	runner.Register(sessionChecker.Checks()...)

	var workspaceUID string
	if k8sClient != nil {
		workspaceUID = checks.ResolveWorkspaceUID(k8sClient, cfg.agentNamespace, cfg.log)
	}

	memoryStore := memoryhttpclient.NewStore(memoryAPIURL, cfg.log)
	memoryChecker := checks.NewMemoryChecker(memoryAPIURL, memoryStore, workspaceUID, agentChecker)
	runner.Register(memoryChecker.Checks()...)

	consolidationChecker := checks.NewConsolidationChecker(memoryAPIURL)
	runner.Register(consolidationChecker.Checks()...)

	privacyChecker := checks.NewPrivacyChecker(memoryAPIURL, sessionAPIURL, workspaceUID, cfg.arenaURL)
	if k8sClient != nil {
		privacyChecker.WithK8sClient(k8sClient)
	}
	runner.Register(privacyChecker.Checks()...)

	// Agent → Sessions must run sequentially (Sessions reads Agent's LastSessionID).
	runner.SequentialGroup("agent-sessions", "Agent", "Sessions")

	return runner, nil
}

// resolveWorkspaceURLs uses service discovery to find per-workspace service URLs.
// On success it overwrites sessionAPIURL and memoryAPIURL; on failure it logs and
// leaves the pointers unchanged so the flag-based fallback values are used.
func resolveWorkspaceURLs(log interface {
	Info(msg string, keysAndValues ...interface{})
}, workspace, serviceGroup string, sessionAPIURL, memoryAPIURL *string) {
	k8sClient, sdErr := k8s.NewClient()
	if sdErr != nil {
		log.Info("service discovery unavailable, using flag URLs", "reason", sdErr.Error())
		return
	}
	resolver := servicediscovery.NewResolver(k8sClient)
	sdCtx, sdCancel := context.WithTimeout(context.Background(), 10*time.Second)
	urls, resolveErr := resolver.ResolveByWorkspaceName(sdCtx, workspace, serviceGroup)
	sdCancel()
	if resolveErr != nil {
		log.Info("service discovery failed, using flag URLs",
			"reason", resolveErr.Error(),
			"workspace", workspace,
			"serviceGroup", serviceGroup,
		)
		return
	}
	*sessionAPIURL = urls.SessionURL
	*memoryAPIURL = urls.MemoryURL
	log.Info("service URLs resolved via workspace",
		"workspace", workspace,
		"serviceGroup", serviceGroup,
		"sessionAPIURL", *sessionAPIURL,
		"memoryAPIURL", *memoryAPIURL,
	)
}

// runOnceResultBegin and runOnceResultEnd bracket the RunResult JSON written
// to stdout in --run-once mode. Container runtimes merge the doctor pod's
// stderr (zap log lines, also JSON-shaped) with stdout into a single
// `kubectl logs` stream, so a downstream parser cannot rely on "the first
// `{\n` is the result". The sentinels give parsers an unambiguous slice.
const (
	runOnceResultBegin = "=== DOCTOR-RUN-RESULT-BEGIN ==="
	runOnceResultEnd   = "=== DOCTOR-RUN-RESULT-END ==="
)

func runOnceMode(runner *doctor.Runner, log interface {
	Info(msg string, keysAndValues ...interface{})
}, exitOnFail bool) {
	results := make(chan doctor.TestResult, 100)
	// Drain the results channel so runner.Run can make progress. The
	// per-test data is also present in the aggregate `run` value emitted
	// below; we don't log per-result here because those lines would
	// interleave with the RunResult JSON in `kubectl logs` output.
	go func() {
		for range results {
		}
	}()

	run := runner.Run(context.Background(), results)

	// Buffer the JSON before writing so the begin/result/end triplet hits
	// stdout in a single Write call, minimising the chance of stderr log
	// lines slicing through it on a noisy run.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(run); err != nil {
		log.Info("failed to encode run-once result", "error", err.Error())
		os.Exit(1)
	}
	out := fmt.Sprintf("\n%s\n%s%s\n", runOnceResultBegin, buf.String(), runOnceResultEnd)
	if _, err := os.Stdout.WriteString(out); err != nil {
		os.Exit(1)
	}

	if exitOnFail && run.Summary.Failed > 0 {
		os.Exit(1)
	}
	os.Exit(0)
}
