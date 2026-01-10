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

package agent

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the agent.
// These metrics are designed to be useful for KEDA autoscaling.
type Metrics struct {
	// ConnectionsActive is the current number of active WebSocket connections.
	// This is the primary scaling signal for KEDA.
	ConnectionsActive prometheus.Gauge

	// ConnectionsTotal is the total number of WebSocket connections since startup.
	ConnectionsTotal prometheus.Counter

	// SessionsActive is the current number of active sessions.
	SessionsActive prometheus.Gauge

	// RequestsInflight is the current number of requests being processed.
	// This indicates how many LLM API calls are pending.
	RequestsInflight prometheus.Gauge

	// RequestsTotal is the total number of requests processed.
	RequestsTotal *prometheus.CounterVec

	// RequestDuration is the histogram of request processing times.
	RequestDuration *prometheus.HistogramVec

	// MessagesReceived is the total number of messages received.
	MessagesReceived prometheus.Counter

	// MessagesSent is the total number of messages sent.
	MessagesSent prometheus.Counter

	// Media metrics
	// UploadsTotal is the total number of upload attempts.
	UploadsTotal *prometheus.CounterVec

	// UploadBytesTotal is the total bytes uploaded.
	UploadBytesTotal prometheus.Counter

	// UploadDuration is the histogram of upload durations.
	UploadDuration prometheus.Histogram

	// DownloadsTotal is the total number of download attempts.
	DownloadsTotal *prometheus.CounterVec

	// DownloadBytesTotal is the total bytes downloaded.
	DownloadBytesTotal prometheus.Counter

	// MediaChunksTotal is the total number of media chunks sent.
	MediaChunksTotal *prometheus.CounterVec

	// MediaChunkBytesTotal is the total bytes sent as media chunks.
	MediaChunkBytesTotal prometheus.Counter
}

// NewMetrics creates and registers all Prometheus metrics.
func NewMetrics(agentName, namespace string) *Metrics {
	labels := prometheus.Labels{
		"agent":     agentName,
		"namespace": namespace,
	}

	return &Metrics{
		ConnectionsActive: promauto.NewGauge(prometheus.GaugeOpts{
			Name:        "omnia_agent_connections_active",
			Help:        "Current number of active WebSocket connections",
			ConstLabels: labels,
		}),

		ConnectionsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "omnia_agent_connections_total",
			Help:        "Total number of WebSocket connections since startup",
			ConstLabels: labels,
		}),

		SessionsActive: promauto.NewGauge(prometheus.GaugeOpts{
			Name:        "omnia_agent_sessions_active",
			Help:        "Current number of active sessions",
			ConstLabels: labels,
		}),

		RequestsInflight: promauto.NewGauge(prometheus.GaugeOpts{
			Name:        "omnia_agent_requests_inflight",
			Help:        "Current number of requests being processed (pending LLM calls)",
			ConstLabels: labels,
		}),

		RequestsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        "omnia_agent_requests_total",
			Help:        "Total number of requests processed",
			ConstLabels: labels,
		}, []string{"status"}), // status: success, error

		RequestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "omnia_agent_request_duration_seconds",
			Help:        "Request processing duration in seconds",
			ConstLabels: labels,
			Buckets:     []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120}, // LLM calls can be slow
		}, []string{"handler"}),

		MessagesReceived: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "omnia_agent_messages_received_total",
			Help:        "Total number of WebSocket messages received",
			ConstLabels: labels,
		}),

		MessagesSent: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "omnia_agent_messages_sent_total",
			Help:        "Total number of WebSocket messages sent",
			ConstLabels: labels,
		}),

		// Media metrics
		UploadsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        "omnia_facade_uploads_total",
			Help:        "Total number of media upload attempts",
			ConstLabels: labels,
		}, []string{"status"}), // status: success, failed

		UploadBytesTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "omnia_facade_upload_bytes_total",
			Help:        "Total bytes uploaded",
			ConstLabels: labels,
		}),

		UploadDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:        "omnia_facade_upload_duration_seconds",
			Help:        "Upload duration in seconds",
			ConstLabels: labels,
			Buckets:     []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120},
		}),

		DownloadsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        "omnia_facade_downloads_total",
			Help:        "Total number of media download attempts",
			ConstLabels: labels,
		}, []string{"status"}), // status: success, failed

		DownloadBytesTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "omnia_facade_download_bytes_total",
			Help:        "Total bytes downloaded",
			ConstLabels: labels,
		}),

		MediaChunksTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        "omnia_facade_media_chunks_total",
			Help:        "Total number of media chunks sent",
			ConstLabels: labels,
		}, []string{"type"}), // type: json, binary

		MediaChunkBytesTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "omnia_facade_media_chunk_bytes_total",
			Help:        "Total bytes sent as media chunks",
			ConstLabels: labels,
		}),
	}
}

// ConnectionOpened records a new connection.
func (m *Metrics) ConnectionOpened() {
	m.ConnectionsActive.Inc()
	m.ConnectionsTotal.Inc()
}

// ConnectionClosed records a closed connection.
func (m *Metrics) ConnectionClosed() {
	m.ConnectionsActive.Dec()
}

// SessionCreated records a new session.
func (m *Metrics) SessionCreated() {
	m.SessionsActive.Inc()
}

// SessionClosed records a closed session.
func (m *Metrics) SessionClosed() {
	m.SessionsActive.Dec()
}

// RequestStarted records the start of a request.
func (m *Metrics) RequestStarted() {
	m.RequestsInflight.Inc()
}

// RequestCompleted records the completion of a request.
func (m *Metrics) RequestCompleted(status string, durationSeconds float64, handler string) {
	m.RequestsInflight.Dec()
	m.RequestsTotal.WithLabelValues(status).Inc()
	m.RequestDuration.WithLabelValues(handler).Observe(durationSeconds)
}

// MessageReceived records a received message.
func (m *Metrics) MessageReceived() {
	m.MessagesReceived.Inc()
}

// MessageSent records a sent message.
func (m *Metrics) MessageSent() {
	m.MessagesSent.Inc()
}

// UploadStarted records the start of a media upload.
func (m *Metrics) UploadStarted() {
	// No-op: we track uploads in UploadCompleted/UploadFailed
}

// UploadCompleted records a successful media upload.
func (m *Metrics) UploadCompleted(bytes int64, durationSeconds float64) {
	m.UploadsTotal.WithLabelValues("success").Inc()
	m.UploadBytesTotal.Add(float64(bytes))
	m.UploadDuration.Observe(durationSeconds)
}

// UploadFailed records a failed media upload.
func (m *Metrics) UploadFailed() {
	m.UploadsTotal.WithLabelValues("failed").Inc()
}

// DownloadStarted records the start of a media download.
func (m *Metrics) DownloadStarted() {
	// No-op: we track downloads in DownloadCompleted/DownloadFailed
}

// DownloadCompleted records a successful media download.
func (m *Metrics) DownloadCompleted(bytes int64) {
	m.DownloadsTotal.WithLabelValues("success").Inc()
	m.DownloadBytesTotal.Add(float64(bytes))
}

// DownloadFailed records a failed media download.
func (m *Metrics) DownloadFailed() {
	m.DownloadsTotal.WithLabelValues("failed").Inc()
}

// MediaChunkSent records a media chunk sent to the client.
func (m *Metrics) MediaChunkSent(isBinary bool, bytes int) {
	chunkType := "json"
	if isBinary {
		chunkType = "binary"
	}
	m.MediaChunksTotal.WithLabelValues(chunkType).Inc()
	m.MediaChunkBytesTotal.Add(float64(bytes))
}
