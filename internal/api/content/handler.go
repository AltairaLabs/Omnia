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

package content

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/api/authz"
	"github.com/altairalabs/omnia/internal/httputil"
)

// pathVarPath is the ServeMux wildcard carrying the client-supplied path
// relative to the workspace's content subtree, e.g.
// /api/v1/workspaces/{workspace}/content/{path...}.
const pathVarPath = "path"

// maxFileSize bounds a single read or write to keep one request from buffering
// an unbounded amount of NFS content into operator memory.
const maxFileSize = 10 << 20 // 10 MiB

// dirPerm / filePerm are the modes new content is created with. They match the
// uniform 65532-owned layout the other server-side writers produce.
const (
	dirPerm  os.FileMode = 0o755
	filePerm os.FileMode = 0o644
)

// Entry is a single directory entry in a Listing.
type Entry struct {
	Name       string `json:"name"`
	Type       string `json:"type"` // "file" | "directory"
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modifiedAt"` // RFC3339
}

// Listing is the response for a GET on a directory.
type Listing struct {
	Path    string  `json:"path"`
	Entries []Entry `json:"entries"`
}

// FileContent is the response for a GET on a file.
type FileContent struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	Encoding   string `json:"encoding"` // "utf-8" | "base64"
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modifiedAt"`
}

// WriteResult is the response for a PUT (file write) or POST (mkdir).
type WriteResult struct {
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modifiedAt"`
	Directory  bool   `json:"directory,omitempty"`
}

// Handler serves confined filesystem operations under contentRoot. Each request
// is scoped to <contentRoot>/<workspace>/<namespace>/ using the workspace and
// namespace from the authz RequestIdentity (set by the authz middleware).
type Handler struct {
	contentRoot string
	log         logr.Logger
}

// NewHandler constructs a content Handler rooted at contentRoot (the operator's
// mount of the shared workspace-content volume).
func NewHandler(contentRoot string, log logr.Logger) *Handler {
	return &Handler{contentRoot: contentRoot, log: log}
}

// workspaceBase validates the request identity and ensures the caller's
// workspace content subtree exists, returning its absolute path. On failure it
// returns a non-200 status and message for the caller to surface.
func (h *Handler) workspaceBase(r *http.Request) (base string, status int, msg string) {
	id, ok := authz.IdentityFromContext(r.Context())
	if !ok || id.Workspace == "" || id.Namespace == "" {
		return "", http.StatusInternalServerError, "missing request identity"
	}
	base = filepath.Join(h.contentRoot, id.Workspace, id.Namespace)
	if err := os.MkdirAll(base, dirPerm); err != nil {
		h.log.Error(err, "ensure workspace content dir", "base", base)
		return "", http.StatusInternalServerError, "content storage unavailable"
	}
	return base, http.StatusOK, ""
}

// confine resolves relpath within base, mapping a path escape to 400 and any
// other resolution error to 500.
func (h *Handler) confine(base, relpath string) (target string, status int, msg string) {
	resolved, err := Confine(base, relpath)
	if err != nil {
		if errors.Is(err, ErrPathEscape) {
			return "", http.StatusBadRequest, "invalid path"
		}
		h.log.Error(err, "confine path", "relpath", relpath)
		return "", http.StatusInternalServerError, "path resolution failed"
	}
	return resolved, http.StatusOK, ""
}

// resolveTarget validates the request identity, ensures the workspace content
// subtree exists, and confines the request path within it. On failure it
// returns a non-200 status and message for the caller to surface.
func (h *Handler) resolveTarget(r *http.Request) (target, relpath string, status int, msg string) {
	base, status, msg := h.workspaceBase(r)
	if status != http.StatusOK {
		return "", "", status, msg
	}
	relpath = r.PathValue(pathVarPath)
	target, status, msg = h.confine(base, relpath)
	if status != http.StatusOK {
		return "", "", status, msg
	}
	return target, relpath, http.StatusOK, ""
}

// Get lists a directory or returns a file's content.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	target, relpath, status, msg := h.resolveTarget(r)
	if status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	info, err := os.Stat(target)
	if errors.Is(err, os.ErrNotExist) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		h.fail(w, err, "stat")
		return
	}
	if info.IsDir() {
		h.writeListing(w, target, relpath)
		return
	}
	h.writeFileContent(w, target, relpath, info)
}

func (h *Handler) writeListing(w http.ResponseWriter, target, relpath string) {
	dirEntries, err := os.ReadDir(target)
	if err != nil {
		h.fail(w, err, "readdir")
		return
	}
	listing := Listing{Path: relpath, Entries: make([]Entry, 0, len(dirEntries))}
	for _, de := range dirEntries {
		info, err := de.Info()
		if err != nil {
			continue
		}
		listing.Entries = append(listing.Entries, entryFor(de.Name(), info))
	}
	h.writeJSON(w, http.StatusOK, listing)
}

func (h *Handler) writeFileContent(w http.ResponseWriter, target, relpath string, info os.FileInfo) {
	if info.Size() > maxFileSize {
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		return
	}
	data, err := os.ReadFile(target)
	if err != nil {
		h.fail(w, err, "readfile")
		return
	}
	content, encoding := encodeContent(data)
	h.writeJSON(w, http.StatusOK, FileContent{
		Path:       relpath,
		Content:    content,
		Encoding:   encoding,
		Size:       info.Size(),
		ModifiedAt: modTime(info),
	})
}

