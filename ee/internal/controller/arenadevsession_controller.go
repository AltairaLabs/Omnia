/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/arena/providers"
)

const (
	// ArenaDevSessionFinalizerName is the finalizer for ArenaDevSession resources.
	ArenaDevSessionFinalizerName = "arenadevsession.arena.omnia.altairalabs.ai/finalizer"

	// Default idle timeout for dev sessions.
	defaultIdleTimeout = 30 * time.Minute

	// Default image for the dev console.
	defaultDevConsoleImage = "ghcr.io/altairalabs/omnia-arena-dev-console:latest"

	// Labels for dev session resources.
	labelDevSession     = "arena.omnia.altairalabs.ai/dev-session"
	labelManagedBy      = "app.kubernetes.io/managed-by"
	labelManagedByValue = "arena-controller"
)

// ArenaDevSessionReconciler reconciles ArenaDevSession objects.
type ArenaDevSessionReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// DevConsoleImage is the default image for dev console pods.
	DevConsoleImage string
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenadevsessions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenadevsessions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenadevsessions/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services;serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=get;list;watch;create;update;patch;delete

// Reconcile handles ArenaDevSession reconciliation.
func (r *ArenaDevSessionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the ArenaDevSession
	session := &omniav1alpha1.ArenaDevSession{}
	if err := r.Get(ctx, req.NamespacedName, session); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !session.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, session)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(session, ArenaDevSessionFinalizerName) {
		controllerutil.AddFinalizer(session, ArenaDevSessionFinalizerName)
		if err := r.Update(ctx, session); err != nil {
			if apierrors.IsConflict(err) {
				// Conflict - requeue to retry with fresh object
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	// Initialize status if needed
	if session.Status.Phase == "" {
		session.Status.Phase = omniav1alpha1.ArenaDevSessionPhasePending
		if err := r.Status().Update(ctx, session); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}

	// Check for idle timeout
	if r.shouldCleanupIdle(session) {
		log.Info("cleaning up idle dev session", "session", session.Name)
		return r.reconcileCleanup(ctx, session)
	}

	// Reconcile resources based on phase
	switch session.Status.Phase {
	case omniav1alpha1.ArenaDevSessionPhasePending:
		return r.reconcileStart(ctx, session)
	case omniav1alpha1.ArenaDevSessionPhaseStarting:
		return r.reconcileWaitReady(ctx, session)
	case omniav1alpha1.ArenaDevSessionPhaseReady:
		// Check periodically for idle timeout
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	case omniav1alpha1.ArenaDevSessionPhaseStopping:
		return r.reconcileCleanup(ctx, session)
	}

	return ctrl.Result{}, nil
}

// reconcileStart creates the dev console resources.
func (r *ArenaDevSessionReconciler) reconcileStart(ctx context.Context, session *omniav1alpha1.ArenaDevSession) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("starting dev session", "session", session.Name)

	// Update phase to Starting
	session.Status.Phase = omniav1alpha1.ArenaDevSessionPhaseStarting
	session.Status.Message = "Creating dev console resources"
	if err := r.Status().Update(ctx, session); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	// Create ServiceAccount
	if err := r.reconcileServiceAccount(ctx, session); err != nil {
		return r.setFailed(ctx, session, "Failed to create ServiceAccount", err)
	}

	// Create Role
	if err := r.reconcileRole(ctx, session); err != nil {
		return r.setFailed(ctx, session, "Failed to create Role", err)
	}

	// Create RoleBinding
	if err := r.reconcileRoleBinding(ctx, session); err != nil {
		return r.setFailed(ctx, session, "Failed to create RoleBinding", err)
	}

	// Create Deployment
	if err := r.reconcileDeployment(ctx, session); err != nil {
		return r.setFailed(ctx, session, "Failed to create Deployment", err)
	}

	// Create Service
	if err := r.reconcileService(ctx, session); err != nil {
		return r.setFailed(ctx, session, "Failed to create Service", err)
	}

	return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
}

// reconcileWaitReady waits for the deployment to be ready.
func (r *ArenaDevSessionReconciler) reconcileWaitReady(ctx context.Context, session *omniav1alpha1.ArenaDevSession) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Check deployment status
	deployment := &appsv1.Deployment{}
	deploymentName := r.resourceName(session)
	if err := r.Get(ctx, client.ObjectKey{Namespace: session.Namespace, Name: deploymentName}, deployment); err != nil {
		if apierrors.IsNotFound(err) {
			// Deployment not created yet, requeue
			return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}

	// Check if deployment is ready
	if deployment.Status.ReadyReplicas > 0 {
		log.Info("dev session ready", "session", session.Name)
		now := metav1.Now()
		session.Status.Phase = omniav1alpha1.ArenaDevSessionPhaseReady
		session.Status.StartedAt = &now
		session.Status.LastActivityAt = &now
		session.Status.ServiceName = deploymentName
		session.Status.Endpoint = fmt.Sprintf("ws://%s.%s.svc:8080/ws", deploymentName, session.Namespace)
		session.Status.Message = "Dev console is ready"
		meta.SetStatusCondition(&session.Status.Conditions, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionTrue,
			Reason:  "DeploymentReady",
			Message: "Dev console deployment is ready",
		})
		if err := r.Status().Update(ctx, session); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Still waiting
	session.Status.Message = "Waiting for dev console to start"
	if err := r.Status().Update(ctx, session); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
}

// reconcileCleanup deletes the dev console resources.
func (r *ArenaDevSessionReconciler) reconcileCleanup(ctx context.Context, session *omniav1alpha1.ArenaDevSession) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("cleaning up dev session", "session", session.Name)

	// Update phase
	if session.Status.Phase != omniav1alpha1.ArenaDevSessionPhaseStopping {
		session.Status.Phase = omniav1alpha1.ArenaDevSessionPhaseStopping
		session.Status.Message = "Cleaning up resources"
		if err := r.Status().Update(ctx, session); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}

	resourceName := r.resourceName(session)

	// Delete Deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: session.Namespace},
	}
	if err := r.Delete(ctx, deployment); err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "failed to delete deployment")
	}

	// Delete Service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: session.Namespace},
	}
	if err := r.Delete(ctx, service); err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "failed to delete service")
	}

	// Delete RoleBinding
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: session.Namespace},
	}
	if err := r.Delete(ctx, roleBinding); err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "failed to delete rolebinding")
	}

	// Delete Role
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: session.Namespace},
	}
	if err := r.Delete(ctx, role); err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "failed to delete role")
	}

	// Delete ServiceAccount
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: session.Namespace},
	}
	if err := r.Delete(ctx, sa); err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "failed to delete serviceaccount")
	}

	// Update status to Stopped
	session.Status.Phase = omniav1alpha1.ArenaDevSessionPhaseStopped
	session.Status.Message = "Session stopped"
	session.Status.Endpoint = ""
	if err := r.Status().Update(ctx, session); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileDelete handles finalizer cleanup.
