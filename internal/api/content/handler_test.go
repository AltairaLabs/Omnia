/*
Copyright 2026.

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

package content

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/api/authz"
	"github.com/altairalabs/omnia/pkg/workspaceauth"
)

const (
	testWorkspace = "ws"
	testNamespace = "ns"
)

func newHandler(t *testing.T) *Handler {
	t.Helper()
	return NewHandler(t.TempDir(), logr.Discard())
}

func req(t *testing.T, method, relpath string, body io.Reader, withIdentity bool) *http.Request {
	t.Helper()
	r := httptest.NewRequest(method, "/x", body)
	r.SetPathValue("path", relpath)
	if withIdentity {
		id := &authz.RequestIdentity{
			VerifiedIdentity: &authz.VerifiedIdentity{Workspace: testWorkspace},
			Role:             workspaceauth.RoleEditor,
			Namespace:        testNamespace,
		}
		r = r.WithContext(authz.ContextWithIdentity(r.Context(), id))
	}
	return r
}

func TestHandler_PutThenGet(t *testing.T) {
	h := newHandler(t)

	rec := httptest.NewRecorder()
	h.Put(rec, req(t, http.MethodPut, "arena/p1/config.yaml", strings.NewReader("hello: world"), true))
	if rec.Code != http.StatusOK {
		t.Fatalf("Put: code = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.Get(rec, req(t, http.MethodGet, "arena/p1/config.yaml", nil, true))
	if rec.Code != http.StatusOK {
		t.Fatalf("Get: code = %d, want 200", rec.Code)
	}
	var fc FileContent
	if err := json.Unmarshal(rec.Body.Bytes(), &fc); err != nil {
		t.Fatalf("decode FileContent: %v", err)
	}
	if fc.Content != "hello: world" {
		t.Errorf("Content = %q, want %q", fc.Content, "hello: world")
	}
	if fc.Encoding != "utf-8" {
		t.Errorf("Encoding = %q, want utf-8", fc.Encoding)
	}
}

func TestHandler_GetMissing(t *testing.T) {
	h := newHandler(t)
	rec := httptest.NewRecorder()
	h.Get(rec, req(t, http.MethodGet, "nope.txt", nil, true))
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing file: code = %d, want 404", rec.Code)
	}
}

func TestHandler_MkDirThenList(t *testing.T) {
	h := newHandler(t)

	rec := httptest.NewRecorder()
	h.MkDir(rec, req(t, http.MethodPost, "arena/projects", nil, true))
	if rec.Code != http.StatusCreated {
		t.Fatalf("MkDir: code = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.Get(rec, req(t, http.MethodGet, "arena", nil, true))
	if rec.Code != http.StatusOK {
		t.Fatalf("Get listing: code = %d, want 200", rec.Code)
	}
	var l Listing
	if err := json.Unmarshal(rec.Body.Bytes(), &l); err != nil {
		t.Fatalf("decode Listing: %v", err)
	}
	if len(l.Entries) != 1 || l.Entries[0].Name != "projects" || l.Entries[0].Type != "directory" {
		t.Errorf("listing = %+v, want one directory 'projects'", l.Entries)
	}
}

func TestHandler_Delete(t *testing.T) {
	h := newHandler(t)

	rec := httptest.NewRecorder()
	h.Put(rec, req(t, http.MethodPut, "f.txt", strings.NewReader("x"), true))
	if rec.Code != http.StatusOK {
		t.Fatalf("Put: code = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.Delete(rec, req(t, http.MethodDelete, "f.txt", nil, true))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("Delete: code = %d, want 204", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.Get(rec, req(t, http.MethodGet, "f.txt", nil, true))
	if rec.Code != http.StatusNotFound {
		t.Errorf("Get after delete: code = %d, want 404", rec.Code)
	}
}

func TestHandler_DeleteMissing(t *testing.T) {
	h := newHandler(t)
	rec := httptest.NewRecorder()
	h.Delete(rec, req(t, http.MethodDelete, "ghost", nil, true))
	if rec.Code != http.StatusNotFound {
		t.Errorf("delete missing: code = %d, want 404", rec.Code)
	}
}

func TestHandler_MoveRenamesFile(t *testing.T) {
	h := newHandler(t)

	rec := httptest.NewRecorder()
	h.Put(rec, req(t, http.MethodPut, "arena/p1/old.yaml", strings.NewReader("hello: world"), true))
	if rec.Code != http.StatusOK {
		t.Fatalf("Put: code = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.Move(rec, req(t, http.MethodPatch, "arena/p1/old.yaml", strings.NewReader(`{"to":"arena/p1/new.yaml"}`), true))
	if rec.Code != http.StatusOK {
		t.Fatalf("Move: code = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var wr WriteResult
	if err := json.Unmarshal(rec.Body.Bytes(), &wr); err != nil {
		t.Fatalf("decode WriteResult: %v", err)
	}
	if wr.Path != "arena/p1/new.yaml" {
		t.Errorf("Path = %q, want arena/p1/new.yaml", wr.Path)
	}

	// Source is gone, destination has the original content.
	rec = httptest.NewRecorder()
	h.Get(rec, req(t, http.MethodGet, "arena/p1/old.yaml", nil, true))
	if rec.Code != http.StatusNotFound {
		t.Errorf("Get old: code = %d, want 404", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.Get(rec, req(t, http.MethodGet, "arena/p1/new.yaml", nil, true))
	if rec.Code != http.StatusOK {
		t.Fatalf("Get new: code = %d, want 200", rec.Code)
	}
	var fc FileContent
	if err := json.Unmarshal(rec.Body.Bytes(), &fc); err != nil {
		t.Fatalf("decode FileContent: %v", err)
	}
	if fc.Content != "hello: world" {
		t.Errorf("Content = %q, want %q", fc.Content, "hello: world")
	}
}

func TestHandler_MoveMissingSource(t *testing.T) {
	h := newHandler(t)
	rec := httptest.NewRecorder()
	h.Move(rec, req(t, http.MethodPatch, "ghost.yaml", strings.NewReader(`{"to":"x.yaml"}`), true))
	if rec.Code != http.StatusNotFound {
		t.Errorf("move missing source: code = %d, want 404", rec.Code)
	}
}

func TestHandler_MoveDestinationExists(t *testing.T) {
	h := newHandler(t)
	for _, p := range []string{"a.yaml", "b.yaml"} {
		rec := httptest.NewRecorder()
		h.Put(rec, req(t, http.MethodPut, p, strings.NewReader("x"), true))
		if rec.Code != http.StatusOK {
			t.Fatalf("Put %s: code = %d", p, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	h.Move(rec, req(t, http.MethodPatch, "a.yaml", strings.NewReader(`{"to":"b.yaml"}`), true))
	if rec.Code != http.StatusConflict {
		t.Errorf("move onto existing: code = %d, want 409", rec.Code)
	}
}

func TestHandler_MoveMissingDestination(t *testing.T) {
	h := newHandler(t)
	rec := httptest.NewRecorder()
	h.Put(rec, req(t, http.MethodPut, "a.yaml", strings.NewReader("x"), true))
	if rec.Code != http.StatusOK {
		t.Fatalf("Put: code = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.Move(rec, req(t, http.MethodPatch, "a.yaml", strings.NewReader(`{"to":""}`), true))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty destination: code = %d, want 400", rec.Code)
	}
}

func TestHandler_MoveInvalidBody(t *testing.T) {
	h := newHandler(t)
	rec := httptest.NewRecorder()
	h.Move(rec, req(t, http.MethodPatch, "a.yaml", strings.NewReader("not json"), true))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid body: code = %d, want 400", rec.Code)
	}
}

func TestHandler_MoveDestinationEscapeRejected(t *testing.T) {
	h := newHandler(t)
	rec := httptest.NewRecorder()
	h.Put(rec, req(t, http.MethodPut, "a.yaml", strings.NewReader("x"), true))
	if rec.Code != http.StatusOK {
		t.Fatalf("Put: code = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.Move(rec, req(t, http.MethodPatch, "a.yaml", strings.NewReader(`{"to":"../../escape.yaml"}`), true))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("destination escape: code = %d, want 400", rec.Code)
	}
}

func TestHandler_PutBinaryGetBase64(t *testing.T) {
	h := newHandler(t)
	binary := string([]byte{0xff, 0xfe, 0x00, 0x01})

	rec := httptest.NewRecorder()
	h.Put(rec, req(t, http.MethodPut, "blob.bin", strings.NewReader(binary), true))
	if rec.Code != http.StatusOK {
		t.Fatalf("Put: code = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.Get(rec, req(t, http.MethodGet, "blob.bin", nil, true))
	var fc FileContent
	if err := json.Unmarshal(rec.Body.Bytes(), &fc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if fc.Encoding != "base64" {
		t.Errorf("Encoding = %q, want base64", fc.Encoding)
	}
}

func TestHandler_PathEscapeRejected(t *testing.T) {
	h := newHandler(t)
	rec := httptest.NewRecorder()
	h.Get(rec, req(t, http.MethodGet, "../../etc/passwd", nil, true))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("path escape: code = %d, want 400", rec.Code)
	}
}

func TestHandler_MissingIdentity(t *testing.T) {
	h := newHandler(t)
	rec := httptest.NewRecorder()
	h.Get(rec, req(t, http.MethodGet, "x", nil, false))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("no identity: code = %d, want 500", rec.Code)
	}
}
