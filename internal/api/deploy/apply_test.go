package deploy

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/go-logr/logr"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

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

	res := a.Apply(context.Background(), "ns", testIntent())
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

	// Re-apply: immutable pack objects are unchanged; agent is updated.
	res2 := a.Apply(context.Background(), "ns", testIntent())
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
