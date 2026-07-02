/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	eelicense "github.com/altairalabs/omnia/ee/pkg/license"
)

// licenseEndpoint serves the canonical license.License JSON at /api/v1/license.
func licenseEndpoint(t *testing.T, lic *eelicense.License) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(lic)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func bufLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, nil)), &buf
}

func TestNagLicenseAtStartup_NagsWhenOpenCore(t *testing.T) {
	t.Setenv(envOperatorAPIURL, licenseEndpoint(t, eelicense.OpenCoreLicense()))
	logger, buf := bufLogger()

	nagLicenseAtStartup(context.Background(), logger)

	assert.Contains(t, buf.String(), "startup license check", "must log that it checked")
	assert.Contains(t, buf.String(), eelicense.LicensingURL, "open-core must produce the nag banner")
}

func TestNagLicenseAtStartup_SilentWhenValid(t *testing.T) {
	t.Setenv(envOperatorAPIURL, licenseEndpoint(t, eelicense.DevLicense()))
	logger, buf := bufLogger()

	nagLicenseAtStartup(context.Background(), logger)

	assert.Contains(t, buf.String(), "startup license check")
	assert.NotContains(t, buf.String(), eelicense.LicensingURL, "a valid license must not nag")
}

func TestNagLicenseAtStartup_SkippedWhenNoURL(t *testing.T) {
	t.Setenv(envOperatorAPIURL, "")
	logger, buf := bufLogger()

	nagLicenseAtStartup(context.Background(), logger)

	assert.Contains(t, buf.String(), "skipped")
	assert.NotContains(t, buf.String(), eelicense.LicensingURL)
}

func TestNagLicenseAtStartup_UnreachableNags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	t.Setenv(envOperatorAPIURL, url)
	logger, buf := bufLogger()

	done := make(chan struct{})
	go func() {
		nagLicenseAtStartup(context.Background(), logger)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("nagLicenseAtStartup blocked on an unreachable operator")
	}

	assert.Contains(t, buf.String(), eelicense.LicensingURL, "unreachable operator must degrade and nag")
}
