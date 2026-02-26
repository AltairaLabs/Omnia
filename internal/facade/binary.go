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

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	// BinaryMagic is the magic bytes at the start of binary frames.
	BinaryMagic = "OMNI"
	// BinaryVersion is the current binary protocol version.
	BinaryVersion = 1
	// BinaryHeaderSize is the size of the binary frame header in bytes.
	BinaryHeaderSize = 32
	// MediaIDSize is the size of the media ID field in bytes.
	MediaIDSize = 12
)

// Binary frame errors.
var (
	ErrInvalidMagic       = errors.New("invalid magic bytes")
	ErrUnsupportedVersion = errors.New("unsupported binary protocol version")
	ErrInvalidHeaderSize  = errors.New("invalid header size")
	ErrMetadataOverflow   = errors.New("metadata length exceeds frame size")
	ErrPayloadOverflow    = errors.New("payload length exceeds frame size")
)

// BinaryFlags represents the flags byte in binary frame headers.
type BinaryFlags uint8

const (
	// FlagCompressed indicates the payload is compressed.
	FlagCompressed BinaryFlags = 1 << iota
	// FlagChunked indicates this is part of a chunked transfer.
	FlagChunked
	// FlagIsLast indicates this is the last chunk in a stream.
	FlagIsLast
)

// IsCompressed returns true if the compressed flag is set.
func (f BinaryFlags) IsCompressed() bool {
	return f&FlagCompressed != 0
}

// IsChunked returns true if the chunked flag is set.
func (f BinaryFlags) IsChunked() bool {
	return f&FlagChunked != 0
}

// IsLast returns true if the is_last flag is set.
func (f BinaryFlags) IsLast() bool {
	return f&FlagIsLast != 0
}

// BinaryMessageType represents message types for binary frames.
type BinaryMessageType uint16

const (
	// BinaryMessageTypeMediaChunk is for streaming media chunks.
	BinaryMessageTypeMediaChunk BinaryMessageType = 1
	// BinaryMessageTypeUpload is for binary upload data.
	BinaryMessageTypeUpload BinaryMessageType = 2
)

