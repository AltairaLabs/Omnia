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
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/altairalabs/omnia/internal/session"
)

// handleRecordToolCall records a tool call for a session.
func (h *Handler) handleRecordToolCall(w http.ResponseWriter, r *http.Request) {
	sessionID, err := sessionIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	h.limitBody(w, r)
	var tc session.ToolCall
	if err := json.NewDecoder(r.Body).Decode(&tc); err != nil {
		if isMaxBytesError(err) {
			writeError(w, ErrBodyTooLarge)
			return
		}
		writeError(w, ErrMissingBody)
		return
	}

	log := h.requestLog(r.Context())
	toRecord := tc
	if enc := h.encryptorFor(sessionID); enc != nil {
		if err := encryptToolCall(enc, &toRecord); err != nil {
			log.Error(err, "EncryptToolCall failed", "sessionID", sessionID)
			writeError(w, err)
			return
		}
	}
	if err := h.service.RecordToolCall(r.Context(), sessionID, &toRecord); err != nil {
		if !errors.Is(err, session.ErrSessionNotFound) {
			log.Error(err, "RecordToolCall failed", "sessionID", sessionID)
		}
		writeError(w, err)
		return
	}

	log.V(2).Info("tool call recorded", "sessionID", sessionID, "toolName", tc.Name)
	w.WriteHeader(http.StatusCreated)
}

// handleGetToolCalls returns tool calls for a session with pagination.
func (h *Handler) handleGetToolCalls(w http.ResponseWriter, r *http.Request) {
	servePaginatedDetail(h, w, r, "GetToolCalls", h.service.GetToolCalls, h.decryptToolCalls)
}

// decryptToolCalls applies the configured encryptor (if any) to each tool call.
func (h *Handler) decryptToolCalls(ctx context.Context, sessionID string, items []*session.ToolCall) ([]*session.ToolCall, error) {
	enc := h.encryptorFor(sessionID)
	if enc == nil {
		return items, nil
	}
	for _, tc := range items {
		if err := decryptToolCall(enc, tc); err != nil {
			return nil, err
		}
	}
	return items, nil
}

// handleRecordProviderCall records a provider call for a session.
func (h *Handler) handleRecordProviderCall(w http.ResponseWriter, r *http.Request) {
	sessionID, err := sessionIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	h.limitBody(w, r)
	var pc session.ProviderCall
	if err := json.NewDecoder(r.Body).Decode(&pc); err != nil {
		if isMaxBytesError(err) {
			writeError(w, ErrBodyTooLarge)
			return
		}
		writeError(w, ErrMissingBody)
		return
	}

	log := h.requestLog(r.Context())
	if err := h.service.RecordProviderCall(r.Context(), sessionID, &pc); err != nil {
		if !errors.Is(err, session.ErrSessionNotFound) {
			log.Error(err, "RecordProviderCall failed", "sessionID", sessionID)
		}
		writeError(w, err)
		return
	}

	log.V(2).Info("provider call recorded", "sessionID", sessionID, "provider", pc.Provider)
	w.WriteHeader(http.StatusCreated)
}

// handleGetProviderCalls returns provider calls for a session with pagination.
func (h *Handler) handleGetProviderCalls(w http.ResponseWriter, r *http.Request) {
	servePaginatedDetail[[]*session.ProviderCall](h, w, r, "GetProviderCalls", h.service.GetProviderCalls, nil)
}

// handleRecordRuntimeEvent records a runtime event for a session.
func (h *Handler) handleRecordRuntimeEvent(w http.ResponseWriter, r *http.Request) {
	sessionID, err := sessionIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	h.limitBody(w, r)
	var evt session.RuntimeEvent
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		if isMaxBytesError(err) {
			writeError(w, ErrBodyTooLarge)
			return
		}
		writeError(w, ErrMissingBody)
		return
	}

	log := h.requestLog(r.Context())
	toRecord := evt
	if enc := h.encryptorFor(sessionID); enc != nil {
		if err := encryptRuntimeEvent(enc, &toRecord); err != nil {
			log.Error(err, "EncryptRuntimeEvent failed", "sessionID", sessionID)
			writeError(w, err)
			return
		}
	}
	if err := h.service.RecordRuntimeEvent(r.Context(), sessionID, &toRecord); err != nil {
		if !errors.Is(err, session.ErrSessionNotFound) {
			log.Error(err, "RecordRuntimeEvent failed", "sessionID", sessionID)
		}
		writeError(w, err)
		return
	}

	log.V(2).Info("runtime event recorded", "sessionID", sessionID, "eventType", evt.EventType)
	w.WriteHeader(http.StatusCreated)
}

// handleGetRuntimeEvents returns runtime events for a session with pagination.
func (h *Handler) handleGetRuntimeEvents(w http.ResponseWriter, r *http.Request) {
	servePaginatedDetail(h, w, r, "GetRuntimeEvents", h.service.GetRuntimeEvents, h.decryptRuntimeEvents)
}

// decryptRuntimeEvents applies the configured encryptor (if any) to each event.
func (h *Handler) decryptRuntimeEvents(ctx context.Context, sessionID string, items []*session.RuntimeEvent) ([]*session.RuntimeEvent, error) {
	enc := h.encryptorFor(sessionID)
	if enc == nil {
		return items, nil
	}
	for _, evt := range items {
		if err := decryptRuntimeEvent(enc, evt); err != nil {
			return nil, err
		}
	}
	return items, nil
}
