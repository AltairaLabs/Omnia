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
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// URL scheme prefixes for media resolution.
const (
	schemeFile  = "file://"
	schemeMock  = "mock://"
	schemeHTTP  = "http://"
	schemeHTTPS = "https://"
)

// MediaResolver resolves media URLs to base64-encoded data.
type MediaResolver struct {
	mediaBasePath string
}

// NewMediaResolver creates a new MediaResolver with the given base path.
func NewMediaResolver(mediaBasePath string) *MediaResolver {
	return &MediaResolver{
		mediaBasePath: mediaBasePath,
	}
}

// ResolveURL resolves a media URL to base64-encoded data and MIME type.
// Supports the following URL schemes:
//   - file:// - Reads from the local filesystem
//   - mock:// - Resolves against the configured media base path
//   - http:// and https:// - Returns empty data (URL should be used directly)
//
// Returns:
//   - base64Data: Base64-encoded file contents (empty for http/https URLs)
//   - mimeType: Inferred MIME type from file extension
//   - isPassthrough: True if the URL should be passed through unchanged (http/https)
//   - error: Any error encountered during resolution
func (r *MediaResolver) ResolveURL(url string) (base64Data, mimeType string, isPassthrough bool, err error) {
	switch {
	case strings.HasPrefix(url, schemeFile):
		path := strings.TrimPrefix(url, schemeFile)
		return r.resolveLocalFile(path)

	case strings.HasPrefix(url, schemeMock):
		filename := strings.TrimPrefix(url, schemeMock)
		path := filepath.Join(r.mediaBasePath, filename)
		return r.resolveLocalFile(path)

	case strings.HasPrefix(url, schemeHTTP), strings.HasPrefix(url, schemeHTTPS):
		// HTTP/HTTPS URLs are passed through unchanged
		mimeType := inferMIMETypeFromExtension(url)
		return "", mimeType, true, nil

	default:
		return "", "", false, fmt.Errorf("unsupported URL scheme: %s", url)
	}
}

// resolveLocalFile reads a local file and returns its base64-encoded contents.
func (r *MediaResolver) resolveLocalFile(path string) (base64Data, mimeType string, isPassthrough bool, err error) {
	// Validate the path exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", "", false, fmt.Errorf("media file not found: %s", path)
	}

	// Read the file
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", false, fmt.Errorf("failed to read media file %s: %w", path, err)
	}

	// Encode to base64
	base64Data = base64.StdEncoding.EncodeToString(data)

	// Infer MIME type from extension
	mimeType = inferMIMETypeFromExtension(path)

	return base64Data, mimeType, false, nil
}

// inferMIMETypeFromExtension infers the MIME type from a file path or URL extension.
func inferMIMETypeFromExtension(path string) string {
	// Strip query parameters from URLs
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	// Images
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".bmp":
		return "image/bmp"
	case ".ico":
		return "image/x-icon"

	// Audio
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".ogg", ".oga":
		return "audio/ogg"
	case ".flac":
		return "audio/flac"
	case ".aac":
		return "audio/aac"
	case ".m4a":
		return "audio/mp4"
	case ".weba":
		return "audio/webm"

	// Video
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".ogv":
		return "video/ogg"
	case ".avi":
		return "video/x-msvideo"
	case ".mov":
		return "video/quicktime"
	case ".mkv":
		return "video/x-matroska"

	// Documents
	case ".pdf":
		return "application/pdf"

	default:
		return "application/octet-stream"
	}
}

// IsMediaURL checks if a URL is a media URL that can be resolved.
func IsMediaURL(url string) bool {
	return strings.HasPrefix(url, schemeFile) ||
		strings.HasPrefix(url, schemeMock) ||
		strings.HasPrefix(url, schemeHTTP) ||
		strings.HasPrefix(url, schemeHTTPS)
}

// IsResolvableURL checks if a URL needs to be resolved (file:// or mock://).
// HTTP/HTTPS URLs don't need resolution - they're passed through.
func IsResolvableURL(url string) bool {
	return strings.HasPrefix(url, schemeFile) || strings.HasPrefix(url, schemeMock)
}
