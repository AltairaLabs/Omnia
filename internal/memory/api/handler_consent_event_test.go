/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/memory"
)

// --- test doubles ---

// stubPruner records the most-recent call and returns canned values.
type stubPruner struct {
	softCalled bool
	hardCalled bool
	callCount  int
	returnN    int64
	returnErr  error
	// capture last call args
	lastWorkspace string
	lastUserID    string
	lastCategory  string
}

func (s *stubPruner) SoftDeleteUserConsentCategory(_ context.Context, ws, uid, cat string) (int64, error) {
	s.softCalled = true
	s.callCount++
	s.lastWorkspace, s.lastUserID, s.lastCategory = ws, uid, cat
	return s.returnN, s.returnErr
}

func (s *stubPruner) HardDeleteUserConsentCategory(_ context.Context, ws, uid, cat string) (int64, error) {
	s.hardCalled = true
	s.callCount++
	s.lastWorkspace, s.lastUserID, s.lastCategory = ws, uid, cat
	return s.returnN, s.returnErr
}

// policyWithAction returns a StaticPolicyLoader whose MemoryPolicy has the
// given consentRevocation.action set.
func policyWithAction(action omniav1alpha1.ConsentRevocationAction) memory.PolicyLoader {
	return &memory.StaticPolicyLoader{
		Policy: &omniav1alpha1.MemoryPolicy{
			Spec: omniav1alpha1.MemoryPolicySpec{
				ConsentRevocation: &omniav1alpha1.MemoryConsentRevocationConfig{
					Action: action,
				},
			},
		},
	}
}

// makeConsentEventHandler builds an enterprise-enabled handler wired with the
// given pruner and policy loader.
func makeConsentEventHandler(pruner memory.ConsentEventPruner, loader memory.PolicyLoader) http.Handler {
	svc := NewMemoryService(&mockStore{}, nil, MemoryServiceConfig{}, logr.Discard())
	if pruner != nil {
		svc.SetConsentEventPruner(pruner)
	}
	if loader != nil {
		svc.SetPolicyLoader(loader)
	}
	h := NewHandler(svc, logr.Discard()).WithEnterprise(true)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

// postConsentEvent fires POST /api/v1/memories/consent-events with the given
// body against the supplied handler and returns the recorder.
func postConsentEvent(h http.Handler, workspace string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	path := "/api/v1/memories/consent-events"
	if workspace != "" {
		path += "?workspace=" + workspace
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// --- tests ---

func TestConsentEventHandler_SoftDeleteAction(t *testing.T) {
	pruner := &stubPruner{returnN: 3}
	h := makeConsentEventHandler(pruner, policyWithAction(omniav1alpha1.ConsentRevocationSoftDelete))

	rr := postConsentEvent(h, "ws-1", ConsentEventRequest{UserID: "u1", Category: "memory:health"})

	require.Equal(t, http.StatusOK, rr.Code)
	var resp ConsentEventResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.EqualValues(t, 3, resp.Deleted)

	assert.True(t, pruner.softCalled, "SoftDelete must be called for SoftDelete action")
	assert.False(t, pruner.hardCalled)
	assert.Equal(t, "ws-1", pruner.lastWorkspace)
	assert.Equal(t, "u1", pruner.lastUserID)
	assert.Equal(t, "memory:health", pruner.lastCategory)
}

func TestConsentEventHandler_HardDeleteAction(t *testing.T) {
	pruner := &stubPruner{returnN: 5}
	h := makeConsentEventHandler(pruner, policyWithAction(omniav1alpha1.ConsentRevocationHardDelete))

	rr := postConsentEvent(h, "ws-2", ConsentEventRequest{UserID: "u2", Category: "memory:context"})

	require.Equal(t, http.StatusOK, rr.Code)
	var resp ConsentEventResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.EqualValues(t, 5, resp.Deleted)

	assert.True(t, pruner.hardCalled, "HardDelete must be called for HardDelete action")
	assert.False(t, pruner.softCalled)
}

func TestConsentEventHandler_StopAction_NoOp(t *testing.T) {
	pruner := &stubPruner{}
	h := makeConsentEventHandler(pruner, policyWithAction(omniav1alpha1.ConsentRevocationStop))

	rr := postConsentEvent(h, "ws-1", ConsentEventRequest{UserID: "u1", Category: "memory:health"})

	require.Equal(t, http.StatusOK, rr.Code)
	var resp ConsentEventResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.EqualValues(t, 0, resp.Deleted)

	assert.Equal(t, 0, pruner.callCount, "Stop action must not call the pruner")
}

func TestConsentEventHandler_MissingUserID_Returns400(t *testing.T) {
	h := makeConsentEventHandler(&stubPruner{}, nil)
	rr := postConsentEvent(h, "ws-1", ConsentEventRequest{Category: "memory:health"})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestConsentEventHandler_MissingCategory_Returns400(t *testing.T) {
	h := makeConsentEventHandler(&stubPruner{}, nil)
	rr := postConsentEvent(h, "ws-1", ConsentEventRequest{UserID: "u1"})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestConsentEventHandler_MissingWorkspace_Returns400(t *testing.T) {
	h := makeConsentEventHandler(&stubPruner{}, nil)
	rr := postConsentEvent(h, "", ConsentEventRequest{UserID: "u1", Category: "memory:health"})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestConsentEventHandler_NotEnterprise_Returns403(t *testing.T) {
	svc := NewMemoryService(&mockStore{}, nil, MemoryServiceConfig{}, logr.Discard())
	h := NewHandler(svc, logr.Discard()).WithEnterprise(false) // NOT enterprise
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rr := postConsentEvent(mux, "ws-1", ConsentEventRequest{UserID: "u1", Category: "memory:health"})
	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestConsentEventHandler_InvalidJSON_Returns400(t *testing.T) {
	h := makeConsentEventHandler(&stubPruner{}, nil)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/memories/consent-events?workspace=ws-1",
		bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestConsentEventHandler_DefaultPolicySoftDelete verifies that when no
// policy loader is wired the handler defaults to SoftDelete.
func TestConsentEventHandler_DefaultPolicySoftDelete(t *testing.T) {
	pruner := &stubPruner{returnN: 1}
	h := makeConsentEventHandler(pruner, nil) // no loader → default SoftDelete

	rr := postConsentEvent(h, "ws-1", ConsentEventRequest{UserID: "u1", Category: "memory:health"})
	require.Equal(t, http.StatusOK, rr.Code)
	assert.True(t, pruner.softCalled)
}
