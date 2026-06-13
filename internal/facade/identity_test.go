/*
Copyright 2026.

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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altairalabs/omnia/pkg/identity"
	"github.com/altairalabs/omnia/pkg/policy"
)

// requestWithHeaderAndQuery builds a request carrying the given header value
// (when non-empty) and device_id query param (when non-empty).
func requestWithHeaderAndQuery(t *testing.T, headerUserID, deviceID string) *http.Request {
	t.Helper()
	url := "http://example.test/ws?agent=test-agent"
	if deviceID != "" {
		url += "&device_id=" + deviceID
	}
	r := httptest.NewRequest(http.MethodGet, url, nil)
	if headerUserID != "" {
		r.Header.Set(policy.HeaderUserID, headerUserID)
	}
	return r
}

// TestResolveUserPseudonym_MgmtPlaneHeaderWins verifies that on a mgmt-plane
// origin the trusted X-Omnia-User-ID header is the authoritative end-user id,
// pseudonymized.
func TestResolveUserPseudonym_MgmtPlaneHeaderWins(t *testing.T) {
	r := requestWithHeaderAndQuery(t, "real-user", "some-device")
	authIdentity := &policy.AuthenticatedIdentity{
		Origin:  policy.OriginManagementPlane,
		EndUser: "operator@example.com",
	}

	got := ResolveUserPseudonym(r, authIdentity)
	want := identity.PseudonymizeID("real-user")
	if got != want {
		t.Errorf("ResolveUserPseudonym = %q, want %q (pseudonymize(header user))", got, want)
	}
}

// TestResolveUserPseudonym_NonMgmtPlaneIgnoresHeader is the security boundary:
// off the management plane the X-Omnia-User-ID header MUST NOT be trusted. The
// EndUser from the validated identity is used instead.
func TestResolveUserPseudonym_NonMgmtPlaneIgnoresHeader(t *testing.T) {
	r := requestWithHeaderAndQuery(t, "attacker-spoofed", "")
	authIdentity := &policy.AuthenticatedIdentity{
		Origin:  policy.OriginOIDC,
		EndUser: "carol@example.com",
	}

	got := ResolveUserPseudonym(r, authIdentity)
	want := identity.PseudonymizeID("carol@example.com")
	if got != want {
		t.Errorf("ResolveUserPseudonym = %q, want %q (EndUser, header ignored off mgmt-plane)", got, want)
	}
	if got == identity.PseudonymizeID("attacker-spoofed") {
		t.Fatal("X-Omnia-User-ID header was trusted off the management plane (security boundary violated)")
	}
}

// TestResolveUserPseudonym_DeviceIDFallback verifies the device_id query param
// is used when no auth identity is present and no Istio header carries a user.
func TestResolveUserPseudonym_DeviceIDFallback(t *testing.T) {
	r := requestWithHeaderAndQuery(t, "", "device-123")

	got := ResolveUserPseudonym(r, nil)
	want := identity.PseudonymizeID("device-123")
	if got != want {
		t.Errorf("ResolveUserPseudonym = %q, want %q (pseudonymize(device_id))", got, want)
	}
}

// TestResolveUserPseudonym_MgmtPlaneDeviceIDWhenNoHeader verifies that on a
// mgmt-plane origin with no header, the device_id wins over the EndUser
// (operator subject) — preserving the #1255 memory-scoping precedence.
func TestResolveUserPseudonym_MgmtPlaneDeviceIDWhenNoHeader(t *testing.T) {
	r := requestWithHeaderAndQuery(t, "", "device-xyz")
	authIdentity := &policy.AuthenticatedIdentity{
		Origin:  policy.OriginManagementPlane,
		EndUser: "operator-subject",
	}

	got := ResolveUserPseudonym(r, authIdentity)
	want := identity.PseudonymizeID("device-xyz")
	if got != want {
		t.Errorf("ResolveUserPseudonym = %q, want %q (device_id over operator subject)", got, want)
	}
}

// TestResolveUserPseudonym_IstioHeaderFallback verifies the Istio-injected
// header is used when no auth identity admitted the request.
func TestResolveUserPseudonym_IstioHeaderFallback(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://example.test/ws?agent=test-agent", nil)
	r.Header.Set(policy.IstioHeaderUserID, "istio-user")

	got := ResolveUserPseudonym(r, nil)
	want := identity.PseudonymizeID("istio-user")
	if got != want {
		t.Errorf("ResolveUserPseudonym = %q, want %q (Istio header)", got, want)
	}
}

// TestResolveUserPseudonym_AnonymousEmpty verifies a truly anonymous request
// (no auth, no headers, no device_id) yields an empty pseudonym. The session
// layer is responsible for the per-connection anonymous fallback.
func TestResolveUserPseudonym_AnonymousEmpty(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://example.test/ws?agent=test-agent", nil)

	if got := ResolveUserPseudonym(r, nil); got != "" {
		t.Errorf("ResolveUserPseudonym = %q, want empty for anonymous request", got)
	}
}
