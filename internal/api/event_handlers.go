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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
