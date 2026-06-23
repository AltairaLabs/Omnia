package facade

import (
	"context"
	"time"
)

// RouteStore publishes a best-effort hint mapping a parked session to the pod
// holding it. It is an affinity optimization, never a source of truth; failures
// must not break parking.
type RouteStore interface {
	PutRoute(ctx context.Context, sessionID, addr string, ttl time.Duration) error
	DeleteRoute(ctx context.Context, sessionID string) error
}

// noopRouteStore is the default when no route store is wired.
type noopRouteStore struct{}

func (noopRouteStore) PutRoute(context.Context, string, string, time.Duration) error { return nil }
func (noopRouteStore) DeleteRoute(context.Context, string) error                     { return nil }