// String returns a string representation of the binary message type.
func (t BinaryMessageType) String() string {
	switch t {
	case BinaryMessageTypeMediaChunk:
		return "media_chunk"
	case BinaryMessageTypeUpload:
		return "upload"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

// BinaryHeader represents the 32-byte header for binary WebSocket frames.
type BinaryHeader struct {
	Magic       [4]byte           // "OMNI"
	Version     uint8             // Protocol version
	Flags       BinaryFlags       // Bit flags
	MessageType BinaryMessageType // Message type
	MetadataLen uint32            // Length of JSON metadata
	PayloadLen  uint32            // Length of binary payload
	Sequence    uint32            // Sequence number
	MediaID     [MediaIDSize]byte // Media stream identifier
}

// Validate checks if the header is valid.
func (h *BinaryHeader) Validate() error {
	if string(h.Magic[:]) != BinaryMagic {
		return ErrInvalidMagic
	}
	if h.Version != BinaryVersion {
		return fmt.Errorf("%w: got %d, expected %d", ErrUnsupportedVersion, h.Version, BinaryVersion)
	}
	return nil
}

// Encode serializes the header to bytes.
func (h *BinaryHeader) Encode() []byte {
	buf := make([]byte, BinaryHeaderSize)
	copy(buf[0:4], h.Magic[:])
	buf[4] = h.Version
	buf[5] = byte(h.Flags)
	binary.BigEndian.PutUint16(buf[6:8], uint16(h.MessageType))
	binary.BigEndian.PutUint32(buf[8:12], h.MetadataLen)
	binary.BigEndian.PutUint32(buf[12:16], h.PayloadLen)
	binary.BigEndian.PutUint32(buf[16:20], h.Sequence)
	copy(buf[20:32], h.MediaID[:])
	return buf
}

// DecodeHeader parses bytes into a BinaryHeader.
func DecodeHeader(data []byte) (*BinaryHeader, error) {
	if len(data) < BinaryHeaderSize {
		return nil, ErrInvalidHeaderSize
	}

	h := &BinaryHeader{
		Version:     data[4],
		Flags:       BinaryFlags(data[5]),
		MessageType: BinaryMessageType(binary.BigEndian.Uint16(data[6:8])),
		MetadataLen: binary.BigEndian.Uint32(data[8:12]),
		PayloadLen:  binary.BigEndian.Uint32(data[12:16]),
		Sequence:    binary.BigEndian.Uint32(data[16:20]),
	}
	copy(h.Magic[:], data[0:4])
	copy(h.MediaID[:], data[20:32])

	if err := h.Validate(); err != nil {
		return nil, err
	}

	return h, nil
}

// BinaryFrame represents a complete binary WebSocket frame.
type BinaryFrame struct {
	Header   BinaryHeader
	Metadata json.RawMessage // JSON metadata
	Payload  []byte          // Binary payload
}

// BinaryMediaChunkMetadata is the JSON metadata for media chunk binary frames.
type BinaryMediaChunkMetadata struct {
	SessionID string `json:"session_id"`
	MimeType  string `json:"mime_type"`
}

// Encode serializes a BinaryFrame to bytes.
func (f *BinaryFrame) Encode() ([]byte, error) {
	// Update lengths in header
	f.Header.MetadataLen = uint32(len(f.Metadata))
	f.Header.PayloadLen = uint32(len(f.Payload))

	// Calculate total size
	totalSize := BinaryHeaderSize + len(f.Metadata) + len(f.Payload)
	buf := make([]byte, totalSize)

	// Encode header
	headerBytes := f.Header.Encode()
	copy(buf[0:BinaryHeaderSize], headerBytes)

	// Copy metadata
	if len(f.Metadata) > 0 {
		copy(buf[BinaryHeaderSize:BinaryHeaderSize+len(f.Metadata)], f.Metadata)
	}

	// Copy payload
	if len(f.Payload) > 0 {
		copy(buf[BinaryHeaderSize+len(f.Metadata):], f.Payload)
	}

	return buf, nil
}

// DecodeBinaryFrame parses bytes into a BinaryFrame.
func DecodeBinaryFrame(data []byte) (*BinaryFrame, error) {
	if len(data) < BinaryHeaderSize {
		return nil, ErrInvalidHeaderSize
	}

	header, err := DecodeHeader(data)
	if err != nil {
		return nil, err
	}

	// Validate lengths
	expectedSize := BinaryHeaderSize + int(header.MetadataLen) + int(header.PayloadLen)
	if len(data) < expectedSize {
		if len(data) < BinaryHeaderSize+int(header.MetadataLen) {
			return nil, ErrMetadataOverflow
		}
		return nil, ErrPayloadOverflow
	}

	frame := &BinaryFrame{
		Header: *header,
	}

	// Extract metadata
	metadataStart := BinaryHeaderSize
	metadataEnd := metadataStart + int(header.MetadataLen)
	if header.MetadataLen > 0 {
		frame.Metadata = make(json.RawMessage, header.MetadataLen)
		copy(frame.Metadata, data[metadataStart:metadataEnd])
	}

	// Extract payload
	payloadStart := metadataEnd
	payloadEnd := payloadStart + int(header.PayloadLen)
	if header.PayloadLen > 0 {
		frame.Payload = make([]byte, header.PayloadLen)
		copy(frame.Payload, data[payloadStart:payloadEnd])
	}

	return frame, nil
}

// NewMediaChunkFrame creates a new binary frame for a media chunk.
func NewMediaChunkFrame(sessionID string, mediaID [MediaIDSize]byte, sequence uint32, isLast bool, mimeType string, payload []byte) (*BinaryFrame, error) {
	metadata := BinaryMediaChunkMetadata{
		SessionID: sessionID,
		MimeType:  mimeType,
	}
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	flags := BinaryFlags(0)
	if isLast {
		flags |= FlagIsLast
	}

	return &BinaryFrame{
		Header: BinaryHeader{
			Magic:       [4]byte{'O', 'M', 'N', 'I'},
			Version:     BinaryVersion,
			Flags:       flags,
			MessageType: BinaryMessageTypeMediaChunk,
			MetadataLen: uint32(len(metadataBytes)),
			PayloadLen:  uint32(len(payload)),
			Sequence:    sequence,
			MediaID:     mediaID,
		},
		Metadata: metadataBytes,
		Payload:  payload,
	}, nil
}

// MediaIDToString converts a MediaID to a string, trimming null bytes.
func MediaIDToString(id [MediaIDSize]byte) string {
	// Find the first null byte
	n := 0
	for i, b := range id {
		if b == 0 {
			n = i
			break
		}
		n = i + 1
	}
	return string(id[:n])
}
