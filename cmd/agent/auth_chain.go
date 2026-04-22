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

package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/facade/auth"
)

// sharedTokenSecretKey is the data key the chart-managed Secret holds
// the bearer value under. Matches the existing OMNIA_A2A_AUTH_TOKEN
// pattern (and the documented A2A integration).
const sharedTokenSecretKey = "token"

// buildAuthChain assembles the facade's per-agent auth chain. Order:
//
//	sharedToken → mgmt-plane
//
// (apiKeys, oidc, edgeTrust slot in here in PRs 2c/2d/2e — they are
// no-ops here when their CRD field is unset.)
//
// Inputs:
//
//   - k8s: client for reading the AgentRuntime CR + the referenced
//     SharedToken Secret. Pass nil when the agent is running outside a
//     cluster (dev/test); the chain falls back to mgmt-plane only.
//   - mgmtPlane: the mgmt-plane validator built earlier in startup
//     (loadMgmtPlaneValidator). Pass nil to omit it from the chain.
//
// Returns nil if the chain ends up empty — the facade then runs in the
// PR 1a/c default unauthenticated-upgrade mode.
func buildAuthChain(
	ctx context.Context,
	k8s client.Client,
	log logr.Logger,
	agentName, namespace string,
	mgmtPlane auth.Validator,
) (auth.Chain, error) {
	chain := auth.Chain{}

	if k8s != nil && agentName != "" && namespace != "" {
		ar := &omniav1alpha1.AgentRuntime{}
		err := k8s.Get(ctx, types.NamespacedName{Name: agentName, Namespace: namespace}, ar)
		switch {
		case apierrors.IsNotFound(err):
			// Agent is being created or deleted out-of-band — fall back
			// to mgmt-plane only and let the controller catch up.
			log.V(1).Info("agent runtime not found at startup, skipping data-plane validators",
				"agent", agentName, "namespace", namespace)
		case err != nil:
			return nil, fmt.Errorf("get AgentRuntime %s/%s: %w", namespace, agentName, err)
		default:
			// Fold the deprecated spec.a2a.authentication.secretRef into
			// spec.externalAuth.sharedToken. The reconciler does the same
			// on its in-memory copy, but that copy is never persisted, so
			// this side needs to re-run the projection — otherwise legacy
			// CRs without spec.externalAuth produce an empty data-plane
			// chain and every A2A request 401s after PR 3's default flip.
			omniav1alpha1.ProjectLegacyA2AAuth(ar)
			validators, err := buildDataPlaneValidators(ctx, k8s, log, ar)
			if err != nil {
				return nil, err
			}
			chain = append(chain, validators...)
		}
	} else {
		log.V(1).Info("data-plane chain build skipped",
			"reason", "no k8s client or identity",
			"hasK8sClient", k8s != nil,
			"hasAgentName", agentName != "",
			"hasNamespace", namespace != "")
	}

	if mgmtPlane != nil {
		chain = append(chain, mgmtPlane)
	}

	if len(chain) == 0 {
		return nil, nil
	}
	return chain, nil
}

// buildDataPlaneValidators reads the AgentRuntime's spec.externalAuth
// and constructs the matching validators. Each sub-block is independent;
// missing blocks contribute zero validators (the chain just walks the
// remaining ones in order).
func buildDataPlaneValidators(
	ctx context.Context,
	k8s client.Client,
	log logr.Logger,
	ar *omniav1alpha1.AgentRuntime,
) ([]auth.Validator, error) {
	if ar.Spec.ExternalAuth == nil {
		return nil, nil
	}

	var out []auth.Validator
	if v, err := buildSharedTokenValidator(ctx, k8s, log, ar); err != nil {
		return nil, err
	} else if v != nil {
		out = append(out, v)
	}
	if v, err := buildAPIKeyValidator(ctx, k8s, log, ar); err != nil {
		return nil, err
	} else if v != nil {
		out = append(out, v)
	}
	if v, err := buildOIDCValidator(ctx, k8s, log, ar); err != nil {
		return nil, err
	} else if v != nil {
		out = append(out, v)
	}
	if v := buildEdgeTrustValidator(log, ar); v != nil {
		out = append(out, v)
	}
	return out, nil
}

// buildOIDCValidator constructs the OIDC validator when
// spec.externalAuth.oidc is set. Reads the JWKS from the per-agent
// Secret maintained by the AgentRuntime controller (PR 2d-2); missing
// or malformed JWKS is fatal so operator misconfig surfaces at pod
// startup rather than silently 401ing every request.
func buildOIDCValidator(
	ctx context.Context,
	k8s client.Client,
	log logr.Logger,
	ar *omniav1alpha1.AgentRuntime,
) (auth.Validator, error) {
	oidc := ar.Spec.ExternalAuth.OIDC
	if oidc == nil {
		return nil, nil
	}
	if oidc.Issuer == "" || oidc.Audience == "" {
		return nil, fmt.Errorf("spec.externalAuth.oidc requires issuer and audience")
	}

	secretName := OIDCJWKSSecretNameFor(ar.Name)
	store, err := NewSecretBackedJWKSStore(ctx, k8s, ar.Namespace, secretName, log)
	if err != nil {
		return nil, fmt.Errorf("init OIDC JWKS store: %w", err)
	}

	opts := []auth.OIDCOption{}
	if oidc.ClaimMapping != nil {
		opts = append(opts, auth.WithOIDCClaimMapping(auth.OIDCClaimMapping{
			Subject: oidc.ClaimMapping.Subject,
			Role:    oidc.ClaimMapping.Role,
			EndUser: oidc.ClaimMapping.EndUser,
		}))
	}
	v, err := auth.NewOIDCValidator(oidc.Issuer, oidc.Audience, store, opts...)
	if err != nil {
		return nil, fmt.Errorf("construct OIDC validator: %w", err)
	}
	log.Info("oidc validator enabled",
		"issuer", oidc.Issuer,
		"audience", oidc.Audience,
		"hasClaimMapping", oidc.ClaimMapping != nil)
	return v, nil
}

