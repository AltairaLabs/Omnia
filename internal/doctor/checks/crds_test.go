package checks

import (
	"context"
	"testing"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/doctor"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := omniav1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	return s
}

// TestCRD_AgentRuntimes_Exist verifies a pass result when AgentRuntimes are present.
func TestCRD_AgentRuntimes_Exist(t *testing.T) {
	objs := []runtime.Object{
		&omniav1alpha1.AgentRuntime{
			ObjectMeta: metav1.ObjectMeta{Name: "rt-1", Namespace: "default"},
			Status:     omniav1alpha1.AgentRuntimeStatus{Phase: omniav1alpha1.AgentRuntimePhaseRunning},
		},
		&omniav1alpha1.AgentRuntime{
			ObjectMeta: metav1.ObjectMeta{Name: "rt-2", Namespace: "default"},
			Status:     omniav1alpha1.AgentRuntimeStatus{Phase: omniav1alpha1.AgentRuntimePhasePending},
		},
		&omniav1alpha1.AgentRuntime{
			ObjectMeta: metav1.ObjectMeta{Name: "rt-3", Namespace: "default"},
			Status:     omniav1alpha1.AgentRuntimeStatus{Phase: omniav1alpha1.AgentRuntimePhaseFailed},
		},
	}
	checker := newCRDCheckerWithObjects(t, objs)
	result := checker.checkAgentRuntimes(context.Background())

	assertPass(t, result)
	assertDetailContains(t, result, "3 AgentRuntimes")
}

// TestCRD_AgentRuntimes_None verifies a fail result when no AgentRuntimes exist.
func TestCRD_AgentRuntimes_None(t *testing.T) {
	checker := newCRDCheckerWithObjects(t, nil)
	result := checker.checkAgentRuntimes(context.Background())

	assertFail(t, result)
	assertDetailContains(t, result, "no AgentRuntimes found")
}

// TestCRD_PromptPacks_Exist verifies a pass result when PromptPacks are present.
func TestCRD_PromptPacks_Exist(t *testing.T) {
	objs := []runtime.Object{
		&omniav1alpha1.PromptPack{
			ObjectMeta: metav1.ObjectMeta{Name: "pp-1", Namespace: "default"},
			Spec: omniav1alpha1.PromptPackSpec{
				Source:  omniav1alpha1.PromptPackSource{Type: omniav1alpha1.PromptPackSourceTypeConfigMap},
				Version: "1.0.0",
				Rollout: omniav1alpha1.RolloutStrategy{Type: omniav1alpha1.RolloutStrategyImmediate},
			},
			Status: omniav1alpha1.PromptPackStatus{Phase: omniav1alpha1.PromptPackPhaseActive},
		},
		&omniav1alpha1.PromptPack{
			ObjectMeta: metav1.ObjectMeta{Name: "pp-2", Namespace: "default"},
			Spec: omniav1alpha1.PromptPackSpec{
				Source:  omniav1alpha1.PromptPackSource{Type: omniav1alpha1.PromptPackSourceTypeConfigMap},
				Version: "2.0.0",
				Rollout: omniav1alpha1.RolloutStrategy{Type: omniav1alpha1.RolloutStrategyImmediate},
			},
			Status: omniav1alpha1.PromptPackStatus{Phase: omniav1alpha1.PromptPackPhasePending},
		},
	}
	checker := newCRDCheckerWithObjects(t, objs)
	result := checker.checkPromptPacks(context.Background())

	assertPass(t, result)
	assertDetailContains(t, result, "2 PromptPacks")
}

// TestCRD_PromptPacks_None verifies a fail result when no PromptPacks exist.
func TestCRD_PromptPacks_None(t *testing.T) {
	checker := newCRDCheckerWithObjects(t, nil)
	result := checker.checkPromptPacks(context.Background())

	assertFail(t, result)
	assertDetailContains(t, result, "no PromptPacks found")
}

