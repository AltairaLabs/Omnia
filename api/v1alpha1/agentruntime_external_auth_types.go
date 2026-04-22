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

package v1alpha1

import corev1 "k8s.io/api/core/v1"

// AgentExternalAuth configures authentication for data-plane clients
// (external apps streaming to this agent's facade).
//
// Each validator is independent. When multiple are configured, any one
// that accepts the request admits it. Not configuring any validator here
// makes the agent reachable only from the management plane (the
// dashboard's debug view) — no customer traffic until an admin fills in
// at least one validator. This is the secure default.
//
// PR 2a (this struct + the legacy projection) is behaviour-preserving —
// no validator is wired into the facade middleware until later PRs in
// the series. The CRD shape lands first so operators can start adopting
// it without a deploy-flag-day.
type AgentExternalAuth struct {
	// allowManagementPlane governs whether dashboard-minted management-
	// plane tokens (the "Try this agent" debug view) are accepted for
	// this agent. Defaults to true so the debug view works out of the
	// box; paranoid customers wanting strict data-plane-only isolation
	// set this to false explicitly.
	//
	// Pointer with default=true so an explicit `false` stays
	// distinguishable from "field omitted" for future audit/migration
	// logic. Only consulted when spec.externalAuth is set; when the whole
	// block is unset the facade defaults to mgmt-plane-only.
	// +kubebuilder:default=true
	// +optional
	AllowManagementPlane *bool `json:"allowManagementPlane,omitempty"`

	// sharedToken validates a single bearer token shared across all
	// callers of this agent. Simplest partner integration; one token in
	// a Kubernetes Secret, rotated by editing the Secret.
	//
	// Subsumes the existing spec.a2a.authentication.secretRef field
	// (which is now deprecated and transparently projected into this
	// location by the AgentRuntime controller at reconcile time).
	// +optional
	SharedToken *SharedTokenAuth `json:"sharedToken,omitempty"`

	// apiKeys configures per-caller API keys for this agent. Each key is
	// stored as a Kubernetes Secret in the agent's namespace with a
	// sha256 hash of the raw value, scopes, and expiry. Created via the
	// dashboard UI; never stored in the CR. The presence of this field
	// (even an empty struct) tells the facade to treat keys labelled for
	// this agent as valid.
	// +optional
	APIKeys *APIKeysAuth `json:"apiKeys,omitempty"`

	// oidc validates JWTs issued by a customer's OIDC provider inside
	// the facade. Uses the standard discovery document at
	// {issuer}/.well-known/openid-configuration to fetch JWKS. Caches
	// keys in a Secret alongside the AgentRuntime.
	// +optional
	OIDC *OIDCAuth `json:"oidc,omitempty"`

	// edgeTrust consumes claim-headers injected by an upstream JWT
	// validator (Istio RequestAuthentication + outputClaimToHeaders, an
	// API gateway, or any other edge that terminates the JWT). The
	// facade does NOT re-verify the token — it trusts the configured
	// headers and maps them onto the Identity struct + the downstream
	// X-Omnia-Claim-<name> contract.
	//
	// Operator MUST ensure the configured claim-headers cannot be
	// injected by anyone other than the trusted edge (Istio's
	// AuthorizationPolicy already strips inbound claim-headers on the
	// chart's authentication.enabled=true setup).
	// +optional
	EdgeTrust *EdgeTrustAuth `json:"edgeTrust,omitempty"`
}

// SharedTokenAuth validates a single bearer token shared by all callers.
type SharedTokenAuth struct {
	// secretRef references a Secret with key "token" holding the bearer
	// value.
	// +kubebuilder:validation:Required
	SecretRef corev1.LocalObjectReference `json:"secretRef"`

	// trustEndUserHeader lets the caller forward the end-user identity
	// via the X-End-User-Id request header. Off by default — when off,
	// Identity.EndUser is set equal to Identity.Subject (the token
	// itself), so per-user audit granularity is coarse.
	//
	// Turn on only when the calling app is trusted to faithfully forward
	// user context. A malicious app holding a valid token can spoof
	// arbitrary end-users, so ToolPolicy rules gating on identity.endUser
	// must be paired with an app-level trust assessment.
	// +optional
	TrustEndUserHeader bool `json:"trustEndUserHeader,omitempty"`
}