func (r *ArenaDevSessionReconciler) reconcileDelete(ctx context.Context, session *omniav1alpha1.ArenaDevSession) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("handling deletion", "session", session.Name)

	// Clean up resources first
	if session.Status.Phase != omniav1alpha1.ArenaDevSessionPhaseStopped {
		result, err := r.reconcileCleanup(ctx, session)
		if err != nil {
			return result, err
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(session, ArenaDevSessionFinalizerName)
	if err := r.Update(ctx, session); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// shouldCleanupIdle checks if the session should be cleaned up due to idle timeout.
func (r *ArenaDevSessionReconciler) shouldCleanupIdle(session *omniav1alpha1.ArenaDevSession) bool {
	if session.Status.Phase != omniav1alpha1.ArenaDevSessionPhaseReady {
		return false
	}
	if session.Status.LastActivityAt == nil {
		return false
	}

	timeout := defaultIdleTimeout
	if session.Spec.IdleTimeout != "" {
		if parsed, err := time.ParseDuration(session.Spec.IdleTimeout); err == nil {
			timeout = parsed
		}
	}

	return time.Since(session.Status.LastActivityAt.Time) > timeout
}

// setFailed updates the session to failed state.
func (r *ArenaDevSessionReconciler) setFailed(ctx context.Context, session *omniav1alpha1.ArenaDevSession, message string, err error) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Error(err, message)

	session.Status.Phase = omniav1alpha1.ArenaDevSessionPhaseFailed
	session.Status.Message = fmt.Sprintf("%s: %v", message, err)
	meta.SetStatusCondition(&session.Status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  "Failed",
		Message: message,
	})
	if statusErr := r.Status().Update(ctx, session); statusErr != nil {
		log.Error(statusErr, "failed to update status")
	}

	return ctrl.Result{}, err
}

