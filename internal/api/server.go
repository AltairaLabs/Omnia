// Package api provides a REST API server for the Omnia dashboard.
// It uses the controller-runtime cached client to serve CRD data efficiently.
package api

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// Error message constants.
const (
	errMethodNotAllowed = "method not allowed"
)

// Server provides REST API endpoints for the Omnia dashboard.
type Server struct {
	client    client.Client
	clientset kubernetes.Interface
	log       logr.Logger
}

// NewServer creates a new API server with the given cached client and clientset.
func NewServer(c client.Client, clientset kubernetes.Interface, log logr.Logger) *Server {
	return &Server{
		client:    c,
		clientset: clientset,
		log:       log.WithName("api-server"),
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
	// Note: /api/v1/agents/ handled by handleAgentOrLogs below for both agent details and logs

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

	// Namespaces endpoint
	mux.HandleFunc("/api/v1/namespaces", corsHandler(s.handleNamespaces))

	// Logs endpoint
	mux.HandleFunc("/api/v1/agents/", corsHandler(s.handleAgentOrLogs))

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

// LogEntry represents a single log entry.
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Container string    `json:"container,omitempty"`
}

// handleAgentLogs fetches logs from pods belonging to an agent.
func (s *Server) handleAgentLogs(w http.ResponseWriter, r *http.Request, namespace, name string) {
	// Check if clientset is available
	if s.clientset == nil {
		s.writeError(w, http.StatusInternalServerError, "logs endpoint not available")
		return
	}

	ctx := r.Context()

	// Parse query parameters
	tailLines := int64(100)
	if t := r.URL.Query().Get("tailLines"); t != "" {
		if parsed, err := strconv.ParseInt(t, 10, 64); err == nil && parsed > 0 {
			tailLines = parsed
		}
	}

	sinceSeconds := int64(3600) // Default: last hour
	if s := r.URL.Query().Get("sinceSeconds"); s != "" {
		if parsed, err := strconv.ParseInt(s, 10, 64); err == nil && parsed > 0 {
			sinceSeconds = parsed
		}
	}

	containerFilter := r.URL.Query().Get("container")

	// Find pods for this agent using the instance label selector
	labelSelector := "app.kubernetes.io/instance=" + name

	pods, err := s.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		s.log.Error(err, "failed to list pods", "namespace", namespace, "name", name)
		s.writeError(w, http.StatusInternalServerError, "failed to list pods")
		return
	}

	var allLogs []LogEntry

	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			// Filter by container if specified
			if containerFilter != "" && container.Name != containerFilter {
				continue
			}

			logOpts := &corev1.PodLogOptions{
				Container:    container.Name,
				TailLines:    &tailLines,
				SinceSeconds: &sinceSeconds,
				Timestamps:   true,
			}

			req := s.clientset.CoreV1().Pods(namespace).GetLogs(pod.Name, logOpts)
			stream, err := req.Stream(ctx)
			if err != nil {
				s.log.V(1).Info("failed to get logs", "pod", pod.Name, "container", container.Name, "error", err)
				continue
			}

			logs := s.parseLogStream(stream, container.Name)
			_ = stream.Close()
			allLogs = append(allLogs, logs...)
		}
	}

	// Sort by timestamp (newest first)
	sortLogsByTimestamp(allLogs)

	s.writeJSON(w, http.StatusOK, allLogs)
}

// parseLogStream parses a log stream into LogEntry objects.
func (s *Server) parseLogStream(stream io.Reader, containerName string) []LogEntry {
	var entries []LogEntry
	scanner := bufio.NewScanner(stream)

	for scanner.Scan() {
		line := scanner.Text()
		entry := parseLogLine(line, containerName)
		entries = append(entries, entry)
	}

	return entries
}

