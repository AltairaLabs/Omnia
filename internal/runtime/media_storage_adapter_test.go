package runtime

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/storage"
	"github.com/altairalabs/omnia/internal/media"
)

// roundTripFunc adapts a func to http.RoundTripper so tests can hand back a
// synthetic *http.Response with full control over headers — a real
// net/http server auto-sniffs and fills in Content-Type when a handler
// doesn't set it, which defeats testing the "Content-Type genuinely absent"
// fallback path.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// fakeStorage implements media.Storage for adapter tests.
type fakeStorage struct {
	downloadURL string
	deleted     string

	// mediaInfo / mediaInfoErr control GetMediaInfo's return, so tests can
	// exercise RetrieveMedia's Content-Type-empty MIME fallback.
	mediaInfo    *media.MediaInfo
	mediaInfoErr error
}

func (f *fakeStorage) GetUploadURL(context.Context, media.UploadRequest) (*media.UploadCredentials, error) {
	return nil, nil
}
func (f *fakeStorage) GetDownloadURL(_ context.Context, _ string) (string, error) {
	return f.downloadURL, nil
}
func (f *fakeStorage) GetMediaInfo(context.Context, string) (*media.MediaInfo, error) {
	return f.mediaInfo, f.mediaInfoErr
}
func (f *fakeStorage) Delete(_ context.Context, ref string) error { f.deleted = ref; return nil }
func (f *fakeStorage) Close() error                               { return nil }

func TestOmniaMediaStore_RetrieveMedia_FetchesBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47})
	}))
	defer srv.Close()

	store := newOmniaMediaStore(&fakeStorage{downloadURL: srv.URL}, nil)
	got, err := store.RetrieveMedia(context.Background(), storage.Reference("omnia://sessions/s1/media/m1"))
	if err != nil {
		t.Fatalf("RetrieveMedia: %v", err)
	}
	if got.Data == nil || *got.Data != "iVBORw==" {
		t.Errorf("Data = %v, want base64 of PNG magic", got.Data)
	}
	if got.MIMEType != "image/png" {
		t.Errorf("MIMEType = %q, want image/png", got.MIMEType)
	}
}

// noContentTypeClient returns an *http.Client whose Transport hands back body
// with no Content-Type header set at all — bypassing net/http's real sniffing
// so the fallback branch in RetrieveMedia is actually exercised.
func noContentTypeClient(body string) *http.Client {
	return &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}
}

// TestOmniaMediaStore_RetrieveMedia_FallsBackToStoredMIMEType covers the
// case where the download response has no Content-Type header (some
// backends/proxies omit it): RetrieveMedia should fall back to the MIME type
// recorded at upload time via GetMediaInfo rather than handing the model an
// empty MIMEType.
func TestOmniaMediaStore_RetrieveMedia_FallsBackToStoredMIMEType(t *testing.T) {
	fs := &fakeStorage{
		downloadURL: "http://media.test/x",
		mediaInfo:   &media.MediaInfo{MIMEType: "application/pdf"},
	}
	store := newOmniaMediaStore(fs, noContentTypeClient("%PDF-1.4"))
	got, err := store.RetrieveMedia(context.Background(), storage.Reference("omnia://sessions/s1/media/m2"))
	if err != nil {
		t.Fatalf("RetrieveMedia: %v", err)
	}
	if got.MIMEType != "application/pdf" {
		t.Errorf("MIMEType = %q, want fallback application/pdf from GetMediaInfo", got.MIMEType)
	}
}

// TestOmniaMediaStore_RetrieveMedia_MIMEFallbackErrorLeavesEmpty covers the
// case where both Content-Type is empty AND GetMediaInfo fails: RetrieveMedia
// must not error out — the fallback is best-effort, so MIMEType stays empty,
// same as the pre-fallback behavior.
func TestOmniaMediaStore_RetrieveMedia_MIMEFallbackErrorLeavesEmpty(t *testing.T) {
	fs := &fakeStorage{
		downloadURL:  "http://media.test/x",
		mediaInfoErr: errors.New("not found"),
	}
	store := newOmniaMediaStore(fs, noContentTypeClient("data"))
	got, err := store.RetrieveMedia(context.Background(), storage.Reference("omnia://sessions/s1/media/m3"))
	if err != nil {
		t.Fatalf("RetrieveMedia: %v", err)
	}
	if got.MIMEType != "" {
		t.Errorf("MIMEType = %q, want empty when GetMediaInfo fails", got.MIMEType)
	}
}

func TestOmniaMediaStore_GetURL_DelegatesToStorage(t *testing.T) {
	store := newOmniaMediaStore(&fakeStorage{downloadURL: "https://example/x"}, nil)
	url, err := store.GetURL(context.Background(), storage.Reference("omnia://r"), 0)
	if err != nil || url != "https://example/x" {
		t.Fatalf("GetURL = %q, %v", url, err)
	}
}

func TestOmniaMediaStore_DeleteMedia_DelegatesToStorage(t *testing.T) {
	fs := &fakeStorage{}
	store := newOmniaMediaStore(fs, nil)
	if err := store.DeleteMedia(context.Background(), storage.Reference("omnia://r")); err != nil {
		t.Fatalf("DeleteMedia: %v", err)
	}
	if fs.deleted != "omnia://r" {
		t.Errorf("deleted = %q, want omnia://r", fs.deleted)
	}
}
