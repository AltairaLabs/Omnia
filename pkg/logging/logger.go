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

// Package logging provides shared logger initialization for Omnia binaries.
package logging

import (
	"log/slog"
	"os"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
)

// NewLogger creates a logr.Logger backed by Zap.
// It checks the LOG_LEVEL environment variable: "debug" or "trace" selects a
// development config with debug-level output; any other value (including empty)
// selects production config.
// Returns the logger and a sync function the caller should defer.
func NewLogger() (logr.Logger, func(), error) {
	zapLog, err := newZapLogger(os.Getenv("LOG_LEVEL"))
	if err != nil {
		return logr.Logger{}, nil, err
	}
	sync := func() { _ = zapLog.Sync() }
	return zapr.NewLogger(zapLog), sync, nil
}

// NewZapLogger creates a *zap.Logger configured via the LOG_LEVEL env var.
// Use this when you need both a logr.Logger (via zapr.NewLogger) and an
// *slog.Logger (via SlogFromZap) backed by the same Zap core.
func NewZapLogger() (*zap.Logger, error) {
	return newZapLogger(os.Getenv("LOG_LEVEL"))
}

// SlogFromZap creates an *slog.Logger that writes directly to the Zap core.
// This produces output identical to other Zap-backed loggers (logr via zapr,
// native Zap) — same JSON structure, level names, and timestamps.
//
// Prefer this over SlogFromLogr, which introduces a lossy double-bridge
// (slog → logr → zapr → Zap) that can mismap levels and drop timestamps.
func SlogFromZap(z *zap.Logger) *slog.Logger {
	return slog.New(zapslog.NewHandler(z.Core(), zapslog.WithCaller(true)))
}

// SlogFromLogr converts a logr.Logger into an *slog.Logger.
//
// Deprecated: This uses a lossy bridge (slog → logr.ToSlogHandler → zapr → Zap)
// that can mismap warn levels and drop slog record timestamps.
// Use SlogFromZap with NewZapLogger instead.
func SlogFromLogr(l logr.Logger) *slog.Logger {
	return slog.New(logr.ToSlogHandler(l))
}

func newZapLogger(level string) (*zap.Logger, error) {
	if level == "debug" || level == "trace" {
		cfg := zap.NewDevelopmentConfig()
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
		return cfg.Build()
	}
	return zap.NewProduction()
}
