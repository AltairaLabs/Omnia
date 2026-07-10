package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/storage"
	"github.com/altairalabs/omnia/internal/media"
)

// fakeStorage implements media.Storage for adapter tests.
type fakeStorage struct {
	downloadURL string
	deleted     string
}

func (f *fakeStorage) GetUploadURL(context.Context, media.UploadRequest) (*media.UploadCredentials, error) {
	return nil, nil
}
func (f *fakeStorage) GetDownloadURL(_ context.Context, _ string) (string, error) {
	return f.downloadURL, nil
}
func (f *fakeStorage) GetMediaInfo(context.Context, string) (*media.MediaInfo, error) {
	return nil, nil
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