// JSONLogEntry represents a structured JSON log entry from zap logger.
type JSONLogEntry struct {
	Level      string  `json:"level"`
	TS         float64 `json:"ts"`
	Caller     string  `json:"caller"`
	Msg        string  `json:"msg"`
	Logger     string  `json:"logger,omitempty"`
	Error      string  `json:"error,omitempty"`
	Stacktrace string  `json:"stacktrace,omitempty"`
	// Common additional fields
	Agent     string `json:"agent,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Addr      string `json:"addr,omitempty"`
	Address   string `json:"address,omitempty"`
	Pod       string `json:"pod,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// parseTimestampPrefix extracts the timestamp and message content from a log line with kubectl --timestamps prefix.
func parseTimestampPrefix(line string) (time.Time, string) {
	// Format: 2006-01-02T15:04:05.123456789Z <message>
	if len(line) > 20 && line[4] == '-' && line[7] == '-' && line[10] == 'T' {
		spaceIdx := strings.Index(line, " ")
		if spaceIdx > 20 && spaceIdx < 40 {
			if ts, err := time.Parse(time.RFC3339Nano, line[:spaceIdx]); err == nil {
				return ts, strings.TrimSpace(line[spaceIdx+1:])
			}
		}
	}
	return time.Now(), line
}

// buildJSONLogMessage constructs a human-readable message from a JSON log entry.
func buildJSONLogMessage(jsonLog JSONLogEntry) string {
	var msgParts []string

	if jsonLog.Logger != "" {
		msgParts = append(msgParts, "["+jsonLog.Logger+"]")
	}
	if jsonLog.Caller != "" {
		msgParts = append(msgParts, "["+jsonLog.Caller+"]")
	}
	msgParts = append(msgParts, jsonLog.Msg)

	contextParts := buildContextParts(jsonLog)
	if len(contextParts) > 0 {
		msgParts = append(msgParts, "("+strings.Join(contextParts, ", ")+")")
	}
	if jsonLog.Error != "" {
		msgParts = append(msgParts, "error: "+jsonLog.Error)
	}

	return strings.Join(msgParts, " ")
}

// buildContextParts extracts context key-value pairs from a JSON log entry.
func buildContextParts(jsonLog JSONLogEntry) []string {
	var parts []string
	if jsonLog.Agent != "" {
		parts = append(parts, "agent="+jsonLog.Agent)
	}
	if jsonLog.Namespace != "" {
		parts = append(parts, "namespace="+jsonLog.Namespace)
	}
	if jsonLog.Addr != "" {
		parts = append(parts, "addr="+jsonLog.Addr)
	}
	if jsonLog.Address != "" {
		parts = append(parts, "address="+jsonLog.Address)
	}
	if jsonLog.SessionID != "" {
		parts = append(parts, "session_id="+jsonLog.SessionID)
	}
	return parts
}

// detectLogLevel infers the log level from plain text message content.
func detectLogLevel(message string) string {
	msgLower := strings.ToLower(message)
	switch {
	case strings.Contains(msgLower, "error"):
		return "error"
	case strings.Contains(msgLower, "warn"):
		return "warn"
	case strings.Contains(msgLower, "debug"):
		return "debug"
	default:
		return "info"
	}
}

// parseLogLine parses a single log line (with timestamp prefix from kubectl logs --timestamps).
func parseLogLine(line, containerName string) LogEntry {
	timestamp, messageContent := parseTimestampPrefix(line)
	entry := LogEntry{
		Timestamp: timestamp,
		Level:     "info",
		Message:   messageContent,
		Container: containerName,
	}

	// Try to parse as JSON log (zap format) - fallback to text if parsing fails
	if strings.HasPrefix(messageContent, "{") {
		var jsonLog JSONLogEntry
		if json.Unmarshal([]byte(messageContent), &jsonLog) == nil {
			if jsonLog.Level != "" {
				entry.Level = strings.ToLower(jsonLog.Level)
			}
			if jsonLog.TS > 0 {
				sec := int64(jsonLog.TS)
				nsec := int64((jsonLog.TS - float64(sec)) * 1e9)
				entry.Timestamp = time.Unix(sec, nsec)
			}
			entry.Message = buildJSONLogMessage(jsonLog)
			return entry
		}
	}

	// Fallback: detect log level from message content
	entry.Level = detectLogLevel(messageContent)

	return entry
}

// sortLogsByTimestamp sorts logs by timestamp (newest first).
func sortLogsByTimestamp(logs []LogEntry) {
	for i := 0; i < len(logs); i++ {
		for j := i + 1; j < len(logs); j++ {
			if logs[i].Timestamp.Before(logs[j].Timestamp) {
				logs[i], logs[j] = logs[j], logs[i]
			}
		}
	}
}

// K8sEvent represents a Kubernetes event for the dashboard.
type K8sEvent struct {
	Type           string            `json:"type"`
	Reason         string            `json:"reason"`
	Message        string            `json:"message"`
	FirstTimestamp string            `json:"firstTimestamp"`
	LastTimestamp  string            `json:"lastTimestamp"`
	Count          int32             `json:"count"`
	Source         K8sEventSource    `json:"source"`
	InvolvedObject K8sInvolvedObject `json:"involvedObject"`
}

// K8sEventSource represents the source of a K8s event.
type K8sEventSource struct {
	Component string `json:"component,omitempty"`
	Host      string `json:"host,omitempty"`
}

// K8sInvolvedObject represents the object involved in a K8s event.
type K8sInvolvedObject struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// handleAgentEvents fetches Kubernetes events for an agent and its pods.
func (s *Server) handleAgentEvents(w http.ResponseWriter, r *http.Request, namespace, name string) {
	if s.clientset == nil {
		s.writeError(w, http.StatusInternalServerError, "events endpoint not available")
		return
	}

	ctx := r.Context()

	// Get events for this namespace
	events, err := s.clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		s.log.Error(err, "failed to list events", "namespace", namespace)
		s.writeError(w, http.StatusInternalServerError, "failed to list events")
		return
	}

	// Filter events related to this agent
	var agentEvents []K8sEvent
	for _, event := range events.Items {
		// Match events for the AgentRuntime itself
		if event.InvolvedObject.Kind == "AgentRuntime" && event.InvolvedObject.Name == name {
			agentEvents = append(agentEvents, convertEvent(event))
			continue
		}

		// Match events for pods belonging to this agent (by label selector pattern)
		if event.InvolvedObject.Kind == "Pod" && strings.HasPrefix(event.InvolvedObject.Name, name+"-") {
			agentEvents = append(agentEvents, convertEvent(event))
			continue
		}

		// Match events for the deployment
		if event.InvolvedObject.Kind == "Deployment" && event.InvolvedObject.Name == name {
			agentEvents = append(agentEvents, convertEvent(event))
			continue
		}

		// Match events for ReplicaSets belonging to this agent
		if event.InvolvedObject.Kind == "ReplicaSet" && strings.HasPrefix(event.InvolvedObject.Name, name+"-") {
			agentEvents = append(agentEvents, convertEvent(event))
			continue
		}
	}

	// Sort by lastTimestamp (newest first)
	sortEventsByTimestamp(agentEvents)

	s.writeJSON(w, http.StatusOK, agentEvents)
}

