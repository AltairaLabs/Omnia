/*
Copyright 2025-2026.

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

package facade

import (
	"context"
	"time"
)

// drainReasonAllDrained is the drain completion reason when all sessions finished gracefully.
const drainReasonAllDrained = "all_drained"

// drainReasonDeadline is the drain completion reason when the drain timeout elapsed.
const drainReasonDeadline = "deadline"

// drainReasonCtxCanceled is the drain completion reason when the context was canceled.
const drainReasonCtxCanceled = "ctx_canceled"

// markDraining puts the server into drain mode: new upgrades are rejected and
// /readyz reports not-ready, but active connections keep being served.
// Idempotent — calling it multiple times is safe.
func (s *Server) markDraining() { s.draining.Store(true) }

// IsDraining reports whether the server has entered drain mode.
func (s *Server) IsDraining() bool { return s.draining.Load() }

// liveRealtimeSessions returns the total number of active (in-flight) plus
// parked realtime sessions on this pod.
func (s *Server) liveRealtimeSessions() int {
	return int(s.activeAudioSessions.Load()) + s.parked.len()
}

// Drain marks the server draining and blocks until there are no active or
// parked realtime sessions, or DrainTimeout elapses (whichever first). Returns
// the number of sessions still live at return (0 = fully drained). Safe to call
// once; subsequent calls return immediately.
func (s *Server) Drain(ctx context.Context) int {
	s.markDraining()
	s.metrics.RealtimeDrainStarted()
	drainStart := time.Now()
	initialSessions := s.liveRealtimeSessions()

	finishDrain := func(reason string, remaining int) int {
		elapsed := time.Since(drainStart).Seconds()
		drained := initialSessions - remaining
		if drained < 0 {
			drained = 0
		}
		s.metrics.RealtimeDrainCompleted(reason, elapsed, drained, remaining)
		return remaining
	}

	deadline := time.NewTimer(s.config.DrainTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	s.log.Info("facade draining started",
		"drainTimeout", s.config.DrainTimeout,
		"liveSessions", initialSessions)
	for {
		if n := s.liveRealtimeSessions(); n == 0 {
			s.log.Info("facade drain complete", "reason", drainReasonAllDrained)
			return finishDrain(drainReasonAllDrained, 0)
		}
		select {
		case <-deadline.C:
			n := s.liveRealtimeSessions()
			s.log.Info("facade drain complete", "reason", drainReasonDeadline, "remaining", n)
			return finishDrain(drainReasonDeadline, n)
		case <-ctx.Done():
			return finishDrain(drainReasonCtxCanceled, s.liveRealtimeSessions())
		case <-ticker.C:
		}
	}
}

// DrainTimeoutForShutdown returns the configured drain timeout so that
// cmd/agent shutdownAll can scope the drain context without reaching into
// unexported config.
func (s *Server) DrainTimeoutForShutdown() time.Duration {
	return s.config.DrainTimeout
}