// buildAPIKeyValidator constructs the api-key validator when
// spec.externalAuth.apiKeys is set. Returns nil when not configured.
// The KeyStore lifetime is tied to the validator's — it leaks for the
// life of the process; that's fine because the facade only constructs
// the chain once at startup.
func buildAPIKeyValidator(
	ctx context.Context,
	k8s client.Client,
	log logr.Logger,
	ar *omniav1alpha1.AgentRuntime,
) (auth.Validator, error) {
	ak := ar.Spec.ExternalAuth.APIKeys
	if ak == nil {
		return nil, nil
	}

	store, err := NewSecretBackedKeyStore(ctx, k8s, ar.Namespace, ar.Name, log)
	if err != nil {
		return nil, fmt.Errorf("init api-key store: %w", err)
	}

	opts := []auth.APIKeyOption{}
	if ak.DefaultRole != "" {
		opts = append(opts, auth.WithAPIKeyDefaultRole(ak.DefaultRole))
	}
	if ak.TrustEndUserHeader {
		opts = append(opts, auth.WithAPIKeyTrustEndUserHeader(true))
	}
	v := auth.NewAPIKeyValidator(store, opts...)
	log.Info("api-key validator enabled",
		"defaultRole", ak.DefaultRole,
		"trustEndUserHeader", ak.TrustEndUserHeader)
	return v, nil
}

// buildEdgeTrustValidator constructs the edgeTrust validator when
// spec.externalAuth.edgeTrust is set. Pure-Go construction — no Secret
// reads, no API calls — so this can never fail. The validator trusts
// inbound headers the operator has guaranteed (via Istio
// AuthorizationPolicy or equivalent) cannot be spoofed by external
// callers.
func buildEdgeTrustValidator(log logr.Logger, ar *omniav1alpha1.AgentRuntime) auth.Validator {
	et := ar.Spec.ExternalAuth.EdgeTrust
	if et == nil {
		return nil
	}

	opts := []auth.EdgeTrustOption{}
	if et.HeaderMapping != nil {
		opts = append(opts,
			auth.WithEdgeTrustSubjectHeader(et.HeaderMapping.Subject),
			auth.WithEdgeTrustRoleHeader(et.HeaderMapping.Role),
			auth.WithEdgeTrustEndUserHeader(et.HeaderMapping.EndUser),
			auth.WithEdgeTrustEmailHeader(et.HeaderMapping.Email),
		)
	}
	if len(et.ClaimsFromHeaders) > 0 {
		opts = append(opts, auth.WithEdgeTrustExtraClaims(et.ClaimsFromHeaders))
	}
	v := auth.NewEdgeTrustValidator(opts...)
	log.Info("edgeTrust validator enabled",
		"hasHeaderMapping", et.HeaderMapping != nil,
		"extraClaims", len(et.ClaimsFromHeaders))
	return v
}

// buildSharedTokenValidator resolves spec.externalAuth.sharedToken into
// a *SharedTokenValidator by reading the referenced Secret. Returns nil
// (no validator) when the field is unset; returns an error when the
// Secret can't be read or is missing the "token" key — operators expect
// loud failure on misconfig rather than silent skip.
func buildSharedTokenValidator(
	ctx context.Context,
	k8s client.Client,
	log logr.Logger,
	ar *omniav1alpha1.AgentRuntime,
) (auth.Validator, error) {
	st := ar.Spec.ExternalAuth.SharedToken
	if st == nil {
		return nil, nil
	}
	if st.SecretRef.Name == "" {
		return nil, errors.New("spec.externalAuth.sharedToken.secretRef.name is empty")
	}

	secret := &corev1.Secret{}
	err := k8s.Get(ctx, types.NamespacedName{Name: st.SecretRef.Name, Namespace: ar.Namespace}, secret)
	if err != nil {
		return nil, fmt.Errorf("get sharedToken secret %s/%s: %w", ar.Namespace, st.SecretRef.Name, err)
	}
	tokenBytes, ok := secret.Data[sharedTokenSecretKey]
	if !ok || len(tokenBytes) == 0 {
		return nil, fmt.Errorf("sharedToken secret %s/%s missing %q data key",
			ar.Namespace, st.SecretRef.Name, sharedTokenSecretKey)
	}

	opts := []auth.SharedTokenOption{}
	if st.TrustEndUserHeader {
		opts = append(opts, auth.WithSharedTokenTrustEndUserHeader(true))
	}
	v, err := auth.NewSharedTokenValidator(string(tokenBytes), opts...)
	if err != nil {
		return nil, fmt.Errorf("construct sharedToken validator: %w", err)
	}
	log.Info("sharedToken validator enabled",
		"secret", st.SecretRef.Name,
		"trustEndUserHeader", st.TrustEndUserHeader)
	return v, nil
}
