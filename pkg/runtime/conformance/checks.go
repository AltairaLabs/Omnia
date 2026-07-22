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

package conformance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

const (
	probeSession    = "conformance"
	maxProbeFrames  = 16
	converseTimeout = 10 * time.Second

	healthFailedReason = "skipped: Health/contract check failed"
	legacyReason       = "skipped: runtime advertises no capabilities (legacy — pre-negotiation)"
)

// readFrames drains up to max ServerMessages from a Converse stream, stopping at
// a clean EOF. A non-EOF error is returned with the frames read so far.
func readFrames(stream runtimev1.RuntimeService_ConverseClient, max int) ([]*runtimev1.ServerMessage, error) {
	out := make([]*runtimev1.ServerMessage, 0, max)
	for i := 0; i < max; i++ {
		m, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		out = append(out, m)
	}
	return out, nil
}

// capsMatch reports whether the hello's capability slice equals the Health set.
func capsMatch(helloCaps []string, healthCaps map[string]bool) bool {
	if len(helloCaps) != len(healthCaps) {
		return false
	}
	for _, c := range helloCaps {
		if !healthCaps[c] {
			return false
		}
	}
	return true
}

// converseSkip returns a skip reason when the Converse-based probes cannot run:
// health failed (nil caps) or the runtime is legacy (empty caps → no hello).
func converseSkip(caps map[string]bool) (bool, string) {
	if caps == nil {
		return true, healthFailedReason
	}
	if len(caps) == 0 {
		return true, legacyReason
	}
	return false, ""
}

// checkConverse probes the text-turn path: the first ServerMessage must be a
// RuntimeHello whose capabilities equal Health's, and the turn must end with a
// done with no done arriving before the hello.
func checkConverse(ctx context.Context, client rtClient, caps map[string]bool) []CheckResult {
	const helloName = "hello-first"
	const turnName = "text-turn-shape"
	if skip, reason := converseSkip(caps); skip {
		return []CheckResult{{helloName, StatusSkip, reason}, {turnName, StatusSkip, reason}}
	}

	cctx, cancel := context.WithTimeout(ctx, converseTimeout)
	defer cancel()
	stream, err := client.Converse(cctx)
	if err != nil {
		d := fmt.Sprintf("Converse failed to open: %v", err)
		return []CheckResult{{helloName, StatusFail, d}, {turnName, StatusFail, d}}
	}
	_ = stream.Send(&runtimev1.ClientMessage{SessionId: probeSession, Content: "conformance probe"})
	_ = stream.CloseSend()
	frames, recvErr := readFrames(stream, maxProbeFrames)

	return []CheckResult{
		evalHelloFirst(helloName, frames, caps),
		evalTurnShape(turnName, frames, recvErr),
	}
}

// evalHelloFirst asserts the first frame is a RuntimeHello matching Health caps.
func evalHelloFirst(name string, frames []*runtimev1.ServerMessage, caps map[string]bool) CheckResult {
	if len(frames) == 0 {
		return CheckResult{name, StatusFail, "Converse produced no ServerMessages"}
	}
	hello := frames[0].GetRuntimeHello()
	if hello == nil {
		d := fmt.Sprintf("first frame is %T, want RuntimeHello", frames[0].GetMessage())
		return CheckResult{name, StatusFail, d}
	}
	if !capsMatch(hello.GetCapabilities(), caps) {
		return CheckResult{name, StatusFail, "RuntimeHello.capabilities do not match Health.capabilities"}
	}
	return CheckResult{name, StatusPass, ""}
}

// evalTurnShape asserts the turn ends with a done and no done precedes the hello.
func evalTurnShape(name string, frames []*runtimev1.ServerMessage, recvErr error) CheckResult {
	if recvErr != nil {
		return CheckResult{name, StatusFail, fmt.Sprintf("stream error before completion: %v", recvErr)}
	}
	seenHello, seenDone := false, false
	for _, f := range frames {
		if f.GetRuntimeHello() != nil {
			seenHello = true
		}
		if f.GetDone() != nil {
			if !seenHello {
				return CheckResult{name, StatusFail, "done arrived before RuntimeHello"}
			}
			seenDone = true
		}
	}
	if !seenDone {
		return CheckResult{name, StatusFail, "text turn did not end with a done frame"}
	}
	return CheckResult{name, StatusPass, ""}
}

// checkMalformedInput sends an empty ClientMessage (no content, no oneof body)
// and requires the runtime to answer on-protocol (an Error frame or a clean
// close) rather than tearing the stream down with an internal transport error.
func checkMalformedInput(ctx context.Context, client rtClient, caps map[string]bool) CheckResult {
	const name = "graceful-malformed-input"
	if caps == nil {
		return CheckResult{name, StatusSkip, healthFailedReason}
	}
	cctx, cancel := context.WithTimeout(ctx, converseTimeout)
	defer cancel()
	stream, err := client.Converse(cctx)
	if err != nil {
		return CheckResult{name, StatusFail, fmt.Sprintf("Converse failed to open: %v", err)}
	}
	_ = stream.Send(&runtimev1.ClientMessage{SessionId: probeSession})
	_ = stream.CloseSend()
	_, recvErr := readFrames(stream, maxProbeFrames)
	if recvErr != nil && isTransportCrash(recvErr) {
		return CheckResult{name, StatusFail, fmt.Sprintf("stream crashed on empty input: %v", recvErr)}
	}
	return CheckResult{name, StatusPass, ""}
}

// isTransportCrash reports whether err is an internal/unknown gRPC failure — the
// signature of a runtime that crashed rather than handling input on-protocol.
func isTransportCrash(err error) bool {
	switch status.Code(err) {
	case codes.Internal, codes.Unknown, codes.DataLoss:
		return true
	default:
		return false
	}
}
