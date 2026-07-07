/*
Copyright 2025.

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

// ToolAuth.Type values. Additional mechanisms (serviceAccount, workloadIdentity)
// are added alongside their operator support in later phases.
const (
	ToolAuthTypeNone   = "none"
	ToolAuthTypeBearer = "bearer"
	ToolAuthTypeBasic  = "basic"
)

// EffectiveAuth returns the handler's effective tool authentication. It is the
// single seam every consumer (operator config generation, credential mounting,
// runtime application) reads, so no layer needs to know about the deprecated
// per-config authType/authSecretRef fields.
//
// Precedence: an explicit handler-level auth stanza wins; otherwise the legacy
// httpConfig/openAPIConfig authType/authSecretRef are normalized into a ToolAuth
// (preserving the historical "secretRef present without an explicit type means
// bearer" default). Returns nil when no authentication is configured.
func (h *HandlerDefinition) EffectiveAuth() *ToolAuth {
	if h.Auth != nil {
		return h.Auth
	}
	authType, secretRef := h.legacyAuthFields()
	if authType == nil && secretRef == nil {
		return nil
	}
	t := ToolAuthTypeBearer // legacy default when a secretRef is present without an explicit type
	if authType != nil {
		t = *authType
	}
	return &ToolAuth{Type: t, SecretRef: secretRef}
}

// legacyAuthFields returns the deprecated authType/authSecretRef from whichever
// per-config carries them (only http and openapi ever did). Returns nil/nil when
// neither is set.
func (h *HandlerDefinition) legacyAuthFields() (*string, *SecretKeySelector) {
	switch {
	case h.HTTPConfig != nil && (h.HTTPConfig.AuthType != nil || h.HTTPConfig.AuthSecretRef != nil):
		return h.HTTPConfig.AuthType, h.HTTPConfig.AuthSecretRef
	case h.OpenAPIConfig != nil && (h.OpenAPIConfig.AuthType != nil || h.OpenAPIConfig.AuthSecretRef != nil):
		return h.OpenAPIConfig.AuthType, h.OpenAPIConfig.AuthSecretRef
	default:
		return nil, nil
	}
}
