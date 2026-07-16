package deploy

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/go-logr/logr"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// promptpackLabel mirrors packselect.Label's value as an external-contract
// assertion (the persisted label a consumer of the apply result would see).
const promptpackLabel = "omnia.altairalabs.ai/promptpack"

var errBoom = errors.New("boom")

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := omniav1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func testIntent() DeployIntent {
	return DeployIntent{
		APIVersion: APIVersionV1,
		Pack:       PackIntent{Name: "support", Version: "1.0.0", Content: "{}"},
		Agents:     []AgentIntent{{Name: "support", Providers: []ProviderBind{{Name: "default", Ref: "claude"}}}},
	}
}

func TestApply_CreatesThenUnchanged(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).Build()
	a := NewApplier(c, logr.Discard())

	intent := testIntent()
	res := a.Apply(context.Background(), "ns", intent)
	if !res.Succeeded {
		t.Fatalf("apply failed: %+v", res.Results)
	}
	// PromptPack + ConfigMap + AgentRuntime all created.
	byKind := map[string]string{}
	for _, r := range res.Results {
		byKind[r.Kind] = r.Action
	}
	if byKind["PromptPack"] != ActionCreated || byKind["ConfigMap"] != ActionCreated || byKind["AgentRuntime"] != ActionCreated {
		t.Fatalf("first apply actions = %+v", res.Results)
	}

	assertPackObjectsPersisted(t, c, intent)
	assertAgentRuntimePersisted(t, c, "ns", intent.Agents[0].Name, intent.Pack.Name, intent.Pack.Version, "claude")

	// Re-apply with a VARIED intent (different provider ref on the agent) to
	// prove Update genuinely writes the new desired spec, not just that it
	// reports "updated" while leaving the stale first-apply data in place.
	intent2 := testIntent()
	intent2.Agents[0].Providers[0].Ref = "gpt"
	res2 := a.Apply(context.Background(), "ns", intent2)
	byKind2 := map[string]string{}
	for _, r := range res2.Results {
		byKind2[r.Kind] = r.Action
	}
	if byKind2["PromptPack"] != ActionUnchanged || byKind2["ConfigMap"] != ActionUnchanged {
		t.Errorf("re-apply pack actions = %+v", res2.Results)
	}
	if byKind2["AgentRuntime"] != ActionUpdated {
		t.Errorf("re-apply agent action = %s", byKind2["AgentRuntime"])
	}

	assertAgentRuntimePersisted(t, c, "ns", intent.Agents[0].Name, intent.Pack.Name, intent.Pack.Version, "gpt")
}

// assertPackObjectsPersisted verifies the pack content ConfigMap and the
// PromptPack it backs were actually written to the client with the intent's
// data, not just reported "created" by the apply result.
func assertPackObjectsPersisted(t *testing.T, c client.Client, intent DeployIntent) {
	t.Helper()
	packObjName := omniav1alpha1.PromptPackObjectName(intent.Pack.Name, intent.Pack.Version)

	cm := &corev1.ConfigMap{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: packObjName + "-content", Namespace: "ns"}, cm); err != nil {
		t.Fatalf("get ConfigMap: %v", err)
	}
	if cm.Data["pack.json"] != intent.Pack.Content {
		t.Errorf("ConfigMap Data[pack.json] = %q, want %q", cm.Data["pack.json"], intent.Pack.Content)
	}

	pp := &omniav1alpha1.PromptPack{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: packObjName, Namespace: "ns"}, pp); err != nil {
		t.Fatalf("get PromptPack: %v", err)
	}
	if pp.Spec.PackName != intent.Pack.Name {
		t.Errorf("PromptPack Spec.PackName = %q, want %q", pp.Spec.PackName, intent.Pack.Name)
	}
	if pp.Spec.Version != intent.Pack.Version {
		t.Errorf("PromptPack Spec.Version = %q, want %q", pp.Spec.Version, intent.Pack.Version)
	}
	if pp.Labels[promptpackLabel] != intent.Pack.Name {
		t.Errorf("PromptPack label %s = %q, want %q", promptpackLabel, pp.Labels[promptpackLabel], intent.Pack.Name)
	}
}

