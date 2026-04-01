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
	"github.com/altairalabs/omnia/pkg/k8s"
	"github.com/altairalabs/omnia/pkg/logging"
)

const (
	defaultNamespace      = "omnia-system"
	defaultAgentNamespace = "omnia-demo"
	defaultAgentName      = "tools-demo"
	defaultAPIPort        = 8080

	serviceSessionAPI = "omnia-session-api"
	serviceMemoryAPI  = "omnia-memory-api"
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

	agentFacadeURL := discoverServiceURL(*agentNamespace, *agentName, defaultAPIPort)

	runner := doctor.NewRunner()

	runner.Register(checks.InfrastructureChecks(map[string]string{
		"SessionAPI": sessionAPIURL,
		"MemoryAPI":  memoryAPIURL,
	})...)

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
	})
	runner.Register(agentChecker.Checks()...)

	sessionChecker := checks.NewSessionChecker(sessionAPIURL, *agentNamespace, func() string {
		return agentChecker.LastSessionID
	})
	runner.Register(sessionChecker.Checks()...)

	var workspaceUID string
	if k8sClient != nil {
		workspaceUID = checks.ResolveWorkspaceUID(k8sClient, *agentNamespace, log)
	}

	memoryChecker := checks.NewMemoryChecker(memoryAPIURL, workspaceUID, agentChecker)
	runner.Register(memoryChecker.Checks()...)

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
