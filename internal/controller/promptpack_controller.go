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
	"fmt"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/promptpack/packselect"
	"github.com/altairalabs/omnia/internal/schema"
)

// PromptPack condition types
const (
	PromptPackConditionTypeSourceValid    = "SourceValid"
	PromptPackConditionTypeSchemaValid    = "SchemaValid"
	PromptPackConditionTypeAgentsNotified = "AgentsNotified"
	// PromptPackConditionTypeSuperseded is True when a newer version of the
	// pack has been published on the version-object's channel (stable or
	// prerelease). Its lastTransitionTime marks the supersession time, which
	// retention GC uses for its min-age guard.
	PromptPackConditionTypeSuperseded = "Superseded"
)

// LabelPromptPackName indexes a PromptPack version-object by its logical pack
// name. It aliases packselect.Label, the single source of truth for the value.
const LabelPromptPackName = packselect.Label

// Event reasons for PromptPack
const (
	EventReasonSourceValidationFailed = "SourceValidationFailed"
	EventReasonSchemaValidationFailed = "SchemaValidationFailed"
	EventReasonValidationSucceeded    = "ValidationSucceeded"
)

// PromptPackReconciler reconciles a PromptPack object
type PromptPackReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	SchemaValidator *schema.SchemaValidator
	Recorder        record.EventRecorder

	// WorkspaceContentPath is the base path for workspace content volumes.
	// Used to read SkillSource artifacts and write the per-pack skill
	// manifest. Empty disables skill resolution.
	WorkspaceContentPath string
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=promptpacks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=promptpacks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=promptpacks/finalizers,verbs=update
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentruntimes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *PromptPackReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling PromptPack", "name", req.Name, "namespace", req.Namespace)

	// Fetch the PromptPack instance
	promptPack := &omniav1alpha1.PromptPack{}
	if err := r.Get(ctx, req.NamespacedName, promptPack); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("PromptPack resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get PromptPack")
		return ctrl.Result{}, err
	}

	// Initialize status if needed
	if promptPack.Status.Phase == "" {
		promptPack.Status.Phase = omniav1alpha1.PromptPackPhasePending
	}

	// Ensure the resolution index label matches spec.packName. Labels are
	// metadata (not covered by the spec freeze), so this is safe to patch.
	// The Update call refreshes promptPack's ResourceVersion in place, so
	// reconciliation continues below using the same object rather than
	// requeuing for a second pass.
	if promptPack.Labels[LabelPromptPackName] != promptPack.Spec.PackName {
		if promptPack.Labels == nil {
			promptPack.Labels = map[string]string{}
		}
		promptPack.Labels[LabelPromptPackName] = promptPack.Spec.PackName
		if err := r.Update(ctx, promptPack); err != nil {
			log.Error(err, "failed to set promptpack label")
			return ctrl.Result{}, err
		}
	}

	// Step 1: Validate the source configuration (ConfigMap exists, has pack.json)
	packJSON, err := r.validateSource(ctx, promptPack)
	if err != nil {
		SetCondition(&promptPack.Status.Conditions, promptPack.Generation, PromptPackConditionTypeSourceValid, metav1.ConditionFalse,
			"SourceValidationFailed", err.Error())
		// Clear schema condition when source is invalid
		SetCondition(&promptPack.Status.Conditions, promptPack.Generation, PromptPackConditionTypeSchemaValid, metav1.ConditionUnknown,
			"SourceInvalid", "Cannot validate schema: source is invalid")
		promptPack.Status.Phase = omniav1alpha1.PromptPackPhaseFailed
		if r.Recorder != nil {
			r.Recorder.Event(promptPack, corev1.EventTypeWarning, EventReasonSourceValidationFailed, err.Error())
		}
		if statusErr := r.Status().Update(ctx, promptPack); statusErr != nil {
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		return ctrl.Result{}, err
	}
	SetCondition(&promptPack.Status.Conditions, promptPack.Generation, PromptPackConditionTypeSourceValid, metav1.ConditionTrue,
		"SourceValid", "Source configuration is valid")

	// Step 2: Validate pack.json content against the PromptPack schema
	if err := r.validateSchema(promptPack, packJSON); err != nil {
		SetCondition(&promptPack.Status.Conditions, promptPack.Generation, PromptPackConditionTypeSchemaValid, metav1.ConditionFalse,
			"SchemaValidationFailed", err.Error())
		promptPack.Status.Phase = omniav1alpha1.PromptPackPhaseFailed
		if r.Recorder != nil {
			r.Recorder.Event(promptPack, corev1.EventTypeWarning, EventReasonSchemaValidationFailed, err.Error())
		}
		if statusErr := r.Status().Update(ctx, promptPack); statusErr != nil {
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		return ctrl.Result{}, err
	}
	SetCondition(&promptPack.Status.Conditions, promptPack.Generation, PromptPackConditionTypeSchemaValid, metav1.ConditionTrue,
		"SchemaValid", "pack.json content is valid")

	// Step 3: Resolve spec.skills against SkillSources and emit the manifest.
	r.reconcileSkills(ctx, promptPack, packJSON)

	// Find all AgentRuntimes referencing this PromptPack
	referencingRuntimes, err := r.findReferencingAgentRuntimes(ctx, promptPack)
	if err != nil {
		log.Error(err, "Failed to find referencing AgentRuntimes")
		return ctrl.Result{}, err
	}
	log.V(1).Info("Found referencing AgentRuntimes", "count", len(referencingRuntimes))

	// List sibling version-objects (same packName) so phase can be computed
	// channel-aware: a version is Active iff it is the stable- or
	// prerelease-channel-max of its pack; otherwise Superseded.
	siblings, err := r.listSiblings(ctx, promptPack)
	if err != nil {
		log.Error(err, "Failed to list sibling PromptPacks")
		return ctrl.Result{}, err
	}

	// Update status based on rollout strategy
	r.updateRolloutStatus(promptPack, referencingRuntimes, siblings)

	// Set notification condition
	if len(referencingRuntimes) > 0 {
		SetCondition(&promptPack.Status.Conditions, promptPack.Generation, PromptPackConditionTypeAgentsNotified, metav1.ConditionTrue,
			"AgentsNotified", fmt.Sprintf("Notified %d AgentRuntime(s)", len(referencingRuntimes)))
	} else {
		SetCondition(&promptPack.Status.Conditions, promptPack.Generation, PromptPackConditionTypeAgentsNotified, metav1.ConditionTrue,
			"NoAgentsToNotify", "No AgentRuntimes reference this PromptPack")
	}

	// Update last updated timestamp
	now := metav1.Now()
	promptPack.Status.LastUpdated = &now

	if err := r.Status().Update(ctx, promptPack); err != nil {
		log.Error(err, "Failed to update PromptPack status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileSkills resolves spec.skills, validates allowed-tools, emits the
// manifest, and surfaces SkillsResolved/SkillsValid/SkillToolsResolved
// conditions. No-ops when WorkspaceContentPath is unset OR spec.skills is
// empty.
func (r *PromptPackReconciler) reconcileSkills(ctx context.Context, pack *omniav1alpha1.PromptPack, packJSON string) {
	log := logf.FromContext(ctx)

	// Treat "no skills" and "content storage unavailable" as distinct states
	// so operators can tell why a pack isn't resolving skills.
	if len(pack.Spec.Skills) == 0 {
		SetCondition(&pack.Status.Conditions, pack.Generation,
			omniav1alpha1.PromptPackConditionSkillsResolved, metav1.ConditionTrue,
			"NoSkills", "pack does not declare spec.skills")
		return
	}
	if r.WorkspaceContentPath == "" {
		SetCondition(&pack.Status.Conditions, pack.Generation,
			omniav1alpha1.PromptPackConditionSkillsResolved, metav1.ConditionFalse,
			"ContentStorageUnavailable",
			"PromptPack declares spec.skills but the operator was started without workspace content storage (chart value workspaceContent.enabled=false). Enable workspaceContent in the chart and restart the operator to resolve skills.")
		log.V(1).Info("skill resolution skipped: workspace content storage disabled",
			"reason", "ContentStorageUnavailable")
		return
	}

	res := ResolvePromptPackSkills(ctx, r.Client, pack, r.WorkspaceContentPath)

	// SkillsResolved
	if len(res.LookupErrors) > 0 {
		msgs := make([]string, 0, len(res.LookupErrors))
		for _, e := range res.LookupErrors {
			msgs = append(msgs, e.Error())
		}
		SetCondition(&pack.Status.Conditions, pack.Generation,
			omniav1alpha1.PromptPackConditionSkillsResolved, metav1.ConditionFalse,
			"LookupFailed", strings.Join(msgs, "; "))
	} else {
		SetCondition(&pack.Status.Conditions, pack.Generation,
			omniav1alpha1.PromptPackConditionSkillsResolved, metav1.ConditionTrue,
			"AllSkillsResolved", fmt.Sprintf("resolved %d skills", len(res.Manifest.Skills)))
	}

	// SkillsValid
	if len(res.CollisionErrors) > 0 {
		msgs := make([]string, 0, len(res.CollisionErrors))
		for _, e := range res.CollisionErrors {
			msgs = append(msgs, e.Error())
		}
		SetCondition(&pack.Status.Conditions, pack.Generation,
			omniav1alpha1.PromptPackConditionSkillsValid, metav1.ConditionFalse,
			"NameCollision", strings.Join(msgs, "; "))
	} else {
		SetCondition(&pack.Status.Conditions, pack.Generation,
			omniav1alpha1.PromptPackConditionSkillsValid, metav1.ConditionTrue,
			"NoCollisions", fmt.Sprintf("%d skills, no collisions", len(res.Manifest.Skills)))
	}

	// SkillToolsResolved
	packTools := ExtractPackTools(packJSON)
	bad := ValidateSkillTools(res.AllowedToolsBySkill, packTools)
	if len(bad) > 0 {
		SetCondition(&pack.Status.Conditions, pack.Generation,
			omniav1alpha1.PromptPackConditionSkillToolsResolved, metav1.ConditionFalse,
			"UnknownTool", "skill allowed-tools not declared by pack: "+strings.Join(bad, ", "))
	} else {
		SetCondition(&pack.Status.Conditions, pack.Generation,
			omniav1alpha1.PromptPackConditionSkillToolsResolved, metav1.ConditionTrue,
			"AllToolsResolved", "all skill allowed-tools are declared by the pack")
	}

	// Emit the manifest into the workspace PVC. Failure is non-fatal —
	// the conditions above record what we know; missing manifest just
	// means the runtime won't load skills until next reconcile.
	workspaceName := GetWorkspaceForNamespace(ctx, r.Client, pack.Namespace)
	manifestRoot := filepath.Join(r.WorkspaceContentPath, workspaceName, pack.Namespace)
	if err := WriteSkillManifest(manifestRoot, pack.Name, res.Manifest); err != nil {
		log.Error(err, "write skill manifest", "pack", pack.Name)
	}
}

// validateSource validates the source configuration and returns the pack.json content.
// Returns the pack.json content as a string for subsequent schema validation.
func (r *PromptPackReconciler) validateSource(ctx context.Context, promptPack *omniav1alpha1.PromptPack) (string, error) {
	switch promptPack.Spec.Source.Type {
	case omniav1alpha1.PromptPackSourceTypeConfigMap:
		return r.validateConfigMapSource(ctx, promptPack)
	default:
		return "", fmt.Errorf("unsupported source type: %s", promptPack.Spec.Source.Type)
	}
}

// validateConfigMapSource validates that the referenced ConfigMap exists and contains pack.json.
// Returns the pack.json content for subsequent schema validation.
func (r *PromptPackReconciler) validateConfigMapSource(ctx context.Context, promptPack *omniav1alpha1.PromptPack) (string, error) {
	if promptPack.Spec.Source.ConfigMapRef == nil {
		return "", fmt.Errorf("configMapRef is required when source type is configmap")
	}

	configMap := &corev1.ConfigMap{}
	key := types.NamespacedName{
		Name:      promptPack.Spec.Source.ConfigMapRef.Name,
		Namespace: promptPack.Namespace,
	}

	if err := r.Get(ctx, key, configMap); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("ConfigMap %q not found in namespace %q", key.Name, key.Namespace)
		}
		return "", fmt.Errorf("failed to get ConfigMap %q: %w", key.Name, err)
	}

	// Validate that the ConfigMap has at least some data
	if len(configMap.Data) == 0 && len(configMap.BinaryData) == 0 {
		return "", fmt.Errorf("ConfigMap %q is empty", key.Name)
	}

	// Check for pack.json file
	packJSON, ok := configMap.Data["pack.json"]
	if !ok {
		return "", fmt.Errorf("ConfigMap %q does not contain required 'pack.json' key", key.Name)
	}

	return packJSON, nil
}

// validateSchema validates the pack.json content against the published PromptPack schema.
func (r *PromptPackReconciler) validateSchema(_ *omniav1alpha1.PromptPack, packJSON string) error {
	if r.SchemaValidator == nil {
		// No validator configured, skip schema validation
		return nil
	}

	if err := r.SchemaValidator.Validate([]byte(packJSON)); err != nil {
		return fmt.Errorf("invalid pack.json: %w", err)
	}

	return nil
}

// findReferencingAgentRuntimes finds all AgentRuntimes that reference this PromptPack.
func (r *PromptPackReconciler) findReferencingAgentRuntimes(ctx context.Context, promptPack *omniav1alpha1.PromptPack) ([]omniav1alpha1.AgentRuntime, error) {
	// List all AgentRuntimes in the same namespace
	agentRuntimeList := &omniav1alpha1.AgentRuntimeList{}
	if err := r.List(ctx, agentRuntimeList, client.InNamespace(promptPack.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list AgentRuntimes: %w", err)
	}

	var referencingRuntimes []omniav1alpha1.AgentRuntime
	for _, ar := range agentRuntimeList.Items {
		// AgentRuntimes reference a PromptPack by its logical packName
		// (spec.packName), NOT its object name (metadata.name is now a
		// deterministic pp-<hash>, per Phase 1 of #1837).
		if ar.Spec.PromptPackRef.Name == promptPack.Spec.PackName {
			// Exact version pin: only the matching version-object references it.
			// The comparison is semver-aware (e.g. "1.2.0" == "1.2.0+build.5")
			// rather than raw string equality, with a defensive string-equality
			// fallback for unparseable values. Otherwise (track set, or neither
			// set -> defaults to the stable channel): the AR tracks the
			// packName as a whole, so every version-object of that packName
			// "references" it.
			if ar.Spec.PromptPackRef.Version != nil {
				if versionsEqual(*ar.Spec.PromptPackRef.Version, promptPack.Spec.Version) {
					referencingRuntimes = append(referencingRuntimes, ar)
				}
			} else {
				referencingRuntimes = append(referencingRuntimes, ar)
			}
		}
	}

	return referencingRuntimes, nil
}

// listSiblings returns the version-objects sharing this pack's logical name
// (matched by the LabelPromptPackName label). The reconciling pack is always
// included: it reached this point post-validation, so it participates as a
// valid channel-max candidate even if its listed copy is momentarily stale
// (eventual consistency) or missing from the index.
func (r *PromptPackReconciler) listSiblings(ctx context.Context, pack *omniav1alpha1.PromptPack) ([]omniav1alpha1.PromptPack, error) {
	var sibs omniav1alpha1.PromptPackList
	if err := r.List(ctx, &sibs,
		client.InNamespace(pack.Namespace),
		client.MatchingLabels{LabelPromptPackName: pack.Spec.PackName}); err != nil {
		return nil, err
	}
	items := sibs.Items
	for i := range items {
		if items[i].Name == pack.Name {
			return items, nil
		}
	}
	return append(items, *pack), nil
}

// resolvePackPhase reports whether self is Active — i.e. its version equals the
// stable-channel-max OR the prerelease-channel-max of the eligible siblings.
// Eligible = VALIDATED siblings (Phase Active or Superseded) plus self. Self is
// always eligible: it reached updateRolloutStatus post-validation, so it is a
// valid channel-max candidate regardless of its currently-stored phase.
//
// Excluding not-yet-validated siblings (Phase Pending or "") is deliberate: a
// newer version whose pack.json is still unvalidated (or bad) must NOT supersede
// a live older version. The sibling watch re-enqueues older siblings on the
// newer version's phase transition (Pending→Active), so a valid newer version
// still converges the older one to Superseded on a subsequent reconcile.
func resolvePackPhase(self *omniav1alpha1.PromptPack, siblings []omniav1alpha1.PromptPack) bool {
	eligible := make([]omniav1alpha1.PromptPack, 0, len(siblings)+1)
	selfIncluded := false
	for i := range siblings {
		s := siblings[i]
		if s.Name == self.Name && s.Namespace == self.Namespace {
			eligible = append(eligible, *self)
			selfIncluded = true
			continue
		}
		if !isValidatedPhase(s.Status.Phase) {
			continue
		}
		eligible = append(eligible, s)
	}
	if !selfIncluded {
		eligible = append(eligible, *self)
	}

	stableMax, _ := packselect.ChannelMax(eligible, promptPackTrackStable)
	preMax, _ := packselect.ChannelMax(eligible, promptPackTrackPrerelease)
	return (stableMax != nil && versionsEqual(self.Spec.Version, stableMax.Spec.Version)) ||
		(preMax != nil && versionsEqual(self.Spec.Version, preMax.Spec.Version))
}

// isValidatedPhase reports whether a sibling has passed validation and so counts
// toward channel-max. Only Active and Superseded qualify; Failed and the
// not-yet-reconciled Pending/"" phases are excluded.
func isValidatedPhase(phase omniav1alpha1.PromptPackPhase) bool {
	return phase == omniav1alpha1.PromptPackPhaseActive ||
		phase == omniav1alpha1.PromptPackPhaseSuperseded
}

// updateRolloutStatus sets the pack's phase channel-aware: Active when it is the
// stable- or prerelease-channel-max of its siblings, otherwise Superseded. The
// referencingRuntimes count is retained for future metrics.
func (r *PromptPackReconciler) updateRolloutStatus(promptPack *omniav1alpha1.PromptPack, referencingRuntimes []omniav1alpha1.AgentRuntime, siblings []omniav1alpha1.PromptPack) {
	version := promptPack.Spec.Version
	_ = len(referencingRuntimes) // Track affected agents count for future metrics

	if resolvePackPhase(promptPack, siblings) {
		promptPack.Status.Phase = omniav1alpha1.PromptPackPhaseActive
		promptPack.Status.ActiveVersion = &version
		SetCondition(&promptPack.Status.Conditions, promptPack.Generation,
			PromptPackConditionTypeSuperseded, metav1.ConditionFalse,
			"IsChannelMax", "This version is the current channel-max for its pack")
		return
	}

	promptPack.Status.Phase = omniav1alpha1.PromptPackPhaseSuperseded
	// Clear the stale ActiveVersion: a pack that was previously the channel-max
	// no longer serves as the active version once superseded.
	promptPack.Status.ActiveVersion = nil
	SetCondition(&promptPack.Status.Conditions, promptPack.Generation,
		PromptPackConditionTypeSuperseded, metav1.ConditionTrue,
		"NewerVersionPublished", "A newer version of this pack has been published on its channel")
}

// findPromptPacksForConfigMap maps a ConfigMap to PromptPacks that reference it.
func (r *PromptPackReconciler) findPromptPacksForConfigMap(ctx context.Context, obj client.Object) []reconcile.Request {
	configMap := obj.(*corev1.ConfigMap)
	log := logf.FromContext(ctx)

	promptPackList := &omniav1alpha1.PromptPackList{}
	if err := r.List(ctx, promptPackList, client.InNamespace(configMap.Namespace)); err != nil {
		log.Error(err, "Failed to list PromptPacks for ConfigMap mapping")
		return nil
	}

	var requests []reconcile.Request
	for _, pp := range promptPackList.Items {
		if pp.Spec.Source.Type == omniav1alpha1.PromptPackSourceTypeConfigMap &&
			pp.Spec.Source.ConfigMapRef != nil &&
			pp.Spec.Source.ConfigMapRef.Name == configMap.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      pp.Name,
					Namespace: pp.Namespace,
				},
			})
		}
	}

	return requests
}

