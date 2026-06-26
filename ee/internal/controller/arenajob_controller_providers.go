/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/arena/partitioner"
	"github.com/altairalabs/omnia/ee/pkg/arena/providers"
	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
)

// resolveProviderGroups resolves the new spec.Providers field.
// For each entry, it fetches the Provider CRD (providerRef) or validates the AgentRuntime (agentRef).
// Returns resolved groups and a flat list of all Provider CRDs (for env var injection).
func (r *ArenaJobReconciler) resolveProviderGroups(
	ctx context.Context, arenaJob *omniav1alpha1.ArenaJob,
) (map[string]*resolvedProviderGroup, []*corev1alpha1.Provider, error) {
	log := logf.FromContext(ctx)

	groups := make(map[string]*resolvedProviderGroup, len(arenaJob.Spec.Providers))
	var allProviders []*corev1alpha1.Provider
	seen := make(map[string]bool)

	for groupName, pg := range arenaJob.Spec.Providers {
		grp, err := r.resolveProviderGroup(ctx, arenaJob.Namespace, pg, seen, &allProviders)
		if err != nil {
			return nil, nil, fmt.Errorf("group %q: %w", groupName, err)
		}

		groups[groupName] = grp
		log.V(1).Info("resolved provider group",
			"group", groupName,
			"providers", len(grp.providers),
			"agents", len(grp.agentWSURLs),
		)
	}

	return groups, allProviders, nil
}

// resolveProviderGroup resolves a single provider group's entries. It appends
// newly-seen Provider CRDs to allProviders (deduped via seen) so the caller can
// build the flat provider list across all groups.
func (r *ArenaJobReconciler) resolveProviderGroup(
	ctx context.Context, namespace string, pg omniav1alpha1.ArenaProviderGroup,
	seen map[string]bool, allProviders *[]*corev1alpha1.Provider,
) (*resolvedProviderGroup, error) {
	grp := &resolvedProviderGroup{
		agentWSURLs: make(map[string]string),
		mapMode:     pg.IsMapMode(),
	}

	for _, entry := range pg.AllEntries() {
		switch {
		case entry.ProviderRef != nil:
			provider, err := r.resolveProviderEntry(ctx, namespace, *entry.ProviderRef)
			if err != nil {
				return nil, err
			}
			grp.providers = append(grp.providers, provider)
			key := provider.Namespace + "/" + provider.Name
			if !seen[key] {
				seen[key] = true
				*allProviders = append(*allProviders, provider)
			}
		case entry.AgentRef != nil:
			wsURL, err := r.resolveAgentEntry(ctx, namespace, entry.AgentRef.Name)
			if err != nil {
				return nil, err
			}
			grp.agentWSURLs[entry.AgentRef.Name] = wsURL
		}
	}

	return grp, nil
}

// resolveProviderEntry fetches a Provider CRD from a ProviderRef.
func (r *ArenaJobReconciler) resolveProviderEntry(
	ctx context.Context, defaultNamespace string, ref corev1alpha1.ProviderRef,
) (*corev1alpha1.Provider, error) {
	ns := defaultNamespace
	if ref.Namespace != nil {
		ns = *ref.Namespace
	}

	provider := &corev1alpha1.Provider{}
	if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ns}, provider); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("provider %s/%s not found", ns, ref.Name)
		}
		return nil, fmt.Errorf("failed to get provider %s/%s: %w", ns, ref.Name, err)
	}
	return provider, nil
}

// resolveAgentEntry validates an AgentRuntime exists and is ready, returning its WebSocket URL.
func (r *ArenaJobReconciler) resolveAgentEntry(
	ctx context.Context, namespace, name string,
) (string, error) {
	agentRuntime := &corev1alpha1.AgentRuntime{}
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, agentRuntime); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("agentRuntime %s/%s not found", namespace, name)
		}
		return "", fmt.Errorf("failed to get agentRuntime %s/%s: %w", namespace, name, err)
	}

	if agentRuntime.Status.ServiceEndpoint == "" {
		return "", fmt.Errorf("agentRuntime %s/%s has no service endpoint (not ready)", namespace, name)
	}

	return fmt.Sprintf("ws://%s/ws?agent=%s&namespace=%s",
		agentRuntime.Status.ServiceEndpoint, name, namespace), nil
}

