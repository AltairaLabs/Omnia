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

import "context"

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
	// The context is used to extract trace ID for Prometheus exemplars.
	RequestCompleted(ctx context.Context, status string, durationSeconds float64, handler string)
	// MessageReceived records a received message.
	MessageReceived()
	// MessageSent records a sent message.
	MessageSent()
	// RecordingDropped records a dropped async recording task.
	RecordingDropped()

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

	// Audio duplex metrics

	// AudioSessionStarted records that a new duplex audio session was opened.
	AudioSessionStarted()
	// AudioSessionEnded records that a duplex audio session was torn down.
	AudioSessionEnded()
	// AudioIngestLatency records the facade-receive-to-sink-send latency for
	// an inbound audio frame, in seconds.
	AudioIngestLatency(seconds float64)
	// MediaFrameReceived records an admitted inbound binary media frame of the
	// given size in bytes (data-plane throughput).
	MediaFrameReceived(bytes int)
	// MediaFrameRateLimited records a binary media frame shed by the
	// per-connection media byte-rate limiter.
	MediaFrameRateLimited()
	// ControlMessageRateLimited records a text/control message shed by the
	// per-connection message-count rate limiter.
	ControlMessageRateLimited()

	// Realtime blip-resume metrics

	// RealtimeSessionParked records that a realtime session was parked after
	// a client disconnect, awaiting reconnect within the grace window.
	RealtimeSessionParked()
	// RealtimeSessionReattached records that a client successfully reattached
	// to a parked realtime session.
	RealtimeSessionReattached()
	// RealtimeSessionParkExpired records that a parked realtime session expired
	// without a client reconnecting within the grace window.
	RealtimeSessionParkExpired()

	// Realtime drain metrics

	// RealtimeDrainStarted records that the facade has entered drain mode,
	// rejecting new realtime sessions while waiting for active ones to finish.
	RealtimeDrainStarted()
	// RealtimeDrainCompleted records the outcome of a drain operation.
	// reason is one of "all_drained", "deadline", or "ctx_canceled".
	// durationSeconds is the elapsed time since drain started.
	// drained is the count of sessions that completed gracefully.
	// forceEnded is the count of sessions still live at drain exit.
	RealtimeDrainCompleted(reason string, durationSeconds float64, drained, forceEnded int)
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
func (n *NoOpMetrics) RequestCompleted(context.Context, string, float64, string) { /* no-op: null object pattern */
}

// MessageReceived is a no-op - metrics are disabled.
func (n *NoOpMetrics) MessageReceived() { /* no-op: null object pattern */ }

// MessageSent is a no-op - metrics are disabled.
func (n *NoOpMetrics) MessageSent() { /* no-op: null object pattern */ }

// RecordingDropped is a no-op - metrics are disabled.
func (n *NoOpMetrics) RecordingDropped() { /* no-op: null object pattern */ }

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

// AudioSessionStarted is a no-op - metrics are disabled.
func (n *NoOpMetrics) AudioSessionStarted() { /* no-op: null object pattern */ }

// AudioSessionEnded is a no-op - metrics are disabled.
func (n *NoOpMetrics) AudioSessionEnded() { /* no-op: null object pattern */ }

// AudioIngestLatency is a no-op - metrics are disabled.
func (n *NoOpMetrics) AudioIngestLatency(float64) { /* no-op: null object pattern */ }

// MediaFrameReceived is a no-op - metrics are disabled.
func (n *NoOpMetrics) MediaFrameReceived(int) { /* no-op: null object pattern */ }

// MediaFrameRateLimited is a no-op - metrics are disabled.
func (n *NoOpMetrics) MediaFrameRateLimited() { /* no-op: null object pattern */ }

// ControlMessageRateLimited is a no-op - metrics are disabled.
func (n *NoOpMetrics) ControlMessageRateLimited() { /* no-op: null object pattern */ }

// RealtimeSessionParked is a no-op - metrics are disabled.
func (n *NoOpMetrics) RealtimeSessionParked() { /* no-op: null object pattern */ }

// RealtimeSessionReattached is a no-op - metrics are disabled.
func (n *NoOpMetrics) RealtimeSessionReattached() { /* no-op: null object pattern */ }

// RealtimeSessionParkExpired is a no-op - metrics are disabled.
func (n *NoOpMetrics) RealtimeSessionParkExpired() { /* no-op: null object pattern */ }

// RealtimeDrainStarted is a no-op - metrics are disabled.
func (n *NoOpMetrics) RealtimeDrainStarted() { /* no-op: null object pattern */ }

// RealtimeDrainCompleted is a no-op - metrics are disabled.
func (n *NoOpMetrics) RealtimeDrainCompleted(string, float64, int, int) { /* no-op: null object pattern */
}
