/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package facade

import "testing"

// Close on a zero-value client (no dialled connection) must be a safe no-op, so
// a facade can defer Close unconditionally.
func TestRuntimeClient_CloseNilConn(t *testing.T) {
	var c RuntimeClient
	if err := c.Close(); err != nil {
		t.Fatalf("Close on nil conn = %v, want nil", err)
	}
}