// assertAgentRuntimePersisted fetches the AgentRuntime and verifies its
// PromptPackRef and single provider ref actually match what was applied —
// proving Create/Update wrote real data, not just that the action was
// reported as created/updated.
func assertAgentRuntimePersisted(t *testing.T, c client.Client, namespace, name, packName, packVersion, wantProviderRef string) {
	t.Helper()
	ar := &omniav1alpha1.AgentRuntime{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: name, Namespace: namespace}, ar); err != nil {
		t.Fatalf("get AgentRuntime: %v", err)
	}
	if ar.Spec.PromptPackRef.Name != packName {
		t.Errorf("AgentRuntime Spec.PromptPackRef.Name = %q, want %q", ar.Spec.PromptPackRef.Name, packName)
	}
	if ar.Spec.PromptPackRef.Version == nil || *ar.Spec.PromptPackRef.Version != packVersion {
		t.Errorf("AgentRuntime Spec.PromptPackRef.Version = %v, want %q", ar.Spec.PromptPackRef.Version, packVersion)
	}
	if len(ar.Spec.Providers) != 1 || ar.Spec.Providers[0].ProviderRef.Name != wantProviderRef {
		t.Fatalf("AgentRuntime Spec.Providers = %+v, want a single provider ref %q", ar.Spec.Providers, wantProviderRef)
	}
}

// TestApply_ConfigMapCreateFailure exercises the createImmutable error branch
// (a non-AlreadyExists error) for the pack content ConfigMap.
func TestApply_ConfigMapCreateFailure(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithInterceptorFuncs(interceptor.Funcs{
		Create: func(ctx context.Context, cli client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if _, ok := obj.(*corev1.ConfigMap); ok {
				return errBoom
			}
			return cli.Create(ctx, obj, opts...)
		},
	}).Build()
	a := NewApplier(c, logr.Discard())

	res := a.Apply(context.Background(), "ns", testIntent())
	if res.Succeeded {
		t.Fatalf("expected failure, got succeeded=true: %+v", res.Results)
	}
	byKind := map[string]ResourceResult{}
	for _, r := range res.Results {
		byKind[r.Kind] = r
	}
	cmResult, ok := byKind[kindConfigMap]
	if !ok || cmResult.Action != ActionFailed || cmResult.Error == "" {
		t.Fatalf("expected failed ConfigMap result, got %+v", byKind[kindConfigMap])
	}
	// Best-effort: PromptPack + AgentRuntime still attempted despite the failure.
	if byKind[kindPromptPack].Action != ActionCreated {
		t.Errorf("expected PromptPack still created, got %+v", byKind[kindPromptPack])
	}
	if byKind[kindAgentRuntime].Action != ActionCreated {
		t.Errorf("expected AgentRuntime still created, got %+v", byKind[kindAgentRuntime])
	}
}

// TestApply_AgentRuntimeGetFailure exercises the upsertAgentRuntime branch
// where Get fails with a non-NotFound error.
func TestApply_AgentRuntimeGetFailure(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, cli client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*omniav1alpha1.AgentRuntime); ok {
				return errBoom
			}
			return cli.Get(ctx, key, obj, opts...)
		},
	}).Build()
	a := NewApplier(c, logr.Discard())

	res := a.Apply(context.Background(), "ns", testIntent())
	if res.Succeeded {
		t.Fatalf("expected failure, got succeeded=true: %+v", res.Results)
	}
	for _, r := range res.Results {
		if r.Kind == kindAgentRuntime {
			if r.Action != ActionFailed || r.Error == "" {
				t.Fatalf("expected failed AgentRuntime result, got %+v", r)
			}
			return
		}
	}
	t.Fatal("no AgentRuntime result found")
}

// TestApply_AgentRuntimeCreateFailure exercises the upsertAgentRuntime branch
// where Get returns NotFound but the subsequent Create fails.
func TestApply_AgentRuntimeCreateFailure(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithInterceptorFuncs(interceptor.Funcs{
		Create: func(ctx context.Context, cli client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if _, ok := obj.(*omniav1alpha1.AgentRuntime); ok {
				return errBoom
			}
			return cli.Create(ctx, obj, opts...)
		},
	}).Build()
	a := NewApplier(c, logr.Discard())

	res := a.Apply(context.Background(), "ns", testIntent())
	if res.Succeeded {
		t.Fatalf("expected failure, got succeeded=true: %+v", res.Results)
	}
	for _, r := range res.Results {
		if r.Kind == kindAgentRuntime {
			if r.Action != ActionFailed || r.Error == "" {
				t.Fatalf("expected failed AgentRuntime result, got %+v", r)
			}
			return
		}
	}
	t.Fatal("no AgentRuntime result found")
}

