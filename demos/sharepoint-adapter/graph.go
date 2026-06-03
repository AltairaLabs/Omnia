package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Doc is a SharePoint document reference (returned by /list, used for seeding).
type Doc struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Site    string `json:"site"`
	Summary string `json:"summary"`
}

// DocContent is a fetched document (returned by /fetch).
type DocContent struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Text  string `json:"text"`
}

// TokenSource returns a Graph bearer token.
type TokenSource func(ctx context.Context) (string, error)

// GraphError carries the upstream Graph HTTP status so the server can pass it
// through (the governance beat relies on a restricted-site 403 surfacing).
type GraphError struct {
	StatusCode int
	Body       string
}

func (e *GraphError) Error() string {
	return fmt.Sprintf("graph request failed: status=%d body=%s", e.StatusCode, e.Body)
}

// GraphClient talks to Microsoft Graph for a single SharePoint site.
type GraphClient struct {
	baseURL string
	siteID  string
	http    *http.Client
	token   TokenSource
}

// NewGraphClient builds a client. baseURL/httpClient default when empty/nil.
func NewGraphClient(baseURL, siteID string, token TokenSource, httpClient *http.Client) *GraphClient {
	if baseURL == "" {
		baseURL = defaultGraphBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &GraphClient{baseURL: strings.TrimRight(baseURL, "/"), siteID: siteID, token: token, http: httpClient}
}

// encodeShareID converts a sharing URL into a Graph share id ("encoding sharing
// URLs" rule): base64, strip padding, '/'->'_', '+'->'-', prefix "u!".
func encodeShareID(webURL string) string {
	b64 := base64.StdEncoding.EncodeToString([]byte(webURL))
	b64 = strings.TrimRight(b64, "=")
	b64 = strings.ReplaceAll(b64, "/", "_")
	b64 = strings.ReplaceAll(b64, "+", "-")
	return "u!" + b64
}

func (g *GraphClient) newRequest(ctx context.Context, method, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	tok, err := g.token(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func (g *GraphClient) doJSON(ctx context.Context, url string, out any) error {
	req, err := g.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		return err
	}
	resp, err := g.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return &GraphError{StatusCode: resp.StatusCode, Body: string(data)}
	}
	return json.Unmarshal(data, out)
}

func (g *GraphClient) doRaw(ctx context.Context, url string) (string, error) {
	req, err := g.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		return "", err
	}
	resp, err := g.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", &GraphError{StatusCode: resp.StatusCode, Body: string(data)}
	}
	return string(data), nil
}

// List enumerates documents in the site's default drive root (folders skipped).
func (g *GraphClient) List(ctx context.Context) ([]Doc, error) {
	url := fmt.Sprintf("%s/sites/%s/drive/root/children", g.baseURL, g.siteID)
	var resp struct {
		Value []struct {
			Name   string `json:"name"`
			WebURL string `json:"webUrl"`
			File   *struct {
				MimeType string `json:"mimeType"`
			} `json:"file"`
		} `json:"value"`
	}
	if err := g.doJSON(ctx, url, &resp); err != nil {
		return nil, err
	}
	docs := make([]Doc, 0, len(resp.Value))
	for _, item := range resp.Value {
		if item.File == nil { // skip folders
			continue
		}
		docs = append(docs, Doc{
			Title:   item.Name,
			URL:     item.WebURL,
			Site:    g.siteID,
			Summary: item.Name, // demo: summary == title
		})
	}
	return docs, nil
}

// Fetch resolves a sharing URL to its driveItem and returns the content body.
func (g *GraphClient) Fetch(ctx context.Context, webURL string) (*DocContent, error) {
	shareID := encodeShareID(webURL)
	var meta struct {
		Name   string `json:"name"`
		WebURL string `json:"webUrl"`
	}
	if err := g.doJSON(ctx, fmt.Sprintf("%s/shares/%s/driveItem", g.baseURL, shareID), &meta); err != nil {
		return nil, err
	}
	text, err := g.doRaw(ctx, fmt.Sprintf("%s/shares/%s/driveItem/content", g.baseURL, shareID))
	if err != nil {
		return nil, err
	}
	return &DocContent{Title: meta.Name, URL: meta.WebURL, Text: text}, nil
}