// Put writes (creates or overwrites) a file with the request body as content.
func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	target, relpath, status, msg := h.resolveTarget(r)
	if status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		http.Error(w, "path is a directory", http.StatusConflict)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxFileSize+1))
	if err != nil {
		h.fail(w, err, "read body")
		return
	}
	if len(body) > maxFileSize {
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		return
	}
	if err := os.MkdirAll(filepath.Dir(target), dirPerm); err != nil {
		h.fail(w, err, "mkdir parent")
		return
	}
	if err := writeFileNoFollow(target, body); err != nil {
		if errors.Is(err, syscall.ELOOP) {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		h.fail(w, err, "write")
		return
	}
	info, err := os.Stat(target)
	if err != nil {
		h.fail(w, err, "stat after write")
		return
	}
	h.writeJSON(w, http.StatusOK, WriteResult{Path: relpath, Size: info.Size(), ModifiedAt: modTime(info)})
}

// moveRequest is the body for a Move (rename) request: the destination path
// relative to the same workspace content subtree as the source.
type moveRequest struct {
	To string `json:"to"`
}

// Move renames (or moves) the source path to the destination given in the
// request body. Both endpoints are confined to the caller's workspace subtree.
// It refuses to overwrite an existing destination (409) and creates missing
// parent directories of the destination.
func (h *Handler) Move(w http.ResponseWriter, r *http.Request) {
	base, status, msg := h.workspaceBase(r)
	if status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	src, status, msg := h.confine(base, r.PathValue(pathVarPath))
	if status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}

	var body moveRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxFileSize)).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.To == "" {
		http.Error(w, "destination required", http.StatusBadRequest)
		return
	}
	dst, status, msg := h.confine(base, body.To)
	if status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}

	if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	} else if err != nil {
		h.fail(w, err, "stat source")
		return
	}
	if _, err := os.Stat(dst); err == nil {
		http.Error(w, "destination exists", http.StatusConflict)
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		h.fail(w, err, "stat destination")
		return
	}

	if err := os.MkdirAll(filepath.Dir(dst), dirPerm); err != nil {
		h.fail(w, err, "mkdir destination parent")
		return
	}
	if err := os.Rename(src, dst); err != nil {
		h.fail(w, err, "rename")
		return
	}
	info, err := os.Stat(dst)
	if err != nil {
		h.fail(w, err, "stat after rename")
		return
	}
	h.writeJSON(w, http.StatusOK, WriteResult{
		Path:       body.To,
		Size:       info.Size(),
		ModifiedAt: modTime(info),
		Directory:  info.IsDir(),
	})
}

// MkDir creates a directory (and any missing parents) at the target path.
func (h *Handler) MkDir(w http.ResponseWriter, r *http.Request) {
	target, relpath, status, msg := h.resolveTarget(r)
	if status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	if err := os.MkdirAll(target, dirPerm); err != nil {
		if errors.Is(err, syscall.ENOTDIR) {
			http.Error(w, "path component is a file", http.StatusConflict)
			return
		}
		h.fail(w, err, "mkdir")
		return
	}
	info, err := os.Stat(target)
	if err != nil {
		h.fail(w, err, "stat after mkdir")
		return
	}
	h.writeJSON(w, http.StatusCreated, WriteResult{Path: relpath, ModifiedAt: modTime(info), Directory: true})
}

// Delete removes a file or recursively removes a directory.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	target, _, status, msg := h.resolveTarget(r)
	if status != http.StatusOK {
		http.Error(w, msg, status)
		return
	}
	if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	} else if err != nil {
		h.fail(w, err, "stat")
		return
	}
	if err := os.RemoveAll(target); err != nil {
		h.fail(w, err, "remove")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeFileNoFollow writes data to path, refusing to follow a symlink at the
// final component (O_NOFOLLOW) so a planted symlink cannot redirect the write
// outside the confined subtree.
func writeFileNoFollow(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, filePerm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func entryFor(name string, info os.FileInfo) Entry {
	t := "file"
	if info.IsDir() {
		t = "directory"
	}
	return Entry{Name: name, Type: t, Size: info.Size(), ModifiedAt: modTime(info)}
}

// encodeContent returns valid UTF-8 verbatim, otherwise base64-encodes it.
func encodeContent(data []byte) (content, encoding string) {
	if utf8.Valid(data) {
		return string(data), "utf-8"
	}
	return base64.StdEncoding.EncodeToString(data), "base64"
}

func modTime(info os.FileInfo) string {
	return info.ModTime().UTC().Format(time.RFC3339)
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, v any) {
	if err := httputil.WriteJSON(w, status, v); err != nil {
		h.log.Error(err, "write json response")
	}
}

func (h *Handler) fail(w http.ResponseWriter, err error, op string) {
	h.log.Error(err, "content operation failed", "op", op)
	http.Error(w, "content operation failed", http.StatusInternalServerError)
}
