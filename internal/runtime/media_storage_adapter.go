package runtime

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/storage"
	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/altairalabs/omnia/internal/media"
)

// omniaMediaStore adapts Omnia's media.Storage to PromptKit's
// storage.MediaStorageService so the SDK's MediaLoader can resolve
// omnia:// references at provider-call time, every turn.
type omniaMediaStore struct {
	storage media.Storage
	client  *http.Client
}

var _ storage.MediaStorageService = (*omniaMediaStore)(nil)

// newOmniaMediaStore builds the adapter. A nil client uses a 30s default.
func newOmniaMediaStore(s media.Storage, client *http.Client) *omniaMediaStore {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &omniaMediaStore{storage: s, client: client}
}

// GetURL returns a presigned download URL for the reference, but only when it
// is a public HTTPS URL the model can fetch directly.
//
// URL-first providers (Anthropic, Gemini) embed this as a remote image_url
// source that the model's own servers fetch — so it must be publicly reachable
// over HTTPS. The local media backend returns an in-cluster http://…:8080
// download URL that the model cannot reach (Anthropic rejects it outright with
// "Only HTTPS URLs are supported"). Returning an empty string signals the
// loader to fall back to inline base64 via RetrieveMedia, which the runtime
// fetches over the in-pod localhost link. Cloud backends (S3/GCS/Azure) return
// presigned HTTPS URLs, which pass through unchanged.
func (m *omniaMediaStore) GetURL(ctx context.Context, ref storage.Reference, _ time.Duration) (string, error) {
	url, err := m.storage.GetDownloadURL(ctx, string(ref))
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(url, "https://") {
		return "", nil
	}
	return url, nil
}

// RetrieveMedia fetches the referenced bytes via a presigned URL and returns
// them base64-encoded. The presigned URL is self-authenticating, so no storage
// credentials are needed for the GET.
func (m *omniaMediaStore) RetrieveMedia(ctx context.Context, ref storage.Reference) (*types.MediaContent, error) {
	url, err := m.storage.GetDownloadURL(ctx, string(ref))
	if err != nil {
		return nil, fmt.Errorf("get download url: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch media: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch media: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read media body: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		// Some backends/proxies don't set Content-Type on the presigned
		// download response. Fall back to the MIME type recorded at upload
		// time rather than handing the model an empty MIMEType. Best-effort:
		// a GetMediaInfo failure here just leaves mimeType empty, same as
		// before this fallback existed.
		if info, infoErr := m.storage.GetMediaInfo(ctx, string(ref)); infoErr == nil && info != nil {
			mimeType = info.MIMEType
		}
	}
	return &types.MediaContent{Data: &encoded, MIMEType: mimeType}, nil
}

// DeleteMedia removes the referenced media.
func (m *omniaMediaStore) DeleteMedia(ctx context.Context, ref storage.Reference) error {
	return m.storage.Delete(ctx, string(ref))
}

// StoreMedia is not supported by the resolution adapter; the write path
// (inline normalization, binary upload) is added in a later phase.
func (m *omniaMediaStore) StoreMedia(context.Context, *types.MediaContent, *storage.MediaMetadata) (storage.Reference, error) {
	return "", fmt.Errorf("StoreMedia not supported by resolution adapter")
}
