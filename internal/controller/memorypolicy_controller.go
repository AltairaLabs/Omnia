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
	"regexp"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// MemoryPolicy condition types. Mirror the SessionRetentionPolicy
// set so operators see the same shape across both policy CRDs.
const (
	MemRetentionConditionTypePolicyValid        = "PolicyValid"
	MemRetentionConditionTypeWorkspacesResolved = "WorkspacesResolved"
	MemRetentionConditionTypeReady              = "Ready"
)

// MemoryPolicy event reasons.
const (
	MemRetentionEventReasonValidated          = "PolicyValidated"
	MemRetentionEventReasonValidationFailed   = "PolicyValidationFailed"
	MemRetentionEventReasonWorkspacesResolved = "WorkspacesResolved"
	MemRetentionEventReasonWorkspacesMissing  = "WorkspacesMissing"
	MemRetentionEventReasonActive             = "PolicyActive"
)

// MemoryPolicyReconciler reconciles a MemoryPolicy object.
//
// Phase 1 only validates the spec and reports status — the retention worker
// rewrite that actually applies the policy lands in Phase 3.
type MemoryPolicyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=memorypolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=memorypolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=memorypolicies/finalizers,verbs=update

// Reconcile validates the spec, checks that per-workspace overrides
// reference actual Workspaces, and sets status conditions.
func (r *MemoryPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling MemoryPolicy", "name", req.Name)

	policy := &omniav1alpha1.MemoryPolicy{}
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !policy.DeletionTimestamp.IsZero() {
		// No finalizer-owned state to clean up in Phase 1 — just let the
		// delete go through.
		return ctrl.Result{}, nil
	}

	if err := r.validatePolicy(policy); err != nil {
		return r.markError(ctx, policy,
			MemRetentionConditionTypePolicyValid, "ValidationFailed", err.Error(),
			MemRetentionEventReasonValidationFailed)
	}
	SetCondition(&policy.Status.Conditions, policy.Generation, MemRetentionConditionTypePolicyValid,
		metav1.ConditionTrue, "Valid", "Policy spec is valid")
	r.emitEvent(policy, corev1.EventTypeNormal, MemRetentionEventReasonValidated,
		"Policy spec validated successfully")

	resolvedCount, err := r.resolveWorkspaces(ctx, policy)
	if err != nil {
		SetCondition(&policy.Status.Conditions, policy.Generation, MemRetentionConditionTypeWorkspacesResolved,
			metav1.ConditionFalse, "ResolutionFailed", err.Error())
		SetCondition(&policy.Status.Conditions, policy.Generation, MemRetentionConditionTypeReady,
			metav1.ConditionFalse, "WorkspaceResolutionFailed", "Workspace resolution failed")
		r.emitEvent(policy, corev1.EventTypeWarning, MemRetentionEventReasonWorkspacesMissing, err.Error())
		policy.Status.Phase = omniav1alpha1.MemoryPolicyPhaseError
		policy.Status.ObservedGeneration = policy.Generation
		policy.Status.WorkspaceCount = resolvedCount
		if statusErr := r.Status().Update(ctx, policy); statusErr != nil {
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		return ctrl.Result{}, err
	}

	if len(policy.Spec.PerWorkspace) == 0 {
		SetCondition(&policy.Status.Conditions, policy.Generation, MemRetentionConditionTypeWorkspacesResolved,
			metav1.ConditionTrue, "NoOverrides", "No per-workspace overrides configured")
	} else {
		SetCondition(&policy.Status.Conditions, policy.Generation, MemRetentionConditionTypeWorkspacesResolved,
			metav1.ConditionTrue, "AllResolved",
			fmt.Sprintf("All %d workspace references resolved", resolvedCount))
		r.emitEvent(policy, corev1.EventTypeNormal, MemRetentionEventReasonWorkspacesResolved,
			fmt.Sprintf("All %d workspace references resolved", resolvedCount))
	}

	SetCondition(&policy.Status.Conditions, policy.Generation, MemRetentionConditionTypeReady,
		metav1.ConditionTrue, "AllChecksPass", "Policy is valid and workspaces resolved")

	policy.Status.Phase = omniav1alpha1.MemoryPolicyPhaseActive
	policy.Status.ObservedGeneration = policy.Generation
	policy.Status.WorkspaceCount = resolvedCount

	if err := r.Status().Update(ctx, policy); err != nil {
		log.Error(err, logMsgFailedToUpdateStatus)
		return ctrl.Result{}, err
	}

	r.emitEvent(policy, corev1.EventTypeNormal, MemRetentionEventReasonActive,
		fmt.Sprintf("Policy is active with %d workspace overrides", resolvedCount))

	log.V(1).Info("reconciled MemoryPolicy",
		"name", req.Name,
		"phase", policy.Status.Phase,
		"workspaces", resolvedCount,
	)
	return ctrl.Result{}, nil
}

// markError sets the given condition + Ready=false + phase=Error and
// persists status. Returns the validation error so the caller can
// propagate it.
func (r *MemoryPolicyReconciler) markError(
	ctx context.Context,
	policy *omniav1alpha1.MemoryPolicy,
	condType, reason, message string,
	eventReason string,
) (ctrl.Result, error) {
	SetCondition(&policy.Status.Conditions, policy.Generation, condType,
		metav1.ConditionFalse, reason, message)
	SetCondition(&policy.Status.Conditions, policy.Generation, MemRetentionConditionTypeReady,
		metav1.ConditionFalse, reason, "See PolicyValid condition for details")
	r.emitEvent(policy, corev1.EventTypeWarning, eventReason, message)
	policy.Status.Phase = omniav1alpha1.MemoryPolicyPhaseError
	policy.Status.ObservedGeneration = policy.Generation
	if statusErr := r.Status().Update(ctx, policy); statusErr != nil {
		logf.FromContext(ctx).Error(statusErr, logMsgFailedToUpdateStatus)
	}
	return ctrl.Result{}, fmt.Errorf("%s: %s", reason, message)
}

// emitEvent is a nil-safe Recorder helper.
func (r *MemoryPolicyReconciler) emitEvent(obj runtime.Object, eventType, reason, message string) {
	if r.Recorder != nil {
		r.Recorder.Event(obj, eventType, reason, message)
	}
}

// resolveWorkspaces confirms every per-workspace override targets a
// Workspace that actually exists. Missing workspaces produce a clear
// error so operators don't discover a typo only when the retention
// worker skips a workspace silently.
func (r *MemoryPolicyReconciler) resolveWorkspaces(
	ctx context.Context, policy *omniav1alpha1.MemoryPolicy,
) (int32, error) {
	if len(policy.Spec.PerWorkspace) == 0 {
		return 0, nil
	}
	var resolved int32
	var missing []string
	for name := range policy.Spec.PerWorkspace {
		ws := &omniav1alpha1.Workspace{}
		if err := r.Get(ctx, types.NamespacedName{Name: name}, ws); err != nil {
			if apierrors.IsNotFound(err) {
				missing = append(missing, name)
				continue
			}
			return resolved, fmt.Errorf("failed to get workspace %q: %w", name, err)
		}
		resolved++
	}
	if len(missing) > 0 {
		return resolved, fmt.Errorf("workspaces not found: %v", missing)
	}
	return resolved, nil
}

// validatePolicy enforces the semantic rules that can't be expressed
// via kubebuilder markers alone.
func (r *MemoryPolicyReconciler) validatePolicy(
	policy *omniav1alpha1.MemoryPolicy,
) error {
	if err := validateRetentionDefaults(&policy.Spec.Default); err != nil {
		return fmt.Errorf("default: %w", err)
	}
	for name, override := range policy.Spec.PerWorkspace {
		if err := validateWorkspaceOverride(&override); err != nil {
			return fmt.Errorf("perWorkspace[%s]: %w", name, err)
		}
	}
	return nil
}

func validateRetentionDefaults(d *omniav1alpha1.MemoryRetentionDefaults) error {
	if err := validateTierSet(&d.Tiers); err != nil {
		return err
	}
	if d.Schedule != "" {
		if err := validateCronSchedule(d.Schedule); err != nil {
			return fmt.Errorf("schedule: %w", err)
		}
	}
	return nil
}

func validateWorkspaceOverride(o *omniav1alpha1.MemoryWorkspaceRetentionOverride) error {
	if o.Tiers != nil {
		if err := validateTierSet(o.Tiers); err != nil {
			return err
		}
	}
	if o.Schedule != "" {
		if err := validateCronSchedule(o.Schedule); err != nil {
			return fmt.Errorf("schedule: %w", err)
		}
	}
	return nil
}

func validateTierSet(t *omniav1alpha1.MemoryRetentionTierSet) error {
	if t.Institutional != nil {
		if err := validateTierConfig(t.Institutional, "institutional"); err != nil {
			return err
		}
	}
	if t.Agent != nil {
		if err := validateTierConfig(t.Agent, "agent"); err != nil {
			return err
		}
	}
	if t.User != nil {
		if err := validateTierConfig(t.User, "user"); err != nil {
			return err
		}
	}
	return nil
}

func validateTierConfig(c *omniav1alpha1.MemoryTierConfig, tierName string) error {
	if c.TTL != nil {
		if err := validateTTL(c.TTL); err != nil {
			return fmt.Errorf("%s.ttl: %w", tierName, err)
		}
	}
	if c.Decay != nil {
		if err := validateDecay(c.Decay); err != nil {
			return fmt.Errorf("%s.decay: %w", tierName, err)
		}
	}
	if c.LRU != nil && c.LRU.StaleAfter != "" {
		if _, err := parseExtendedDuration(c.LRU.StaleAfter); err != nil {
			return fmt.Errorf("%s.lru.staleAfter %q: %w", tierName, c.LRU.StaleAfter, err)
		}
	}
	for catName, catCfg := range c.PerCategory {
		cc := catCfg
		if err := validateLeafTierConfig(&cc, fmt.Sprintf("%s.perCategory[%s]", tierName, catName)); err != nil {
			return err
		}
	}
	return nil
}

func validateLeafTierConfig(c *omniav1alpha1.MemoryTierLeafConfig, tierName string) error {
	if c.TTL != nil {
		if err := validateTTL(c.TTL); err != nil {
			return fmt.Errorf("%s.ttl: %w", tierName, err)
		}
	}
	if c.Decay != nil {
		if err := validateDecay(c.Decay); err != nil {
			return fmt.Errorf("%s.decay: %w", tierName, err)
		}
	}
	if c.LRU != nil && c.LRU.StaleAfter != "" {
		if _, err := parseExtendedDuration(c.LRU.StaleAfter); err != nil {
			return fmt.Errorf("%s.lru.staleAfter %q: %w", tierName, c.LRU.StaleAfter, err)
		}
	}
	return nil
}

func validateTTL(t *omniav1alpha1.MemoryTTLConfig) error {
	if t.Default != "" {
		if _, err := parseExtendedDuration(t.Default); err != nil {
			return fmt.Errorf("default %q: %w", t.Default, err)
		}
	}
	if t.MaxAge != "" {
		if _, err := parseExtendedDuration(t.MaxAge); err != nil {
			return fmt.Errorf("maxAge %q: %w", t.MaxAge, err)
		}
	}
	if t.Default != "" && t.MaxAge != "" {
		d, _ := parseExtendedDuration(t.Default)
		m, _ := parseExtendedDuration(t.MaxAge)
		if d > m {
			return fmt.Errorf("default (%s) must not exceed maxAge (%s)", t.Default, t.MaxAge)
		}
	}
	return nil
}

func validateDecay(d *omniav1alpha1.MemoryDecayConfig) error {
	if d.MinScore != "" {
		v, err := strconv.ParseFloat(d.MinScore, 64)
		if err != nil {
			return fmt.Errorf("minScore %q: %w", d.MinScore, err)
		}
		if v < 0 || v > 1 {
			return fmt.Errorf("minScore %q must be between 0 and 1", d.MinScore)
		}
	}
	if d.ScoreFormula != nil {
		if err := validateWeight("confidenceWeight", d.ScoreFormula.ConfidenceWeight); err != nil {
			return err
		}
		if err := validateWeight("accessFrequencyWeight", d.ScoreFormula.AccessFrequencyWeight); err != nil {
			return err
		}
		if err := validateWeight("recencyWeight", d.ScoreFormula.RecencyWeight); err != nil {
			return err
		}
	}
	return nil
}

func validateWeight(name, raw string) error {
	if raw == "" {
		return nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fmt.Errorf("%s %q: %w", name, raw, err)
	}
	if v < 0 || v > 1 {
		return fmt.Errorf("%s %q must be between 0 and 1", name, raw)
	}
	return nil
}

// cronBasic covers the standard 5- or 6-field cron expressions and the
// common @every / @hourly / @daily / @weekly / @monthly / @yearly
// shortcuts used elsewhere in Omnia. Anything stranger than that is
// rejected so operators get a clear error instead of a silent no-op in
// Phase 3.
var cronBasic = regexp.MustCompile(
	`^(@(every +[0-9]+(ns|us|µs|ms|s|m|h)|hourly|daily|weekly|monthly|yearly|reboot)` +
		`|((\*|[0-9]+(,[0-9]+)*|[0-9]+(-|/)[0-9]+|\*/[0-9]+)( |$)){5,6})$`,
)

func validateCronSchedule(schedule string) error {
	if !cronBasic.MatchString(schedule) {
		return fmt.Errorf("invalid cron schedule %q", schedule)
	}
	return nil
}

// parseExtendedDuration supports Go duration syntax plus a "d" day
// suffix (matching the CRD's Pattern). "90d" → 90*24h. Pure suffix
// like "d" with no digits is rejected.
func parseExtendedDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	// Fast path: plain time.ParseDuration.
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	// Expand "<N>d" segments to "<N*24>h" and try again.
	expanded, err := expandDays(s)
	if err != nil {
		return 0, err
	}
	return time.ParseDuration(expanded)
}

