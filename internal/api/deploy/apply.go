package deploy

import (
	"context"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const (
	kindPromptPack   = "PromptPack"
	kindConfigMap    = "ConfigMap"
	kindAgentRuntime = "AgentRuntime"
	kindToolRegistry = "ToolRegistry"
	kindAgentPolicy  = "AgentPolicy"
)

// Applier translates a DeployIntent and applies the resulting objects.
type Applier struct {
	client client.Client
	log    logr.Logger
}

// NewApplier constructs an Applier backed by a Kubernetes client.
func NewApplier(c client.Client, log logr.Logger) *Applier {
	return &Applier{client: c, log: log}
}

// Apply materializes the intent: the pack content ConfigMap then the
// PromptPack that references it first, then each AgentRuntime. Best-effort —
// a failed resource is recorded and the rest still apply. Succeeded is false
// if any resource failed.
func (a *Applier) Apply(ctx context.Context, namespace string, intent DeployIntent) DeployResult {
	results := make([]ResourceResult, 0, 2+len(intent.Agents))

	cm := packContentConfigMap(namespace, intent.Pack, intent.Labels)
	results = append(results, a.createImmutable(ctx, kindConfigMap, cm))

	pp := packToPromptPack(namespace, intent.Pack, intent.Labels)
	results = append(results, a.createImmutable(ctx, kindPromptPack, pp))

	if r, ok := a.applyToolRegistry(ctx, namespace, intent); ok {
		results = append(results, r)
	}

	registryName := deployRegistryName(intent.Pack, intent.Tools)

	if desired := agentPolicy(namespace, intent.Pack, intent.Policy, agentNames(intent.Agents), registryName, intent.Labels); desired != nil {
		results = append(results, a.upsertAgentPolicy(ctx, desired))
	}

	for _, agent := range intent.Agents {
		desired := agentToAgentRuntime(namespace, intent.Pack, agent, registryName, intent.Labels)
		results = append(results, a.upsertAgentRuntime(ctx, desired))
	}

	succeeded := true
	for _, r := range results {
		if r.Action == ActionFailed {
			succeeded = false
		}
	}
	return DeployResult{Succeeded: succeeded, Results: results}
}

// createImmutable creates an object that is never updated in place (PromptPack,
// its ConfigMap). An existing object of the same name is reported unchanged.
func (a *Applier) createImmutable(ctx context.Context, kind string, obj client.Object) ResourceResult {
	err := a.client.Create(ctx, obj)
	switch {
	case err == nil:
		return ResourceResult{Kind: kind, Name: obj.GetName(), Action: ActionCreated}
	case apierrors.IsAlreadyExists(err):
		return ResourceResult{Kind: kind, Name: obj.GetName(), Action: ActionUnchanged}
	default:
		a.log.Error(err, "deploy create failed", "kind", kind, "name", obj.GetName())
		return ResourceResult{Kind: kind, Name: obj.GetName(), Action: ActionFailed, Error: err.Error()}
	}
}

// applyToolRegistry translates and create-only applies the ToolRegistry for
// intent.Tools, when it has handlers. Returns ok=false when there's nothing to
// apply — a nil or ref-only ToolsIntent creates no registry object and
// contributes no result entry.
func (a *Applier) applyToolRegistry(ctx context.Context, namespace string, intent DeployIntent) (ResourceResult, bool) {
	if intent.Tools == nil || len(intent.Tools.Handlers) == 0 {
		return ResourceResult{}, false
	}
	tr, err := toolRegistry(namespace, intent.Pack, intent.Tools, intent.Labels)
	if err != nil {
		name := toolRegistryName(intent.Pack.Name)
		a.log.Error(err, "deploy translate failed", "kind", kindToolRegistry, "name", name)
		return ResourceResult{Kind: kindToolRegistry, Name: name, Action: ActionFailed, Error: err.Error()}, true
	}
	return a.createToolRegistry(ctx, tr), true
}

// createToolRegistry creates a ToolRegistry create-only: an existing registry
// is operator/user-owned and is never updated by a deploy (AlreadyExists =>
// unchanged). An RBAC-forbidden create degrades to a logged warning and a
// non-fatal "unchanged" result — the deploy still proceeds — rather than
// failing the whole deploy over a permissions gap on an optional resource.
func (a *Applier) createToolRegistry(ctx context.Context, obj *omniav1alpha1.ToolRegistry) ResourceResult {
	err := a.client.Create(ctx, obj)
	switch {
	case err == nil:
		return ResourceResult{Kind: kindToolRegistry, Name: obj.GetName(), Action: ActionCreated}
	case apierrors.IsAlreadyExists(err):
		return ResourceResult{Kind: kindToolRegistry, Name: obj.GetName(), Action: ActionUnchanged}
	case apierrors.IsForbidden(err):
		a.log.Info("deploy tool registry create forbidden, skipping",
			"kind", kindToolRegistry, "name", obj.GetName(), "reason", err.Error())
		return ResourceResult{Kind: kindToolRegistry, Name: obj.GetName(), Action: ActionUnchanged}
	default:
		a.log.Error(err, "deploy create failed", "kind", kindToolRegistry, "name", obj.GetName())
		return ResourceResult{Kind: kindToolRegistry, Name: obj.GetName(), Action: ActionFailed, Error: err.Error()}
	}
}

// agentNames extracts the AgentRuntime names an AgentPolicy selector should
// target from the intent's agents.
func agentNames(agents []AgentIntent) []string {
	out := make([]string, 0, len(agents))
	for _, agent := range agents {
		out = append(out, agent.Name)
	}
	return out
}

// upsertAgentPolicy creates the AgentPolicy, or updates the existing one's
// spec + labels in place. Unlike AgentRuntime, AgentPolicy carries no
// controller-owned fields (no rollout pin/candidate to preserve), so the
// update path is a plain spec overwrite.
func (a *Applier) upsertAgentPolicy(ctx context.Context, desired *omniav1alpha1.AgentPolicy) ResourceResult {
	live := &omniav1alpha1.AgentPolicy{}
	err := a.client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, live)
	if apierrors.IsNotFound(err) {
		if cerr := a.client.Create(ctx, desired); cerr != nil {
			a.log.Error(cerr, "deploy create failed", "kind", kindAgentPolicy, "name", desired.Name)
			return ResourceResult{Kind: kindAgentPolicy, Name: desired.Name, Action: ActionFailed, Error: cerr.Error()}
		}
		return ResourceResult{Kind: kindAgentPolicy, Name: desired.Name, Action: ActionCreated}
	}
	if err != nil {
		a.log.Error(err, "deploy get failed", "kind", kindAgentPolicy, "name", desired.Name)
		return ResourceResult{Kind: kindAgentPolicy, Name: desired.Name, Action: ActionFailed, Error: err.Error()}
	}

	live.Spec = desired.Spec
	if live.Labels == nil {
		live.Labels = map[string]string{}
	}
	for k, v := range desired.Labels {
		live.Labels[k] = v
	}
	if uerr := a.client.Update(ctx, live); uerr != nil {
		a.log.Error(uerr, "deploy update failed", "kind", kindAgentPolicy, "name", desired.Name)
		return ResourceResult{Kind: kindAgentPolicy, Name: desired.Name, Action: ActionFailed, Error: uerr.Error()}
	}
	return ResourceResult{Kind: kindAgentPolicy, Name: desired.Name, Action: ActionUpdated}
}

