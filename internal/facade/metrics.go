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

package facade

// ServerMetrics defines the interface for server metrics.
// This allows the metrics implementation to be optional and testable.
type ServerMetrics interface {
	// ConnectionOpened records a new connection.
	ConnectionOpened()
	// ConnectionClosed records a closed connection.
	ConnectionClosed()
	// SessionCreated records a new session.
	SessionCreated()
	// SessionClosed records a closed session.
	SessionClosed()
	// RequestStarted records the start of a request.
	RequestStarted()
	// RequestCompleted records the completion of a request.
	RequestCompleted(status string, durationSeconds float64, handler string)
	// MessageReceived records a received message.
	MessageReceived()
	// MessageSent records a sent message.
	MessageSent()

	// Media metrics
	// UploadStarted records the start of a media upload.
	UploadStarted()
	// UploadCompleted records a successful media upload with size and duration.
	UploadCompleted(bytes int64, durationSeconds float64)
	// UploadFailed records a failed media upload.
	UploadFailed()
	// DownloadStarted records the start of a media download.
	DownloadStarted()
	// DownloadCompleted records a successful media download with size.
	DownloadCompleted(bytes int64)
	// DownloadFailed records a failed media download.
	DownloadFailed()
	// MediaChunkSent records a media chunk sent to the client.
	MediaChunkSent(isBinary bool, bytes int)
}

// NoOpMetrics is a no-op implementation of ServerMetrics for when metrics are disabled.
// All methods are intentionally empty as this is a null object pattern implementation.
type NoOpMetrics struct{}

// ConnectionOpened is a no-op - metrics are disabled.
func (n *NoOpMetrics) ConnectionOpened() { /* no-op: null object pattern */ }

// ConnectionClosed is a no-op - metrics are disabled.
func (n *NoOpMetrics) ConnectionClosed() { /* no-op: null object pattern */ }

// SessionCreated is a no-op - metrics are disabled.
func (n *NoOpMetrics) SessionCreated() { /* no-op: null object pattern */ }

// SessionClosed is a no-op - metrics are disabled.
func (n *NoOpMetrics) SessionClosed() { /* no-op: null object pattern */ }

// RequestStarted is a no-op - metrics are disabled.
func (n *NoOpMetrics) RequestStarted() { /* no-op: null object pattern */ }

// RequestCompleted is a no-op - metrics are disabled.
func (n *NoOpMetrics) RequestCompleted(string, float64, string) { /* no-op: null object pattern */ }

// MessageReceived is a no-op - metrics are disabled.
func (n *NoOpMetrics) MessageReceived() { /* no-op: null object pattern */ }

// MessageSent is a no-op - metrics are disabled.
func (n *NoOpMetrics) MessageSent() { /* no-op: null object pattern */ }

// UploadStarted is a no-op - metrics are disabled.
func (n *NoOpMetrics) UploadStarted() { /* no-op: null object pattern */ }

// UploadCompleted is a no-op - metrics are disabled.
func (n *NoOpMetrics) UploadCompleted(int64, float64) { /* no-op: null object pattern */ }

// UploadFailed is a no-op - metrics are disabled.
func (n *NoOpMetrics) UploadFailed() { /* no-op: null object pattern */ }

// DownloadStarted is a no-op - metrics are disabled.
func (n *NoOpMetrics) DownloadStarted() { /* no-op: null object pattern */ }

// DownloadCompleted is a no-op - metrics are disabled.
func (n *NoOpMetrics) DownloadCompleted(int64) { /* no-op: null object pattern */ }

// DownloadFailed is a no-op - metrics are disabled.
func (n *NoOpMetrics) DownloadFailed() { /* no-op: null object pattern */ }

// MediaChunkSent is a no-op - metrics are disabled.
func (n *NoOpMetrics) MediaChunkSent(bool, int) { /* no-op: null object pattern */ }