// TestApply_TriggerModePreservesLivePin verifies that when the live agent is
// in version-trigger mode, a deploy does not advance the PromptPack pin or
// clobber an in-flight canary candidate, while still applying other fields.
func TestApply_TriggerModePreservesLivePin(t *testing.T) {
	// Live agent: trigger-mode, pinned to 1.0.0, mid-canary (candidate set).
	pinned := "1.0.0"
	live := &omniav1alpha1.AgentRuntime{}
	live.Name = "support"
	live.Namespace = "ns"
	live.Spec.PromptPackRef = omniav1alpha1.PromptPackRef{Name: "support", Version: &pinned}
	live.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Trigger:   &omniav1alpha1.RolloutTrigger{PromptPackChannel: "stable"},
		Candidate: &omniav1alpha1.CandidateOverrides{},
		Steps:     []omniav1alpha1.RolloutStep{{}},
	}
	live.Spec.Providers = []omniav1alpha1.NamedProviderRef{{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "old"}}}

	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(live).Build()
	a := NewApplier(c, logr.Discard())

	// Deploy a NEW version (1.1.0) with changed providers, trigger-mode. The
	// intent's rollout carries a step — the CRD requires
	// spec.rollout.steps to be non-empty (MinItems=1), so a valid intent must
	// supply at least one even though the desired steps get overwritten by
	// the preserved live rollout below.
	intent := DeployIntent{
		APIVersion: APIVersionV1,
		Pack:       PackIntent{Name: "support", Version: "1.1.0", Content: "{}"},
		Agents: []AgentIntent{{
			Name:      "support",
			Providers: []ProviderBind{{Name: "default", Ref: "new"}},
			Rollout: &RolloutIntent{
				Trigger: &RolloutTriggerIntent{PromptPackChannel: "stable"},
				Steps:   []RolloutStepIntent{{SetWeight: ptr.To(int32(25))}},
			},
		}},
	}
	res := a.Apply(context.Background(), "ns", intent)
	if !res.Succeeded {
		t.Fatalf("apply failed: %+v", res.Results)
	}

	got := &omniav1alpha1.AgentRuntime{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: "support", Namespace: "ns"}, got); err != nil {
		t.Fatal(err)
	}
	// Pin PRESERVED at 1.0.0 (not advanced to 1.1.0) — the controller canaries.
	if got.Spec.PromptPackRef.Version == nil || *got.Spec.PromptPackRef.Version != "1.0.0" {
		t.Errorf("pin = %v, want preserved 1.0.0", got.Spec.PromptPackRef.Version)
	}
	// In-flight candidate PRESERVED.
	if got.Spec.Rollout == nil || got.Spec.Rollout.Candidate == nil {
		t.Errorf("candidate clobbered: %+v", got.Spec.Rollout)
	}
	// Resulting object is CRD-valid: steps non-empty (MinItems=1).
	if len(got.Spec.Rollout.Steps) == 0 {
		t.Errorf("steps = %+v, want non-empty (CRD MinItems=1)", got.Spec.Rollout.Steps)
	}
	// Other config STILL applied.
	if len(got.Spec.Providers) != 1 || got.Spec.Providers[0].ProviderRef.Name != "new" {
		t.Errorf("providers not updated: %+v", got.Spec.Providers)
	}
}

