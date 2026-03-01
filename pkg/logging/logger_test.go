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
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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
