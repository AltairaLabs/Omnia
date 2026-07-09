/*
Copyright 2026.

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

package controller

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const (
	defaultProbeInterval = 60 * time.Second
	defaultProbeTimeout  = 5 * time.Second
	// maxConcurrentProbes bounds how many endpoints are dialed in parallel per
	// reconcile so a registry with many slow/blackholed endpoints can't occupy a
	// reconcile worker for sum(timeouts); the probe phase is ~one timeout wide.
	maxConcurrentProbes = 8
)

// probeDialContext is the TCP dialer used for reachability probes. It is a
// package variable so tests can substitute a stub.
var probeDialContext = (&net.Dialer{}).DialContext

// probeTools TCP-dials each discovered tool's endpoint when probing is enabled,
// marking it Available or Unavailable so determinePhase can report
// Ready/Degraded/Failed. It is a reachability check (a TCP connect), not a tool
// invocation, so it has no side effects on the backend. Endpoints with no
// network address (client browser tools, stdio MCP) are left unchanged.
// Probing is off by default, so existing registries are unaffected.
func (r *ToolRegistryReconciler) probeTools(ctx context.Context, tr *omniav1alpha1.ToolRegistry, tools []omniav1alpha1.DiscoveredTool) {
	if tr.Spec.Probe == nil || !tr.Spec.Probe.Enabled {
		return
	}
	timeout := durationOr(tr.Spec.Probe.Timeout, defaultProbeTimeout)

	// Probe network-addressable tools concurrently (bounded), each writing its
	// own slice element, so the reconcile isn't blocked for sum(timeouts).
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrentProbes)
	for i := range tools {
		if !isNetworkEndpoint(tools[i].Endpoint) {
			continue // client://browser, stdio://, or empty — no network address
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(tool *omniav1alpha1.DiscoveredTool) {
			defer wg.Done()
			defer func() { <-sem }()
			r.probeOne(ctx, tool, timeout)
		}(&tools[i])
	}
	wg.Wait()
}

// probeOne TCP-dials one tool's endpoint and records the outcome on the tool.
func (r *ToolRegistryReconciler) probeOne(ctx context.Context, tool *omniav1alpha1.DiscoveredTool, timeout time.Duration) {
	log := logf.FromContext(ctx)
	now := metav1.Now()
	tool.LastChecked = &now

	addr, ok := probeAddress(tool.Endpoint)
	if !ok {
		// A network endpoint we can't turn into a host:port is a misconfiguration
		// (e.g. a URL missing its scheme) — surface it rather than leaving the
		// tool Available and unprobed.
		msg := fmt.Sprintf("probe: unrecognized endpoint address %q", tool.Endpoint)
		tool.Status = omniav1alpha1.ToolStatusUnavailable
		tool.Error = &msg
		return
	}

	dctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := probeDialContext(dctx, "tcp", addr)
	if err != nil {
		msg := fmt.Sprintf("probe failed: %v", err)
		tool.Status = omniav1alpha1.ToolStatusUnavailable
		tool.Error = &msg
		log.V(1).Info("tool probe failed", "handler", tool.HandlerName, "addr", addr, "err", err.Error())
		return
	}
	_ = conn.Close()
	tool.Status = omniav1alpha1.ToolStatusAvailable
	tool.Error = nil
}

// isNetworkEndpoint reports whether an endpoint has a network address to probe.
// Client browser tools (client://…), stdio MCP subprocesses (stdio://…), and
// empty endpoints have none and are left unprobed.
func isNetworkEndpoint(endpoint string) bool {
	return endpoint != "" &&
		!strings.HasPrefix(endpoint, "client://") &&
		!strings.HasPrefix(endpoint, "stdio://")
}

// probeAddress extracts a host:port to dial from a resolved network endpoint.
// Returns ok=false when the value can't be parsed into a host:port.
func probeAddress(endpoint string) (string, bool) {
	// URL form (http/https, MCP sse/streamable-http, OpenAPI specURL).
	if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
		port := u.Port()
		if port == "" {
			if u.Scheme == "https" {
				port = "443"
			} else {
				port = "80"
			}
		}
		return net.JoinHostPort(u.Hostname(), port), true
	}
	// host:port form (gRPC endpoint).
	if h, p, err := net.SplitHostPort(endpoint); err == nil && h != "" && p != "" {
		return net.JoinHostPort(h, p), true
	}
	return "", false
}

// probeRequeueAfter returns the interval to re-probe on when probing is enabled,
// or 0 when it is disabled.
func probeRequeueAfter(p *omniav1alpha1.ProbeConfig) time.Duration {
	if p == nil || !p.Enabled {
		return 0
	}
	return durationOr(p.Interval, defaultProbeInterval)
}

func durationOr(d *metav1.Duration, def time.Duration) time.Duration {
	if d == nil || d.Duration <= 0 {
		return def
	}
	return d.Duration
}
