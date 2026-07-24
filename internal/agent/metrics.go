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
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel/trace"
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

	// SessionStore reports which session store backs this facade, as a
	// {mode="httpclient"|"memory"} gauge set to 1 for the active mode and 0 for
	// the others. mode="memory" means the facade fell back to the in-memory
	// store (no session-api recording) — alert on it (issue #1223).
	SessionStore *prometheus.GaugeVec

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

	// RecordingDroppedTotal is the total number of dropped async recording tasks.
	RecordingDroppedTotal prometheus.Counter

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

	// Audio duplex metrics

	// AudioSessionsActive is the current number of live duplex audio sessions.
	AudioSessionsActive prometheus.Gauge

	// AudioIngestDuration is the histogram of facade-receive→sink-send latency
	// per inbound audio frame, in seconds.
	AudioIngestDuration prometheus.Histogram

	// Media data-plane ingest counters. These measure inbound binary media
	// (audio/video) BEFORE it is handed to the runtime/PromptKit, closing the
	// blind spot where a per-connection rate limiter silently dropped audio
	// frames. A rising *RateLimitedTotal against a healthy *ReceivedTotal is the
	// direct fingerprint of that class of bug.

	// AudioFramesReceivedTotal counts inbound binary media frames admitted.
	AudioFramesReceivedTotal prometheus.Counter
	// AudioBytesReceivedTotal counts inbound binary media bytes admitted.
	AudioBytesReceivedTotal prometheus.Counter
	// MediaFramesRateLimitedTotal counts inbound binary media frames shed by the
	// per-connection media byte-rate limiter (data-plane backpressure).
	MediaFramesRateLimitedTotal prometheus.Counter
	// ControlMessagesRateLimitedTotal counts inbound text/control messages shed
	// by the per-connection message-count rate limiter (control-plane flood).
	ControlMessagesRateLimitedTotal prometheus.Counter

	// Realtime blip-resume counters

	// RealtimeSessionsParkedTotal is the total number of realtime sessions parked
	// after a client disconnect, awaiting reconnect within the grace window.
	RealtimeSessionsParkedTotal prometheus.Counter

	// RealtimeReattachTotal is the total number of successful reattaches to
	// a parked realtime session.
	RealtimeReattachTotal prometheus.Counter

	// RealtimeParkExpiredTotal is the total number of parked realtime sessions
	// that expired without a client reconnecting within the grace window.
	RealtimeParkExpiredTotal prometheus.Counter

	// Realtime drain metrics

	// RealtimeDraining is 1 while the facade is in drain mode, 0 otherwise.
	RealtimeDraining prometheus.Gauge

	// RealtimeDrainDuration is the histogram of drain durations in seconds.
	RealtimeDrainDuration *prometheus.HistogramVec

	// RealtimeCallsDrainedTotal is the total number of realtime calls that
	// completed gracefully during a drain.
	RealtimeCallsDrainedTotal prometheus.Counter

	// RealtimeCallsForceEndedTotal is the total number of realtime calls that
	// were still live when the drain timeout or context cancellation fired.
	RealtimeCallsForceEndedTotal prometheus.Counter
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

		SessionStore: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "omnia_agent_session_store",
			Help:        "Active session store by mode (1=active). mode=memory means no session-api recording.",
			ConstLabels: labels,
		}, []string{"mode"}),

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

		RecordingDroppedTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "omnia_facade_recording_dropped_total",
			Help:        "Total number of dropped async recording tasks",
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

		// Audio duplex metrics
		AudioSessionsActive: promauto.NewGauge(prometheus.GaugeOpts{
			Name:        "omnia_facade_audio_sessions_active",
			Help:        "Current number of live duplex audio sessions",
			ConstLabels: labels,
		}),

		AudioIngestDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:        "omnia_facade_audio_ingest_duration_seconds",
			Help:        "Facade-receive to sink-send latency per inbound audio frame, in seconds",
			ConstLabels: labels,
			// Sub-millisecond buckets for audio (10ms frame budgets are typical).
			Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1},
		}),

		AudioFramesReceivedTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "omnia_facade_audio_frames_received_total",
			Help:        "Total inbound binary media frames admitted on the data plane (post rate-limit)",
			ConstLabels: labels,
		}),

		AudioBytesReceivedTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "omnia_facade_audio_bytes_received_total",
			Help:        "Total inbound binary media bytes admitted on the data plane (post rate-limit)",
			ConstLabels: labels,
		}),

		MediaFramesRateLimitedTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "omnia_facade_media_frames_ratelimited_total",
			Help:        "Inbound binary media frames shed by the per-connection media byte-rate limiter",
			ConstLabels: labels,
		}),

		ControlMessagesRateLimitedTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "omnia_facade_control_messages_ratelimited_total",
			Help:        "Inbound text/control messages shed by the per-connection message-count rate limiter",
			ConstLabels: labels,
		}),

		// Realtime blip-resume counters
		RealtimeSessionsParkedTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "omnia_facade_realtime_sessions_parked_total",
			Help:        "Total number of realtime sessions parked after a client disconnect",
			ConstLabels: labels,
		}),

		RealtimeReattachTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "omnia_facade_realtime_reattach_total",
			Help:        "Total number of successful reattaches to a parked realtime session",
			ConstLabels: labels,
		}),

		RealtimeParkExpiredTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "omnia_facade_realtime_park_expired_total",
			Help:        "Total number of parked realtime sessions that expired without reconnect",
			ConstLabels: labels,
		}),

		// Realtime drain metrics
		RealtimeDraining: promauto.NewGauge(prometheus.GaugeOpts{
			Name:        "omnia_facade_realtime_draining",
			Help:        "1 while the facade is in drain mode, 0 otherwise",
			ConstLabels: labels,
		}),

		RealtimeDrainDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "omnia_facade_realtime_drain_duration_seconds",
			Help:        "Duration of realtime drain operations in seconds",
			ConstLabels: labels,
			Buckets:     []float64{1, 5, 10, 15, 20, 30, 45, 60},
		}, []string{"reason"}),

		RealtimeCallsDrainedTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "omnia_facade_realtime_calls_drained_total",
			Help:        "Total number of realtime calls that completed gracefully during drain",
			ConstLabels: labels,
		}),

		RealtimeCallsForceEndedTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "omnia_facade_realtime_calls_force_ended_total",
			Help:        "Total number of realtime calls still live when drain timeout or context cancellation fired",
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

