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

	"github.com/altairalabs/omnia/pkg/identity"
	"github.com/altairalabs/omnia/pkg/policy"
)

// ResolveUserPseudonym derives the per-user memory/session-scoping pseudonym
// for a WebSocket upgrade request. It encapsulates the identity-resolution
// precedence shared by the facade so the WS path and the session-create path
// agree on a single stable id.
//
// Precedence (raw user id, pseudonymized via identity.PseudonymizeID):
//
//  1. When an auth validator admitted the request (authIdentity != nil):
//     - Management-plane origin: mgmtPlaneUserID(r, EndUser) — the trusted
//     on-behalf-of X-Omnia-User-ID header > device_id query param > token
//     subject. Only on the management plane is the header trusted; a raw
//     browser cannot mint a mgmt-plane JWT to reach this branch.
//     - Any other origin: authIdentity.EndUser. The X-Omnia-User-ID header is
//     DELIBERATELY ignored here — it is a security boundary. Off the
//     management plane the header is attacker-controllable and must never
//     override the validated identity.
//  2. With no auth identity: the Istio-injected x-user-id header (preserved
//     for deployments relying on the chart's authentication.enabled gate).
//  3. Last resort: the device_id query param.
//
// Empty input yields an empty string (PseudonymizeID("") == ""); callers that
// require a non-empty value must supply their own fallback.
//
// See #1255 (memory user identity) and #1285 (session attribution).
func ResolveUserPseudonym(r *http.Request, authIdentity *policy.AuthenticatedIdentity) string {
	var rawUserID string
	if authIdentity != nil {
		// Mgmt-plane JWTs identify the *dashboard operator*, not the end user
		// whose memories / sessions we scope. mgmtPlaneUserID resolves the
		// end-user id from the trusted on-behalf-of header (then device_id,
		// then the token subject). Off the management plane the header is NOT
		// trusted — use the validated EndUser only.
		if authIdentity.Origin == policy.OriginManagementPlane {
			rawUserID = mgmtPlaneUserID(r, authIdentity.EndUser)
		} else {
			rawUserID = authIdentity.EndUser
		}
	} else {
		rawUserID = r.Header.Get(policy.IstioHeaderUserID)
	}
	if rawUserID == "" {
		rawUserID = r.URL.Query().Get("device_id")
	}
	return identity.PseudonymizeID(rawUserID)
}

// pseudonymFromIdentity resolves a non-empty virtual-user pseudonym from a
// context-borne authenticated identity, falling back to pseudonymizing
// fallbackSeed when no identity is resolvable. It is the request-less
// counterpart to ResolveUserPseudonym, used by the MCP/invoke path which has
// only a ctx (no *http.Request, so no on-behalf-of header or device_id query).
//
// The guaranteed-non-empty result keeps the NOT-NULL sessions.virtual_user_id
// create from rejecting anonymous function invocations. With no identity, each
// invocation becomes its own virtual user (fallbackSeed is the invocation id).
// See #1285 (session attribution).
func pseudonymFromIdentity(id *policy.AuthenticatedIdentity, fallbackSeed string) string {
	if id != nil {
		if pseudonym := identity.PseudonymizeID(id.EndUser); pseudonym != "" {
			return pseudonym
		}
	}
	return identity.PseudonymizeID(fallbackSeed)
}
