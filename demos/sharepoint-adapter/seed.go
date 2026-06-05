package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// Seeder fetches extracted document text and ingests it via memory-api /ingest.
type Seeder struct {
	src         DocSource
	memoryURL   string
	workspaceID string
	http        *http.Client
	log         *slog.Logger
}

// ingestRequest mirrors internal/memory/api IngestRequest.
type ingestRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Site        string `json:"site"`
	Text        string `json:"text"`
}

// Run lists documents, fetches extracted text for each, and posts to /ingest.
// Per-doc Fetch or ingest failures are logged and skipped; only a List failure
// returns an error. Returns the count of successfully ingested documents.
func (s *Seeder) Run(ctx context.Context) (int, error) {
	docs, err := s.src.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("list documents: %w", err)
	}
	count := 0
	for _, d := range docs {
		if err := s.ingestDoc(ctx, d); err != nil {
			s.log.Warn("skipping document", "url", d.URL, "error", err)
			continue
		}
		s.log.Info("ingested document", "title", d.Title)
		count++
	}
	return count, nil
}

// ingestDoc fetches the full text for one document and posts it to /ingest.
func (s *Seeder) ingestDoc(ctx context.Context, d Doc) error {
	content, err := s.src.Fetch(ctx, d.URL)
	if err != nil {
		return fmt.Errorf("fetch %q: %w", d.URL, err)
	}
	body := ingestRequest{
		WorkspaceID: s.workspaceID,
		Title:       d.Title,
		URL:         d.URL,
		Site:        d.Site,
		Text:        content.Text,
	}
	return s.postIngest(ctx, body)
}

func (s *Seeder) postIngest(ctx context.Context, body ingestRequest) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := strings.TrimRight(s.memoryURL, "/") + "/api/v1/institutional/ingest"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentTypeJSON)
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("memory-api status=%d body=%s", resp.StatusCode, string(data))
	}
	return nil
}
