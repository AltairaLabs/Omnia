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

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// OIDC JWKS mirror constants. Match the facade side in
// cmd/agent/oidc_jwks.go so admins can rely on a stable Secret shape.
const (
	// OIDCJWKSSecretSuffix is appended to `agent-<name>` to form the
	// conventional per-agent JWKS Secret name.
	OIDCJWKSSecretSuffix = "-oidc-jwks"

	// OIDCJWKSDataKey is the key inside the Secret that holds the raw
	// JWKS JSON blob (verbatim from the issuer's jwks_uri).
	OIDCJWKSDataKey = "jwks.json"

	// labelCredentialKind tags per-agent credential Secrets so the
	// dashboard's credential-listing endpoints can enumerate them
	// without a kind-specific label per Secret type.
	labelCredentialKind = "omnia.altairalabs.ai/credential-kind"

	// LabelCredentialKindAgentOIDCJWKS is the value stamped onto
	// labelCredentialKind for the mirror Secret.
	LabelCredentialKindAgentOIDCJWKS = "agent-oidc-jwks"

	// OIDCJWKSRefreshInterval is how often the reconciler re-fetches
	// the JWKS from the issuer. The design doc sets 6h; IdPs that
	// rotate faster than this need the on-demand stale-signal path
	// (deferred to a future PR).
	OIDCJWKSRefreshInterval = 6 * time.Hour

	// OIDCDiscoveryPath is the well-known endpoint per RFC 8414.
	OIDCDiscoveryPath = "/.well-known/openid-configuration"

	// OIDCJWKSHTTPTimeout bounds the issuer round-trip. Aggressive so a
	// wedged IdP does NOT serialise behind every AgentRuntime reconcile
	// (the T8 review finding — a previous 15s cap meant a fleet of N
	// OIDC-backed agents could each pay a 15s hit on a slow issuer per
	// reconcile pass). 5s is enough for any reasonable IdP; slow
	// discovery flows fall back to the cached Secret if one exists.
	OIDCJWKSHTTPTimeout = 5 * time.Second

	// OIDCJWKSFetchedAtAnnotation stamps the Secret with the time of
	// the last successful fetch in RFC3339 format. The reconciler skips
	// the HTTP round-trip entirely when the annotation is newer than
	// now - RefreshInterval, so a reconcile triggered by an unrelated
	// AgentRuntime change doesn't cause an unconditional fetch.
	OIDCJWKSFetchedAtAnnotation = "omnia.altairalabs.ai/oidc-jwks-fetched-at"
)

// ConditionTypeOIDCJWKSReady is the status condition surfacing the
// reconciler's last fetch outcome. Set True on successful upsert,
// False with a Reason of DiscoveryFailed / JWKSFetchFailed /
// JWKSInvalid so operators can see why their agent refuses external
// callers.
const ConditionTypeOIDCJWKSReady = "OIDCJWKSReady"

// oidcDiscovery is the subset of the OIDC provider configuration we
// care about (RFC 8414). We only need jwks_uri; other fields are
// ignored so a surprise IdP can still produce a parseable response.
type oidcDiscovery struct {
	JWKSURI string `json:"jwks_uri"`
}

// reconcileOIDCJWKS fetches the issuer's JWKS and mirrors it into
// `agent-<name>-oidc-jwks`. Returns the requeue duration for the next
// scheduled refresh (0 when OIDC isn't configured — nothing to
// refresh).
//
// The AgentRuntime controller calls this after the deployment + other
// resources are reconciled; failures here don't block agent bring-up
// (the facade's empty JWKS store just 401s OIDC tokens until the next
// reconcile succeeds).
func (r *AgentRuntimeReconciler) reconcileOIDCJWKS(
	ctx context.Context,
	ar *omniav1alpha1.AgentRuntime,
) (time.Duration, error) {
	log := logf.FromContext(ctx).WithValues("oidc-jwks", ar.Name)

	if ar.Spec.ExternalAuth == nil || ar.Spec.ExternalAuth.OIDC == nil {
		// OIDC not configured — drop any stale Secret so a flip from
		// enabled-to-disabled doesn't leave dead JWKS lying around.
		if err := r.deleteOIDCJWKSSecretIfPresent(ctx, ar); err != nil {
			return 0, fmt.Errorf("clean up stale jwks secret: %w", err)
		}
		return 0, nil
	}

	oidc := ar.Spec.ExternalAuth.OIDC
	if oidc.Issuer == "" {
		r.setOIDCJWKSCondition(ar, metav1.ConditionFalse, "MissingIssuer",
			"spec.externalAuth.oidc.issuer is empty")
		return 0, fmt.Errorf("oidc issuer is empty")
	}

	// Fast path: a fresh Secret means we don't need to re-fetch just
	// because the AgentRuntime reconciled for an unrelated reason
	// (deployment update, status change, etc.). Costs one Get on the
	// Secret, saves a blocking HTTP round-trip against the IdP — T8.
	if remaining, fresh := r.jwksSecretStillFresh(ctx, ar); fresh {
		r.setOIDCJWKSCondition(ar, metav1.ConditionTrue, "JWKSUpdated",
			fmt.Sprintf("JWKS mirrored from %s (cached)", oidc.Issuer))
		return remaining, nil
	}

	jwks, err := r.fetchOIDCJWKS(ctx, oidc.Issuer)
	if err != nil {
		r.setOIDCJWKSCondition(ar, metav1.ConditionFalse, "DiscoveryFailed", err.Error())
		// Don't return the error — let the reconciler continue so
		// other agents aren't held up. Next RequeueAfter retries.
		log.Error(err, "OIDC JWKS fetch failed; will retry")
		return OIDCJWKSRefreshInterval, nil
	}

	if err := r.upsertOIDCJWKSSecret(ctx, ar, jwks); err != nil {
		r.setOIDCJWKSCondition(ar, metav1.ConditionFalse, "SecretWriteFailed", err.Error())
		return OIDCJWKSRefreshInterval, fmt.Errorf("upsert jwks secret: %w", err)
	}

	r.setOIDCJWKSCondition(ar, metav1.ConditionTrue, "JWKSUpdated",
		fmt.Sprintf("JWKS mirrored from %s", oidc.Issuer))
	return OIDCJWKSRefreshInterval, nil
}

