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
		{Name: "MemoryEnabled", Category: categoryNameCRDs, Run: c.checkMemoryEnabled},
	}
}

// countPhases builds a map of phase string → count from any slice of strings.
func countPhases(phases []string) map[string]int {
	counts := make(map[string]int, len(phases))
	for _, p := range phases {
		if p == "" {
			p = "Unknown"
		}
		counts[p]++
	}
	return counts
}

func (c *CRDChecker) checkAgentRuntimes(ctx context.Context) doctor.TestResult {
	var list omniav1alpha1.AgentRuntimeList
	if err := c.client.List(ctx, &list); err != nil {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Error:  fmt.Sprintf("list AgentRuntimes: %v", err),
		}
	}
	if len(list.Items) == 0 {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: "no AgentRuntimes found",
		}
	}

	phases := make([]string, len(list.Items))
	for i, item := range list.Items {
		phases[i] = string(item.Status.Phase)
	}
	counts := countPhases(phases)

	return doctor.TestResult{
		Status: doctor.StatusPass,
		Detail: fmt.Sprintf("found %d AgentRuntimes: %s", len(list.Items), formatPhaseCounts(counts)),
	}
}

func (c *CRDChecker) checkPromptPacks(ctx context.Context) doctor.TestResult {
	var list omniav1alpha1.PromptPackList
	if err := c.client.List(ctx, &list); err != nil {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Error:  fmt.Sprintf("list PromptPacks: %v", err),
		}
	}
	if len(list.Items) == 0 {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: "no PromptPacks found",
		}
	}

	phases := make([]string, len(list.Items))
	for i, item := range list.Items {
		phases[i] = string(item.Status.Phase)
	}
	counts := countPhases(phases)

	return doctor.TestResult{
		Status: doctor.StatusPass,
		Detail: fmt.Sprintf("found %d PromptPacks: %s", len(list.Items), formatPhaseCounts(counts)),
	}
}

func (c *CRDChecker) checkToolRegistries(ctx context.Context) doctor.TestResult {
	var list omniav1alpha1.ToolRegistryList
	if err := c.client.List(ctx, &list); err != nil {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Error:  fmt.Sprintf("list ToolRegistries: %v", err),
		}
	}
	if len(list.Items) == 0 {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: "no ToolRegistries found",
		}
	}

	phases := make([]string, len(list.Items))
	totalTools := int32(0)
	for i, item := range list.Items {
		phases[i] = string(item.Status.Phase)
		totalTools += item.Status.DiscoveredToolsCount
	}
	counts := countPhases(phases)

	return doctor.TestResult{
		Status: doctor.StatusPass,
		Detail: fmt.Sprintf("found %d ToolRegistries (%d tools discovered): %s",
			len(list.Items), totalTools, formatPhaseCounts(counts)),
	}
}

func (c *CRDChecker) checkWorkspaces(ctx context.Context) doctor.TestResult {
	var list omniav1alpha1.WorkspaceList
	if err := c.client.List(ctx, &list); err != nil {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Error:  fmt.Sprintf("list Workspaces: %v", err),
		}
	}
	if len(list.Items) == 0 {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Detail: "no Workspaces found",
		}
	}

	phases := make([]string, len(list.Items))
	for i, item := range list.Items {
		phases[i] = string(item.Status.Phase)
	}
	counts := countPhases(phases)

	return doctor.TestResult{
		Status: doctor.StatusPass,
		Detail: fmt.Sprintf("found %d Workspaces: %s", len(list.Items), formatPhaseCounts(counts)),
	}
}

func (c *CRDChecker) checkMemoryEnabled(ctx context.Context) doctor.TestResult {
	var list omniav1alpha1.AgentRuntimeList
	if err := c.client.List(ctx, &list); err != nil {
		return doctor.TestResult{
			Status: doctor.StatusFail,
			Error:  fmt.Sprintf("list AgentRuntimes: %v", err),
		}
	}

	memoryCount := 0
	for _, item := range list.Items {
		if item.Spec.Memory != nil && item.Spec.Memory.Enabled {
			memoryCount++
		}
	}

	return doctor.TestResult{
		Status: doctor.StatusPass,
		Detail: fmt.Sprintf("%d of %d AgentRuntimes have memory enabled", memoryCount, len(list.Items)),
	}
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
