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
	"errors"
	"net/http"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
	"github.com/altairalabs/omnia/pkg/intconv"
)

// handleGetMessages returns messages for a session with filtering.
func (h *Handler) handleGetMessages(w http.ResponseWriter, r *http.Request) {
	sessionID, err := sessionIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	limit := min(parseIntParam(r, "limit", defaultMessageLimit), maxMessageLimit)
	before := intconv.ClampInt32(int64(parseIntParam(r, "before", 0)))
	after := intconv.ClampInt32(int64(parseIntParam(r, "after", 0)))

	opts := providers.MessageQueryOpts{
		Limit:     limit + 1, // fetch one extra to determine hasMore
		BeforeSeq: before,
		AfterSeq:  after,
	}

	ctx := withRequestContext(r.Context(), extractRequestContext(r))
	msgs, err := h.service.GetMessages(ctx, sessionID, opts)
	if err != nil {
		if !errors.Is(err, session.ErrSessionNotFound) {
			h.requestLog(r.Context()).Error(err, "GetMessages failed", "sessionID", sessionID)
		}
		writeError(w, err)
		return
	}

	hasMore := len(msgs) > limit
	if hasMore {
		msgs = msgs[:limit]
	}

	enc := h.encryptorFor(sessionID)
	if enc != nil {
		for _, m := range msgs {
			if derr := decryptMessage(enc, m); derr != nil {
				h.requestLog(r.Context()).Error(derr, "DecryptMessage failed", "sessionID", sessionID)
				writeError(w, derr)
				return
			}
		}
	}

	writeJSON(w, MessagesResponse{
		Messages: msgs,
		HasMore:  hasMore,
	})
}

// handleAppendMessage appends a message to a session.
func (h *Handler) handleAppendMessage(w http.ResponseWriter, r *http.Request) {
	sessionID, err := sessionIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	h.limitBody(w, r)
	var msg session.Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		if isMaxBytesError(err) {
			writeError(w, ErrBodyTooLarge)
			return
		}
		writeError(w, ErrMissingBody)
		return
	}

	log := h.requestLog(r.Context())
	toAppend := msg
	if enc := h.encryptorFor(sessionID); enc != nil {
		if err := encryptMessage(enc, &toAppend); err != nil {
			log.Error(err, "EncryptMessage failed", "sessionID", sessionID)
			writeError(w, err)
			return
		}
	}
	if err := h.service.AppendMessage(r.Context(), sessionID, &toAppend); err != nil {
		if !errors.Is(err, session.ErrSessionNotFound) {
			log.Error(err, "AppendMessage failed", "sessionID", sessionID)
		}
		writeError(w, err)
		return
	}

	log.V(2).Info("message appended", "sessionID", sessionID, "role", msg.Role)
	w.WriteHeader(http.StatusCreated)
}

// handleUpdateStats applies incremental counter updates to a session.
func (h *Handler) handleUpdateStats(w http.ResponseWriter, r *http.Request) {
	sessionID, err := sessionIDFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	h.limitBody(w, r)
	var update session.SessionStatusUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		if isMaxBytesError(err) {
			writeError(w, ErrBodyTooLarge)
			return
		}
		writeError(w, ErrMissingBody)
		return
	}

	log := h.requestLog(r.Context())
	if err := h.service.UpdateSessionStatus(r.Context(), sessionID, update); err != nil {
		if !errors.Is(err, session.ErrSessionNotFound) {
			log.Error(err, "UpdateSessionStatus failed", "sessionID", sessionID)
		}
		writeError(w, err)
		return
	}

	log.V(2).Info("session status updated", "sessionID", sessionID)
	w.WriteHeader(http.StatusOK)
}
