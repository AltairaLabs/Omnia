/*
Copyright 2026 Altaira Labs.

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

package api

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/memory"
)

// errPolicyLoader is a PolicyLoader stub that always returns an error.
type errPolicyLoader struct{ err error }

func (e *errPolicyLoader) Load(_ context.Context) (*omniav1alpha1.MemoryPolicy, error) {
	return nil, e.err
}

// nilPolicyLoader is a PolicyLoader stub that returns (nil, nil) — no policy
// found, no error.
type nilPolicyLoader struct{}

func (n *nilPolicyLoader) Load(_ context.Context) (*omniav1alpha1.MemoryPolicy, error) {
	return nil, nil
}

// newConsentService creates a MemoryService wired with the given pruner and
// loader, matching the pattern used in handler_consent_event_test.go.
func newConsentService(pruner memory.ConsentEventPruner, loader memory.PolicyLoader) *MemoryService {
	svc := NewMemoryService(&mockStore{}, nil, MemoryServiceConfig{}, logr.Discard())
	if pruner != nil {
		svc.SetConsentEventPruner(pruner)
	}
	if loader != nil {
		svc.SetPolicyLoader(loader)
	}
	return svc
}

// TestPruneUserConsentCategory_NilPruner asserts that PruneUserConsentCategory
// returns an error and zero rows when the pruner has not been wired
// (consentEventPruner == nil). This is the guard at service_consent_event.go:43-45.
func TestPruneUserConsentCategory_NilPruner(t *testing.T) {
	svc := newConsentService(nil, nil) // no pruner wired

	n, err := svc.PruneUserConsentCategory(context.Background(), "ws-1", "u1", "memory:health")

	require.Error(t, err, "nil pruner must return an error")
	assert.EqualValues(t, 0, n, "nil pruner must return 0 rows affected")
}

// TestResolveAction_PolicyLoadError asserts that when the PolicyLoader returns
// an error, resolveAction falls back to the default SoftDelete action.
// This exercises the load-error branch at service_consent_event.go:86-88.
func TestResolveAction_PolicyLoadError(t *testing.T) {
	pruner := &stubPruner{returnN: 2}
	loader := &errPolicyLoader{err: errors.New("k8s unavailable")}
	svc := newConsentService(pruner, loader)

	// A policy-load error must not surface as an HTTP error — the service
	// defaults to SoftDelete and continues. Verify via PruneUserConsentCategory
	// so we exercise the full code path including resolveAction.
	n, err := svc.PruneUserConsentCategory(context.Background(), "ws-1", "u1", "memory:health")

	require.NoError(t, err, "policy load error must be absorbed; SoftDelete must proceed")
	assert.EqualValues(t, 2, n, "SoftDelete must return the pruner row count")
	assert.True(t, pruner.softCalled, "SoftDelete must be invoked as the fallback action")
	assert.False(t, pruner.hardCalled)
}

// TestResolveAction_NilPolicy asserts that when the PolicyLoader returns
// (nil, nil) — no policy configured — resolveAction falls back to the
// default SoftDelete action. This exercises the nil-policy branch at
// service_consent_event.go:90-92.
func TestResolveAction_NilPolicy(t *testing.T) {
	pruner := &stubPruner{returnN: 1}
	loader := &nilPolicyLoader{}
	svc := newConsentService(pruner, loader)

	n, err := svc.PruneUserConsentCategory(context.Background(), "ws-1", "u1", "memory:context")

	require.NoError(t, err, "nil policy must fall back to SoftDelete without error")
	assert.EqualValues(t, 1, n)
	assert.True(t, pruner.softCalled, "SoftDelete must be invoked when policy is nil")
	assert.False(t, pruner.hardCalled)
}

// TestResolveAction_ExplicitHardDelete verifies that when the loader returns
// a policy with HardDelete set, the hard-delete path is taken. This is a
// positive control to confirm the loader wiring works in this test file.
func TestResolveAction_ExplicitHardDelete(t *testing.T) {
	pruner := &stubPruner{returnN: 5}
	loader := &memory.StaticPolicyLoader{
		Policy: &omniav1alpha1.MemoryPolicy{
			Spec: omniav1alpha1.MemoryPolicySpec{
				ConsentRevocation: &omniav1alpha1.MemoryConsentRevocationConfig{
					Action: omniav1alpha1.ConsentRevocationHardDelete,
				},
			},
		},
	}
	svc := newConsentService(pruner, loader)

	n, err := svc.PruneUserConsentCategory(context.Background(), "ws-1", "u1", "memory:identity")

	require.NoError(t, err)
	assert.EqualValues(t, 5, n)
	assert.True(t, pruner.hardCalled, "HardDelete must be invoked when policy specifies HardDelete")
	assert.False(t, pruner.softCalled)
}
