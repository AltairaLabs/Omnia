package checks

import (
	"context"
	"fmt"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/doctor"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const categoryNameCRDs = "CRDs"

// CRDChecker reads Omnia CRDs for validation.
type CRDChecker struct {
	client client.Client
}

// NewCRDChecker creates a CRDChecker using the provided controller-runtime client.
func NewCRDChecker(c client.Client) *CRDChecker {
	return &CRDChecker{client: c}
}

// Checks returns the set of CRD validation checks.
func (c *CRDChecker) Checks() []doctor.Check {
	return []doctor.Check{
		{Name: "AgentRuntimesExist", Category: categoryNameCRDs, Run: c.checkAgentRuntimes},
		{Name: "PromptPacksCompiled", Category: categoryNameCRDs, Run: c.checkPromptPacks},
		{Name: "ToolRegistriesDiscovered", Category: categoryNameCRDs, Run: c.checkToolRegistries},
		{Name: "WorkspacesConfigured", Category: categoryNameCRDs, Run: c.checkWorkspaces},
	}
}

// crdListResult captures the common output from listing a CRD type.
type crdListResult struct {
	count  int
	phases map[string]int
}

// listCRDPhases is a generic helper that lists a CRD type, checks for an empty list,
// and collects phase counts. The getPhase function extracts the phase string from each item.
func listCRDPhases[T any](ctx context.Context, k8s client.Client, list client.ObjectList, items func() []T, getPhase func(T) string) (*crdListResult, error) {
	if err := k8s.List(ctx, list); err != nil {
		return nil, err
	}
	all := items()
	phases := make(map[string]int, len(all))
	for _, item := range all {
		p := getPhase(item)
		if p == "" {
			p = "Unknown"
		}
		phases[p]++
	}
	return &crdListResult{count: len(all), phases: phases}, nil
}

// checkCRDExists is a generic check: list CRD items, fail if empty, report phase counts.
func checkCRDExists[T any](ctx context.Context, k8s client.Client, typeName string, list client.ObjectList, items func() []T, getPhase func(T) string) doctor.TestResult {
	result, err := listCRDPhases(ctx, k8s, list, items, getPhase)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: fmt.Sprintf("list %s: %v", typeName, err)}
	}
	if result.count == 0 {
		return doctor.TestResult{Status: doctor.StatusFail, Detail: fmt.Sprintf("no %s found", typeName)}
	}
	return doctor.TestResult{
		Status: doctor.StatusPass,
		Detail: fmt.Sprintf("found %d %s: %s", result.count, typeName, formatPhaseCounts(result.phases)),
	}
}

func (c *CRDChecker) checkAgentRuntimes(ctx context.Context) doctor.TestResult {
	var list omniav1alpha1.AgentRuntimeList
	return checkCRDExists(ctx, c.client, "AgentRuntimes", &list,
		func() []omniav1alpha1.AgentRuntime { return list.Items },
		func(item omniav1alpha1.AgentRuntime) string { return string(item.Status.Phase) },
	)
}

func (c *CRDChecker) checkPromptPacks(ctx context.Context) doctor.TestResult {
	var list omniav1alpha1.PromptPackList
	return checkCRDExists(ctx, c.client, "PromptPacks", &list,
		func() []omniav1alpha1.PromptPack { return list.Items },
		func(item omniav1alpha1.PromptPack) string { return string(item.Status.Phase) },
	)
}

func (c *CRDChecker) checkToolRegistries(ctx context.Context) doctor.TestResult {
	var list omniav1alpha1.ToolRegistryList
	result, err := listCRDPhases(ctx, c.client, &list,
		func() []omniav1alpha1.ToolRegistry { return list.Items },
		func(item omniav1alpha1.ToolRegistry) string { return string(item.Status.Phase) },
	)
	if err != nil {
		return doctor.TestResult{Status: doctor.StatusFail, Error: fmt.Sprintf("list ToolRegistries: %v", err)}
	}
	if result.count == 0 {
		return doctor.TestResult{Status: doctor.StatusFail, Detail: "no ToolRegistries found"}
	}

	totalTools := int32(0)
	for _, item := range list.Items {
		totalTools += item.Status.DiscoveredToolsCount
	}

	return doctor.TestResult{
		Status: doctor.StatusPass,
		Detail: fmt.Sprintf("found %d ToolRegistries (%d tools discovered): %s",
			result.count, totalTools, formatPhaseCounts(result.phases)),
	}
}

func (c *CRDChecker) checkWorkspaces(ctx context.Context) doctor.TestResult {
	var list omniav1alpha1.WorkspaceList
	return checkCRDExists(ctx, c.client, "Workspaces", &list,
		func() []omniav1alpha1.Workspace { return list.Items },
		func(item omniav1alpha1.Workspace) string { return string(item.Status.Phase) },
	)
}

// formatPhaseCounts formats a phase→count map as "Running=2, Pending=1".
func formatPhaseCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return ""
	}
	result := ""
	for phase, count := range counts {
		if result != "" {
			result += ", "
		}
		result += fmt.Sprintf("%s=%d", phase, count)
	}
	return result
}