// upsertAgentRuntime creates the AgentRuntime, or updates the existing one
// (rollout-aware — see reconcileAgentRuntimeSpec).
func (a *Applier) upsertAgentRuntime(ctx context.Context, desired *omniav1alpha1.AgentRuntime) ResourceResult {
	live := &omniav1alpha1.AgentRuntime{}
	err := a.client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, live)
	if apierrors.IsNotFound(err) {
		if cerr := a.client.Create(ctx, desired); cerr != nil {
			a.log.Error(cerr, "deploy create failed", "kind", kindAgentRuntime, "name", desired.Name)
			return ResourceResult{Kind: kindAgentRuntime, Name: desired.Name, Action: ActionFailed, Error: cerr.Error()}
		}
		return ResourceResult{Kind: kindAgentRuntime, Name: desired.Name, Action: ActionCreated}
	}
	if err != nil {
		a.log.Error(err, "deploy get failed", "kind", kindAgentRuntime, "name", desired.Name)
		return ResourceResult{Kind: kindAgentRuntime, Name: desired.Name, Action: ActionFailed, Error: err.Error()}
	}

	reconcileAgentRuntimeSpec(live, desired)
	if uerr := a.client.Update(ctx, live); uerr != nil {
		a.log.Error(uerr, "deploy update failed", "kind", kindAgentRuntime, "name", desired.Name)
		return ResourceResult{Kind: kindAgentRuntime, Name: desired.Name, Action: ActionFailed, Error: uerr.Error()}
	}
	return ResourceResult{Kind: kindAgentRuntime, Name: desired.Name, Action: ActionUpdated}
}

// reconcileAgentRuntimeSpec copies the desired spec onto the live object,
// rollout-aware: when the LIVE agent is in version-trigger mode, the deploy
// must not advance the PromptPack pin or clobber an in-flight canary — the
// #1838 controller owns those. Every other desired field is still applied,
// EXCEPT that a config-only deploy (intent omits the rollout block entirely)
// against a trigger-mode agent leaves the live rollout — trigger, steps, and
// candidate — untouched, since RolloutConfig.Steps is CRD-required
// (MinItems=1) and rebuilding it with only Candidate set would produce an
// invalid object that a real apiserver Update rejects outright.
func reconcileAgentRuntimeSpec(live, desired *omniav1alpha1.AgentRuntime) {
	// Preservation below covers ONLY version-trigger rollouts (spec.rollout.trigger
	// != nil, i.e. the #1838 controller owns the pin/candidate). A manually-driven
	// rollout (candidate set without a trigger) is NOT preserved in Plan A — the
	// desired spec always wins for that case.
	triggerMode := live.Spec.Rollout != nil && live.Spec.Rollout.Trigger != nil

	var (
		preservedRef       omniav1alpha1.PromptPackRef
		preservedRollout   *omniav1alpha1.RolloutConfig
		preservedCandidate *omniav1alpha1.CandidateOverrides
	)
	if triggerMode {
		preservedRef = live.Spec.PromptPackRef
		preservedRollout = live.Spec.Rollout
		preservedCandidate = preservedRollout.Candidate
	}

	live.Spec = desired.Spec

	if triggerMode {
		live.Spec.PromptPackRef = preservedRef
		switch live.Spec.Rollout {
		case nil:
			// Config-only deploy: intent had no rollout block. Keep the live
			// rollout wholesale so trigger+steps+candidate all stay intact.
			live.Spec.Rollout = preservedRollout
		default:
			// Intent specified a rollout (new trigger/steps) — honor it, but
			// the in-flight candidate is still owned by the controller.
			live.Spec.Rollout.Candidate = preservedCandidate
		}
	}

	if live.Labels == nil {
		live.Labels = map[string]string{}
	}
	for k, v := range desired.Labels {
		live.Labels[k] = v
	}
}
