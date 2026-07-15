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
	var results []ResourceResult

	cm := packContentConfigMap(namespace, intent.Pack, intent.Labels)
	results = append(results, a.createImmutable(ctx, kindConfigMap, cm))

	pp := packToPromptPack(namespace, intent.Pack, intent.Labels)
	results = append(results, a.createImmutable(ctx, kindPromptPack, pp))

	for _, agent := range intent.Agents {
		desired := agentToAgentRuntime(namespace, intent.Pack, agent, intent.Labels)
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

// reconcileAgentRuntimeSpec copies the desired spec onto the live object.
// Task 5 makes it rollout-aware; for now it is a full spec replace.
func reconcileAgentRuntimeSpec(live, desired *omniav1alpha1.AgentRuntime) {
	live.Spec = desired.Spec
	if live.Labels == nil {
		live.Labels = map[string]string{}
	}
	for k, v := range desired.Labels {
		live.Labels[k] = v
	}
}