// TestApply_TriggerModeConfigOnlyDeployPreservesRollout verifies that when the
// deploy intent for a trigger-mode agent omits the rollout block entirely
// (config-only deploy), the LIVE rollout is preserved wholesale — trigger,
// steps, AND candidate — not rebuilt with only Candidate set. Rebuilding with
// only Candidate would produce a RolloutConfig with zero Steps, which is
// invalid against the CRD (Steps has MinItems=1) and would cause a real
// apiserver Update to reject the whole AgentRuntime, silently dropping the
// provider/facade changes the deploy intended.
func TestApply_TriggerModeConfigOnlyDeployPreservesRollout(t *testing.T) {
	// Live agent: trigger-mode, pinned to 1.0.0, mid-canary, with a full
	// rollout (trigger + steps + candidate).
	pinned := "1.0.0"
	live := &omniav1alpha1.AgentRuntime{}
	live.Name = "support"
	live.Namespace = "ns"
	live.Spec.PromptPackRef = omniav1alpha1.PromptPackRef{Name: "support", Version: &pinned}
	live.Spec.Rollout = &omniav1alpha1.RolloutConfig{
		Trigger:   &omniav1alpha1.RolloutTrigger{PromptPackChannel: "stable"},
		Candidate: &omniav1alpha1.CandidateOverrides{},
		Steps:     []omniav1alpha1.RolloutStep{{SetWeight: ptr.To(int32(25))}},
	}
	live.Spec.Providers = []omniav1alpha1.NamedProviderRef{{Name: "default", ProviderRef: omniav1alpha1.ProviderRef{Name: "old"}}}

	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(live).Build()
	a := NewApplier(c, logr.Discard())

	// Deploy a NEW version (1.1.0) with changed providers, but NO rollout
	// block on the agent intent — a config-only deploy.
	intent := DeployIntent{
		APIVersion: APIVersionV1,
		Pack:       PackIntent{Name: "support", Version: "1.1.0", Content: "{}"},
		Agents: []AgentIntent{{
			Name:      "support",
			Providers: []ProviderBind{{Name: "default", Ref: "new"}},
		}},
	}
	res := a.Apply(context.Background(), "ns", intent)
	if !res.Succeeded {
		t.Fatalf("apply failed: %+v", res.Results)
	}

	got := &omniav1alpha1.AgentRuntime{}
	if err := c.Get(context.Background(), types.NamespacedName{Name: "support", Namespace: "ns"}, got); err != nil {
		t.Fatal(err)
	}
	// Pin PRESERVED at 1.0.0 (not advanced to 1.1.0).
	if got.Spec.PromptPackRef.Version == nil || *got.Spec.PromptPackRef.Version != "1.0.0" {
		t.Errorf("pin = %v, want preserved 1.0.0", got.Spec.PromptPackRef.Version)
	}
	// Whole live rollout PRESERVED — trigger, steps (CRD-required, MinItems=1),
	// and candidate all intact.
	if got.Spec.Rollout == nil {
		t.Fatal("rollout dropped entirely")
	}
	if got.Spec.Rollout.Trigger == nil || got.Spec.Rollout.Trigger.PromptPackChannel != "stable" {
		t.Errorf("trigger = %+v, want preserved stable channel", got.Spec.Rollout.Trigger)
	}
	if len(got.Spec.Rollout.Steps) < 1 {
		t.Errorf("steps = %+v, want at least 1 (CRD MinItems=1) — object would fail a real apiserver Update", got.Spec.Rollout.Steps)
	}
	if got.Spec.Rollout.Candidate == nil {
		t.Errorf("candidate clobbered: %+v", got.Spec.Rollout)
	}
	// Config change STILL applied.
	if len(got.Spec.Providers) != 1 || got.Spec.Providers[0].ProviderRef.Name != "new" {
		t.Errorf("providers not updated: %+v", got.Spec.Providers)
	}
}

// TestApply_PinnedModeHardSwaps verifies that when the live agent is NOT in
// trigger mode, a deploy hard-swaps the PromptPack pin to the new version.
func TestApply_PinnedModeHardSwaps(t *testing.T) {
	old := "1.0.0"
	live := &omniav1alpha1.AgentRuntime{}
	live.Name = "support"
	live.Namespace = "ns"
	live.Spec.PromptPackRef = omniav1alpha1.PromptPackRef{Name: "support", Version: &old}

	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(live).Build()
	a := NewApplier(c, logr.Discard())

	intent := testIntent()
	intent.Pack.Version = "2.0.0"
	res := a.Apply(context.Background(), "ns", intent)
	if !res.Succeeded {
		t.Fatalf("apply failed: %+v", res.Results)
	}
	got := &omniav1alpha1.AgentRuntime{}
	_ = c.Get(context.Background(), types.NamespacedName{Name: "support", Namespace: "ns"}, got)
	if got.Spec.PromptPackRef.Version == nil || *got.Spec.PromptPackRef.Version != "2.0.0" {
		t.Errorf("pinned mode should hard-swap to 2.0.0, got %v", got.Spec.PromptPackRef.Version)
	}
}

// TestApply_AgentRuntimeUpdateFailure exercises the upsertAgentRuntime branch
// where the AgentRuntime already exists but Update fails.
func TestApply_AgentRuntimeUpdateFailure(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithInterceptorFuncs(interceptor.Funcs{
		Update: func(ctx context.Context, cli client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if _, ok := obj.(*omniav1alpha1.AgentRuntime); ok {
				return errBoom
			}
			return cli.Update(ctx, obj, opts...)
		},
	}).Build()
	a := NewApplier(c, logr.Discard())

	// First apply creates the AgentRuntime (Update interceptor doesn't fire on Create).
	if res := a.Apply(context.Background(), "ns", testIntent()); !res.Succeeded {
		t.Fatalf("initial apply failed: %+v", res.Results)
	}

	res := a.Apply(context.Background(), "ns", testIntent())
	if res.Succeeded {
		t.Fatalf("expected failure, got succeeded=true: %+v", res.Results)
	}
	for _, r := range res.Results {
		if r.Kind == kindAgentRuntime {
			if r.Action != ActionFailed || r.Error == "" {
				t.Fatalf("expected failed AgentRuntime result, got %+v", r)
			}
			return
		}
	}
	t.Fatal("no AgentRuntime result found")
}
