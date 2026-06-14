/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package server

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/altairalabs/omnia/internal/facade"
	"github.com/go-logr/logr"
)

func TestResolveReloadPath_AllowsRelativeUnderBase(t *testing.T) {
	h := &PromptKitHandler{reloadBasePath: "/workspace-content"}
	resolved, err := h.resolveReloadPath("configs/agent.yaml")
	if err != nil {
		t.Fatalf("resolveReloadPath returned error: %v", err)
	}
	want := filepath.Clean("/workspace-content/configs/agent.yaml")
	if resolved != want {
		t.Fatalf("resolved path = %q, want %q", resolved, want)
	}
}

func TestResolveReloadPath_RejectsPathOutsideBase(t *testing.T) {
	h := &PromptKitHandler{reloadBasePath: "/workspace-content"}
	if _, err := h.resolveReloadPath("../../etc/passwd"); err == nil {
		t.Fatal("expected traversal path to be rejected")
	}
}

func TestHandleReload_PathModeReturnsReloadError(t *testing.T) {
	h := &PromptKitHandler{
		log:            logr.Discard(),
		sessions:       make(map[string]*SessionState),
		nsRegistries:   make(map[string]*providers.Registry),
		reloadBasePath: "/workspace-content",
	}
	writer := &MockResponseWriter{}
	msg := &facade.ClientMessage{
		Content: "configs/missing.yaml",
		Metadata: map[string]string{
			"reload": "true",
		},
	}

	err := h.handleReload(context.Background(), msg, writer)
	if err != nil {
		t.Fatalf("handleReload returned unexpected error: %v", err)
	}
	if writer.ErrorCode != "RELOAD_ERROR" {
		t.Fatalf("error code = %q, want RELOAD_ERROR", writer.ErrorCode)
	}
}
