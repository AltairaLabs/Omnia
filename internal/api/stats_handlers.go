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

package api

import (
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// Stats represents aggregated statistics.
type Stats struct {
	Agents struct {
		Total   int `json:"total"`
		Running int `json:"running"`
		Pending int `json:"pending"`
		Failed  int `json:"failed"`
	} `json:"agents"`
	PromptPacks struct {
		Total  int `json:"total"`
		Active int `json:"active"`
		Canary int `json:"canary"`
	} `json:"promptPacks"`
	Tools struct {
		Total     int `json:"total"`
		Available int `json:"available"`
		Degraded  int `json:"degraded"`
	} `json:"tools"`
}

// handleStats returns aggregated statistics.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
		return
	}

	ctx := r.Context()
	stats := Stats{}

	// Count agents (best-effort: ignore errors)
	var agents omniav1alpha1.AgentRuntimeList
	if s.client.List(ctx, &agents) == nil {
		stats.Agents.Total = len(agents.Items)
		for _, a := range agents.Items {
			switch a.Status.Phase {
			case "Running":
				stats.Agents.Running++
			case "Pending":
				stats.Agents.Pending++
			case "Failed":
				stats.Agents.Failed++
			}
		}
	}

	// Count promptpacks (best-effort: ignore errors)
	var packs omniav1alpha1.PromptPackList
	if s.client.List(ctx, &packs) == nil {
		stats.PromptPacks.Total = len(packs.Items)
		for _, p := range packs.Items {
			switch p.Status.Phase {
			case "Active":
				stats.PromptPacks.Active++
			case "Canary":
				stats.PromptPacks.Canary++
			}
		}
	}

	// Count tools (best-effort: ignore errors)
	var registries omniav1alpha1.ToolRegistryList
	if s.client.List(ctx, &registries) == nil {
		for _, reg := range registries.Items {
			stats.Tools.Total += int(reg.Status.DiscoveredToolsCount)
			for _, t := range reg.Status.DiscoveredTools {
				if t.Status == "Available" {
					stats.Tools.Available++
				} else {
					stats.Tools.Degraded++
				}
			}
		}
	}

	s.writeJSON(w, http.StatusOK, stats)
}

// handleNamespaces returns a list of namespaces in the cluster.
func (s *Server) handleNamespaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
		return
	}

	if s.clientset == nil {
		s.writeError(w, http.StatusInternalServerError, "namespaces endpoint not available")
		return
	}

	namespaces, err := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		s.log.Error(err, "failed to list namespaces")
		s.writeError(w, http.StatusInternalServerError, "failed to list namespaces")
		return
	}

	// Extract just the names
	names := make([]string, 0, len(namespaces.Items))
	for _, ns := range namespaces.Items {
		names = append(names, ns.Name)
	}

	s.writeJSON(w, http.StatusOK, names)
}
