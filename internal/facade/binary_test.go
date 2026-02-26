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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBinaryHeaderEncode(t *testing.T) {
	header := BinaryHeader{
		Magic:       [4]byte{'O', 'M', 'N', 'I'},
		Version:     BinaryVersion,
		Flags:       FlagIsLast,
		MessageType: BinaryMessageTypeMediaChunk,
		MetadataLen: 42,
		PayloadLen:  1024,
		Sequence:    5,
		MediaID:     [MediaIDSize]byte{'m', 'e', 'd', 'i', 'a', '-', '1', '2', '3', 0, 0, 0},
	}

	encoded := header.Encode()
	assert.Equal(t, BinaryHeaderSize, len(encoded))

	// Verify magic bytes
	assert.Equal(t, []byte("OMNI"), encoded[0:4])

	// Verify version
	assert.Equal(t, byte(BinaryVersion), encoded[4])

	// Verify flags
	assert.Equal(t, byte(FlagIsLast), encoded[5])

	// Verify message type (big-endian)
	assert.Equal(t, byte(0), encoded[6])
	assert.Equal(t, byte(1), encoded[7]) // BinaryMessageTypeMediaChunk = 1

	// Verify metadata length (big-endian)
	assert.Equal(t, byte(0), encoded[8])
	assert.Equal(t, byte(0), encoded[9])
	assert.Equal(t, byte(0), encoded[10])
	assert.Equal(t, byte(42), encoded[11])

	// Verify payload length (big-endian)
	assert.Equal(t, byte(0), encoded[12])
	assert.Equal(t, byte(0), encoded[13])
	assert.Equal(t, byte(4), encoded[14]) // 1024 = 0x400
	assert.Equal(t, byte(0), encoded[15])

	// Verify sequence (big-endian)
	assert.Equal(t, byte(0), encoded[16])
	assert.Equal(t, byte(0), encoded[17])
	assert.Equal(t, byte(0), encoded[18])
	assert.Equal(t, byte(5), encoded[19])

	// Verify media ID
	assert.Equal(t, []byte("media-123\x00\x00\x00"), encoded[20:32])
}

func TestBinaryHeaderDecode(t *testing.T) {
	header := BinaryHeader{
		Magic:       [4]byte{'O', 'M', 'N', 'I'},
		Version:     BinaryVersion,
		Flags:       FlagChunked | FlagIsLast,
		MessageType: BinaryMessageTypeMediaChunk,
		MetadataLen: 100,
		PayloadLen:  2048,
		Sequence:    42,
		MediaID:     [MediaIDSize]byte{'t', 'e', 's', 't', '-', 'i', 'd'},
	}

	encoded := header.Encode()
	decoded, err := DecodeHeader(encoded)
	require.NoError(t, err)

	assert.Equal(t, header.Magic, decoded.Magic)
	assert.Equal(t, header.Version, decoded.Version)
	assert.Equal(t, header.Flags, decoded.Flags)
	assert.Equal(t, header.MessageType, decoded.MessageType)
	assert.Equal(t, header.MetadataLen, decoded.MetadataLen)
	assert.Equal(t, header.PayloadLen, decoded.PayloadLen)
	assert.Equal(t, header.Sequence, decoded.Sequence)
	assert.Equal(t, header.MediaID, decoded.MediaID)
}