// buildProviderGroupEnvVars builds environment variables that encode the provider groups
// for the worker to read. The worker uses ARENA_JOB_NAME and ARENA_JOB_NAMESPACE
// to read the ArenaJob CRD directly and resolve providers itself.
// This method only adds agent WebSocket URLs as env vars since the worker needs them
// at startup before it can connect to the K8s API.
func buildProviderGroupEnvVars(groups map[string]*resolvedProviderGroup) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	// Encode agent WebSocket URLs as JSON for the worker
	agentURLs := make(map[string]string)
	for _, grp := range groups {
		for name, url := range grp.agentWSURLs {
			agentURLs[name] = url
		}
	}

	if len(agentURLs) > 0 {
		urlsJSON, err := json.Marshal(agentURLs)
		if err == nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "ARENA_AGENT_WS_URLS",
				Value: string(urlsJSON),
			})
		}
	}

	return envVars
}

// getProviderIDsFromGroups extracts provider IDs from resolved provider groups.
// Only array-mode groups participate in the scenario × provider work item matrix.
// Map-mode groups (judges, self-play) are 1:1 config references and are excluded.
// Provider CRDs use their CRD name; agent entries use "agent-{name}".
func getProviderIDsFromGroups(groups map[string]*resolvedProviderGroup) []string {
	seen := make(map[string]bool)
	var ids []string

	for _, grp := range groups {
		if grp.mapMode {
			continue
		}
		for _, p := range grp.providers {
			if !seen[p.Name] {
				seen[p.Name] = true
				ids = append(ids, p.Name)
			}
		}
		for name := range grp.agentWSURLs {
			id := "agent-" + name
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}

	return ids
}

// buildProviderEnvVarsFromCRDs builds environment variables for Provider CRDs.
// This extracts credentials from each provider's secretRef.
// Delegates to the shared providers.BuildEnvVarsFromProviders function.
func (r *ArenaJobReconciler) buildProviderEnvVarsFromCRDs(providerCRDs []*corev1alpha1.Provider) []corev1.EnvVar {
	return providers.BuildEnvVarsFromProviders(providerCRDs)
}

// getOrCreateQueue returns the work queue, creating it lazily if needed.
func (r *ArenaJobReconciler) getOrCreateQueue() (queue.WorkQueue, error) {
	// Return existing queue if already connected
	if r.Queue != nil {
		return r.Queue, nil
	}

	// No Redis configured
	if r.RedisURL == "" {
		return nil, nil
	}

	// Try to connect lazily
	q, err := queue.NewRedisQueue(queue.RedisOptions{
		URL: r.RedisURL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	r.Queue = q
	return q, nil
}

// getContentBasePath computes the filesystem base path for workspace content.
// Returns empty string if filesystem access is not configured.
func (r *ArenaJobReconciler) getContentBasePath(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob, source *omniav1alpha1.ArenaSource) string {
	if r.WorkspaceContentPath == "" {
		return ""
	}
	if source.Status.Artifact == nil || source.Status.Artifact.ContentPath == "" {
		return ""
	}

	workspaceName := GetWorkspaceForNamespace(ctx, r.Client, arenaJob.Namespace)

	// Extract root path from arenaFile
	var rootPath string
	if arenaJob.Spec.ArenaFile != "" {
		rootPath = filepath.Dir(arenaJob.Spec.ArenaFile)
		if rootPath == "." {
			rootPath = ""
		}
	}

	// Structure: {WorkspaceContentPath}/{workspace}/{namespace}/{contentPath}/{rootPath}
	basePath := filepath.Join(r.WorkspaceContentPath, workspaceName, arenaJob.Namespace, source.Status.Artifact.ContentPath)
	if rootPath != "" {
		basePath = filepath.Join(basePath, rootPath)
	}
	return basePath
}

// listScenarios attempts to enumerate scenarios from the arena config file on the filesystem.
// Returns nil if filesystem access is unavailable or scenarios cannot be enumerated.
func (r *ArenaJobReconciler) listScenarios(ctx context.Context, arenaJob *omniav1alpha1.ArenaJob, source *omniav1alpha1.ArenaSource) ([]partitioner.Scenario, error) {
	basePath := r.getContentBasePath(ctx, arenaJob, source)
	if basePath == "" {
		return nil, nil
	}

	arenaFile := arenaJob.Spec.ArenaFile
	if arenaFile == "" {
		arenaFile = defaultArenaConfigFile
	}
	// Use just the filename since basePath already includes rootPath
	arenaFileName := filepath.Base(arenaFile)
	fullPath := filepath.Join(basePath, arenaFileName)

	scenarios, err := partitioner.ListScenariosFromConfig(fullPath)
	if err != nil {
		return nil, err
	}

	return scenarios, nil
}

// buildMatrixWorkItems creates scenario × provider × trial work items using the partitioner.
// providerIDs are the array-mode provider IDs (from both providerRef and agentRef entries).
// Returns nil if partitioning fails or inputs are empty.
func (r *ArenaJobReconciler) buildMatrixWorkItems(
	ctx context.Context, jobName, bundleURL string,
	scenarios []partitioner.Scenario,
	providerIDs []string,
	jobTrials int, jobType omniav1alpha1.ArenaJobType,
) []queue.WorkItem {
	log := logf.FromContext(ctx)

	partProviders := make([]partitioner.Provider, len(providerIDs))
	for i, id := range providerIDs {
		partProviders[i] = partitioner.Provider{
			ID:   id,
			Name: id,
		}
	}

	result, err := partitioner.Partition(partitioner.PartitionInput{
		JobID:      jobName,
		BundleURL:  bundleURL,
		Scenarios:  scenarios,
		Providers:  partProviders,
		MaxRetries: 3,
		JobTrials:  jobTrials,
	})
	if err != nil {
		log.Error(err, "partitioning failed, falling back to per-provider mode")
		return nil
	}

	limit := maxWorkItemsForJobType(jobType)
	if result.TotalCombinations > limit {
		log.Error(nil, "work item matrix exceeds limit, falling back to per-provider mode",
			"totalCombinations", result.TotalCombinations,
			"maxWorkItems", limit)
		return nil
	}

	now := time.Now()
	for i := range result.Items {
		result.Items[i].Status = queue.ItemStatusPending
		result.Items[i].Attempt = 1
		result.Items[i].CreatedAt = now
	}
	log.Info("created scenario × provider × trial work items",
		"scenarios", result.ScenarioCount,
		"providers", result.ProviderCount,
		"trials", result.TrialCount,
		"items", result.TotalCombinations)
	return result.Items
}

// maxWorkItemsForJobType returns the work item limit for a given job type.
func maxWorkItemsForJobType(jobType omniav1alpha1.ArenaJobType) int {
	if jobType == omniav1alpha1.ArenaJobTypeLoadTest {
		return maxWorkItemsLoadTest
	}
	return maxWorkItemsEvaluation
}

// buildFallbackWorkItems creates per-provider work items (or a single default item).
// Used when scenario enumeration is not available.
func buildFallbackWorkItems(jobName, bundleURL string, providerIDs []string) []queue.WorkItem {
	now := time.Now()
	if len(providerIDs) == 0 {
		return []queue.WorkItem{{
			ID:          fmt.Sprintf("%s-default-0", jobName),
			JobID:       jobName,
			ScenarioID:  defaultName,
			ProviderID:  "",
			BundleURL:   bundleURL,
			Status:      queue.ItemStatusPending,
			Attempt:     1,
			MaxAttempts: 3,
			CreatedAt:   now,
		}}
	}

	items := make([]queue.WorkItem, 0, len(providerIDs))
	for i, provider := range providerIDs {
		items = append(items, queue.WorkItem{
			ID:          fmt.Sprintf("%s-%s-%d", jobName, provider, i),
			JobID:       jobName,
			ScenarioID:  defaultName,
			ProviderID:  provider,
			BundleURL:   bundleURL,
			Status:      queue.ItemStatusPending,
			Attempt:     1,
			MaxAttempts: 3,
			CreatedAt:   now,
		})
	}
	return items
}

// enqueueWorkItems creates and enqueues work items for the Arena job.
// When scenarios can be enumerated from the filesystem, work items are created
// for each scenario × provider combination for maximum parallelism.
// Falls back to per-provider items (with ScenarioID "default") when filesystem
// access is unavailable.
// Returns the number of work items enqueued and any error.
func (r *ArenaJobReconciler) enqueueWorkItems(
	ctx context.Context,
	arenaJob *omniav1alpha1.ArenaJob,
	source *omniav1alpha1.ArenaSource,
	providerCRDs []*corev1alpha1.Provider,
	resolvedGroups map[string]*resolvedProviderGroup,
) (int, error) {
	log := logf.FromContext(ctx)

	// Get queue (lazily connect if needed)
	q, err := r.getOrCreateQueue()
	if err != nil {
		return 0, err
	}
	if q == nil {
		log.Info("no work queue configured, skipping work item enqueueing")
		return 0, nil
	}

	// Get bundle URL from source artifact
	bundleURL := ""
	if source.Status.Artifact != nil {
		bundleURL = source.Status.Artifact.URL
	}

	scenarios := r.enumerateScenarios(ctx, arenaJob, source)

	// Build work items: unified scenario × provider matrix
	matrixProviderIDs := r.matrixProviderIDs(resolvedGroups, providerCRDs)
	log.V(1).Info("building work items", "matrixProviderIDs", matrixProviderIDs)

	// Resolve job-level trials override (nil pointer = 0 = use per-scenario defaults)
	jobTrials := 0
	if arenaJob.Spec.Trials != nil {
		jobTrials = int(*arenaJob.Spec.Trials)
	}

	var items []queue.WorkItem
	if len(scenarios) > 0 && len(matrixProviderIDs) > 0 {
		items = r.buildMatrixWorkItems(ctx, arenaJob.Name, bundleURL, scenarios, matrixProviderIDs, jobTrials, arenaJob.Spec.Type)
	}
	if len(items) == 0 {
		if jobTrials > 0 {
			log.Info("trial configuration ignored in fallback mode", "trials", jobTrials)
		}
		items = buildFallbackWorkItems(arenaJob.Name, bundleURL, matrixProviderIDs)
	}

	log.Info("enqueueing work items", "count", len(items))
	if err := q.Push(ctx, arenaJob.Name, items); err != nil {
		return 0, fmt.Errorf("failed to push work items to queue: %w", err)
	}

	return len(items), nil
}

// enumerateScenarios attempts to enumerate scenarios from the filesystem and
// applies the job's include/exclude filters. Returns nil when enumeration is
// unavailable (falls back to per-provider mode).
func (r *ArenaJobReconciler) enumerateScenarios(
	ctx context.Context, arenaJob *omniav1alpha1.ArenaJob, source *omniav1alpha1.ArenaSource,
) []partitioner.Scenario {
	log := logf.FromContext(ctx)

	// Attempt scenario enumeration from the filesystem
	scenarios, scenarioErr := r.listScenarios(ctx, arenaJob, source)
	if scenarioErr != nil {
		log.V(1).Info("could not enumerate scenarios, falling back to per-provider mode", "error", scenarioErr)
	}

	// Apply scenario filters if scenarios were enumerated
	if len(scenarios) > 0 && arenaJob.Spec.Scenarios != nil {
		filtered, filterErr := partitioner.Filter(scenarios, arenaJob.Spec.Scenarios.Include, arenaJob.Spec.Scenarios.Exclude)
		if filterErr != nil {
			log.Error(filterErr, "failed to filter scenarios, using unfiltered list")
		} else {
			scenarios = filtered
		}
	}

	return scenarios
}

// matrixProviderIDs returns the array-mode provider IDs that participate in the
// scenario × provider work-item matrix.
//
// getProviderIDsFromGroups returns array-mode provider IDs only (both providerRef
// and agentRef). Map-mode groups (judges, self-play) don't participate in the matrix.
// When resolvedGroups is nil (no CRD-based providers), fall back to providerCRD names.
func (r *ArenaJobReconciler) matrixProviderIDs(
	resolvedGroups map[string]*resolvedProviderGroup, providerCRDs []*corev1alpha1.Provider,
) []string {
	ids := getProviderIDsFromGroups(resolvedGroups)
	if len(ids) == 0 && len(providerCRDs) > 0 {
		for _, p := range providerCRDs {
			ids = append(ids, p.Name)
		}
	}
	return ids
}
