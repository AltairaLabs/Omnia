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
	"encoding/json"
	"net/http"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// handleAgents lists all AgentRuntimes or creates a new one.
func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listAgents(w, r)
	case http.MethodPost:
		s.createAgent(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
	}
}

// listAgents lists all AgentRuntimes.
func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	namespace := r.URL.Query().Get("namespace")

	var agents omniav1alpha1.AgentRuntimeList
	var err error

	if namespace != "" {
		err = s.client.List(r.Context(), &agents, client.InNamespace(namespace))
	} else {
		err = s.client.List(r.Context(), &agents)
	}

	if err != nil {
		s.log.Error(err, "failed to list agents")
		s.writeError(w, http.StatusInternalServerError, "failed to list agents")
		return
	}

	s.writeJSON(w, http.StatusOK, agents.Items)
}

// createAgent creates a new AgentRuntime.
func (s *Server) createAgent(w http.ResponseWriter, r *http.Request) {
	var agent omniav1alpha1.AgentRuntime
	if err := json.NewDecoder(r.Body).Decode(&agent); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Validate required fields
	if agent.Name == "" {
		s.writeError(w, http.StatusBadRequest, "metadata.name is required")
		return
	}
	if agent.Namespace == "" {
		agent.Namespace = "default"
	}
	if agent.Spec.PromptPackRef.Name == "" {
		s.writeError(w, http.StatusBadRequest, "spec.promptPackRef.name is required")
		return
	}

	// Set API version and kind if not provided
	if agent.APIVersion == "" {
		agent.APIVersion = "omnia.altairalabs.ai/v1alpha1"
	}
	if agent.Kind == "" {
		agent.Kind = "AgentRuntime"
	}

	// Create the agent
	if err := s.client.Create(r.Context(), &agent); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			s.writeError(w, http.StatusConflict, "agent already exists")
			return
		}
		s.log.Error(err, "failed to create agent", "namespace", agent.Namespace, "name", agent.Name)
		s.writeError(w, http.StatusInternalServerError, "failed to create agent: "+err.Error())
		return
	}

	s.log.Info("created agent", "namespace", agent.Namespace, "name", agent.Name)
	s.writeJSON(w, http.StatusCreated, agent)
}

// handleAgentOrLogs routes to agent details, logs, or events based on path.
func (s *Server) handleAgentOrLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
		return
	}

	// Check if this is a logs or events request: /api/v1/agents/{namespace}/{name}/logs or /events
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/agents/")
	parts := strings.Split(path, "/")

	if len(parts) == 3 {
		switch parts[2] {
		case "logs":
			s.handleAgentLogs(w, r, parts[0], parts[1])
			return
		case "events":
			s.handleAgentEvents(w, r, parts[0], parts[1])
			return
		}
	}

	if len(parts) != 2 {
		s.writeError(w, http.StatusBadRequest, "invalid path, expected /api/v1/agents/{namespace}/{name}")
		return
	}

	namespace, name := parts[0], parts[1]
	var agent omniav1alpha1.AgentRuntime
	if err := s.client.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &agent); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.writeError(w, http.StatusNotFound, "agent not found")
			return
		}
		s.log.Error(err, "failed to get agent", "namespace", namespace, "name", name)
		s.writeError(w, http.StatusInternalServerError, "failed to get agent")
		return
	}

	s.writeJSON(w, http.StatusOK, agent)
}