// Session-store modes reported by the SessionStore gauge. Exported so the
// agent binary (which selects the store) and this metric share one source of
// truth.
const (
	SessionStoreModeHTTPClient = "httpclient"
	// SessionStoreModeNone means no archive is configured: session-api could
	// not be discovered, so no session/token/cost data is recorded.
	SessionStoreModeNone = "none"
)

// sessionStoreModes is every mode the SessionStore gauge reports, so setting one
// active also resets the others to 0 (avoids a stale "1" lingering after a mode
// change across restarts on a re-used series).
var sessionStoreModes = []string{SessionStoreModeHTTPClient, SessionStoreModeNone}

// SetSessionStoreMode marks the active session-store mode (1) and the rest (0).
func (m *Metrics) SetSessionStoreMode(mode string) {
	for _, md := range sessionStoreModes {
		v := 0.0
		if md == mode {
			v = 1.0
		}
		m.SessionStore.WithLabelValues(md).Set(v)
	}
}

// RequestStarted records the start of a request.
func (m *Metrics) RequestStarted() {
	m.RequestsInflight.Inc()
}

// RequestCompleted records the completion of a request.
// Attaches trace_id as a Prometheus exemplar when a span context is active,
// enabling metric → trace drill-down in Grafana.
func (m *Metrics) RequestCompleted(ctx context.Context, status string, durationSeconds float64, handler string) {
	m.RequestsInflight.Dec()
	exemplar := traceExemplar(ctx)
	incWithExemplar(m.RequestsTotal.WithLabelValues(status), exemplar)
	observeWithExemplar(m.RequestDuration.WithLabelValues(handler), durationSeconds, exemplar)
}

// traceExemplar extracts the trace_id from the span context for use as a Prometheus exemplar.
func traceExemplar(ctx context.Context) prometheus.Labels {
	sc := trace.SpanFromContext(ctx).SpanContext()
	tid := sc.TraceID()
	if !tid.IsValid() {
		return nil
	}
	return prometheus.Labels{"trace_id": tid.String()}
}