// fetchOIDCJWKS fetches {issuer}/.well-known/openid-configuration,
// follows jwks_uri, and returns the raw JWKS JSON blob. The caller
// stores it verbatim so the facade sees exactly what the IdP published
// — no round-trip through an intermediate struct that might drop
// fields the IdP cares about (x5c, key_ops, etc.).
func (r *AgentRuntimeReconciler) fetchOIDCJWKS(ctx context.Context, issuer string) ([]byte, error) {
	client := r.oidcHTTPClient()
	discoveryURL := strings.TrimRight(issuer, "/") + OIDCDiscoveryPath

	disc, err := fetchOIDCDiscovery(ctx, client, discoveryURL)
	if err != nil {
		return nil, err
	}
	if disc.JWKSURI == "" {
		return nil, fmt.Errorf("discovery document missing jwks_uri")
	}

	blob, err := fetchOIDCBlob(ctx, client, disc.JWKSURI)
	if err != nil {
		return nil, fmt.Errorf("fetch jwks: %w", err)
	}

	// Minimal sanity: must be valid JSON with a `keys` array. Full
	// parse lives on the facade side so a malformed IdP response
	// still reaches operators via the Secret for diagnosis rather
	// than getting dropped here.
	var probe struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.Unmarshal(blob, &probe); err != nil {
		return nil, fmt.Errorf("jwks is not valid JSON: %w", err)
	}
	if len(probe.Keys) == 0 {
		return nil, fmt.Errorf("jwks has no keys")
	}
	return blob, nil
}

// oidcHTTPClient returns the HTTP client used for issuer round-trips.
// Injected via the reconciler's OIDCHTTPClient field so tests can swap
// in an httptest.Server-backed client; falls back to a reasonable
// default otherwise.
func (r *AgentRuntimeReconciler) oidcHTTPClient() *http.Client {
	if r.OIDCHTTPClient != nil {
		return r.OIDCHTTPClient
	}
	return &http.Client{Timeout: OIDCJWKSHTTPTimeout}
}

// fetchOIDCDiscovery GETs the well-known discovery document.
func fetchOIDCDiscovery(ctx context.Context, client *http.Client, url string) (oidcDiscovery, error) {
	blob, err := fetchOIDCBlob(ctx, client, url)
	if err != nil {
		return oidcDiscovery{}, fmt.Errorf("fetch discovery: %w", err)
	}
	var disc oidcDiscovery
	if err := json.Unmarshal(blob, &disc); err != nil {
		return oidcDiscovery{}, fmt.Errorf("parse discovery: %w", err)
	}
	return disc, nil
}

