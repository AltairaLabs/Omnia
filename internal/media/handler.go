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

package media

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
)

// HTTP handler constants to avoid string duplication.
const (
	errMethodNotAllowed = "method not allowed"
	errInvalidPath      = "invalid path"
	errFailedToEncode   = "failed to encode response"
	contentTypeJSON     = "application/json"
	headerContentType   = "Content-Type"
	pathMediaDownload   = "/media/download/"
)

// HandlerMetrics defines the metrics interface for the media handler.
type HandlerMetrics interface {
	UploadStarted()
	UploadCompleted(bytes int64, durationSeconds float64)
	UploadFailed()
	DownloadStarted()
	DownloadCompleted(bytes int64)
	DownloadFailed()
}

// noOpMetrics is a no-op metrics implementation for when metrics are disabled.
type noOpMetrics struct{}

func (n *noOpMetrics) UploadStarted() {
	// Intentionally empty - metrics are disabled
}
func (n *noOpMetrics) UploadCompleted(int64, float64) {
	// Intentionally empty - metrics are disabled
}
func (n *noOpMetrics) UploadFailed() {
	// Intentionally empty - metrics are disabled
}
func (n *noOpMetrics) DownloadStarted() {
	// Intentionally empty - metrics are disabled
}
func (n *noOpMetrics) DownloadCompleted(int64) {
	// Intentionally empty - metrics are disabled
}
func (n *noOpMetrics) DownloadFailed() {
	// Intentionally empty - metrics are disabled
}

// Handler provides HTTP endpoints for media upload and download.
type Handler struct {
	storage Storage
	log     logr.Logger
	metrics HandlerMetrics
	// proxyStorage is set when storage implements ProxyUploadStorage (e.g., LocalStorage)
	proxyStorage ProxyUploadStorage
	// directStorage is set when storage implements DirectUploadStorage (e.g., S3, GCS)
	directStorage DirectUploadStorage
}

// HandlerOption is a functional option for configuring the handler.
type HandlerOption func(*Handler)

// WithHandlerMetrics sets the metrics for the handler.
func WithHandlerMetrics(m HandlerMetrics) HandlerOption {
	return func(h *Handler) {
		h.metrics = m
	}
}

// NewHandler creates a new media HTTP handler.
func NewHandler(storage Storage, log logr.Logger, opts ...HandlerOption) *Handler {
	h := &Handler{
		storage: storage,
		log:     log.WithName("media-handler"),
		metrics: &noOpMetrics{},
	}

	// Check which extended interfaces the storage implements
	if ps, ok := storage.(ProxyUploadStorage); ok {
		h.proxyStorage = ps
	}
	if ds, ok := storage.(DirectUploadStorage); ok {
		h.directStorage = ds
	}

	for _, opt := range opts {
		opt(h)
	}
	return h
}

// RegisterRoutes registers the media routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Common routes for all storage types
	mux.HandleFunc("/media/request-upload", h.handleRequestUpload)
	mux.HandleFunc("/media/info/", h.handleInfo)

	// Routes for proxy storage (LocalStorage) - uploads go through facade
	if h.proxyStorage != nil {
		mux.HandleFunc("/media/upload/", h.handleUpload)
		mux.HandleFunc(pathMediaDownload, h.handleDownload)
	}

	// Routes for direct storage (S3/GCS) - uploads go directly to cloud
	if h.directStorage != nil {
		mux.HandleFunc("/media/confirm-upload/", h.handleConfirmUpload)
		// For direct storage, download returns a redirect to presigned URL
		if h.proxyStorage == nil {
			mux.HandleFunc(pathMediaDownload, h.handleCloudDownload)
		}
	}
}