// findSiblingPromptPacks maps a changed PromptPack to every version-object
// sharing its logical pack name, so publishing a new version re-reconciles the
// older siblings (which then transition to Superseded).
func (r *PromptPackReconciler) findSiblingPromptPacks(ctx context.Context, obj client.Object) []reconcile.Request {
	log := logf.FromContext(ctx)
	packName := obj.GetLabels()[LabelPromptPackName]
	if packName == "" {
		return nil
	}

	promptPackList := &omniav1alpha1.PromptPackList{}
	if err := r.List(ctx, promptPackList,
		client.InNamespace(obj.GetNamespace()),
		client.MatchingLabels{LabelPromptPackName: packName}); err != nil {
		log.Error(err, "Failed to list sibling PromptPacks for watch mapping")
		return nil
	}

	requests := make([]reconcile.Request, 0, len(promptPackList.Items))
	for i := range promptPackList.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      promptPackList.Items[i].Name,
				Namespace: promptPackList.Items[i].Namespace,
			},
		})
	}
	return requests
}

// siblingPhaseChangedPredicate fires the sibling watch on spec (generation)
// changes AND on Status.Phase transitions, but NOT on other status-only writes
// (e.g. LastUpdated timestamp refreshes). Phase transitions must fan out because
// eligibility for channel-max is gated on a sibling's validated phase: a newer
// version going Pending→Active (or Active→Superseded) has to re-enqueue the
// older siblings so they re-resolve their phase. GenerationChangedPredicate
// alone would drop those status-only phase transitions, stalling convergence.
func siblingPhaseChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return false },
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld == nil || e.ObjectNew == nil {
				return false
			}
			if e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() {
				return true
			}
			oldPack, okOld := e.ObjectOld.(*omniav1alpha1.PromptPack)
			newPack, okNew := e.ObjectNew.(*omniav1alpha1.PromptPack)
			if !okOld || !okNew {
				return false
			}
			return oldPack.Status.Phase != newPack.Status.Phase
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *PromptPackReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniav1alpha1.PromptPack{}).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.findPromptPacksForConfigMap),
		).
		// Publishing a new version-object — or a sibling's phase transitioning
		// once validated — must re-reconcile older siblings so they converge to
		// Active/Superseded. The custom predicate fires on generation changes
		// and Status.Phase transitions, but not on LastUpdated-only status
		// writes, avoiding reconcile churn.
		Watches(
			&omniav1alpha1.PromptPack{},
			handler.EnqueueRequestsFromMapFunc(r.findSiblingPromptPacks),
			builder.WithPredicates(siblingPhaseChangedPredicate()),
		).
		Named("promptpack").
		Complete(r)
}
