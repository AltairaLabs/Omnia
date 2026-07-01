/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/serviceauth"
)

func TestSessionGroupEraser_Erase_Success(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		mustWriteJSON(t, w, EraseResult{SessionsDeleted: 4, Errors: []string{}})
	}))
	defer srv.Close()

	tokenPath := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenPath, []byte("sa-tok"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	e := NewSessionGroupEraser(serviceauth.NewTokenSource(tokenPath, time.Minute), logr.Discard())

	res, err := e.Erase(context.Background(), srv.URL, EraseScope{VirtualUserID: testEraseVU, Workspace: "ws"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.SessionsDeleted != 4 {
		t.Fatalf("SessionsDeleted = %d, want 4", res.SessionsDeleted)
	}
	if gotPath != "/api/v1/privacy/sessions/delete-by-user" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer sa-tok" {
		t.Errorf("auth = %q", gotAuth)
	}
	if !json.Valid([]byte(gotBody)) || !strings.Contains(gotBody, `"virtual_user_id":"`+testEraseVU+`"`) {
		t.Errorf("body = %q", gotBody)
	}
}

func TestSessionGroupEraser_Erase_Non2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	e := NewSessionGroupEraser(nil, logr.Discard())
	if _, err := e.Erase(context.Background(), srv.URL, EraseScope{VirtualUserID: testEraseVU}); err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestSessionGroupEraser_Erase_BadJSONIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{not json"))
	}))
	defer srv.Close()

	e := NewSessionGroupEraser(nil, logr.Discard())
	if _, err := e.Erase(context.Background(), srv.URL, EraseScope{VirtualUserID: testEraseVU}); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestSessionGroupEraser_Erase_TransportErrorIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close() // closed → connection refused

	e := NewSessionGroupEraser(nil, logr.Discard())
	if _, err := e.Erase(context.Background(), srv.URL, EraseScope{VirtualUserID: testEraseVU}); err == nil {
		t.Fatal("expected transport error")
	}
}
