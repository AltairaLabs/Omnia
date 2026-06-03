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

// Seeder writes one institutional-memory reference per SharePoint document.
type Seeder struct {
	src         DocSource
	memoryURL   string
	workspaceID string
	http        *http.Client
	log         *slog.Logger
}

// saveInstitutionalRequest mirrors internal/memory/api SaveInstitutionalRequest.
type saveInstitutionalRequest struct {
	WorkspaceID string         `json:"workspace_id"`
	Type        string         `json:"type"`
	Content     string         `json:"content"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Confidence  float64        `json:"confidence"`
}

// Run lists documents and seeds one reference each; returns the count written.
func (s *Seeder) Run(ctx context.Context) (int, error) {
	docs, err := s.src.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("list documents: %w", err)
	}
	count := 0
	for _, d := range docs {
		body := saveInstitutionalRequest{
			WorkspaceID: s.workspaceID,
			Type:        "knowledge_reference",
			Content:     fmt.Sprintf("%s — %s", d.Title, d.Summary),
			Metadata:    map[string]any{"url": d.URL, "site": d.Site},
			Confidence:  1.0,
		}
		if err := s.postMemory(ctx, body); err != nil {
			return count, fmt.Errorf("seed %q: %w", d.Title, err)
		}
		s.log.Info("seeded reference", "title", d.Title)
		count++
	}
	return count, nil
}

func (s *Seeder) postMemory(ctx context.Context, body saveInstitutionalRequest) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := strings.TrimRight(s.memoryURL, "/") + "/api/v1/institutional/memories"
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