// handleRequestUpload generates presigned upload credentials.
// POST /media/request-upload
// Body: {"session_id": "...", "filename": "...", "mime_type": "...", "size_bytes": 123}
func (h *Handler) handleRequestUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, errMethodNotAllowed, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
		Filename  string `json:"filename"`
		MIMEType  string `json:"mime_type"`
		SizeBytes int64  `json:"size_bytes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error(err, "failed to decode request body")
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	creds, err := h.storage.GetUploadURL(r.Context(), UploadRequest{
		SessionID: req.SessionID,
		Filename:  req.Filename,
		MIMEType:  req.MIMEType,
		SizeBytes: req.SizeBytes,
	})
	if err != nil {
		h.log.Error(err, "failed to generate upload URL")
		h.writeError(w, err)
		return
	}

	w.Header().Set(headerContentType, contentTypeJSON)
	if err := json.NewEncoder(w).Encode(creds); err != nil {
		h.log.Error(err, errFailedToEncode)
	}
}

// handleUpload receives uploaded media content.
// PUT /media/upload/{upload-id}
func (h *Handler) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, errMethodNotAllowed, http.StatusMethodNotAllowed)
		return
	}

	startTime := time.Now()
	h.metrics.UploadStarted()

	// Extract upload ID from path
	uploadID := strings.TrimPrefix(r.URL.Path, "/media/upload/")
	if uploadID == "" {
		h.metrics.UploadFailed()
		http.Error(w, "upload ID required", http.StatusBadRequest)
		return
	}

	// Get the file path for this upload (only works with proxy storage)
	if h.proxyStorage == nil {
		h.metrics.UploadFailed()
		http.Error(w, "direct upload not supported for this storage type", http.StatusBadRequest)
		return
	}

	filePath, err := h.proxyStorage.GetUploadPath(uploadID)
	if err != nil {
		h.metrics.UploadFailed()
		h.log.Error(err, "failed to get upload path", "uploadID", uploadID)
		h.writeError(w, err)
		return
	}

	// Create the file and copy the request body
	file, err := createFile(filePath)
	if err != nil {
		h.metrics.UploadFailed()
		h.log.Error(err, "failed to create file", "path", filePath)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			h.log.Error(cerr, "failed to close file", "path", filePath)
		}
	}()

	written, err := io.Copy(file, r.Body)
	if err != nil {
		h.metrics.UploadFailed()
		h.log.Error(err, "failed to write file", "path", filePath)
		http.Error(w, "failed to store file", http.StatusInternalServerError)
		return
	}

	// Complete the upload
	if err := h.proxyStorage.CompleteUpload(r.Context(), uploadID, written); err != nil {
		h.metrics.UploadFailed()
		h.log.Error(err, "failed to complete upload", "uploadID", uploadID)
		h.writeError(w, err)
		return
	}

	duration := time.Since(startTime).Seconds()
	h.metrics.UploadCompleted(written, duration)
	h.log.Info("upload completed", "uploadID", uploadID, "bytes", written, "duration_seconds", duration)
	w.WriteHeader(http.StatusNoContent)
}

// parseMediaPath extracts session ID and media ID from a path like "/{prefix}/{session-id}/{media-id}".
// Returns the StorageRef or an error if the path is invalid.
func parseMediaPath(urlPath, prefix string) (*StorageRef, error) {
	path := strings.TrimPrefix(urlPath, prefix)
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, ErrInvalidStorageRef
	}
	return &StorageRef{SessionID: parts[0], MediaID: parts[1]}, nil
}

// handleDownload serves media content.
// GET /media/download/{session-id}/{media-id}
func (h *Handler) handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, errMethodNotAllowed, http.StatusMethodNotAllowed)
		return
	}

	h.metrics.DownloadStarted()

	ref, err := parseMediaPath(r.URL.Path, pathMediaDownload)
	if err != nil {
		h.metrics.DownloadFailed()
		http.Error(w, errInvalidPath, http.StatusBadRequest)
		return
	}

	info, err := h.storage.GetMediaInfo(r.Context(), ref.String())
	if err != nil {
		h.metrics.DownloadFailed()
		h.log.Error(err, "failed to get media info", "ref", ref.String())
		h.writeError(w, err)
		return
	}

	filePath, err := h.proxyStorage.GetMediaPath(ref.String())
	if err != nil {
		h.metrics.DownloadFailed()
		h.log.Error(err, "failed to get media path", "ref", ref.String())
		h.writeError(w, err)
		return
	}

	w.Header().Set("Content-Type", info.MIMEType)
	if info.Filename != "" {
		w.Header().Set("Content-Disposition", "inline; filename=\""+info.Filename+"\"")
	}
	if info.SizeBytes > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(info.SizeBytes, 10))
	}

	h.metrics.DownloadCompleted(info.SizeBytes)
	http.ServeFile(w, r, filePath)
}

// handleConfirmUpload confirms that a direct upload completed (S3/GCS).
// POST /media/confirm-upload/{upload-id}
func (h *Handler) handleConfirmUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, errMethodNotAllowed, http.StatusMethodNotAllowed)
		return
	}

	startTime := time.Now()
	h.metrics.UploadStarted()

	uploadID := strings.TrimPrefix(r.URL.Path, "/media/confirm-upload/")
	if uploadID == "" {
		h.metrics.UploadFailed()
		http.Error(w, "upload ID required", http.StatusBadRequest)
		return
	}

	if h.directStorage == nil {
		h.metrics.UploadFailed()
		http.Error(w, "confirm not supported for this storage type", http.StatusBadRequest)
		return
	}

	info, err := h.directStorage.ConfirmUpload(r.Context(), uploadID)
	if err != nil {
		h.metrics.UploadFailed()
		h.log.Error(err, "failed to confirm upload", "uploadID", uploadID)
		h.writeError(w, err)
		return
	}

	duration := time.Since(startTime).Seconds()
	h.metrics.UploadCompleted(info.SizeBytes, duration)
	h.log.Info("upload confirmed", "uploadID", uploadID, "bytes", info.SizeBytes)

	w.Header().Set(headerContentType, contentTypeJSON)
	if err := json.NewEncoder(w).Encode(info); err != nil {
		h.log.Error(err, errFailedToEncode)
	}
}

// handleCloudDownload returns a presigned download URL or redirects to it (S3/GCS).
// GET /media/download/{session-id}/{media-id}
func (h *Handler) handleCloudDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, errMethodNotAllowed, http.StatusMethodNotAllowed)
		return
	}

	h.metrics.DownloadStarted()

	ref, err := parseMediaPath(r.URL.Path, pathMediaDownload)
	if err != nil {
		h.metrics.DownloadFailed()
		http.Error(w, errInvalidPath, http.StatusBadRequest)
		return
	}

	downloadURL, err := h.storage.GetDownloadURL(r.Context(), ref.String())
	if err != nil {
		h.metrics.DownloadFailed()
		h.log.Error(err, "failed to get download URL", "ref", ref.String())
		h.writeError(w, err)
		return
	}

	// Redirect to the presigned URL
	h.metrics.DownloadCompleted(0) // Size not known until client downloads
	http.Redirect(w, r, downloadURL, http.StatusTemporaryRedirect)
}

// handleInfo returns metadata about stored media.
// GET /media/info/{session-id}/{media-id}
func (h *Handler) handleInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, errMethodNotAllowed, http.StatusMethodNotAllowed)
		return
	}

	ref, err := parseMediaPath(r.URL.Path, "/media/info/")
	if err != nil {
		http.Error(w, errInvalidPath, http.StatusBadRequest)
		return
	}

	info, err := h.storage.GetMediaInfo(r.Context(), ref.String())
	if err != nil {
		h.log.Error(err, "failed to get media info", "ref", ref.String())
		h.writeError(w, err)
		return
	}

	w.Header().Set(headerContentType, contentTypeJSON)
	if err := json.NewEncoder(w).Encode(info); err != nil {
		h.log.Error(err, errFailedToEncode)
	}
}

// writeError writes an appropriate HTTP error based on the error type.
func (h *Handler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrMediaNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrMediaExpired):
		http.Error(w, "media expired", http.StatusGone)
	case errors.Is(err, ErrInvalidStorageRef):
		http.Error(w, "invalid storage reference", http.StatusBadRequest)
	case errors.Is(err, ErrUploadFailed):
		http.Error(w, "upload failed", http.StatusBadRequest)
	case errors.Is(err, ErrFileTooLarge):
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
	case errors.Is(err, ErrUnsupportedMIMEType):
		http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// createFile creates a file for writing, imported from os for testability.
var createFile = func(path string) (io.WriteCloser, error) {
	return createFileImpl(path)
}
