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
	"strconv"
	"time"

	"github.com/altairalabs/omnia/internal/httputil"
)

// FunctionInvocationsListResponse is the JSON body for the list endpoint.
// Rows is non-nil so callers can json-serialise without a null check.
type FunctionInvocationsListResponse struct {
	Rows []*FunctionInvocation `json:"rows"`
}

// SetFunctionInvocationsService wires a FunctionInvocationsService onto
// the handler. When unset, the endpoints return 503.
func (h *Handler) SetFunctionInvocationsService(svc *FunctionInvocationsService) {
	h.functionInvocationsService = svc
}

// handleCreateFunctionInvocation persists one invocation record.
// POST /api/v1/function-invocations
func (h *Handler) handleCreateFunctionInvocation(w http.ResponseWriter, r *http.Request) {
	if h.functionInvocationsService == nil {
		writeFunctionInvocationsError(w, ErrMissingFunctionInvocationsStore)
		return
	}

	h.limitBody(w, r)
	var inv FunctionInvocation
	if err := json.NewDecoder(r.Body).Decode(&inv); err != nil {
		if isMaxBytesError(err) {
			writeError(w, ErrBodyTooLarge)
			return
		}
		writeError(w, ErrMissingBody)
		return
	}
	if inv.Namespace == "" || inv.FunctionName == "" || inv.Status == "" || inv.ID == "" {
		writeBadRequest(w, "id, namespace, functionName, and status are required")
		return
	}
	if inv.CreatedAt.IsZero() {
		inv.CreatedAt = time.Now().UTC()
	}

	if err := h.functionInvocationsService.CreateFunctionInvocation(r.Context(), &inv); err != nil {
		writeFunctionInvocationsError(w, err)
		return
	}

	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(&inv)
}

// handleListFunctionInvocations returns recent invocations for a namespace.
// GET /api/v1/function-invocations?namespace=X[&function=Y&from=...&to=...&limit=N]
func (h *Handler) handleListFunctionInvocations(w http.ResponseWriter, r *http.Request) {
	if h.functionInvocationsService == nil {
		writeFunctionInvocationsError(w, ErrMissingFunctionInvocationsStore)
		return
	}

	q := r.URL.Query()
	namespace := q.Get("namespace")
	if namespace == "" {
		writeBadRequest(w, errAggregateMissingNamespace.Error())
		return
	}

	opts := FunctionInvocationListOpts{
		Namespace:    namespace,
		FunctionName: q.Get("function"),
	}
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeBadRequest(w, "from must be RFC3339")
			return
		}
		opts.From = t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeBadRequest(w, "to must be RFC3339")
			return
		}
		opts.To = t
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeBadRequest(w, "limit must be a non-negative integer")
			return
		}
		opts.Limit = n
	}

	rows, err := h.functionInvocationsService.ListFunctionInvocations(r.Context(), opts)
	if err != nil {
		writeFunctionInvocationsError(w, err)
		return
	}
	if rows == nil {
		rows = []*FunctionInvocation{}
	}

	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	_ = json.NewEncoder(w).Encode(FunctionInvocationsListResponse{Rows: rows})
}

// handleGetFunctionInvocation returns a single row.
// GET /api/v1/function-invocations/{id}?namespace=X
func (h *Handler) handleGetFunctionInvocation(w http.ResponseWriter, r *http.Request) {
	if h.functionInvocationsService == nil {
		writeFunctionInvocationsError(w, ErrMissingFunctionInvocationsStore)
		return
	}

	namespace := r.URL.Query().Get("namespace")
	if namespace == "" {
		writeBadRequest(w, errAggregateMissingNamespace.Error())
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeBadRequest(w, "id is required")
		return
	}

	inv, err := h.functionInvocationsService.GetFunctionInvocation(r.Context(), namespace, id)
	if err != nil {
		writeFunctionInvocationsError(w, err)
		return
	}
	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	_ = json.NewEncoder(w).Encode(inv)
}

// writeBadRequest is a small 400 helper for parse failures that don't
// match a sentinel writeError maps to 400.
func writeBadRequest(w http.ResponseWriter, msg string) {
	w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: msg})
}

// writeFunctionInvocationsError maps store-not-configured to 503 and
// not-found to 404; everything else falls through to writeError.
func writeFunctionInvocationsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrMissingFunctionInvocationsStore):
		w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "function invocations store not configured"})
	case errors.Is(err, ErrFunctionInvocationNotFound):
		w.Header().Set(httputil.HeaderContentType, httputil.ContentTypeJSON)
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "function invocation not found"})
	default:
		writeError(w, err)
	}
}
