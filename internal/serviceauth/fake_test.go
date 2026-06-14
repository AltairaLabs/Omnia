/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package serviceauth

import "context"

// Shared test fixtures for the serviceauth package.
const (
	testNS      = "ws-ns"    // a trusted workspace namespace
	testBearerX = "Bearer x" // a non-empty bearer header whose token value is irrelevant
)

// fakeReviewer is a configurable TokenReviewer for tests (no real k8s).
type fakeReviewer struct {
	authenticated bool
	username      string
	err           error
}

func (f fakeReviewer) ReviewToken(_ context.Context, _ string) (bool, string, error) {
	return f.authenticated, f.username, f.err
}
