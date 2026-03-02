/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

package logging

import (
	"log/slog"
	"testing"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewZapLogger_Production(t *testing.T) {
	logger, err := newZapLogger("")
	if err != nil {
		t.Fatalf("newZapLogger returned error: %v", err)
	}
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	// Production logger uses info level by default
	if logger.Core().Enabled(zap.DebugLevel) {
		t.Error("production logger should not enable debug level")
	}
}

func TestNewZapLogger_Debug(t *testing.T) {
	logger, err := newZapLogger("debug")
	if err != nil {
		t.Fatalf("newZapLogger returned error: %v", err)
	}
	if !logger.Core().Enabled(zap.DebugLevel) {
		t.Error("debug logger should enable debug level")
	}
}

func TestNewZapLogger_Trace(t *testing.T) {
	logger, err := newZapLogger("trace")
	if err != nil {
		t.Fatalf("newZapLogger returned error: %v", err)
	}
	if !logger.Core().Enabled(zap.DebugLevel) {
		t.Error("trace logger should enable debug level")
	}
}

func TestNewZapLogger_UnknownLevel(t *testing.T) {
	logger, err := newZapLogger("warn")
	if err != nil {
		t.Fatalf("newZapLogger returned error: %v", err)
	}
	// Unknown levels fall through to production config
	if logger.Core().Enabled(zap.DebugLevel) {
		t.Error("unknown level should fall through to production (no debug)")
	}
}

func TestNewLogger_UsesEnvVar(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")

	log, sync, err := NewLogger()
	if err != nil {
		t.Fatalf("NewLogger returned error: %v", err)
	}
	if sync == nil {
		t.Fatal("expected non-nil sync function")
	}
	defer sync()

	if !log.GetSink().Enabled(int(zapcore.DebugLevel)) {
		t.Error("logger should be debug-enabled when LOG_LEVEL=debug")
	}
}

func TestSlogFromLogr(t *testing.T) {
	// Create an observable Zap core so we can verify the log reaches the Zap backend.
	core, logs := observer.New(zapcore.InfoLevel)
	zapLogger := zap.New(core)
	logrLogger := zapr.NewLogger(zapLogger)

	sl := SlogFromLogr(logrLogger)

	if sl == nil {
		t.Fatal("expected non-nil *slog.Logger")
	}

	// Log through slog and verify it arrives in the Zap observer.
	sl.Info("bridge test", slog.String("key", "value"))

	if logs.Len() != 1 {
		t.Fatalf("expected 1 log entry, got %d", logs.Len())
	}

	entry := logs.All()[0]
	if entry.Message != "bridge test" {
		t.Errorf("expected message %q, got %q", "bridge test", entry.Message)
	}

	// Verify structured key-value pair survived the bridge.
	found := false
	for _, f := range entry.ContextMap() {
		if f == "value" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected key=value in context, got %v", entry.ContextMap())
	}
}

func TestNewZapLogger_UsesEnvVar(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")

	zapLog, err := NewZapLogger()
	if err != nil {
		t.Fatalf("NewZapLogger returned error: %v", err)
	}

	if !zapLog.Core().Enabled(zap.DebugLevel) {
		t.Error("logger should be debug-enabled when LOG_LEVEL=debug")
	}
}

func TestSlogFromZap(t *testing.T) {
	// Create an observable Zap core so we can verify the log reaches the Zap backend.
	core, logs := observer.New(zapcore.InfoLevel)
	zapLogger := zap.New(core)

	sl := SlogFromZap(zapLogger)

	if sl == nil {
		t.Fatal("expected non-nil *slog.Logger")
	}

	// Log through slog and verify it arrives in the Zap observer.
	sl.Info("direct bridge test", slog.String("key", "value"))

	if logs.Len() != 1 {
		t.Fatalf("expected 1 log entry, got %d", logs.Len())
	}

	entry := logs.All()[0]
	if entry.Message != "direct bridge test" {
		t.Errorf("expected message %q, got %q", "direct bridge test", entry.Message)
	}

	// Verify structured key-value pair survived the bridge.
	found := false
	for _, f := range entry.ContextMap() {
		if f == "value" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected key=value in context, got %v", entry.ContextMap())
	}
}

func TestSlogFromZap_WarnLevel(t *testing.T) {
	// Verify that slog.Warn maps to Zap WarnLevel (not info, which was the bug
	// with the old logr.ToSlogHandler bridge).
	core, logs := observer.New(zapcore.DebugLevel)
	zapLogger := zap.New(core)

	sl := SlogFromZap(zapLogger)
	sl.Warn("warning test")

	if logs.Len() != 1 {
		t.Fatalf("expected 1 log entry, got %d", logs.Len())
	}

	entry := logs.All()[0]
	if entry.Level != zapcore.WarnLevel {
		t.Errorf("expected WarnLevel, got %v", entry.Level)
	}
}

func TestNewLogger_ProductionDefault(t *testing.T) {
	t.Setenv("LOG_LEVEL", "")

	log, sync, err := NewLogger()
	if err != nil {
		t.Fatalf("NewLogger returned error: %v", err)
	}
	defer sync()

	// Production logger: V(0) is info (enabled), V(1) is debug (disabled)
	if log.V(1).Enabled() {
		t.Error("production logger should not enable V(1) debug")
	}
}
