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

package controller

import (
	"context"
	"errors"
	"fmt"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// errNamespaceConflict is returned by reconcileNamespace when the target
// namespace already exists and carries a different Workspace's owner label.
// Reconcile maps it to a NamespaceConflict condition (not the generic
// NamespaceFailed) so the collision is unambiguous in status. A namespace must
// have exactly one owning Workspace (#1821).
var errNamespaceConflict = errors.New("namespace already owned by another workspace")

//nolint:gocognit,gocyclo // Namespace reconcile handles create + update-existing label/tag merge paths
func (r *WorkspaceReconciler) reconcileNamespace(ctx context.Context, workspace *omniav1alpha1.Workspace) error {
	log := logf.FromContext(ctx)
	namespaceName := workspace.Spec.Namespace.Name

	ns := &corev1.Namespace{}
	err := r.Get(ctx, client.ObjectKey{Name: namespaceName}, ns)

	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get namespace %s: %w", namespaceName, err)
	}

	namespaceExists := err == nil

	if !namespaceExists {
		if !workspace.Spec.Namespace.Create {
			return fmt.Errorf("namespace %s does not exist and spec.namespace.create is false", namespaceName)
		}

		// Create the namespace
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName,
				Labels: map[string]string{
					labelWorkspace:        workspace.Name,
					labelWorkspaceManaged: labelValueTrue,
					labelEnvironment:      string(workspace.Spec.Environment),
				},
			},
		}

		// Add spec labels
		for k, v := range workspace.Spec.Namespace.Labels {
			ns.Labels[k] = v
		}

		// Add default tags as labels
		for k, v := range workspace.Spec.DefaultTags {
			ns.Labels[k] = v
		}

		// Add spec annotations
		ns.Annotations = workspace.Spec.Namespace.Annotations

		if err := r.Create(ctx, ns); err != nil {
			return fmt.Errorf("failed to create namespace %s: %w", namespaceName, err)
		}

		log.Info("Created namespace", "name", namespaceName)

		// Update status to indicate we created the namespace
		workspace.Status.Namespace = &omniav1alpha1.NamespaceStatus{
			Name:    namespaceName,
			Created: true,
		}
	} else {
		// Refuse to adopt a namespace already owned by a *different* Workspace.
		// Reading a nil map returns "", so this is safe before the nil-init below.
		// Only an unowned namespace (no owner label — e.g. a pre-existing
		// namespace.create=false install) or one already owned by self may be
		// adopted; anything else is a collision we must not silently claim (#1821).
		if owner := ns.Labels[labelWorkspace]; owner != "" && owner != workspace.Name {
			return fmt.Errorf("%w: namespace %s is already owned by workspace %s",
				errNamespaceConflict, namespaceName, owner)
		}

		// Namespace exists, update labels if needed
		updated := false
		if ns.Labels == nil {
			ns.Labels = make(map[string]string)
		}

		// Ensure workspace labels are present
		if ns.Labels[labelWorkspace] != workspace.Name {
			ns.Labels[labelWorkspace] = workspace.Name
			updated = true
		}
		if ns.Labels[labelEnvironment] != string(workspace.Spec.Environment) {
			ns.Labels[labelEnvironment] = string(workspace.Spec.Environment)
			updated = true
		}

		// Add spec namespace labels. These must reconcile onto an existing
		// namespace too (not just at creation) — e.g. namespace.create=false
		// installs, or enabling istio.io/dataplane-mode=ambient after the fact.
		// Mirrors the create branch so labels and DefaultTags behave the same.
		for k, v := range workspace.Spec.Namespace.Labels {
			if ns.Labels[k] != v {
				ns.Labels[k] = v
				updated = true
			}
		}

		// Add default tags
		for k, v := range workspace.Spec.DefaultTags {
			if ns.Labels[k] != v {
				ns.Labels[k] = v
				updated = true
			}
		}

		if updated {
			if err := r.Update(ctx, ns); err != nil {
				return fmt.Errorf("failed to update namespace %s: %w", namespaceName, err)
			}
			log.Info("Updated namespace labels", "name", namespaceName)
		}

		workspace.Status.Namespace = &omniav1alpha1.NamespaceStatus{
			Name:    namespaceName,
			Created: false,
		}
	}

	return nil
}

// namespaceConditionReason maps a reconcileNamespace error to the condition
// reason surfaced on the Workspace: NamespaceConflict for an ownership
// collision (#1821), NamespaceFailed for anything else.
func namespaceConditionReason(err error) string {
	if errors.Is(err, errNamespaceConflict) {
		return "NamespaceConflict"
	}
	return "NamespaceFailed"
}

