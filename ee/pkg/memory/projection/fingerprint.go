/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

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