// observeWithExemplar records a histogram observation with an optional trace exemplar.
func observeWithExemplar(observer prometheus.Observer, value float64, exemplar prometheus.Labels) {
	if exemplar != nil {
		if eo, ok := observer.(prometheus.ExemplarObserver); ok {
			eo.ObserveWithExemplar(value, exemplar)
			return
		}
	}
	observer.Observe(value)
}

// incWithExemplar increments a counter with an optional trace exemplar.
func incWithExemplar(counter prometheus.Counter, exemplar prometheus.Labels) {
	if exemplar != nil {
		if ea, ok := counter.(prometheus.ExemplarAdder); ok {
			ea.AddWithExemplar(1, exemplar)
			return
		}
	}
	counter.Inc()
}

// MessageReceived records a received message.
func (m *Metrics) MessageReceived() {
	m.MessagesReceived.Inc()
}

// MessageSent records a sent message.
func (m *Metrics) MessageSent() {
	m.MessagesSent.Inc()
}

// RecordingDropped records a dropped async recording task.
func (m *Metrics) RecordingDropped() {
	m.RecordingDroppedTotal.Inc()
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

// AudioSessionStarted records that a new duplex audio session was opened.
func (m *Metrics) AudioSessionStarted() {
	m.AudioSessionsActive.Inc()
}

// AudioSessionEnded records that a duplex audio session was torn down.
func (m *Metrics) AudioSessionEnded() {
	m.AudioSessionsActive.Dec()
}

// AudioIngestLatency records the facade-receive-to-sink-send latency for
// an inbound audio frame, in seconds.
func (m *Metrics) AudioIngestLatency(seconds float64) {
	m.AudioIngestDuration.Observe(seconds)
}

// MediaFrameReceived records an admitted inbound binary media frame of the
// given size in bytes (data-plane throughput).
func (m *Metrics) MediaFrameReceived(bytes int) {
	m.AudioFramesReceivedTotal.Inc()
	m.AudioBytesReceivedTotal.Add(float64(bytes))
}

// MediaFrameRateLimited records a binary media frame shed by the per-connection
// media byte-rate limiter.
func (m *Metrics) MediaFrameRateLimited() {
	m.MediaFramesRateLimitedTotal.Inc()
}

// ControlMessageRateLimited records a text/control message shed by the
// per-connection message-count rate limiter.
func (m *Metrics) ControlMessageRateLimited() {
	m.ControlMessagesRateLimitedTotal.Inc()
}

// RealtimeSessionParked records that a realtime session was parked after
// a client disconnect, awaiting reconnect within the grace window.
func (m *Metrics) RealtimeSessionParked() {
	m.RealtimeSessionsParkedTotal.Inc()
}

// RealtimeSessionReattached records that a client successfully reattached
// to a parked realtime session.
func (m *Metrics) RealtimeSessionReattached() {
	m.RealtimeReattachTotal.Inc()
}

// RealtimeSessionParkExpired records that a parked realtime session expired
// without a client reconnecting within the grace window.
func (m *Metrics) RealtimeSessionParkExpired() {
	m.RealtimeParkExpiredTotal.Inc()
}

// RealtimeDrainStarted records that the facade has entered drain mode.
func (m *Metrics) RealtimeDrainStarted() {
	m.RealtimeDraining.Set(1)
}

// RealtimeDrainCompleted records the outcome of a drain operation.
// reason is one of "all_drained", "deadline", or "ctx_canceled".
// durationSeconds is the elapsed time since drain started.
// drained is the count of sessions that completed gracefully;
// forceEnded is the count still live at drain exit.
func (m *Metrics) RealtimeDrainCompleted(reason string, durationSeconds float64, drained, forceEnded int) {
	m.RealtimeDraining.Set(0)
	m.RealtimeDrainDuration.WithLabelValues(reason).Observe(durationSeconds)
	if drained > 0 {
		m.RealtimeCallsDrainedTotal.Add(float64(drained))
	}
	if forceEnded > 0 {
		m.RealtimeCallsForceEndedTotal.Add(float64(forceEnded))
	}
}
