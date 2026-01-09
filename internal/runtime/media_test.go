/*
Copyright 2025.

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
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMediaResolver(t *testing.T) {
	resolver := NewMediaResolver("/test/media")
	assert.NotNil(t, resolver)
	assert.Equal(t, "/test/media", resolver.mediaBasePath)
}

func TestMediaResolver_ResolveURL_FileScheme(t *testing.T) {
	// Create a temp file with test content
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.png")
	testContent := []byte("fake png content")
	err := os.WriteFile(testFile, testContent, 0644)
	require.NoError(t, err)

	resolver := NewMediaResolver(tmpDir)

	// Test file:// URL resolution
	base64Data, mimeType, isPassthrough, err := resolver.ResolveURL("file://" + testFile)
	require.NoError(t, err)
	assert.False(t, isPassthrough)
	assert.Equal(t, "image/png", mimeType)

	// Verify base64 decoding
	decoded, err := base64.StdEncoding.DecodeString(base64Data)
	require.NoError(t, err)
	assert.Equal(t, testContent, decoded)
}

func TestMediaResolver_ResolveURL_MockScheme(t *testing.T) {
	// Create a temp dir as media base path
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "response.jpg")
	testContent := []byte("fake jpeg content")
	err := os.WriteFile(testFile, testContent, 0644)
	require.NoError(t, err)

	resolver := NewMediaResolver(tmpDir)

	// Test mock:// URL resolution
	base64Data, mimeType, isPassthrough, err := resolver.ResolveURL("mock://response.jpg")
	require.NoError(t, err)
	assert.False(t, isPassthrough)
	assert.Equal(t, "image/jpeg", mimeType)

	// Verify base64 decoding
	decoded, err := base64.StdEncoding.DecodeString(base64Data)
	require.NoError(t, err)
	assert.Equal(t, testContent, decoded)
}

func TestMediaResolver_ResolveURL_HTTPPassthrough(t *testing.T) {
	resolver := NewMediaResolver("/test/media")

	testCases := []struct {
		name     string
		url      string
		mimeType string
	}{
		{"http URL", "http://example.com/image.png", "image/png"},
		{"https URL", "https://example.com/audio.mp3", "audio/mpeg"},
		{"http with query params", "http://cdn.example.com/video.mp4?token=abc", "video/mp4"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			base64Data, mimeType, isPassthrough, err := resolver.ResolveURL(tc.url)
			require.NoError(t, err)
			assert.True(t, isPassthrough, "HTTP URLs should be passthrough")
			assert.Empty(t, base64Data, "HTTP URLs should not have base64 data")
			assert.Equal(t, tc.mimeType, mimeType)
		})
	}
}

func TestMediaResolver_ResolveURL_MissingFile(t *testing.T) {
	resolver := NewMediaResolver("/test/media")

	_, _, _, err := resolver.ResolveURL("file:///nonexistent/file.png")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestMediaResolver_ResolveURL_UnsupportedScheme(t *testing.T) {
	resolver := NewMediaResolver("/test/media")

	_, _, _, err := resolver.ResolveURL("ftp://example.com/file.png")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported URL scheme")
}

func TestInferMIMETypeFromExtension(t *testing.T) {
	testCases := []struct {
		path     string
		expected string
	}{
		// Images
		{"/path/to/image.jpg", "image/jpeg"},
		{"/path/to/image.jpeg", "image/jpeg"},
		{"/path/to/image.png", "image/png"},
		{"/path/to/image.gif", "image/gif"},
		{"/path/to/image.webp", "image/webp"},
		{"/path/to/image.svg", "image/svg+xml"},
		{"/path/to/image.bmp", "image/bmp"},
		{"/path/to/image.ico", "image/x-icon"},

		// Audio
		{"/path/to/audio.mp3", "audio/mpeg"},
		{"/path/to/audio.wav", "audio/wav"},
		{"/path/to/audio.ogg", "audio/ogg"},
		{"/path/to/audio.flac", "audio/flac"},
		{"/path/to/audio.aac", "audio/aac"},
		{"/path/to/audio.m4a", "audio/mp4"},
		{"/path/to/audio.weba", "audio/webm"},

		// Video
		{"/path/to/video.mp4", "video/mp4"},
		{"/path/to/video.webm", "video/webm"},
		{"/path/to/video.ogv", "video/ogg"},
		{"/path/to/video.avi", "video/x-msvideo"},
		{"/path/to/video.mov", "video/quicktime"},
		{"/path/to/video.mkv", "video/x-matroska"},

		// Documents
		{"/path/to/doc.pdf", "application/pdf"},

		// Unknown
		{"/path/to/file.xyz", "application/octet-stream"},
		{"/path/to/noextension", "application/octet-stream"},

		// Case insensitive
		{"/path/to/IMAGE.PNG", "image/png"},
		{"/path/to/AUDIO.MP3", "audio/mpeg"},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			result := inferMIMETypeFromExtension(tc.path)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestIsMediaURL(t *testing.T) {
	testCases := []struct {
		url      string
		expected bool
	}{
		{"file:///path/to/file.png", true},
		{"mock://response.jpg", true},
		{"http://example.com/image.png", true},
		{"https://example.com/audio.mp3", true},
		{"ftp://example.com/file.png", false},
		{"data:image/png;base64,abc", false},
		{"relative/path.png", false},
		{"", false},
	}

	for _, tc := range testCases {
		t.Run(tc.url, func(t *testing.T) {
			result := IsMediaURL(tc.url)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestIsResolvableURL(t *testing.T) {
	testCases := []struct {
		url      string
		expected bool
	}{
		{"file:///path/to/file.png", true},
		{"mock://response.jpg", true},
		{"http://example.com/image.png", false},
		{"https://example.com/audio.mp3", false},
		{"ftp://example.com/file.png", false},
		{"", false},
	}

	for _, tc := range testCases {
		t.Run(tc.url, func(t *testing.T) {
			result := IsResolvableURL(tc.url)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestMediaResolver_ResolveURL_AudioFiles(t *testing.T) {
	tmpDir := t.TempDir()

	testCases := []struct {
		filename string
		mimeType string
	}{
		{"audio.mp3", "audio/mpeg"},
		{"audio.wav", "audio/wav"},
		{"audio.ogg", "audio/ogg"},
	}

	for _, tc := range testCases {
		t.Run(tc.filename, func(t *testing.T) {
			testFile := filepath.Join(tmpDir, tc.filename)
			err := os.WriteFile(testFile, []byte("fake audio"), 0644)
			require.NoError(t, err)

			resolver := NewMediaResolver(tmpDir)
			_, mimeType, isPassthrough, err := resolver.ResolveURL("mock://" + tc.filename)
			require.NoError(t, err)
			assert.False(t, isPassthrough)
			assert.Equal(t, tc.mimeType, mimeType)
		})
	}
}

func TestMediaResolver_ResolveURL_VideoFiles(t *testing.T) {
	tmpDir := t.TempDir()

	testCases := []struct {
		filename string
		mimeType string
	}{
		{"video.mp4", "video/mp4"},
		{"video.webm", "video/webm"},
		{"video.ogv", "video/ogg"},
	}

	for _, tc := range testCases {
		t.Run(tc.filename, func(t *testing.T) {
			testFile := filepath.Join(tmpDir, tc.filename)
			err := os.WriteFile(testFile, []byte("fake video"), 0644)
			require.NoError(t, err)

			resolver := NewMediaResolver(tmpDir)
			_, mimeType, isPassthrough, err := resolver.ResolveURL("mock://" + tc.filename)
			require.NoError(t, err)
			assert.False(t, isPassthrough)
			assert.Equal(t, tc.mimeType, mimeType)
		})
	}
}
