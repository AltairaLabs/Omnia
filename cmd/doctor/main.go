package main

import (
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

	"github.com/altairalabs/omnia/internal/doctor"
	"github.com/altairalabs/omnia/internal/doctor/checks"
	memoryhttpclient "github.com/altairalabs/omnia/internal/memory/httpclient"
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
	serviceRedis           = "omnia-redis-master"
	serviceArenaController = "omnia-arena-controller"
	defaultOllamaPort      = 11434
	defaultOperatorPort    = 8083
	defaultDashboardPort   = 3000
	defaultRedisPort       = 6379
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
	redisAddrFlag := flag.String("redis-addr", "", "override Redis address (host:port)")
	arenaURLFlag := flag.String("arena-url", "", "override arena controller URL")
	workspaceFlag := flag.String("workspace", "", "workspace name for per-workspace service discovery (optional)")
	serviceGroupFlag := flag.String("service-group", "default", "service group to resolve within the workspace")
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

	redisAddr := *redisAddrFlag
	if redisAddr == "" {
		redisAddr = fmt.Sprintf("%s.%s.svc.cluster.local:%d", serviceRedis, *namespace, defaultRedisPort)
	}

	arenaURL := *arenaURLFlag
	if arenaURL == "" {
		arenaURL = discoverServiceURL(*namespace, serviceArenaController, defaultArenaPort)
	}

	// If --workspace is set, use service discovery to resolve per-workspace URLs.
	// This overrides the flag-based URL (but the flag-based URL is still the fallback
	// when --workspace is not set, for local/singleton testing).
	if *workspaceFlag != "" {
		resolveWorkspaceURLs(log, *workspaceFlag, *serviceGroupFlag, &sessionAPIURL, &memoryAPIURL)
	}

	agentFacadeURL := discoverServiceURL(*agentNamespace, *agentName, defaultAPIPort)

	sessionStore := httpclient.NewStore(sessionAPIURL, log, httpclient.WithBufferCapacity(0))
	defer sessionStore.Close() //nolint:errcheck

	runner := doctor.NewRunner()

	runner.Register(checks.InfrastructureChecks(map[string]string{
		"SessionAPI": sessionAPIURL,
		"MemoryAPI":  memoryAPIURL,
	})...)
	runner.Register(checks.OllamaCheck(ollamaURL))
	runner.Register(checks.OperatorAPICheck(operatorURL))
	runner.Register(checks.DashboardCheck(dashboardURL))
	runner.Register(checks.TCPCheck("Redis", redisAddr))
	runner.Register(checks.ArenaControllerCheck(arenaURL))

	k8sClient, k8sErr := k8s.NewClient()
	if k8sErr != nil {
		log.Info("k8s client unavailable, CRD checks will be skipped", "error", k8sErr.Error())
	}
	if k8sClient != nil {
		crdChecker := checks.NewCRDChecker(k8sClient)
		runner.Register(crdChecker.Checks()...)
	}

	agentChecker := checks.NewAgentChecker(checks.AgentConfig{
		FacadeURL:     agentFacadeURL,
		AgentName:     *agentName,
		Namespace:     *agentNamespace,
		SessionAPIURL: sessionAPIURL,
		SessionStore:  sessionStore,
	})
	runner.Register(agentChecker.Checks()...)

	sessionChecker := checks.NewSessionChecker(sessionAPIURL, *agentNamespace, sessionStore, func() string {
		return agentChecker.LastSessionID
	})
	runner.Register(sessionChecker.Checks()...)

	var workspaceUID string
	if k8sClient != nil {
		workspaceUID = checks.ResolveWorkspaceUID(k8sClient, *agentNamespace, log)
	}

	memoryStore := memoryhttpclient.NewStore(memoryAPIURL, log)
	memoryChecker := checks.NewMemoryChecker(memoryAPIURL, memoryStore, workspaceUID, agentChecker)
	runner.Register(memoryChecker.Checks()...)

	privacyChecker := checks.NewPrivacyChecker(memoryAPIURL, sessionAPIURL, workspaceUID, arenaURL)
	if k8sClient != nil {
		privacyChecker.WithK8sClient(k8sClient)
	}
	runner.Register(privacyChecker.Checks()...)

	// Agent → Sessions must run sequentially (Sessions reads Agent's LastSessionID).
	runner.SequentialGroup("agent-sessions", "Agent", "Sessions")

	if *runOnce {
		runOnceMode(runner, log, *exitCode)
		return
	}

	srv := doctor.NewServer(runner, *addr, log)
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

func runOnceMode(runner *doctor.Runner, log interface {
	Info(msg string, keysAndValues ...interface{})
}, exitOnFail bool) {
	results := make(chan doctor.TestResult, 100)
	go func() {
		for r := range results {
			log.Info("test completed", "name", r.Name, "status", r.Status, "detail", r.Detail)
		}
	}()

	run := runner.Run(context.Background(), results)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(run); err != nil {
		os.Exit(1)
	}

	if exitOnFail && run.Summary.Failed > 0 {
		os.Exit(1)
	}
	os.Exit(0)
}