// convertEvent converts a K8s core event to our K8sEvent type.
func convertEvent(event corev1.Event) K8sEvent {
	firstTimestamp := event.FirstTimestamp.Time
	lastTimestamp := event.LastTimestamp.Time

	// Use EventTime if available (newer events API)
	if event.EventTime.After(lastTimestamp) {
		lastTimestamp = event.EventTime.Time
	}

	return K8sEvent{
		Type:           event.Type,
		Reason:         event.Reason,
		Message:        event.Message,
		FirstTimestamp: firstTimestamp.Format(time.RFC3339),
		LastTimestamp:  lastTimestamp.Format(time.RFC3339),
		Count:          event.Count,
		Source: K8sEventSource{
			Component: event.Source.Component,
			Host:      event.Source.Host,
		},
		InvolvedObject: K8sInvolvedObject{
			Kind:      event.InvolvedObject.Kind,
			Name:      event.InvolvedObject.Name,
			Namespace: event.InvolvedObject.Namespace,
		},
	}
}

// sortEventsByTimestamp sorts events by lastTimestamp (newest first).
func sortEventsByTimestamp(events []K8sEvent) {
	for i := 0; i < len(events); i++ {
		for j := i + 1; j < len(events); j++ {
			ti, _ := time.Parse(time.RFC3339, events[i].LastTimestamp)
			tj, _ := time.Parse(time.RFC3339, events[j].LastTimestamp)
			if ti.Before(tj) {
				events[i], events[j] = events[j], events[i]
			}
		}
	}
}

// handlePromptPacks lists all PromptPacks.
func (s *Server) handlePromptPacks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
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
		s.writeError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
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
		s.writeError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
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
		s.writeError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
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
		s.writeError(w, http.StatusMethodNotAllowed, errMethodNotAllowed)
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

// Run starts the API server. It blocks until the context is cancelled.
func (s *Server) Run(ctx context.Context, addr string) error {
	server := &http.Server{
		Addr:    addr,
		Handler: s.Handler(),
	}

	// Graceful shutdown with timeout
	// Note: We use a fresh context because ctx is already cancelled when this runs
	go func() {
		<-ctx.Done()
		s.log.Info("shutting down API server")
		shutdownCtx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			s.log.Error(err, "error shutting down API server")
		}
	}()

	s.log.Info("starting API server", "addr", addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}