// fetchOIDCBlob is a tiny GET helper shared by the discovery + JWKS
// paths. Pulled out so both can surface the same error shape.
func fetchOIDCBlob(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("new request %s: %w", url, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	// Reasonable cap — JWKS payloads should be well under 64 KB.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	return body, nil
}

// upsertOIDCJWKSSecret writes the JWKS into the per-agent Secret. Sets
// controller-reference so the Secret is GC'd when the AgentRuntime is
// deleted; if that fails (rare — cross-namespace, etc.) we still
// upsert the content so the facade isn't broken by an ownership edge
// case.
func (r *AgentRuntimeReconciler) upsertOIDCJWKSSecret(
	ctx context.Context,
	ar *omniav1alpha1.AgentRuntime,
	jwks []byte,
) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      oidcJWKSSecretName(ar.Name),
			Namespace: ar.Namespace,
		},
	}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if err := controllerutil.SetControllerReference(ar, secret, r.Scheme); err != nil {
			return err
		}
		if secret.Labels == nil {
			secret.Labels = map[string]string{}
		}
		secret.Labels[labelCredentialKind] = LabelCredentialKindAgentOIDCJWKS
		secret.Labels[labelAppInstance] = ar.Name
		secret.Labels[labelAppManagedBy] = labelValueOmniaOperator
		if secret.Annotations == nil {
			secret.Annotations = map[string]string{}
		}
		// Stamp the fetch time so subsequent reconciles can skip the
		// HTTP round-trip when the Secret is still within the refresh
		// window (T8).
		secret.Annotations[OIDCJWKSFetchedAtAnnotation] = r.jwksNow().UTC().Format(time.RFC3339)
		if secret.Data == nil {
			secret.Data = map[string][]byte{}
		}
		secret.Data[OIDCJWKSDataKey] = jwks
		return nil
	})
	if err != nil {
		return err
	}
	logf.FromContext(ctx).V(1).Info("oidc jwks secret reconciled",
		"operation", op,
		"secret", secret.Name)
	return nil
}

// deleteOIDCJWKSSecretIfPresent removes the per-agent JWKS Secret.
// Called when spec.externalAuth.oidc is turned off; NotFound is
// swallowed so the reconciler stays idempotent.
func (r *AgentRuntimeReconciler) deleteOIDCJWKSSecretIfPresent(
	ctx context.Context,
	ar *omniav1alpha1.AgentRuntime,
) error {
	secret := &corev1.Secret{}
	key := types.NamespacedName{Name: oidcJWKSSecretName(ar.Name), Namespace: ar.Namespace}
	err := r.Get(ctx, key, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if err := r.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// oidcJWKSSecretName derives the per-agent Secret name. Kept package-
// local so callers don't cross-import cmd/agent.
func oidcJWKSSecretName(agentName string) string {
	return "agent-" + agentName + OIDCJWKSSecretSuffix
}

// setOIDCJWKSCondition updates the OIDCJWKSReady status condition in-
// memory on the AgentRuntime. The caller's status update pushes it to
// the API server.
func (r *AgentRuntimeReconciler) setOIDCJWKSCondition(
	ar *omniav1alpha1.AgentRuntime,
	status metav1.ConditionStatus,
	reason, message string,
) {
	SetCondition(&ar.Status.Conditions, ar.Generation,
		ConditionTypeOIDCJWKSReady, status, reason, message)
}

// scheduleOIDCJWKSRefresh returns a ctrl.Result with RequeueAfter set
// to the next scheduled JWKS refresh, or a zero Result when OIDC is
// not configured.
func scheduleOIDCJWKSRefresh(next time.Duration) ctrl.Result {
	if next == 0 {
		return ctrl.Result{}
	}
	return ctrl.Result{RequeueAfter: next}
}

// jwksNow returns the current time via the reconciler's injectable
// clock when one is set (tests inject a deterministic clock), or
// time.Now otherwise.
func (r *AgentRuntimeReconciler) jwksNow() time.Time {
	if r.JWKSClock != nil {
		return r.JWKSClock()
	}
	return time.Now()
}

// jwksSecretStillFresh reads the current mirror Secret and decides
// whether an HTTP fetch can be skipped. Returns (remaining, true) when
// the existing Secret is still within the refresh window so the caller
// can requeue at the exact refresh time; (0, false) means fetch now.
//
// Get errors (NotFound, API pressure) fall through to fetch rather
// than risking a silently-stale cache.
func (r *AgentRuntimeReconciler) jwksSecretStillFresh(
	ctx context.Context,
	ar *omniav1alpha1.AgentRuntime,
) (time.Duration, bool) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      oidcJWKSSecretName(ar.Name),
		Namespace: ar.Namespace,
	}, secret)
	if err != nil {
		return 0, false
	}
	raw, ok := secret.Annotations[OIDCJWKSFetchedAtAnnotation]
	if !ok {
		return 0, false
	}
	fetchedAt, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return 0, false
	}
	// Data key must also be present — an annotation without content
	// suggests a hand-edited Secret.
	if _, ok := secret.Data[OIDCJWKSDataKey]; !ok {
		return 0, false
	}
	elapsed := r.jwksNow().Sub(fetchedAt)
	if elapsed >= OIDCJWKSRefreshInterval {
		return 0, false
	}
	return OIDCJWKSRefreshInterval - elapsed, true
}