func TestBinaryHeaderValidation(t *testing.T) {
	tests := []struct {
		name    string
		header  BinaryHeader
		wantErr error
	}{
		{
			name: "valid header",
			header: BinaryHeader{
				Magic:   [4]byte{'O', 'M', 'N', 'I'},
				Version: BinaryVersion,
			},
			wantErr: nil,
		},
		{
			name: "invalid magic",
			header: BinaryHeader{
				Magic:   [4]byte{'B', 'A', 'D', '!'},
				Version: BinaryVersion,
			},
			wantErr: ErrInvalidMagic,
		},
		{
			name: "unsupported version",
			header: BinaryHeader{
				Magic:   [4]byte{'O', 'M', 'N', 'I'},
				Version: 99,
			},
			wantErr: ErrUnsupportedVersion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.header.Validate()
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestInvalidMagicBytes(t *testing.T) {
	data := make([]byte, BinaryHeaderSize)
	copy(data[0:4], "BAD!")
	data[4] = BinaryVersion

	_, err := DecodeHeader(data)
	assert.ErrorIs(t, err, ErrInvalidMagic)
}

func TestUnsupportedVersion(t *testing.T) {
	data := make([]byte, BinaryHeaderSize)
	copy(data[0:4], BinaryMagic)
	data[4] = 99 // Unsupported version

	_, err := DecodeHeader(data)
	assert.ErrorIs(t, err, ErrUnsupportedVersion)
}

func TestBinaryFrameRoundTrip(t *testing.T) {
	metadata := BinaryMediaChunkMetadata{
		SessionID: "test-session-123",
		MimeType:  "audio/mp3",
	}
	metadataBytes, err := json.Marshal(metadata)
	require.NoError(t, err)

	payload := []byte("test binary payload data")

	frame := &BinaryFrame{
		Header: BinaryHeader{
			Magic:       [4]byte{'O', 'M', 'N', 'I'},
			Version:     BinaryVersion,
			Flags:       FlagIsLast,
			MessageType: BinaryMessageTypeMediaChunk,
			MetadataLen: uint32(len(metadataBytes)),
			PayloadLen:  uint32(len(payload)),
			Sequence:    1,
			MediaID:     [MediaIDSize]byte{'m', 'e', 'd', 'i', 'a'},
		},
		Metadata: metadataBytes,
		Payload:  payload,
	}

	encoded, err := frame.Encode()
	require.NoError(t, err)

	decoded, err := DecodeBinaryFrame(encoded)
	require.NoError(t, err)

	assert.Equal(t, frame.Header.Magic, decoded.Header.Magic)
	assert.Equal(t, frame.Header.Version, decoded.Header.Version)
	assert.Equal(t, frame.Header.Flags, decoded.Header.Flags)
	assert.Equal(t, frame.Header.MessageType, decoded.Header.MessageType)
	assert.Equal(t, frame.Header.Sequence, decoded.Header.Sequence)
	assert.Equal(t, frame.Header.MediaID, decoded.Header.MediaID)
	assert.Equal(t, frame.Metadata, decoded.Metadata)
	assert.Equal(t, frame.Payload, decoded.Payload)

	// Verify metadata can be unmarshaled
	var decodedMetadata BinaryMediaChunkMetadata
	err = json.Unmarshal(decoded.Metadata, &decodedMetadata)
	require.NoError(t, err)
	assert.Equal(t, metadata.SessionID, decodedMetadata.SessionID)
	assert.Equal(t, metadata.MimeType, decodedMetadata.MimeType)
}

// testMediaIDFromString is a test helper that converts a string to a MediaID.
func testMediaIDFromString(s string) [MediaIDSize]byte {
	var id [MediaIDSize]byte
	copy(id[:], s)
	return id
}

func TestNewMediaChunkFrame(t *testing.T) {
	mediaID := testMediaIDFromString("audio-stream")
	payload := []byte("chunk data here")

	frame, err := NewMediaChunkFrame("session-123", mediaID, 5, true, "audio/wav", payload)
	require.NoError(t, err)

	assert.Equal(t, [4]byte{'O', 'M', 'N', 'I'}, frame.Header.Magic)
	assert.Equal(t, uint8(BinaryVersion), frame.Header.Version)
	assert.True(t, frame.Header.Flags.IsLast())
	assert.False(t, frame.Header.Flags.IsChunked())
	assert.False(t, frame.Header.Flags.IsCompressed())
	assert.Equal(t, BinaryMessageTypeMediaChunk, frame.Header.MessageType)
	assert.Equal(t, uint32(5), frame.Header.Sequence)
	assert.Equal(t, mediaID, frame.Header.MediaID)
	assert.Equal(t, payload, frame.Payload)

	// Verify metadata
	var metadata BinaryMediaChunkMetadata
	err = json.Unmarshal(frame.Metadata, &metadata)
	require.NoError(t, err)
	assert.Equal(t, "session-123", metadata.SessionID)
	assert.Equal(t, "audio/wav", metadata.MimeType)
}

func TestMediaIDToString(t *testing.T) {
	tests := []struct {
		name     string
		input    [MediaIDSize]byte
		expected string
	}{
		{
			name:     "with null bytes",
			input:    [MediaIDSize]byte{'t', 'e', 's', 't', 0, 0, 0, 0, 0, 0, 0, 0},
			expected: "test",
		},
		{
			name:     "full length",
			input:    [MediaIDSize]byte{'1', '2', '3', '4', '5', '6', '7', '8', '9', '0', '1', '2'},
			expected: "123456789012",
		},
		{
			name:     "empty",
			input:    [MediaIDSize]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MediaIDToString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBinaryFlags(t *testing.T) {
	tests := []struct {
		name         string
		flags        BinaryFlags
		isCompressed bool
		isChunked    bool
		isLast       bool
	}{
		{
			name:         "no flags",
			flags:        0,
			isCompressed: false,
			isChunked:    false,
			isLast:       false,
		},
		{
			name:         "compressed only",
			flags:        FlagCompressed,
			isCompressed: true,
			isChunked:    false,
			isLast:       false,
		},
		{
			name:         "chunked only",
			flags:        FlagChunked,
			isCompressed: false,
			isChunked:    true,
			isLast:       false,
		},
		{
			name:         "is_last only",
			flags:        FlagIsLast,
			isCompressed: false,
			isChunked:    false,
			isLast:       true,
		},
		{
			name:         "all flags",
			flags:        FlagCompressed | FlagChunked | FlagIsLast,
			isCompressed: true,
			isChunked:    true,
			isLast:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.isCompressed, tt.flags.IsCompressed())
			assert.Equal(t, tt.isChunked, tt.flags.IsChunked())
			assert.Equal(t, tt.isLast, tt.flags.IsLast())
		})
	}
}

func TestBinaryMessageTypeString(t *testing.T) {
	tests := []struct {
		msgType  BinaryMessageType
		expected string
	}{
		{BinaryMessageTypeMediaChunk, "media_chunk"},
		{BinaryMessageTypeUpload, "upload"},
		{BinaryMessageType(99), "unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.msgType.String())
		})
	}
}

func TestDecodeHeaderTooShort(t *testing.T) {
	data := make([]byte, BinaryHeaderSize-1)
	_, err := DecodeHeader(data)
	assert.ErrorIs(t, err, ErrInvalidHeaderSize)
}

func TestDecodeBinaryFrameTooShort(t *testing.T) {
	data := make([]byte, BinaryHeaderSize-1)
	_, err := DecodeBinaryFrame(data)
	assert.ErrorIs(t, err, ErrInvalidHeaderSize)
}

func TestDecodeBinaryFrameMetadataOverflow(t *testing.T) {
	header := BinaryHeader{
		Magic:       [4]byte{'O', 'M', 'N', 'I'},
		Version:     BinaryVersion,
		MetadataLen: 1000, // Metadata length exceeds actual data
		PayloadLen:  0,
	}

	data := header.Encode()
	// Only add a few bytes of metadata, less than declared
	data = append(data, []byte("short")...)

	_, err := DecodeBinaryFrame(data)
	assert.ErrorIs(t, err, ErrMetadataOverflow)
}

func TestDecodeBinaryFramePayloadOverflow(t *testing.T) {
	header := BinaryHeader{
		Magic:       [4]byte{'O', 'M', 'N', 'I'},
		Version:     BinaryVersion,
		MetadataLen: 5,
		PayloadLen:  1000, // Payload length exceeds actual data
	}

	data := header.Encode()
	// Add exact metadata but insufficient payload
	data = append(data, []byte("meta!")...)
	data = append(data, []byte("short")...)

	_, err := DecodeBinaryFrame(data)
	assert.ErrorIs(t, err, ErrPayloadOverflow)
}

func TestBinaryFrameEmptyMetadataAndPayload(t *testing.T) {
	frame := &BinaryFrame{
		Header: BinaryHeader{
			Magic:       [4]byte{'O', 'M', 'N', 'I'},
			Version:     BinaryVersion,
			Flags:       0,
			MessageType: BinaryMessageTypeMediaChunk,
			MetadataLen: 0,
			PayloadLen:  0,
			Sequence:    0,
		},
		Metadata: nil,
		Payload:  nil,
	}

	encoded, err := frame.Encode()
	require.NoError(t, err)
	assert.Equal(t, BinaryHeaderSize, len(encoded))

	decoded, err := DecodeBinaryFrame(encoded)
	require.NoError(t, err)
	assert.Empty(t, decoded.Metadata)
	assert.Empty(t, decoded.Payload)
}
