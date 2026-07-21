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
	"sync"
	"time"

	"github.com/go-logr/logr"
)

type parkedSession struct {
	session *audioSession
	ownerID string
	timer   *time.Timer
	// persisted records whether the session had an archive row when it was
	// parked. A pure-audio session never persists one — binary frames bypass
	// processMessage — so completing it on expiry would write a terminal status
	// for a row that does not exist.
	persisted bool
}

type realtimeRegistry struct {
	mu       sync.Mutex
	parked   map[string]*parkedSession
	routes   RouteStore
	podAddr  string
	grace    time.Duration
	log      logr.Logger
	onExpire func(sessionID string, persisted bool)
}

func newRealtimeRegistry(routes RouteStore, podAddr string, grace time.Duration, log logr.Logger) *realtimeRegistry {
	return &realtimeRegistry{
		parked:  make(map[string]*parkedSession),
		routes:  routes,
		podAddr: podAddr,
		grace:   grace,
		log:     log,
	}
}

// park stores the session under sessionID owned by ownerID, writes the route
// hint, and arms a grace timer that calls expire(sessionID) on fire.
func (r *realtimeRegistry) park(ctx context.Context, sessionID, ownerID string, as *audioSession, persisted bool) {
	timer := time.AfterFunc(r.grace, func() { r.expire(sessionID) })
	r.mu.Lock()
	r.parked[sessionID] = &parkedSession{session: as, ownerID: ownerID, timer: timer, persisted: persisted}
	r.mu.Unlock()

	if err := r.routes.PutRoute(ctx, sessionID, r.podAddr, r.grace); err != nil {
		r.log.Error(err, "realtime route hint write failed", "sessionID", sessionID)
	}
	r.log.V(1).Info("realtime session parked", "sessionID", sessionID)
}

// take atomically removes and returns a parked session if present AND owned by
// ownerID; cancels its timer and deletes the route hint. Returns (nil, false)
// on miss or owner mismatch.
func (r *realtimeRegistry) take(ctx context.Context, sessionID, ownerID string) (*audioSession, bool) {
	r.mu.Lock()
	ps, ok := r.parked[sessionID]
	if !ok || ps.ownerID != ownerID {
		r.mu.Unlock()
		return nil, false
	}
	delete(r.parked, sessionID)
	ps.timer.Stop()
	r.mu.Unlock()

	if err := r.routes.DeleteRoute(ctx, sessionID); err != nil {
		r.log.Error(err, "realtime route hint delete failed", "sessionID", sessionID)
	}
	r.log.V(1).Info("realtime session reattached", "sessionID", sessionID)
	return ps.session, true
}

// expire fires when the grace timer elapses: remove, close the session, delete
// the route hint, invoke onExpire.
func (r *realtimeRegistry) expire(sessionID string) {
	r.mu.Lock()
	ps, ok := r.parked[sessionID]
	if !ok {
		r.mu.Unlock()
		return
	}
	delete(r.parked, sessionID)
	r.mu.Unlock()

	if err := ps.session.close(); err != nil {
		r.log.Error(err, "parked session close failed", "sessionID", sessionID)
	}
	if err := r.routes.DeleteRoute(context.Background(), sessionID); err != nil {
		r.log.Error(err, "realtime route hint delete failed", "sessionID", sessionID)
	}
	if r.onExpire != nil {
		r.onExpire(sessionID, ps.persisted)
	}
	r.log.V(1).Info("realtime session park expired", "sessionID", sessionID)
}

// len reports the number of currently parked sessions (test/metrics).
func (r *realtimeRegistry) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.parked)
}
