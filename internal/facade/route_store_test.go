package facade

import (
	"context"
	"testing"
	"time"
)

func TestNoopRouteStore_MethodsAreNoops(t *testing.T) {
	var rs RouteStore = noopRouteStore{}
	if err := rs.PutRoute(context.Background(), "sid", "10.0.0.1:8080", time.Second); err != nil {
		t.Fatalf("PutRoute: %v", err)
	}
	if err := rs.DeleteRoute(context.Background(), "sid"); err != nil {
		t.Fatalf("DeleteRoute: %v", err)
	}
}