func (r *WorkspaceReconciler) reconcileServiceAccounts(ctx context.Context, workspace *omniav1alpha1.Workspace) error {
	log := logf.FromContext(ctx)
	namespaceName := workspace.Spec.Namespace.Name

	// Create ServiceAccounts for each role
	roles := []struct {
		role   omniav1alpha1.WorkspaceRole
		suffix string
	}{
		{omniav1alpha1.WorkspaceRoleOwner, "owner"},
		{omniav1alpha1.WorkspaceRoleEditor, "editor"},
		{omniav1alpha1.WorkspaceRoleViewer, "viewer"},
	}

	saStatus := &omniav1alpha1.ServiceAccountStatus{}

	for _, roleInfo := range roles {
		saName := fmt.Sprintf("workspace-%s-%s-sa", workspace.Name, roleInfo.suffix)

		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      saName,
				Namespace: namespaceName,
			},
		}

		result, err := controllerutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
			sa.Labels = map[string]string{
				labelWorkspace:        workspace.Name,
				labelWorkspaceManaged: labelValueTrue,
				labelWorkspaceRole:    string(roleInfo.role),
			}
			return nil
		})

		if err != nil {
			return fmt.Errorf("failed to create/update ServiceAccount %s: %w", saName, err)
		}

		if result != controllerutil.OperationResultNone {
			log.Info("ServiceAccount reconciled", "name", saName, "result", result)
		}

		// Update status
		switch roleInfo.role {
		case omniav1alpha1.WorkspaceRoleOwner:
			saStatus.Owner = saName
		case omniav1alpha1.WorkspaceRoleEditor:
			saStatus.Editor = saName
		case omniav1alpha1.WorkspaceRoleViewer:
			saStatus.Viewer = saName
		}
	}

	workspace.Status.ServiceAccounts = saStatus
	return nil
}

//nolint:gocognit // Binds built-in role SAs plus external spec RoleBindings in one pass
func (r *WorkspaceReconciler) reconcileRoleBindings(ctx context.Context, workspace *omniav1alpha1.Workspace) error {
	log := logf.FromContext(ctx)
	namespaceName := workspace.Spec.Namespace.Name

	// The per-workspace reader that agent pods and eval-workers bind, so they
	// can get their own Workspace without listing every one (#1875).
	if err := r.reconcileWorkspaceReaderClusterRole(ctx, workspace); err != nil {
		return err
	}

	// Create RoleBindings for workspace ServiceAccounts
	roles := []struct {
		role        omniav1alpha1.WorkspaceRole
		suffix      string
		clusterRole string
	}{
		{omniav1alpha1.WorkspaceRoleOwner, "owner", clusterRoleOwner},
		{omniav1alpha1.WorkspaceRoleEditor, "editor", clusterRoleEditor},
		{omniav1alpha1.WorkspaceRoleViewer, "viewer", clusterRoleViewer},
	}

	for _, role := range roles {
		saName := fmt.Sprintf("workspace-%s-%s-sa", workspace.Name, role.suffix)
		rbName := fmt.Sprintf("workspace-%s-%s", workspace.Name, role.suffix)

		rb := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rbName,
				Namespace: namespaceName,
			},
		}

		result, err := controllerutil.CreateOrUpdate(ctx, r.Client, rb, func() error {
			rb.Labels = map[string]string{
				labelWorkspace:        workspace.Name,
				labelWorkspaceManaged: labelValueTrue,
				labelWorkspaceRole:    string(role.role),
			}
			rb.RoleRef = rbacv1.RoleRef{
				APIGroup: rbacAPIGroup,
				Kind:     kindClusterRole,
				Name:     role.clusterRole,
			}
			rb.Subjects = []rbacv1.Subject{
				{
					Kind:      kindServiceAccount,
					Name:      saName,
					Namespace: namespaceName,
				},
			}
			return nil
		})

		if err != nil {
			return fmt.Errorf("failed to create/update RoleBinding %s: %w", rbName, err)
		}

		if result != controllerutil.OperationResultNone {
			log.Info("RoleBinding reconciled", "name", rbName, "result", result)
		}
	}

	// Reconcile RoleBindings for external ServiceAccounts from spec
	for _, binding := range workspace.Spec.RoleBindings {
		if len(binding.ServiceAccounts) == 0 {
			continue // Skip group-only bindings (handled at app layer)
		}

		clusterRole := r.getClusterRoleForRole(binding.Role)

		for _, sa := range binding.ServiceAccounts {
			rbName := fmt.Sprintf("%s-sa-%s-%s", workspace.Name, sanitizeName(sa.Name), sanitizeName(sa.Namespace))

			rb := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rbName,
					Namespace: namespaceName,
				},
			}

			result, err := controllerutil.CreateOrUpdate(ctx, r.Client, rb, func() error {
				rb.Labels = map[string]string{
					labelWorkspace:        workspace.Name,
					labelWorkspaceManaged: labelValueTrue,
					labelWorkspaceRole:    string(binding.Role),
				}
				rb.RoleRef = rbacv1.RoleRef{
					APIGroup: rbacAPIGroup,
					Kind:     kindClusterRole,
					Name:     clusterRole,
				}
				rb.Subjects = []rbacv1.Subject{
					{
						Kind:      kindServiceAccount,
						Name:      sa.Name,
						Namespace: sa.Namespace,
					},
				}
				return nil
			})

			if err != nil {
				return fmt.Errorf("failed to create/update RoleBinding %s: %w", rbName, err)
			}

			if result != controllerutil.OperationResultNone {
				log.Info("External ServiceAccount RoleBinding reconciled", "name", rbName, "result", result)
			}
		}
	}

	return nil
}