// TestCRD_ToolRegistries_Exist verifies a pass result when ToolRegistries are present.
func TestCRD_ToolRegistries_Exist(t *testing.T) {
	objs := []runtime.Object{
		&omniav1alpha1.ToolRegistry{
			ObjectMeta: metav1.ObjectMeta{Name: "tr-1", Namespace: "default"},
			Spec: omniav1alpha1.ToolRegistrySpec{
				Handlers: []omniav1alpha1.HandlerDefinition{
					{Name: "h1", Type: omniav1alpha1.HandlerTypeHTTP},
				},
			},
			Status: omniav1alpha1.ToolRegistryStatus{
				Phase:                omniav1alpha1.ToolRegistryPhaseReady,
				DiscoveredToolsCount: 5,
			},
		},
		&omniav1alpha1.ToolRegistry{
			ObjectMeta: metav1.ObjectMeta{Name: "tr-2", Namespace: "default"},
			Spec: omniav1alpha1.ToolRegistrySpec{
				Handlers: []omniav1alpha1.HandlerDefinition{
					{Name: "h2", Type: omniav1alpha1.HandlerTypeMCP},
				},
			},
			Status: omniav1alpha1.ToolRegistryStatus{
				Phase:                omniav1alpha1.ToolRegistryPhaseDegraded,
				DiscoveredToolsCount: 2,
			},
		},
	}
	checker := newCRDCheckerWithObjects(t, objs)
	result := checker.checkToolRegistries(context.Background())

	assertPass(t, result)
	assertDetailContains(t, result, "2 ToolRegistries")
	assertDetailContains(t, result, "7 tools discovered")
}

// TestCRD_ToolRegistries_None verifies a fail result when no ToolRegistries exist.
func TestCRD_ToolRegistries_None(t *testing.T) {
	checker := newCRDCheckerWithObjects(t, nil)
	result := checker.checkToolRegistries(context.Background())

	assertFail(t, result)
	assertDetailContains(t, result, "no ToolRegistries found")
}

// TestCRD_Workspaces_Exist verifies a pass result when Workspaces are present.
func TestCRD_Workspaces_Exist(t *testing.T) {
	objs := []runtime.Object{
		&omniav1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "ws-1"},
			Spec: omniav1alpha1.WorkspaceSpec{
				DisplayName: "Workspace 1",
				Namespace:   omniav1alpha1.NamespaceConfig{Name: "ws-ns-1"},
			},
			Status: omniav1alpha1.WorkspaceStatus{Phase: omniav1alpha1.WorkspacePhaseReady},
		},
		&omniav1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "ws-2"},
			Spec: omniav1alpha1.WorkspaceSpec{
				DisplayName: "Workspace 2",
				Namespace:   omniav1alpha1.NamespaceConfig{Name: "ws-ns-2"},
			},
			Status: omniav1alpha1.WorkspaceStatus{Phase: omniav1alpha1.WorkspacePhasePending},
		},
	}
	checker := newCRDCheckerWithObjects(t, objs)
	result := checker.checkWorkspaces(context.Background())

	assertPass(t, result)
	assertDetailContains(t, result, "2 Workspaces")
}

// TestCRD_Workspaces_None verifies a fail result when no Workspaces exist.
func TestCRD_Workspaces_None(t *testing.T) {
	checker := newCRDCheckerWithObjects(t, nil)
	result := checker.checkWorkspaces(context.Background())

	assertFail(t, result)
	assertDetailContains(t, result, "no Workspaces found")
}

// TestCRD_MemoryEnabled_Pass verifies pass when at least one AgentRuntime has memory enabled.
func TestCRD_MemoryEnabled_Pass(t *testing.T) {
	objs := []runtime.Object{
		&omniav1alpha1.AgentRuntime{
			ObjectMeta: metav1.ObjectMeta{Name: "rt-1", Namespace: "default"},
			Spec: omniav1alpha1.AgentRuntimeSpec{
				Memory: &omniav1alpha1.MemoryConfig{Enabled: true},
			},
		},
		&omniav1alpha1.AgentRuntime{
			ObjectMeta: metav1.ObjectMeta{Name: "rt-2", Namespace: "default"},
		},
	}
	checker := newCRDCheckerWithObjects(t, objs)
	result := checker.checkMemoryEnabled(context.Background())

	assertPass(t, result)
	assertDetailContains(t, result, "1/2")
	assertDetailContains(t, result, "rt-1")
}

