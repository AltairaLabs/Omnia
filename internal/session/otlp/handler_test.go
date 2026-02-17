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
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

func newTestHandler() (*Handler, *MockSessionWriter) {
	writer := newMockWriter()
	transformer := NewTransformer(writer, logr.Discard())
	handler := NewHandler(transformer, logr.Discard())
	return handler, writer
}

func buildExportRequest(conversationID string) *coltracepb.ExportTraceServiceRequest {
	attrs := combineAttrs(
		outputMsgAttrs(makeMessageValue("assistant", "HTTP response")),
		tokenAttrs(30, 15),
	)
	span := makeSpan(conversationID, uint64(time.Now().UnixNano()), attrs)
	rs := makeResourceSpans("staging", "http-agent", span)
	return &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{rs},
	}
}

func TestHandler_Protobuf(t *testing.T) {
	handler, writer := newTestHandler()

	req := buildExportRequest("http-conv-1")
	body, err := proto.Marshal(req)
	require.NoError(t, err)

	httpReq := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", contentTypeProtobuf)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httpReq)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, contentTypeProtobuf, rec.Header().Get("Content-Type"))

	resp := &coltracepb.ExportTraceServiceResponse{}
	require.NoError(t, proto.Unmarshal(rec.Body.Bytes(), resp))

	assert.NotNil(t, writer.sessions["http-conv-1"])
	assert.Len(t, writer.messages["http-conv-1"], 1)
}

func TestHandler_JSON(t *testing.T) {
	handler, writer := newTestHandler()

	req := buildExportRequest("json-conv-1")
	body, err := protojson.Marshal(req)
	require.NoError(t, err)

	httpReq := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", contentTypeJSON)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httpReq)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, contentTypeJSON, rec.Header().Get("Content-Type"))

	resp := &coltracepb.ExportTraceServiceResponse{}
	require.NoError(t, protojson.Unmarshal(rec.Body.Bytes(), resp))

	assert.NotNil(t, writer.sessions["json-conv-1"])
	assert.Len(t, writer.messages["json-conv-1"], 1)
}

func TestHandler_WrongMethod(t *testing.T) {
	handler, _ := newTestHandler()

	httpReq := httptest.NewRequest(http.MethodGet, "/v1/traces", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httpReq)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestHandler_WrongContentType(t *testing.T) {
	handler, _ := newTestHandler()

	httpReq := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader("{}"))
	httpReq.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httpReq)

	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
}

func TestHandler_InvalidProtobuf(t *testing.T) {
	handler, _ := newTestHandler()

	httpReq := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader("not-protobuf"))
	httpReq.Header.Set("Content-Type", contentTypeProtobuf)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httpReq)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_InvalidJSON(t *testing.T) {
	handler, _ := newTestHandler()

	httpReq := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader("{invalid"))
	httpReq.Header.Set("Content-Type", contentTypeJSON)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httpReq)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_BodyTooLarge(t *testing.T) {
	handler, _ := newTestHandler()

	largeBody := make([]byte, maxBodySize+1)
	httpReq := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(largeBody))
	httpReq.Header.Set("Content-Type", contentTypeProtobuf)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httpReq)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestHandler_EmptyBody(t *testing.T) {
	handler, _ := newTestHandler()

	httpReq := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader([]byte{}))
	httpReq.Header.Set("Content-Type", contentTypeProtobuf)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httpReq)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_EmptyJSON(t *testing.T) {
	handler, _ := newTestHandler()

	httpReq := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader("{}"))
	httpReq.Header.Set("Content-Type", contentTypeJSON)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httpReq)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_RegisterRoutes(t *testing.T) {
	handler, _ := newTestHandler()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := &coltracepb.ExportTraceServiceRequest{}
	body, err := proto.Marshal(req)
	require.NoError(t, err)

	httpReq := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", contentTypeProtobuf)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, httpReq)

	assert.Equal(t, http.StatusOK, rec.Code)
}
