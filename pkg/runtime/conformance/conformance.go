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

// Package conformance is a protocol-only, provider-agnostic conformance suite
// for the omnia.runtime.v1 contract. It dials a runtime's gRPC endpoint and
// runs capability-gated probes, asserting frame shape, ordering, and honest
// capability advertisement — never response content. It depends only on the
// generated proto, grpc, and the contract's known capability vocabulary; it
// MUST NOT import internal/... so it works against any runtime in any language.
package conformance

import (
	"context"
	"fmt"
	"regexp"

	"google.golang.org/grpc"

	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// Check status values.
const (
	StatusPass = "pass"
	StatusFail = "fail"
	StatusSkip = "skip"
)

// Config parameterises a conformance run. Conn is a dialed (or in-process)
// runtime endpoint; the suite takes no scenario parameters because it is
// protocol-only.
type Config struct {
	Conn grpc.ClientConnInterface
}

// CheckResult is the outcome of a single conformance probe.
type CheckResult struct {
	Name   string // stable check identifier, e.g. "hello-first"
	Status string // StatusPass | StatusFail | StatusSkip
	Detail string // failure/skip reason; the exchange that failed
}

// Result aggregates every probe. Passed is false if any check failed (skips do
// not fail the run).
type Result struct {
	Checks []CheckResult
	Passed bool
}

// rtClient aliases the generated client to keep probe signatures within the
// line-length limit.
type rtClient = runtimev1.RuntimeServiceClient

var semverRE = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// Run executes the protocol-only conformance probes against the runtime
// reachable via cfg.Conn and returns a structured result. It never panics: each
// check records its own failure rather than aborting the run.
func Run(ctx context.Context, cfg Config) Result {
	client := runtimev1.NewRuntimeServiceClient(cfg.Conn)
	var checks []CheckResult

	caps, hc := checkHealth(ctx, client)
	checks = append(checks, hc)
	checks = append(checks, checkConverse(ctx, client, caps)...)
	checks = append(checks, checkMalformedInput(ctx, client, caps))
	checks = append(checks, checkInvokeHonesty(ctx, client, caps))
	checks = append(checks, checkDuplexHonesty(ctx, client, caps))

	return finalize(checks)
}

// finalize computes Passed from the collected checks.
func finalize(checks []CheckResult) Result {
	passed := true
	for _, c := range checks {
		if c.Status == StatusFail {
			passed = false
		}
	}
	return Result{Checks: checks, Passed: passed}
}

// checkHealth verifies Health returns healthy, a semver contract_version, and a
// capability list. It returns the advertised capability set (nil on failure, so
// downstream checks skip) and the check result.
func checkHealth(ctx context.Context, client rtClient) (map[string]bool, CheckResult) {
	const name = "health/contract"
	resp, err := client.Health(ctx, &runtimev1.HealthRequest{})
	if err != nil {
		return nil, CheckResult{name, StatusFail, fmt.Sprintf("Health RPC failed: %v", err)}
	}
	if !resp.GetHealthy() {
		return nil, CheckResult{name, StatusFail, "Health reports healthy=false"}
	}
	if v := resp.GetContractVersion(); !semverRE.MatchString(v) {
		return nil, CheckResult{name, StatusFail, fmt.Sprintf("contract_version %q is not semver (want N.N.N)", v)}
	}
	caps := make(map[string]bool, len(resp.GetCapabilities()))
	for _, c := range resp.GetCapabilities() {
		caps[c] = true
	}
	return caps, CheckResult{name, StatusPass, ""}
}
