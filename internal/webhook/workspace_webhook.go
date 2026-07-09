/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package webhook

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// WorkspaceValidator rejects a Workspace whose spec.namespace.name is already
// claimed by a different Workspace. A namespace must have exactly one owning
// Workspace; two owners produce duplicate operator-managed session-api/memory-api
// deployments, wrong service-group resolution, owner-label thrash, and a
// dangerous cleanup path (#1821). This is the fail-fast admission guard; the
// controller's reconcileNamespace ownership check is the runtime backstop for
// the create-race window and for when the webhook is disabled.
type WorkspaceValidator struct {
	Client client.Client
}

var workspaceLog = logf.Log.WithName("workspace-webhook")

// +kubebuilder:webhook:path=/validate-omnia-altairalabs-ai-v1alpha1-workspace,mutating=false,failurePolicy=ignore,sideEffects=None,groups=omnia.altairalabs.ai,resources=workspaces,verbs=create;update,versions=v1alpha1,name=vworkspace.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*corev1alpha1.Workspace] = &WorkspaceValidator{}

// SetupWorkspaceWebhookWithManager registers the webhook with the manager.
func SetupWorkspaceWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &corev1alpha1.Workspace{}).
		WithValidator(&WorkspaceValidator{Client: mgr.GetClient()}).
		Complete()
}

// ValidateCreate rejects a Workspace colliding with an existing namespace owner.
func (v *WorkspaceValidator) ValidateCreate(ctx context.Context, ws *corev1alpha1.Workspace) (admission.Warnings, error) {
	workspaceLog.Info("validating create", "name", ws.Name, "namespace", ws.Spec.Namespace.Name)
	return nil, v.checkNamespaceOwnership(ctx, ws)
}

// ValidateUpdate re-checks ownership, since spec.namespace.name may change.
func (v *WorkspaceValidator) ValidateUpdate(ctx context.Context, _, ws *corev1alpha1.Workspace) (admission.Warnings, error) {
	workspaceLog.Info("validating update", "name", ws.Name, "namespace", ws.Spec.Namespace.Name)
	return nil, v.checkNamespaceOwnership(ctx, ws)
}

// ValidateDelete permits all deletions.
func (v *WorkspaceValidator) ValidateDelete(_ context.Context, _ *corev1alpha1.Workspace) (admission.Warnings, error) {
	return nil, nil
}

// checkNamespaceOwnership rejects ws when another Workspace already declares the
// same spec.namespace.name. Re-applying the same Workspace (matched by name)
// never collides. Transient list errors stay advisory (return nil) so an
// apiserver hiccup can't block all Workspace writes — the controller guard is
// the backstop.
func (v *WorkspaceValidator) checkNamespaceOwnership(ctx context.Context, ws *corev1alpha1.Workspace) error {
	if v.Client == nil || ws.Spec.Namespace.Name == "" {
		return nil
	}
	var list corev1alpha1.WorkspaceList
	if err := v.Client.List(ctx, &list); err != nil {
		workspaceLog.Error(err, "listing workspaces for ownership check", "name", ws.Name)
		return nil
	}
	for i := range list.Items {
		other := &list.Items[i]
		if other.Name == ws.Name {
			continue
		}
		if other.Spec.Namespace.Name == ws.Spec.Namespace.Name {
			return fmt.Errorf(
				"namespace %q is already claimed by workspace %q; a namespace can be owned by only one workspace — "+
					"add a service group to your existing workspace instead of creating a second one",
				ws.Spec.Namespace.Name, other.Name)
		}
	}
	return nil
}
