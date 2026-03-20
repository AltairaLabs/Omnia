/*
Copyright 2026.

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

package runtime

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AltairaLabs/PromptKit/runtime/types"

	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// --- resolveResponseParts tests ---

func TestResolveResponseParts_TextPart(t *testing.T) {
	server := NewServer(WithLogger(logr.Discard()))
	ctx := context.Background()

	text := "hello world"
	parts := []types.ContentPart{
		{Type: types.ContentTypeText, Text: &text},
	}

	result, err := server.resolveResponseParts(ctx, parts)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, types.ContentTypeText, result[0].Type)
	assert.Equal(t, "hello world", result[0].Text)
}

func TestResolveResponseParts_TextPart_NilText(t *testing.T) {
	server := NewServer(WithLogger(logr.Discard()))
	ctx := context.Background()

	parts := []types.ContentPart{
		{Type: types.ContentTypeText, Text: nil},
	}

	result, err := server.resolveResponseParts(ctx, parts)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, types.ContentTypeText, result[0].Type)
	assert.Equal(t, "", result[0].Text)
}

func TestResolveResponseParts_EmptyParts(t *testing.T) {
	server := NewServer(WithLogger(logr.Discard()))
	ctx := context.Background()

	result, err := server.resolveResponseParts(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestResolveResponseParts_MediaPart_NilMedia_Skipped(t *testing.T) {
	server := NewServer(WithLogger(logr.Discard()))
	ctx := context.Background()

	parts := []types.ContentPart{
		{Type: types.ContentTypeImage, Media: nil},
	}

	result, err := server.resolveResponseParts(ctx, parts)
	require.NoError(t, err)
	// nil media parts are skipped via continue
	assert.Empty(t, result)
}

func TestResolveResponseParts_MediaPart_WithData(t *testing.T) {
	tmpDir := t.TempDir()
	server := NewServer(
		WithLogger(logr.Discard()),
		WithMediaBasePath(tmpDir),
	)
	ctx := context.Background()

	b64 := base64.StdEncoding.EncodeToString([]byte("image data"))
	parts := []types.ContentPart{
		{
			Type: types.ContentTypeImage,
			Media: &types.MediaContent{
				Data:     &b64,
				MIMEType: "image/png",
			},
		},
	}

	result, err := server.resolveResponseParts(ctx, parts)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, types.ContentTypeImage, result[0].Type)
	assert.Equal(t, b64, result[0].Media.Data)
	assert.Equal(t, "image/png", result[0].Media.MimeType)
}

func TestResolveResponseParts_MediaPart_ResolveError_Skipped(t *testing.T) {
	// Use a resolver that will fail (file not found)
	tmpDir := t.TempDir()
	server := NewServer(
		WithLogger(logr.Discard()),
		WithMediaBasePath(tmpDir),
	)
	ctx := context.Background()

	url := "file:///nonexistent/file.png"
	parts := []types.ContentPart{
		{
			Type: types.ContentTypeImage,
			Media: &types.MediaContent{
				URL:      &url,
				MIMEType: "image/png",
			},
		},
	}

	result, err := server.resolveResponseParts(ctx, parts)
	require.NoError(t, err)
	// Error in resolve causes continue, so the part is skipped
	assert.Empty(t, result)
}

func TestResolveResponseParts_MultipleParts_MixedTypes(t *testing.T) {
	tmpDir := t.TempDir()
	server := NewServer(
		WithLogger(logr.Discard()),
		WithMediaBasePath(tmpDir),
	)
	ctx := context.Background()

	text := "some text"
	b64 := base64.StdEncoding.EncodeToString([]byte("audio bytes"))
	parts := []types.ContentPart{
		{Type: types.ContentTypeText, Text: &text},
		{Type: types.ContentTypeAudio, Media: &types.MediaContent{Data: &b64, MIMEType: "audio/mp3"}},
	}

	result, err := server.resolveResponseParts(ctx, parts)
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, types.ContentTypeText, result[0].Type)
	assert.Equal(t, "some text", result[0].Text)
	assert.Equal(t, types.ContentTypeAudio, result[1].Type)
	assert.Equal(t, b64, result[1].Media.Data)
}

func TestResolveResponseParts_VideoPart(t *testing.T) {
	server := NewServer(WithLogger(logr.Discard()))
	ctx := context.Background()

	b64 := base64.StdEncoding.EncodeToString([]byte("video data"))
	parts := []types.ContentPart{
		{
			Type: types.ContentTypeVideo,
			Media: &types.MediaContent{
				Data:     &b64,
				MIMEType: "video/mp4",
			},
		},
	}

	result, err := server.resolveResponseParts(ctx, parts)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, types.ContentTypeVideo, result[0].Type)
	assert.Equal(t, b64, result[0].Media.Data)
}

// --- resolveMediaContent tests ---

func TestResolveMediaContent_WithBase64Data(t *testing.T) {
	server := NewServer(WithLogger(logr.Discard()))
	ctx := context.Background()

	data := base64.StdEncoding.EncodeToString([]byte("test data"))
	media := &types.MediaContent{
		Data:     &data,
		MIMEType: "image/jpeg",
	}

	result, err := server.resolveMediaContent(ctx, media)
	require.NoError(t, err)
	assert.Equal(t, data, result.Data)
	assert.Equal(t, "image/jpeg", result.MimeType)
	assert.Empty(t, result.Url)
}

func TestResolveMediaContent_WithEmptyData_FallsThrough(t *testing.T) {
	server := NewServer(WithLogger(logr.Discard()))
	ctx := context.Background()

	empty := ""
	httpURL := "https://example.com/image.png"
	media := &types.MediaContent{
		Data:     &empty,
		URL:      &httpURL,
		MIMEType: "image/png",
	}

	result, err := server.resolveMediaContent(ctx, media)
	require.NoError(t, err)
	// Empty data falls through to URL check
	assert.Equal(t, httpURL, result.Url)
	assert.Equal(t, "image/png", result.MimeType)
}

func TestResolveMediaContent_WithNilData_FallsThrough(t *testing.T) {
	server := NewServer(WithLogger(logr.Discard()))
	ctx := context.Background()

	httpURL := "https://example.com/image.png"
	media := &types.MediaContent{
		Data:     nil,
		URL:      &httpURL,
		MIMEType: "image/png",
	}

	result, err := server.resolveMediaContent(ctx, media)
	require.NoError(t, err)
	assert.Equal(t, httpURL, result.Url)
}

func TestResolveMediaContent_WithHTTPURL_Passthrough(t *testing.T) {
	server := NewServer(WithLogger(logr.Discard()))
	ctx := context.Background()

	url := "https://cdn.example.com/photo.jpg"
	media := &types.MediaContent{
		URL:      &url,
		MIMEType: "image/jpeg",
	}

	result, err := server.resolveMediaContent(ctx, media)
	require.NoError(t, err)
	assert.Equal(t, url, result.Url)
	assert.Equal(t, "image/jpeg", result.MimeType)
	assert.Empty(t, result.Data)
}

func TestResolveMediaContent_WithResolvableURL_NoResolver(t *testing.T) {
	// Server without media resolver
	server := NewServer(WithLogger(logr.Discard()))
	ctx := context.Background()

	url := "file:///some/file.png"
	media := &types.MediaContent{
		URL:      &url,
		MIMEType: "image/png",
	}

	_, err := server.resolveMediaContent(ctx, media)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "media resolver not configured")
}

func TestResolveMediaContent_WithResolvableURL_ResolveError(t *testing.T) {
	tmpDir := t.TempDir()
	server := NewServer(
		WithLogger(logr.Discard()),
		WithMediaBasePath(tmpDir),
	)
	ctx := context.Background()

	url := "file:///nonexistent/file.png"
	media := &types.MediaContent{
		URL:      &url,
		MIMEType: "image/png",
	}

	_, err := server.resolveMediaContent(ctx, media)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve media URL")
}

func TestResolveMediaContent_WithResolvableURL_Success(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.png")
	require.NoError(t, os.WriteFile(testFile, []byte("png data"), 0644))

	server := NewServer(
		WithLogger(logr.Discard()),
		WithMediaBasePath(tmpDir),
	)
	ctx := context.Background()

	url := "file://" + testFile
	media := &types.MediaContent{
		URL:      &url,
		MIMEType: "image/png",
	}

	result, err := server.resolveMediaContent(ctx, media)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Data)
	assert.Equal(t, "image/png", result.MimeType)
	assert.Empty(t, result.Url)
}

func TestResolveMediaContent_WithResolvableURL_HTTPPassthrough(t *testing.T) {
	tmpDir := t.TempDir()
	server := NewServer(
		WithLogger(logr.Discard()),
		WithMediaBasePath(tmpDir),
	)
	ctx := context.Background()

	// HTTP URLs go through resolver but come back as passthrough
	url := "http://example.com/image.png"
	media := &types.MediaContent{
		URL:      &url,
		MIMEType: "image/png",
	}

	result, err := server.resolveMediaContent(ctx, media)
	require.NoError(t, err)
	// Non-resolvable HTTP URL goes to the else branch (not IsResolvableURL)
	assert.Equal(t, url, result.Url)
	assert.Equal(t, "image/png", result.MimeType)
}

func TestResolveMediaContent_WithFilePath(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "audio.mp3")
	require.NoError(t, os.WriteFile(testFile, []byte("audio data"), 0644))

	server := NewServer(
		WithLogger(logr.Discard()),
		WithMediaBasePath(tmpDir),
	)
	ctx := context.Background()

	media := &types.MediaContent{
		FilePath: strPtr(testFile),
		MIMEType: "audio/mpeg",
	}

	result, err := server.resolveMediaContent(ctx, media)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Data)
	assert.Equal(t, "audio/mpeg", result.MimeType)
}

func TestResolveMediaContent_WithFilePath_Error(t *testing.T) {
	tmpDir := t.TempDir()
	server := NewServer(
		WithLogger(logr.Discard()),
		WithMediaBasePath(tmpDir),
	)
	ctx := context.Background()

	media := &types.MediaContent{
		FilePath: strPtr("/nonexistent/audio.mp3"),
		MIMEType: "audio/mpeg",
	}

	_, err := server.resolveMediaContent(ctx, media)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read media file")
}

func TestResolveMediaContent_NoDataSource(t *testing.T) {
	server := NewServer(WithLogger(logr.Discard()))
	ctx := context.Background()

	media := &types.MediaContent{
		MIMEType: "image/png",
	}

	_, err := server.resolveMediaContent(ctx, media)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no data source")
}

func TestResolveMediaContent_EmptyURL_FallsThrough(t *testing.T) {
	server := NewServer(WithLogger(logr.Discard()))
	ctx := context.Background()

	empty := ""
	media := &types.MediaContent{
		URL:      &empty,
		MIMEType: "image/png",
	}

	_, err := server.resolveMediaContent(ctx, media)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no data source")
}

func TestResolveMediaContent_EmptyFilePath_FallsThrough(t *testing.T) {
	server := NewServer(WithLogger(logr.Discard()))
	ctx := context.Background()

	empty := ""
	media := &types.MediaContent{
		FilePath: &empty,
		MIMEType: "image/png",
	}

	_, err := server.resolveMediaContent(ctx, media)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no data source")
}

// --- buildSendOptions tests ---

func TestBuildSendOptions_NilParts(t *testing.T) {
	result := buildSendOptions(nil, logr.Discard())
	assert.Nil(t, result)
}

func TestBuildSendOptions_EmptyParts(t *testing.T) {
	result := buildSendOptions([]*runtimev1.ContentPart{}, logr.Discard())
	assert.Nil(t, result)
}

func TestBuildSendOptions_NilMedia_Skipped(t *testing.T) {
	parts := []*runtimev1.ContentPart{
		{Type: "text", Text: "hello", Media: nil},
	}
	result := buildSendOptions(parts, logr.Discard())
	assert.Nil(t, result)
}

func TestBuildSendOptions_ImageData(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("image bytes"))
	parts := []*runtimev1.ContentPart{
		{
			Type: "image",
			Media: &runtimev1.MediaContent{
				Data:     b64,
				MimeType: "image/png",
			},
		},
	}
	result := buildSendOptions(parts, logr.Discard())
	require.Len(t, result, 1)
	assert.NotNil(t, result[0])
}

func TestBuildSendOptions_ImageURL(t *testing.T) {
	parts := []*runtimev1.ContentPart{
		{
			Type: "image",
			Media: &runtimev1.MediaContent{
				Url:      "https://example.com/image.png",
				MimeType: "image/png",
			},
		},
	}
	result := buildSendOptions(parts, logr.Discard())
	require.Len(t, result, 1)
	assert.NotNil(t, result[0])
}

func TestBuildSendOptions_AudioData(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("audio bytes"))
	parts := []*runtimev1.ContentPart{
		{
			Type: "audio",
			Media: &runtimev1.MediaContent{
				Data:     b64,
				MimeType: "audio/mpeg",
			},
		},
	}
	result := buildSendOptions(parts, logr.Discard())
	require.Len(t, result, 1)
	assert.NotNil(t, result[0])
}

func TestBuildSendOptions_AudioURL(t *testing.T) {
	parts := []*runtimev1.ContentPart{
		{
			Type: "audio",
			Media: &runtimev1.MediaContent{
				Url:      "https://example.com/audio.mp3",
				MimeType: "audio/mpeg",
			},
		},
	}
	result := buildSendOptions(parts, logr.Discard())
	require.Len(t, result, 1)
	assert.NotNil(t, result[0])
}

func TestBuildSendOptions_FileData(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("pdf bytes"))
	parts := []*runtimev1.ContentPart{
		{
			Type: "document",
			Media: &runtimev1.MediaContent{
				Data:     b64,
				MimeType: "application/pdf",
			},
		},
	}
	result := buildSendOptions(parts, logr.Discard())
	require.Len(t, result, 1)
	assert.NotNil(t, result[0])
}

func TestBuildSendOptions_MultipleParts(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("data"))
	parts := []*runtimev1.ContentPart{
		{Type: "text", Text: "hello", Media: nil},
		{Type: "image", Media: &runtimev1.MediaContent{Data: b64, MimeType: "image/png"}},
		{Type: "audio", Media: &runtimev1.MediaContent{Data: b64, MimeType: "audio/mpeg"}},
	}
	result := buildSendOptions(parts, logr.Discard())
	// text part is skipped (nil media), image and audio produce options
	require.Len(t, result, 2)
}

// --- processMediaPart tests ---

func TestProcessMediaPart_Image(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("img"))
	media := &runtimev1.MediaContent{Data: b64, MimeType: "image/png"}
	opt := processMediaPart(media, logr.Discard())
	assert.NotNil(t, opt)
}

func TestProcessMediaPart_Audio(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("aud"))
	media := &runtimev1.MediaContent{Data: b64, MimeType: "audio/wav"}
	opt := processMediaPart(media, logr.Discard())
	assert.NotNil(t, opt)
}

func TestProcessMediaPart_File(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("file"))
	media := &runtimev1.MediaContent{Data: b64, MimeType: "application/pdf"}
	opt := processMediaPart(media, logr.Discard())
	assert.NotNil(t, opt)
}

// --- processImageMedia tests ---

func TestProcessImageMedia_WithData(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("image bytes"))
	media := &runtimev1.MediaContent{Data: b64, MimeType: "image/png"}
	opt := processImageMedia(media, logr.Discard())
	assert.NotNil(t, opt)
}

func TestProcessImageMedia_WithURL(t *testing.T) {
	media := &runtimev1.MediaContent{Url: "https://example.com/img.jpg", MimeType: "image/jpeg"}
	opt := processImageMedia(media, logr.Discard())
	assert.NotNil(t, opt)
}

func TestProcessImageMedia_NoDataNoURL(t *testing.T) {
	media := &runtimev1.MediaContent{MimeType: "image/png"}
	opt := processImageMedia(media, logr.Discard())
	assert.Nil(t, opt)
}

func TestProcessImageMedia_InvalidBase64(t *testing.T) {
	media := &runtimev1.MediaContent{Data: "not-valid-base64!!!", MimeType: "image/png"}
	opt := processImageMedia(media, logr.Discard())
	assert.Nil(t, opt)
}

// --- processAudioMedia tests ---

func TestProcessAudioMedia_WithData(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("audio bytes"))
	media := &runtimev1.MediaContent{Data: b64, MimeType: "audio/mpeg"}
	opt := processAudioMedia(media, logr.Discard())
	assert.NotNil(t, opt)
}

func TestProcessAudioMedia_WithURL(t *testing.T) {
	media := &runtimev1.MediaContent{Url: "https://example.com/song.mp3", MimeType: "audio/mpeg"}
	opt := processAudioMedia(media, logr.Discard())
	assert.NotNil(t, opt)
}

func TestProcessAudioMedia_NoDataNoURL(t *testing.T) {
	media := &runtimev1.MediaContent{MimeType: "audio/mpeg"}
	opt := processAudioMedia(media, logr.Discard())
	assert.Nil(t, opt)
}

func TestProcessAudioMedia_InvalidBase64(t *testing.T) {
	media := &runtimev1.MediaContent{Data: "not-valid-base64!!!", MimeType: "audio/mpeg"}
	opt := processAudioMedia(media, logr.Discard())
	assert.Nil(t, opt)
}

// --- processFileMedia tests ---

func TestProcessFileMedia_WithData(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("pdf content"))
	media := &runtimev1.MediaContent{Data: b64, MimeType: "application/pdf"}
	opt := processFileMedia(media, logr.Discard())
	assert.NotNil(t, opt)
}

func TestProcessFileMedia_NoData(t *testing.T) {
	media := &runtimev1.MediaContent{MimeType: "application/pdf"}
	opt := processFileMedia(media, logr.Discard())
	assert.Nil(t, opt)
}

func TestProcessFileMedia_InvalidBase64(t *testing.T) {
	media := &runtimev1.MediaContent{Data: "not-valid-base64!!!", MimeType: "application/pdf"}
	opt := processFileMedia(media, logr.Discard())
	assert.Nil(t, opt)
}

func TestProcessFileMedia_EmptyData(t *testing.T) {
	media := &runtimev1.MediaContent{Data: "", MimeType: "application/pdf"}
	opt := processFileMedia(media, logr.Discard())
	// Empty string means Data == "" so returns nil
	assert.Nil(t, opt)
}