// TestCRD_MemoryEnabled_Fail verifies fail when no AgentRuntimes have memory enabled.
func TestCRD_MemoryEnabled_Fail(t *testing.T) {
	objs := []runtime.Object{
		&omniav1alpha1.AgentRuntime{
			ObjectMeta: metav1.ObjectMeta{Name: "rt-1", Namespace: "default"},
		},
	}
	checker := newCRDCheckerWithObjects(t, objs)
	result := checker.checkMemoryEnabled(context.Background())

	assertFail(t, result)
	assertDetailContains(t, result, "none of 1")
}

// TestCRD_MemoryEnabled_Skip verifies skip when no AgentRuntimes exist at all.
func TestCRD_MemoryEnabled_Skip(t *testing.T) {
	checker := newCRDCheckerWithObjects(t, nil)
	result := checker.checkMemoryEnabled(context.Background())

	if result.Status != doctor.StatusSkip {
		t.Errorf("expected StatusSkip, got %s (detail=%q)", result.Status, result.Detail)
	}
	assertDetailContains(t, result, "no AgentRuntimes")
}

// TestCRD_MemoryEnabled_Fail_DisabledExplicitly verifies fail when Memory is set but Enabled is false.
func TestCRD_MemoryEnabled_Fail_DisabledExplicitly(t *testing.T) {
	objs := []runtime.Object{
		&omniav1alpha1.AgentRuntime{
			ObjectMeta: metav1.ObjectMeta{Name: "rt-1", Namespace: "default"},
			Spec: omniav1alpha1.AgentRuntimeSpec{
				Memory: &omniav1alpha1.MemoryConfig{Enabled: false},
			},
		},
	}
	checker := newCRDCheckerWithObjects(t, objs)
	result := checker.checkMemoryEnabled(context.Background())

	assertFail(t, result)
	assertDetailContains(t, result, "none of 1")
}

// TestCRD_Checks_Count verifies Checks() returns the expected number of checks.
func TestCRD_Checks_Count(t *testing.T) {
	checker := newCRDCheckerWithObjects(t, nil)
	checks := checker.Checks()
	if len(checks) != 5 {
		t.Errorf("expected 5 checks, got %d", len(checks))
	}
	for _, ch := range checks {
		if ch.Category != categoryNameCRDs {
			t.Errorf("expected category %q, got %q for check %q", categoryNameCRDs, ch.Category, ch.Name)
		}
	}
}

// --- helpers ---

func newCRDCheckerWithObjects(t *testing.T, objs []runtime.Object) *CRDChecker {
	t.Helper()
	s := newTestScheme(t)
	builder := fake.NewClientBuilder().WithScheme(s)
	if len(objs) > 0 {
		builder = builder.WithRuntimeObjects(objs...)
	}
	return NewCRDChecker(builder.Build())
}

func assertPass(t *testing.T, result doctor.TestResult) {
	t.Helper()
	if result.Status != doctor.StatusPass {
		t.Errorf("expected StatusPass, got %s (detail=%q, error=%q)", result.Status, result.Detail, result.Error)
	}
}

func assertFail(t *testing.T, result doctor.TestResult) {
	t.Helper()
	if result.Status != doctor.StatusFail {
		t.Errorf("expected StatusFail, got %s (detail=%q)", result.Status, result.Detail)
	}
}

func assertDetailContains(t *testing.T, result doctor.TestResult, substr string) {
	t.Helper()
	if result.Detail == "" && result.Error == "" {
		t.Errorf("expected detail/error to contain %q, but both are empty", substr)
		return
	}
	combined := result.Detail + result.Error
	if !contains(combined, substr) {
		t.Errorf("expected detail/error to contain %q, got detail=%q error=%q", substr, result.Detail, result.Error)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		indexStr(s, substr) >= 0)
}

func indexStr(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
