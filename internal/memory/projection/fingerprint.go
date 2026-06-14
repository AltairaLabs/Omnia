package projection

import (
	"fmt"
	"time"
)

// Fingerprint identifies a scope's memory state for cache invalidation.
// Recompute when the live fingerprint differs from the stored one.
func Fingerprint(count int, maxObservedAt time.Time) string {
	return fmt.Sprintf("%d:%d", count, maxObservedAt.UTC().UnixNano())
}