func expandDays(s string) (string, error) {
	if !strings.Contains(s, "d") {
		return s, nil
	}
	var out strings.Builder
	var digits strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			digits.WriteByte(c)
			continue
		}
		if c == 'd' {
			if digits.Len() == 0 {
				return "", fmt.Errorf("dangling 'd' suffix in %q", s)
			}
			days, err := strconv.Atoi(digits.String())
			if err != nil {
				return "", fmt.Errorf("invalid day count in %q: %w", s, err)
			}
			hours := days * 24
			fmt.Fprintf(&out, "%dh", hours)
			digits.Reset()
			continue
		}
		// Any non-digit, non-'d' char: flush pending digits then keep going.
		if digits.Len() > 0 {
			out.WriteString(digits.String())
			digits.Reset()
		}
		out.WriteByte(c)
	}
	if digits.Len() > 0 {
		out.WriteString(digits.String())
	}
	return out.String(), nil
}

// findPoliciesForWorkspace watches Workspaces so a retention policy
// that previously failed validation (missing workspace) re-reconciles
// when the workspace lands.
func (r *MemoryPolicyReconciler) findPoliciesForWorkspace(
	ctx context.Context, obj client.Object,
) []reconcile.Request {
	ws, ok := obj.(*omniav1alpha1.Workspace)
	if !ok {
		return nil
	}
	policies := &omniav1alpha1.MemoryPolicyList{}
	if err := r.List(ctx, policies); err != nil {
		logf.FromContext(ctx).Error(err,
			"failed to list MemoryRetentionPolicies for Workspace mapping")
		return nil
	}
	var requests []reconcile.Request
	for _, p := range policies.Items {
		if _, exists := p.Spec.PerWorkspace[ws.Name]; exists {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: p.Name},
			})
		}
	}
	return requests
}

// SetupWithManager wires the controller into the manager.
func (r *MemoryPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 3}).
		For(&omniav1alpha1.MemoryPolicy{}).
		Watches(
			&omniav1alpha1.Workspace{},
			handler.EnqueueRequestsFromMapFunc(r.findPoliciesForWorkspace),
		).
		Named("memorypolicy").
		Complete(r)
}
