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

package otlp

import (
	"io"
	"net/http"

	"github.com/go-logr/logr"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// maxBodySize is the maximum allowed request body size (4 MB).
const maxBodySize = 4 * 1024 * 1024

// Supported Content-Type values.
const (
	contentTypeProtobuf = "application/x-protobuf"
	contentTypeJSON     = "application/json"
)

// Handler serves the OTLP/HTTP trace export endpoint.
// Supports both application/x-protobuf and application/json content types.
type Handler struct {
	transformer *Transformer
	log         logr.Logger
}

// NewHandler creates a new HTTP OTLP handler.
func NewHandler(transformer *Transformer, log logr.Logger) *Handler {
	return &Handler{
		transformer: transformer,
		log:         log.WithName("otlp-handler"),
	}
}

// ServeHTTP handles POST /v1/traces requests.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ct := r.Header.Get("Content-Type")
	if ct != contentTypeProtobuf && ct != contentTypeJSON {
		http.Error(w, "unsupported content type; expected application/x-protobuf or application/json", http.StatusUnsupportedMediaType)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize+1))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	if len(body) > maxBodySize {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}

	req, err := unmarshalRequest(body, ct)
	if err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	processed, procErr := h.transformer.ProcessExport(r.Context(), req.GetResourceSpans())
	if procErr != nil {
		h.log.Error(procErr, "partial export failure", "processed", processed)
	}

	h.writeResponse(w, ct)
}

// unmarshalRequest decodes the request body based on content type.
func unmarshalRequest(body []byte, contentType string) (*coltracepb.ExportTraceServiceRequest, error) {
	req := &coltracepb.ExportTraceServiceRequest{}
	if contentType == contentTypeJSON {
		return req, protojson.Unmarshal(body, req)
	}
	return req, proto.Unmarshal(body, req)
}

// writeResponse serializes and writes the response in the same format as the request.
func (h *Handler) writeResponse(w http.ResponseWriter, contentType string) {
	resp := &coltracepb.ExportTraceServiceResponse{}

	var respBytes []byte
	var err error
	if contentType == contentTypeJSON {
		respBytes, err = protojson.Marshal(resp)
	} else {
		respBytes, err = proto.Marshal(resp)
	}
	if err != nil {
		http.Error(w, "failed to serialize response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(respBytes)
}

// RegisterRoutes registers the OTLP/HTTP handler on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("POST /v1/traces", h)
}
