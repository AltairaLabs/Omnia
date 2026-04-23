/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

// Command a2a-invoker makes a single one-shot A2A call against a target
// AgentRuntime and exits. Designed to be the container image a Kubernetes
// CronJob runs to wake up a scheduled agent on an interval — the first
// consumer is the memory summarizer (see
// docs/local-backlog/2026-04-23-memory-summarization-via-agent.md), but the
// same binary covers any "trigger this agent once" use case.
//
// Usage:
//
//	a2a-invoker --agent=summarizer-agent --namespace=workspace-support \
//	            --message="Run compaction now." --timeout=30m
//
// The invoker deliberately omits A2A contextID so each call starts a fresh
// conversation on the server side, avoiding state leakage across scheduled
// runs. See the a2a package analysis in the plan doc.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/AltairaLabs/PromptKit/runtime/a2a"

	"github.com/altairalabs/omnia/pkg/logging"
)

// Default A2A port for AgentRuntimes in dual-protocol mode. Must stay in
// sync with internal/controller/constants.go:DefaultA2APort. Duplicated
// here so the invoker doesn't have to depend on the controller package.
const defaultA2APort = 9999

// defaultTokenPath is the standard Kubernetes projected service-account
// token mount. Present when the CronJob runs with a non-default SA.
// nolint:gosec // well-known mount path, not a credential.
const defaultTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

type flags struct {
	agent     string
	namespace string
	message   string
	port      int
	timeout   time.Duration
	tokenPath string
	baseURL   string
}

func parseFlags() (*flags, error) {
	f := &flags{}
	flag.StringVar(&f.agent, "agent", "", "Target AgentRuntime name (required unless --base-url is set)")
	flag.StringVar(&f.namespace, "namespace", "", "Target AgentRuntime namespace (required unless --base-url is set)")
	flag.StringVar(&f.message, "message", "Run scheduled task.", "User message to send to the agent")
	flag.IntVar(&f.port, "port", defaultA2APort, "A2A port on the target service")
	flag.DurationVar(&f.timeout, "timeout", 10*time.Minute, "Per-invocation timeout")
	flag.StringVar(&f.tokenPath, "token-path", defaultTokenPath, "Path to a bearer token file; empty disables auth")
	flag.StringVar(&f.baseURL, "base-url", "", "Override full A2A base URL (skips agent+namespace resolution)")
	flag.Parse()

	if f.baseURL == "" {
		if f.agent == "" {
			return nil, fmt.Errorf("--agent is required when --base-url is not set")
		}
		if f.namespace == "" {
			return nil, fmt.Errorf("--namespace is required when --base-url is not set")
		}
	}
	if f.timeout <= 0 {
		return nil, fmt.Errorf("--timeout must be positive")
	}
	return f, nil
}

// resolveBaseURL returns the A2A base URL for the target. --base-url wins
// when set; otherwise we compose the standard in-cluster service URL using
// the AgentRuntime's conventional service name (== its metadata.name).
func resolveBaseURL(f *flags) string {
	if f.baseURL != "" {
		return f.baseURL
	}
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", f.agent, f.namespace, f.port)
}

// loadToken reads a bearer token from tokenPath. Empty path disables auth;
// missing file at a non-empty path is treated as "no auth" (the typical
// case when running locally without a projected SA token), not fatal.
func loadToken(tokenPath string) string {
	if tokenPath == "" {
		return ""
	}
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return ""
	}
	return string(data)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	f, err := parseFlags()
	if err != nil {
		return err
	}

	log, syncLog, err := logging.NewLogger()
	if err != nil {
		return fmt.Errorf("creating logger: %w", err)
	}
	defer syncLog()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	ctx, cancelTimeout := context.WithTimeout(ctx, f.timeout)
	defer cancelTimeout()

	baseURL := resolveBaseURL(f)

	clientOpts := []a2a.ClientOption{}
	if token := loadToken(f.tokenPath); token != "" {
		clientOpts = append(clientOpts, a2a.WithAuth("Bearer", token))
	}
	client := a2a.NewClient(baseURL, clientOpts...)

	log.Info("invoking agent",
		"baseURL", baseURL,
		"timeout", f.timeout,
	)

	task, err := client.SendMessage(ctx, buildRequest(f.message))
	if err != nil {
		log.Error(err, "A2A send failed", "baseURL", baseURL)
		return fmt.Errorf("send: %w", err)
	}

	responseText := a2a.ExtractResponseText(task)
	log.Info("agent responded",
		"taskID", task.ID,
		"state", task.Status.State,
		"responseLength", len(responseText),
	)
	// Print the response to stdout so CronJob logs capture the summary
	// verbatim without having to grep logr-formatted output.
	if responseText != "" {
		fmt.Println(responseText)
	}
	return nil
}

// buildRequest constructs the SendMessageRequest. ContextID is intentionally
// left unset so each scheduled invocation gets a fresh server-side
// conversation; MessageID is a fresh UUID per call.
func buildRequest(message string) *a2a.SendMessageRequest {
	text := message
	return &a2a.SendMessageRequest{
		Message: a2a.Message{
			MessageID: uuid.NewString(),
			Role:      a2a.RoleUser,
			Parts: []a2a.Part{
				{Text: &text},
			},
		},
	}
}