// resourceName returns the name for child resources.
// Uses a shortened prefix and hash to ensure the name stays under the
// 63-character Kubernetes DNS label limit.
func (r *ArenaDevSessionReconciler) resourceName(session *omniav1alpha1.ArenaDevSession) string {
	// Prefix "adc-" = 4 chars, leaving 59 chars for name + hash
	prefix := "adc-"
	maxLen := 63

	// If name already fits, use it directly with prefix
	fullName := prefix + session.Name
	if len(fullName) <= maxLen {
		return fullName
	}

	// Otherwise, truncate and add a hash suffix for uniqueness
	// Hash suffix: 8 chars, separator: 1 char = 9 chars reserved
	hash := sha256.Sum256([]byte(session.Name))
	hashSuffix := hex.EncodeToString(hash[:])[:8]

	// Available for name portion: 63 - 4 (prefix) - 1 (dash) - 8 (hash) = 50 chars
	maxNameLen := maxLen - len(prefix) - 1 - 8
	truncatedName := session.Name
	if len(truncatedName) > maxNameLen {
		truncatedName = truncatedName[:maxNameLen]
	}

	return fmt.Sprintf("%s%s-%s", prefix, truncatedName, hashSuffix)
}

// reconcileServiceAccount creates or updates the ServiceAccount.
func (r *ArenaDevSessionReconciler) reconcileServiceAccount(ctx context.Context, session *omniav1alpha1.ArenaDevSession) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.resourceName(session),
			Namespace: session.Namespace,
			Labels:    r.commonLabels(session),
		},
	}

	if err := controllerutil.SetControllerReference(session, sa, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.ServiceAccount{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(sa), existing); err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, sa)
		}
		return err
	}
	return nil
}

// reconcileRole creates or updates the Role.
func (r *ArenaDevSessionReconciler) reconcileRole(ctx context.Context, session *omniav1alpha1.ArenaDevSession) error {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.resourceName(session),
			Namespace: session.Namespace,
			Labels:    r.commonLabels(session),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"omnia.altairalabs.ai"},
				Resources: []string{"providers"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"omnia.altairalabs.ai"},
				Resources: []string{"toolregistries"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"get"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"get", "list"},
			},
		},
	}

	if err := controllerutil.SetControllerReference(session, role, r.Scheme); err != nil {
		return err
	}

	existing := &rbacv1.Role{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(role), existing); err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, role)
		}
		return err
	}
	return nil
}

// reconcileRoleBinding creates or updates the RoleBinding.
func (r *ArenaDevSessionReconciler) reconcileRoleBinding(ctx context.Context, session *omniav1alpha1.ArenaDevSession) error {
	resourceName := r.resourceName(session)
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: session.Namespace,
			Labels:    r.commonLabels(session),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     resourceName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      resourceName,
				Namespace: session.Namespace,
			},
		},
	}

	if err := controllerutil.SetControllerReference(session, rb, r.Scheme); err != nil {
		return err
	}

	existing := &rbacv1.RoleBinding{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(rb), existing); err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, rb)
		}
		return err
	}
	return nil
}

