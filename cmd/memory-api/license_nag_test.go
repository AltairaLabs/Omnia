package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/assert"

	eelicense "github.com/altairalabs/omnia/ee/pkg/license"
)

// licenseEndpoint serves the canonical license.License JSON at /api/v1/license,
// exactly as the operator/arena-controller endpoint does.
func licenseEndpoint(t *testing.T, lic *eelicense.License) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(lic)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func recordingLog(msgs *[]string) logrSink {
	return func(_, args string) { *msgs = append(*msgs, args) }
}

type logrSink = func(prefix, args string)

func TestNagLicenseAtStartup_NagsWhenOpenCore(t *testing.T) {
	url := licenseEndpoint(t, eelicense.OpenCoreLicense())
	var msgs []string
	log := funcr.New(recordingLog(&msgs), funcr.Options{})

	nagLicenseAtStartup(context.Background(), &flags{enterprise: true, operatorAPIURL: url}, log)

	joined := ""
	for _, m := range msgs {
		joined += m + "\n"
	}
	assert.Contains(t, joined, "startup license check", "must log that it checked (wiring proof)")
	assert.Contains(t, joined, eelicense.LicensingURL, "open-core must produce the nag banner")
}

func TestNagLicenseAtStartup_SilentWhenValid(t *testing.T) {
	url := licenseEndpoint(t, eelicense.DevLicense())
	var msgs []string
	log := funcr.New(recordingLog(&msgs), funcr.Options{})

	nagLicenseAtStartup(context.Background(), &flags{enterprise: true, operatorAPIURL: url}, log)

	joined := ""
	for _, m := range msgs {
		joined += m + "\n"
	}
	assert.Contains(t, joined, "startup license check")
	assert.NotContains(t, joined, eelicense.LicensingURL, "a valid enterprise license must not nag")
}

func TestNagLicenseAtStartup_SkippedWhenNotEnterprise(t *testing.T) {
	var msgs []string
	log := funcr.New(recordingLog(&msgs), funcr.Options{})

	nagLicenseAtStartup(context.Background(), &flags{enterprise: false, operatorAPIURL: "http://unused"}, log)

	assert.Empty(t, msgs, "no license work when enterprise is off")
}

func TestNagLicenseAtStartup_UnreachableOperatorNags(t *testing.T) {
	// A closed server → client degrades to open-core → nag fires (never blocks).
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	var msgs []string
	log := funcr.New(recordingLog(&msgs), funcr.Options{})

	done := make(chan struct{})
	go func() {
		nagLicenseAtStartup(context.Background(), &flags{enterprise: true, operatorAPIURL: url}, log)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("nagLicenseAtStartup blocked on an unreachable operator")
	}

	joined := ""
	for _, m := range msgs {
		joined += m + "\n"
	}
	assert.Contains(t, joined, eelicense.LicensingURL, "unreachable operator must degrade to open-core and nag")
}
