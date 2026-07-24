/*
Copyright 2025-2026.

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

import "time"

// ServerConfig contains configuration for the WebSocket server.
type ServerConfig struct {
	// ReadBufferSize is the size of the read buffer.
	ReadBufferSize int
	// WriteBufferSize is the size of the write buffer.
	WriteBufferSize int
	// PingInterval is how often to send ping messages.
	PingInterval time.Duration
	// PongTimeout is how long to wait for a pong response.
	PongTimeout time.Duration
	// WriteTimeout is the timeout for write operations.
	WriteTimeout time.Duration
	// MaxMessageSize is the maximum message size.
	MaxMessageSize int64
	// MaxConnections is the maximum number of concurrent WebSocket connections.
	// 0 means unlimited (not recommended for production).
	MaxConnections int
	// PromptPackName is the PromptPack associated with this agent (from env).
	PromptPackName string
	// PromptPackVersion is the PromptPack version (from env).
	PromptPackVersion string
	// MessageRateLimit is the maximum sustained TEXT/control messages per second
	// per connection. 0 disables rate limiting. Binary media frames (audio/video/
	// uploads) are NOT counted here — they are the data plane and are governed by
	// MediaByteRateLimit instead. A message-count limit is the wrong tool for
	// media: the frame rate depends on the negotiated sample rate and frame size
	// (e.g. ~187 frames/s at 24 kHz, ~375 at 48 kHz), so any count would either
	// throttle legitimate audio or fail to bound a flood.
	MessageRateLimit float64
	// MessageRateBurst is the maximum burst size for per-connection text rate limiting.
	MessageRateBurst int
	// MediaByteRateLimit is the maximum sustained BYTES per second of inbound
	// binary media (audio/video/upload frames) per connection. Bandwidth, not
	// message count, is the right bound for the data plane: it covers audio (many
	// tiny frames) and video (fewer large frames) uniformly. 0 disables the media
	// byte-rate limit.
	MediaByteRateLimit float64
	// MediaByteRateBurst is the token-bucket burst (in bytes) for the media
	// byte-rate limiter. MUST be >= MaxMessageSize so any single legal frame can
	// pass; otherwise a max-size frame would be rejected outright.
	MediaByteRateBurst int
	// MaxInFlightMessagesPerConnection limits concurrently processed messages per
	// connection. 0 disables this cap.
	MaxInFlightMessagesPerConnection int
	// WorkspaceName is the workspace this agent belongs to (for session metadata).
	WorkspaceName string
	// MaxAudioSessions is the maximum number of concurrent audio sessions per pod.
	// When a new session would exceed this cap the request is shed with
	// ErrorCodeRateLimited. 0 applies the conservative default (8).
	MaxAudioSessions int
	// DrainTimeout is how long the facade keeps serving active realtime calls
	// after receiving SIGTERM before tearing down remaining connections.
	// New calls are shed immediately on drain. Defaults to 30s.
	DrainTimeout time.Duration
}

// DefaultServerConfig returns a ServerConfig with default values.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		ReadBufferSize:   64 * 1024, // 64KB to reduce reallocation for larger messages
		WriteBufferSize:  64 * 1024, // 64KB to reduce reallocation for larger messages
		PingInterval:     30 * time.Second,
		PongTimeout:      60 * time.Second,
		WriteTimeout:     10 * time.Second,
		MaxMessageSize:   16 * 1024 * 1024, // 16MB to support base64-encoded images
		MaxConnections:   500,
		MessageRateLimit: 50,
		MessageRateBurst: 100,
		// Media data-plane bandwidth cap. 2 MiB/s sustained comfortably fits
		// 48 kHz stereo PCM16 (~192 KB/s) plus compressed video with headroom,
		// while still bounding a flooding client. Burst == MaxMessageSize so any
		// single legal frame (incl. a 16 MiB upload chunk) always passes.
		MediaByteRateLimit: 2 * 1024 * 1024,
		MediaByteRateBurst: 16 * 1024 * 1024,
		// Keep one in-flight request per connection to avoid unbounded runtime
		// stream fan-out and chunk interleaving the client cannot correlate.
		MaxInFlightMessagesPerConnection: 1,
		// Conservative audio session cap. Overridden via ServerConfig.MaxAudioSessions.
		MaxAudioSessions: 8,
		DrainTimeout:     30 * time.Second,
	}
}
