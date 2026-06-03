package main

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

const vcrCassette = "testdata/graph-contract"

// TestGraphContract_VCR pins the Microsoft Graph contract to RECORDED REAL
// responses. By default it REPLAYS testdata/graph-contract.yaml; if that
// cassette is absent it SKIPS — it is deliberately NOT a hand-written fake, so
// it only asserts against responses actually captured from live Graph.
//
// To capture or refresh the cassette against a real AAD app + SharePoint site:
//
//	RECORD=1 \
//	  AZURE_TENANT_ID=… AZURE_CLIENT_ID=… AZURE_CLIENT_SECRET=… \
//	  SHAREPOINT_SITE_ID=… \
//	  go test ./demos/sharepoint-adapter/ -run TestGraphContract_VCR -count=1
//
// then commit testdata/graph-contract.yaml. The Authorization header is stripped
// before the cassette is written (AfterCaptureHook), so no credentials persist.
func TestGraphContract_VCR(t *testing.T) {
	recording := os.Getenv("RECORD") == "1"
	if !recording {
		if _, err := os.Stat(vcrCassette + ".yaml"); err != nil {
			t.Skipf("no cassette %s.yaml — record with RECORD=1 + AZURE_*/SHAREPOINT_SITE_ID (see test doc)", vcrCassette)
		}
	}

	mode := recorder.ModeReplayOnly
	if recording {
		mode = recorder.ModeRecordOnly
	}

	rec, err := recorder.New(vcrCassette,
		recorder.WithMode(mode),
		recorder.WithHook(func(i *cassette.Interaction) error {
			i.Request.Headers.Del("Authorization") // never persist credentials
			return nil
		}, recorder.AfterCaptureHook),
	)
	require.NoError(t, err)
	defer func() { _ = rec.Stop() }()

	var token TokenSource = staticToken
	siteID := "demo-site"
	baseURL := defaultGraphBaseURL
	if recording {
		cfg := loadConfig()
		ts, tErr := newAzureTokenSource(cfg)
		require.NoError(t, tErr)
		token, siteID, baseURL = ts, cfg.SiteID, cfg.GraphBaseURL
	}

	g := NewGraphClient(baseURL, siteID, token, rec.GetDefaultClient())

	docs, err := g.List(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, docs, "List should return at least one document")

	doc, err := g.Fetch(context.Background(), docs[0].URL)
	require.NoError(t, err)
	assert.NotEmpty(t, doc.Text, "Fetch should return document content")
}
