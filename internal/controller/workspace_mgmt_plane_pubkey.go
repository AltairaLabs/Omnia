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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// tlsCertSecretKey is the data key inside a kubernetes.io/tls Secret that
// holds the PEM-encoded X.509 certificate. Matches the Helm chart's
// signing-keypair template and k8s TLS secret conventions.
const tlsCertSecretKey = "tls.crt"

// reconcileMgmtPlanePubkey mirrors the public half of the dashboard's
// mgmt-plane signing keypair into a ConfigMap in the workspace namespace.
//
// Why: facade pods live in workspace namespaces and cannot cross-mount
// secrets from the operator namespace. The chart keeps the keypair in
// the operator namespace (a Secret) so the dashboard can sign with the
// private half; the reconciler copies just the public cert into a
// namespace-local ConfigMap that every facade pod in this workspace
// mounts read-only.
//
// Semantics:
//   - r.OperatorNamespace == "" OR r.MgmtPlaneSigningSecret == ""  → skip
//     (leaves any existing mirror in place; cleanup lives with ns delete).
//   - source Secret not found  → delete the mirror if present (chart was
//     uninstalled or dashboard.enabled flipped to false). Non-fatal.
//   - source Secret found but missing tls.crt  → return an error so the
//     Workspace status surfaces the misconfiguration.
//   - source present and valid  → upsert the ConfigMap so data["pubkey.pem"]
//     matches the source's tls.crt.
func (r *WorkspaceReconciler) reconcileMgmtPlanePubkey(ctx context.Context, workspace *omniav1alpha1.Workspace) error {
	log := logf.FromContext(ctx)

	if r.OperatorNamespace == "" || r.MgmtPlaneSigningSecret == "" {
		log.V(1).Info("mgmt-plane pubkey mirror skipped",
			"reason", "not configured",
			"operatorNamespace", r.OperatorNamespace,
			"signingSecret", r.MgmtPlaneSigningSecret)
		return nil
	}

	namespaceName := workspace.Spec.Namespace.Name

	source := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Namespace: r.OperatorNamespace, Name: r.MgmtPlaneSigningSecret}, source)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Source missing — drop any stale mirror. Idempotent.
			return r.deleteMgmtPlanePubkeyMirror(ctx, namespaceName)
		}
		return fmt.Errorf("get signing secret %s/%s: %w", r.OperatorNamespace, r.MgmtPlaneSigningSecret, err)
	}

	certBytes, ok := source.Data[tlsCertSecretKey]
	if !ok || len(certBytes) == 0 {
		return fmt.Errorf("signing secret %s/%s missing %q data key",
			r.OperatorNamespace, r.MgmtPlaneSigningSecret, tlsCertSecretKey)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MgmtPlanePubkeyConfigMapName,
			Namespace: namespaceName,
		},
	}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		if cm.Labels == nil {
			cm.Labels = map[string]string{}
		}
		cm.Labels[labelWorkspace] = workspace.Name
		cm.Labels[labelWorkspaceManaged] = labelValueTrue
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		cm.Data[MgmtPlanePubkeyDataKey] = string(certBytes)
		return nil
	})
	if err != nil {
		return fmt.Errorf("upsert mgmt-plane pubkey configmap %s/%s: %w",
			namespaceName, MgmtPlanePubkeyConfigMapName, err)
	}
	if op != controllerutil.OperationResultNone {
		log.Info("mgmt-plane pubkey mirror reconciled",
			"namespace", namespaceName,
			"configMap", MgmtPlanePubkeyConfigMapName,
			"operation", op)
	}
	return nil
}

// deleteMgmtPlanePubkeyMirror removes the mirror ConfigMap. Used when the
// source Secret no longer exists (dashboard turned off, chart uninstalled).
// NotFound errors are swallowed — cleanup is idempotent.
func (r *WorkspaceReconciler) deleteMgmtPlanePubkeyMirror(ctx context.Context, namespace string) error {
	log := logf.FromContext(ctx)

	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: MgmtPlanePubkeyConfigMapName}, cm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("get mgmt-plane pubkey configmap %s/%s for cleanup: %w",
			namespace, MgmtPlanePubkeyConfigMapName, err)
	}
	if err := r.Delete(ctx, cm, &client.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete mgmt-plane pubkey configmap %s/%s: %w",
			namespace, MgmtPlanePubkeyConfigMapName, err)
	}
	log.Info("mgmt-plane pubkey mirror removed",
		"reason", "source signing secret absent",
		"namespace", namespace)
	return nil
}