// reconcileDeployment creates or updates the Deployment.
func (r *ArenaDevSessionReconciler) reconcileDeployment(ctx context.Context, session *omniav1alpha1.ArenaDevSession) error {
	log := logf.FromContext(ctx)
	resourceName := r.resourceName(session)
	image := r.DevConsoleImage
	if session.Spec.Image != "" {
		image = session.Spec.Image
	}
	if image == "" {
		image = defaultDevConsoleImage
	}

	// List all providers in the namespace to mount their credentials as env vars
	providerEnvVars, err := r.buildProviderEnvVars(ctx, session.Namespace)
	if err != nil {
		log.Error(err, "failed to build provider env vars, continuing without provider credentials")
		providerEnvVars = []corev1.EnvVar{}
	}

	// Build the complete env var list
	envVars := []corev1.EnvVar{
		{
			Name: "POD_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
	}
	envVars = append(envVars, providerEnvVars...)

	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: session.Namespace,
			Labels:    r.commonLabels(session),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: r.selectorLabels(session),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: r.commonLabels(session),
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: resourceName,
					Containers: []corev1.Container{
						{
							Name:            "arena-dev-console",
							Image:           image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Args: []string{
								"--http-port=8080",
								"--health-port=8081",
								"--session-ttl=30m",
								"--dev-mode",
							},
							Ports: []corev1.ContainerPort{
								{Name: "http", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
								{Name: "health", ContainerPort: 8081, Protocol: corev1.ProtocolTCP},
							},
							Env: envVars,
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromString("health"),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/readyz",
										Port: intstr.FromString("health"),
									},
								},
								InitialDelaySeconds: 2,
								PeriodSeconds:       5,
							},
							Resources: r.getResources(session),
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr[bool](false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								RunAsNonRoot: ptr[bool](true),
								SeccompProfile: &corev1.SeccompProfile{
									Type: corev1.SeccompProfileTypeRuntimeDefault,
								},
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(session, deployment, r.Scheme); err != nil {
		return err
	}

	existing := &appsv1.Deployment{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(deployment), existing); err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, deployment)
		}
		return err
	}
	return nil
}

// reconcileService creates or updates the Service.
func (r *ArenaDevSessionReconciler) reconcileService(ctx context.Context, session *omniav1alpha1.ArenaDevSession) error {
	resourceName := r.resourceName(session)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: session.Namespace,
			Labels:    r.commonLabels(session),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: r.selectorLabels(session),
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       8080,
					TargetPort: intstr.FromString("http"),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(session, svc, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.Service{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(svc), existing); err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, svc)
		}
		return err
	}
	return nil
}

// commonLabels returns labels for all resources.
func (r *ArenaDevSessionReconciler) commonLabels(session *omniav1alpha1.ArenaDevSession) map[string]string {
	return map[string]string{
		labelDevSession: session.Name,
		labelManagedBy:  labelManagedByValue,
	}
}

// selectorLabels returns labels for pod selection.
func (r *ArenaDevSessionReconciler) selectorLabels(session *omniav1alpha1.ArenaDevSession) map[string]string {
	return map[string]string{
		labelDevSession: session.Name,
	}
}

// getResources returns resource requirements.
func (r *ArenaDevSessionReconciler) getResources(session *omniav1alpha1.ArenaDevSession) corev1.ResourceRequirements {
	resources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	}

	if session.Spec.Resources != nil {
		if session.Spec.Resources.Requests != nil {
			for k, v := range session.Spec.Resources.Requests {
				if q, err := resource.ParseQuantity(v); err == nil {
					resources.Requests[corev1.ResourceName(k)] = q
				}
			}
		}
		if session.Spec.Resources.Limits != nil {
			for k, v := range session.Spec.Resources.Limits {
				if q, err := resource.ParseQuantity(v); err == nil {
					resources.Limits[corev1.ResourceName(k)] = q
				}
			}
		}
	}

	return resources
}

// buildProviderEnvVars lists all Provider CRDs in the namespace and builds
// environment variables for their credentials using the shared providers package.
func (r *ArenaDevSessionReconciler) buildProviderEnvVars(ctx context.Context, namespace string) ([]corev1.EnvVar, error) {
	// List all Provider CRDs in the namespace
	providerList := &corev1alpha1.ProviderList{}
	if err := r.List(ctx, providerList, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list providers: %w", err)
	}

	// Convert to pointer slice for the shared function
	providerPtrs := make([]*corev1alpha1.Provider, len(providerList.Items))
	for i := range providerList.Items {
		providerPtrs[i] = &providerList.Items[i]
	}

	return providers.BuildEnvVarsFromProviders(providerPtrs), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ArenaDevSessionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniav1alpha1.ArenaDevSession{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Complete(r)
}