// APIKeysAuth turns on per-caller API key validation for this agent.
// The key list itself lives in Secrets (keyed by sha256 of the raw value),
// not in the CR — this struct is a policy toggle.
type APIKeysAuth struct {
	// defaultRole is applied to API keys that don't specify one.
	// +kubebuilder:validation:Enum=viewer;editor;admin
	// +kubebuilder:default=viewer
	// +optional
	DefaultRole string `json:"defaultRole,omitempty"`

	// trustEndUserHeader — same semantics as SharedTokenAuth; see that
	// field's doc comment for the security trade-off.
	// +optional
	TrustEndUserHeader bool `json:"trustEndUserHeader,omitempty"`
}

// OIDCAuth configures the facade to validate customer-issued JWTs
// against an OIDC discovery document.
type OIDCAuth struct {
	// issuer is the OIDC issuer URL (without trailing slash). Controller
	// fetches {issuer}/.well-known/openid-configuration.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Issuer string `json:"issuer"`

	// audience is the expected `aud` claim value.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Audience string `json:"audience"`

	// claimMapping maps JWT claim names to the internal identity fields.
	// Defaults: sub→Subject, omnia.role→Role, sub→EndUser.
	// +optional
	ClaimMapping *OIDCClaimMapping `json:"claimMapping,omitempty"`
}

// OIDCClaimMapping overrides the JWT claim names the OIDC validator
// reads when populating the AuthenticatedIdentity.
type OIDCClaimMapping struct {
	// subject names the claim used as the caller's stable identifier
	// (the token-holder — app or user). Defaults to "sub".
	// +optional
	Subject string `json:"subject,omitempty"`

	// role names the claim used to resolve the caller's role. Defaults
	// to "omnia.role". The claim value must be one of viewer/editor/admin.
	// +optional
	Role string `json:"role,omitempty"`

	// endUser names the claim used to identify the end-user on whose
	// behalf this token was issued. Defaults to "sub" — correct for
	// end-user tokens. For service/client-credentials tokens carrying
	// an actor or on-behalf-of claim, set this to that claim name (e.g.,
	// "actor", "on_behalf_of") so the end-user is extracted rather than
	// the calling service.
	//
	// If the named claim is missing from a given token, the validator
	// falls back to Subject.
	// +optional
	EndUser string `json:"endUser,omitempty"`
}

// EdgeTrustAuth configures the facade to trust claim-headers injected
// by an upstream JWT validator (typically Istio RequestAuthentication
// with outputClaimToHeaders).
type EdgeTrustAuth struct {
	// headerMapping maps outbound X-Omnia-Claim-<name> keys to the
	// inbound headers the upstream edge is known to inject.
	//
	// The defaults cover the chart's current authentication.enabled=true
	// layout (charts/omnia/templates/gateway/authentication.yaml):
	//   subject: "x-user-id"
	//   role:    "x-user-roles"
	//   endUser: "x-user-id"
	//   email:   "x-user-email"
	//
	// Operators running a different edge (custom EnvoyFilter, API
	// gateway, service mesh other than Istio) override these to name the
	// headers their edge actually emits.
	// +optional
	HeaderMapping *EdgeTrustHeaderMapping `json:"headerMapping,omitempty"`

	// claimsFromHeaders lists any additional inbound headers whose
	// values should be exposed to ToolPolicy as X-Omnia-Claim-<claim>.
	// Keyed by the inbound header name (case-insensitive); value is the
	// claim name to emit.
	//
	// Example: {"x-user-groups": "groups"} makes Istio-injected group
	// claims visible to CEL rules as identity.claims.groups.
	// +optional
	ClaimsFromHeaders map[string]string `json:"claimsFromHeaders,omitempty"`
}

// EdgeTrustHeaderMapping overrides the inbound header names the
// edgeTrust validator reads when an upstream edge uses non-default
// claim-header conventions.
type EdgeTrustHeaderMapping struct {
	// +optional
	Subject string `json:"subject,omitempty"`
	// +optional
	Role string `json:"role,omitempty"`
	// +optional
	EndUser string `json:"endUser,omitempty"`
	// +optional
	Email string `json:"email,omitempty"`
}
