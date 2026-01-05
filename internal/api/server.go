// Package api provides a REST API server for the Omnia dashboard.
// It uses the controller-runtime cached client to serve CRD data efficiently.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// Server provides REST API endpoints for the Omnia dashboard.
type Server struct {
	client client.Client
	log    logr.Logger
}

// NewServer creates a new API server with the given cached client.
func NewServer(c client.Client, log logr.Logger) *Server {
	return &Server{
		client: c,
		log:    log.WithName("api-server"),
	}
}

// Handler returns an http.Handler for the API server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// CORS middleware wrapper
	corsHandler := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Set CORS headers
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			h(w, r)
		}
	}

	// AgentRuntime endpoints
	mux.HandleFunc("/api/v1/agents", corsHandler(s.handleAgents))
	mux.HandleFunc("/api/v1/agents/", corsHandler(s.handleAgent))

	// PromptPack endpoints
	mux.HandleFunc("/api/v1/promptpacks", corsHandler(s.handlePromptPacks))
	mux.HandleFunc("/api/v1/promptpacks/", corsHandler(s.handlePromptPack))

	// ToolRegistry endpoints
	mux.HandleFunc("/api/v1/toolregistries", corsHandler(s.handleToolRegistries))
	mux.HandleFunc("/api/v1/toolregistries/", corsHandler(s.handleToolRegistry))

	// Provider endpoints
	mux.HandleFunc("/api/v1/providers", corsHandler(s.handleProviders))

	// Stats endpoint
	mux.HandleFunc("/api/v1/stats", corsHandler(s.handleStats))

	return mux
}

// writeJSON writes a JSON response.
func (s *Server) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.log.Error(err, "failed to encode JSON response")
	}
}

// writeError writes an error response.
func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, map[string]string{"error": message})
}

// parseNamespaceName extracts namespace and name from path like /api/v1/agents/{namespace}/{name}
func parseNamespaceName(path, prefix string) (namespace, name string, ok bool) {
	trimmed := strings.TrimPrefix(path, prefix)
	trimmed = strings.TrimPrefix(trimmed, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// handleAgents lists all AgentRuntimes.
func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

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

// handleAgent gets a specific AgentRuntime.
func (s *Server) handleAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	namespace, name, ok := parseNamespaceName(r.URL.Path, "/api/v1/agents")
	if !ok {
		s.writeError(w, http.StatusBadRequest, "invalid path, expected /api/v1/agents/{namespace}/{name}")
		return
	}

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

// handlePromptPacks lists all PromptPacks.
func (s *Server) handlePromptPacks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	namespace := r.URL.Query().Get("namespace")

	var packs omniav1alpha1.PromptPackList
	var err error

	if namespace != "" {
		err = s.client.List(r.Context(), &packs, client.InNamespace(namespace))
	} else {
		err = s.client.List(r.Context(), &packs)
	}

	if err != nil {
		s.log.Error(err, "failed to list promptpacks")
		s.writeError(w, http.StatusInternalServerError, "failed to list promptpacks")
		return
	}

	s.writeJSON(w, http.StatusOK, packs.Items)
}

// handlePromptPack gets a specific PromptPack.
func (s *Server) handlePromptPack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	namespace, name, ok := parseNamespaceName(r.URL.Path, "/api/v1/promptpacks")
	if !ok {
		s.writeError(w, http.StatusBadRequest, "invalid path, expected /api/v1/promptpacks/{namespace}/{name}")
		return
	}

	var pack omniav1alpha1.PromptPack
	if err := s.client.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &pack); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.writeError(w, http.StatusNotFound, "promptpack not found")
			return
		}
		s.log.Error(err, "failed to get promptpack", "namespace", namespace, "name", name)
		s.writeError(w, http.StatusInternalServerError, "failed to get promptpack")
		return
	}

	s.writeJSON(w, http.StatusOK, pack)
}

// handleToolRegistries lists all ToolRegistries.
func (s *Server) handleToolRegistries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	namespace := r.URL.Query().Get("namespace")

	var registries omniav1alpha1.ToolRegistryList
	var err error

	if namespace != "" {
		err = s.client.List(r.Context(), &registries, client.InNamespace(namespace))
	} else {
		err = s.client.List(r.Context(), &registries)
	}

	if err != nil {
		s.log.Error(err, "failed to list toolregistries")
		s.writeError(w, http.StatusInternalServerError, "failed to list toolregistries")
		return
	}

	s.writeJSON(w, http.StatusOK, registries.Items)
}

// handleToolRegistry gets a specific ToolRegistry.
func (s *Server) handleToolRegistry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	namespace, name, ok := parseNamespaceName(r.URL.Path, "/api/v1/toolregistries")
	if !ok {
		s.writeError(w, http.StatusBadRequest, "invalid path, expected /api/v1/toolregistries/{namespace}/{name}")
		return
	}

	var registry omniav1alpha1.ToolRegistry
	if err := s.client.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &registry); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.writeError(w, http.StatusNotFound, "toolregistry not found")
			return
		}
		s.log.Error(err, "failed to get toolregistry", "namespace", namespace, "name", name)
		s.writeError(w, http.StatusInternalServerError, "failed to get toolregistry")
		return
	}

	s.writeJSON(w, http.StatusOK, registry)
}

// handleProviders lists all Providers.
func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	namespace := r.URL.Query().Get("namespace")

	var providers omniav1alpha1.ProviderList
	var err error

	if namespace != "" {
		err = s.client.List(r.Context(), &providers, client.InNamespace(namespace))
	} else {
		err = s.client.List(r.Context(), &providers)
	}

	if err != nil {
		s.log.Error(err, "failed to list providers")
		s.writeError(w, http.StatusInternalServerError, "failed to list providers")
		return
	}

	s.writeJSON(w, http.StatusOK, providers.Items)
}

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
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx := r.Context()
	stats := Stats{}

	// Count agents
	var agents omniav1alpha1.AgentRuntimeList
	if err := s.client.List(ctx, &agents); err == nil {
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

	// Count promptpacks
	var packs omniav1alpha1.PromptPackList
	if err := s.client.List(ctx, &packs); err == nil {
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

	// Count tools
	var registries omniav1alpha1.ToolRegistryList
	if err := s.client.List(ctx, &registries); err == nil {
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

// Run starts the API server. It blocks until the context is cancelled.
func (s *Server) Run(ctx context.Context, addr string) error {
	server := &http.Server{
		Addr:    addr,
		Handler: s.Handler(),
	}

	// Graceful shutdown
	go func() {
		<-ctx.Done()
		s.log.Info("shutting down API server")
		if err := server.Shutdown(context.Background()); err != nil {
			s.log.Error(err, "error shutting down API server")
		}
	}()

	s.log.Info("starting API server", "addr", addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}
