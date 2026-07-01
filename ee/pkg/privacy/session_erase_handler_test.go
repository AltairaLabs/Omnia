/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
)

func newTestEraseHandler(deleter SessionDeleter) *http.ServeMux {
	h := NewSessionEraseHandler(NewSessionEraser(deleter, logr.Discard()), logr.Discard())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func postErase(mux *http.ServeMux, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/privacy/sessions/delete-by-user", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestSessionEraseHandler_Success(t *testing.T) {
	mux := newTestEraseHandler(&mockSessionDeleter{ids: []string{"s1", "s2"}})
	rec := postErase(mux, `{"virtual_user_id":"vu-1","workspace":"ws"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"sessions_deleted":2`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestSessionEraseHandler_MissingUserReturns400(t *testing.T) {
	mux := newTestEraseHandler(&mockSessionDeleter{listErr: ErrMissingVirtualUserID})
	rec := postErase(mux, `{}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSessionEraseHandler_BadJSONReturns400(t *testing.T) {
	mux := newTestEraseHandler(&mockSessionDeleter{})
	rec := postErase(mux, `{not json`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSessionEraseHandler_StoreErrorReturns500(t *testing.T) {
	mux := newTestEraseHandler(&mockSessionDeleter{listErr: errors.New("db down")})
	rec := postErase(mux, `{"virtual_user_id":"vu-1"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestSessionEraseHandler_ParsesDateRange(t *testing.T) {
	deleter := &mockSessionDeleter{ids: []string{"s1"}}
	mux := newTestEraseHandler(deleter)
	body := `{"virtual_user_id":"vu-1","date_from":"2026-01-01T00:00:00Z","date_to":"2026-02-01T00:00:00Z"}`
	rec := postErase(mux, body)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if deleter.gotUserID != testEraseVU {
		t.Fatalf("gotUserID = %q", deleter.gotUserID)
	}
}
