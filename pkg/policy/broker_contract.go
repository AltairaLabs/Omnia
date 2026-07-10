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

package policy

// This file defines the wire contract for the ToolPolicy decision broker
// (POST /v1/decision). It lives in the shared pkg/policy package — rather
// than the enterprise-only ee/pkg/policy package where the broker HTTP
// handler is implemented — because internal/runtime (core) must be able to
// build a DecisionRequest and parse a DecisionResponse without importing
// ee/. ee/pkg/policy.BrokerHandler aliases these same types so the wire
// shape stays identical on both sides of the hop.

// DecisionRequest is the JSON request body for POST /v1/decision. The
// runtime sends the same (headers, body) shape the evaluator already
// understands, plus a structured Identity so `identity.*` CEL rules and
// identity-aware header injection work without lossy header-flattening.
type DecisionRequest struct {
	Headers  map[string]string      `json:"headers"`
	Body     map[string]interface{} `json:"body"`
	Identity *IdentityPayload       `json:"identity"`
}

// IdentityPayload carries the caller's AuthenticatedIdentity fields over the
// wire so the broker can rebuild an AuthenticatedIdentity and attach it to
// the evaluation context.
//
// IssuedAt/ExpiresAt are deliberately NOT carried here: no ToolPolicy rule
// references token-freshness today (identity.origin/subject/role/claims
// cover current use cases), so the wire-format complexity of serializing
// time.Time (clock skew, zero-value vs "not applicable") isn't earning its
// keep yet. Add them (and thread through IdentityPayloadFromIdentity /
// IdentityPayloadFromPropagation / ee/pkg/policy.withIdentityFromPayload) if
// a CEL rule ever needs freshness checking.
type IdentityPayload struct {
	Origin    string            `json:"origin"`
	Subject   string            `json:"subject"`
	EndUser   string            `json:"endUser"`
	Workspace string            `json:"workspace"`
	Agent     string            `json:"agent"`
	Claims    map[string]string `json:"claims"`
}

// DecisionResponse is the JSON response body for POST /v1/decision.
type DecisionResponse struct {
	Allow           bool              `json:"allow"`
	DeniedBy        string            `json:"deniedBy"`
	Message         string            `json:"message"`
	Mode            string            `json:"mode"`
	WouldDeny       bool              `json:"wouldDeny"`
	InjectedHeaders map[string]string `json:"injectedHeaders"`
}

// IdentityPayloadFromIdentity builds an IdentityPayload from an
// AuthenticatedIdentity for transmission over the wire. Returns nil when id
// is nil, so callers can pass IdentityFromContext(ctx) straight through.
func IdentityPayloadFromIdentity(id *AuthenticatedIdentity) *IdentityPayload {
	if id == nil {
		return nil
	}
	return &IdentityPayload{
		Origin:    id.Origin,
		Subject:   id.Subject,
		EndUser:   id.EndUser,
		Workspace: id.Workspace,
		Agent:     id.Agent,
		Claims:    id.Claims,
	}
}

// IdentityPayloadFromPropagation builds an IdentityPayload from the flat
// PropagationFields the runtime actually has available. This is the payload
// path used in production: the structured AuthenticatedIdentity the facade
// builds (PropagationFields.Identity) is in-process only and never crosses
// the facade -> runtime gRPC hop (see context.go's ContextKeyIdentity doc),
// so IdentityPayloadFromIdentity(IdentityFromContext(ctx)) is always nil on
// the runtime side and identity.* ToolPolicy rules never fire. This
// reconstructs what it faithfully can from the rehydrated flat fields
// (internal/runtime/interceptor.go's extractPolicyFromMetadata):
//
//   - Subject / EndUser <- fields.UserID. The wire format carries a single
//     pseudonymised caller id; both AuthenticatedIdentity roles collapse
//     onto it since there's no separate propagated "acting on behalf of"
//     value.
//   - Claims <- fields.Claims, verbatim. Role rides in Claims["role"] rather
//     than a dedicated field — ToolPolicy rules gate on identity.claims.role,
//     not a structured identity.role. See identity.go.
//   - Agent  <- fields.AgentName, the agent this tool call is running
//     under. Sourced from the request/env (not the auth token), but it is
//     the same value ToolPolicy fixtures use for identity.agent and the
//     only agent-identifying context the runtime has.
//   - Origin / Workspace are left unset (zero value). Origin (which
//     validator admitted the request) and Workspace (the auth token's
//     workspace scope) live only on the facade's in-process
//     AuthenticatedIdentity; PropagationFields.Namespace is a distinct
//     concept (the K8s namespace, not a workspace claim) and there is no
//     dedicated propagation header for either today. ToolPolicy rules keyed
//     on identity.origin / identity.workspace will not match until a
//     dedicated propagation path is added — do not fabricate values for
//     them here.
//
// Returns nil when none of the reconstructible fields are set, so callers
// that always invoke this don't send an empty-but-non-nil identity object
// for unauthenticated/dev-mode traffic.
func IdentityPayloadFromPropagation(fields *PropagationFields) *IdentityPayload {
	if fields == nil {
		return nil
	}
	if fields.UserID == "" && fields.AgentName == "" && len(fields.Claims) == 0 {
		return nil
	}
	return &IdentityPayload{
		Subject: fields.UserID,
		EndUser: fields.UserID,
		Agent:   fields.AgentName,
		Claims:  fields.Claims,
	}
}
