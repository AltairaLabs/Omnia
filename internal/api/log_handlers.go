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
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LogEntry represents a single log entry.
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Container string    `json:"container,omitempty"`
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
