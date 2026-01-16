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
	"context"
	"fmt"

	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/AltairaLabs/PromptKit/sdk"
	"github.com/go-logr/logr"

	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"

	"github.com/altairalabs/omnia/pkg/logctx"
)

// resolveResponseParts converts PromptKit ContentParts to gRPC ContentParts,
// resolving any file:// or mock:// URLs to base64 data.
func (s *Server) resolveResponseParts(ctx context.Context, parts []types.ContentPart) ([]*runtimev1.ContentPart, error) {
	log := logctx.LoggerWithContext(s.log, ctx)
	result := make([]*runtimev1.ContentPart, 0, len(parts))

	for _, part := range parts {
		grpcPart := &runtimev1.ContentPart{
			Type: part.Type,
		}

		switch part.Type {
		case types.ContentTypeText:
			if part.Text != nil {
				grpcPart.Text = *part.Text
			}

		case types.ContentTypeImage, types.ContentTypeAudio, types.ContentTypeVideo:
			if part.Media == nil {
				continue
			}

			mediaContent, err := s.resolveMediaContent(ctx, part.Media)
			if err != nil {
				log.Error(err, "failed to resolve media content", "type", part.Type)
				continue
			}
			grpcPart.Media = mediaContent
		}

		result = append(result, grpcPart)
	}

	return result, nil
}

// resolveMediaContent resolves a PromptKit MediaContent to a gRPC MediaContent,
// converting file:// and mock:// URLs to base64 data.
func (s *Server) resolveMediaContent(ctx context.Context, media *types.MediaContent) (*runtimev1.MediaContent, error) {
	log := logctx.LoggerWithContext(s.log, ctx)

	// If we already have base64 data, use it directly
	if media.Data != nil && *media.Data != "" {
		return &runtimev1.MediaContent{
			Data:     *media.Data,
			MimeType: media.MIMEType,
		}, nil
	}

	// If we have a URL, try to resolve it
	if media.URL != nil && *media.URL != "" {
		url := *media.URL

		// Check if URL needs resolution (file:// or mock://)
		if IsResolvableURL(url) {
			if s.mediaResolver == nil {
				return nil, fmt.Errorf("media resolver not configured, cannot resolve URL: %s", url)
			}

			base64Data, mimeType, isPassthrough, err := s.mediaResolver.ResolveURL(url)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve media URL %s: %w", url, err)
			}

			if isPassthrough {
				// HTTP/HTTPS URL - pass through unchanged
				return &runtimev1.MediaContent{
					Url:      url,
					MimeType: mimeType,
				}, nil
			}

			log.V(1).Info("resolved media URL", "url", url, "mimeType", mimeType, "dataSize", len(base64Data))
			return &runtimev1.MediaContent{
				Data:     base64Data,
				MimeType: mimeType,
			}, nil
		}

		// HTTP/HTTPS URL - pass through unchanged
		return &runtimev1.MediaContent{
			Url:      url,
			MimeType: media.MIMEType,
		}, nil
	}

	// If we have a file path, read it
	if media.FilePath != nil && *media.FilePath != "" {
		base64Data, mimeType, _, err := s.mediaResolver.ResolveURL("file://" + *media.FilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read media file %s: %w", *media.FilePath, err)
		}

		return &runtimev1.MediaContent{
			Data:     base64Data,
			MimeType: mimeType,
		}, nil
	}

	return nil, fmt.Errorf("media content has no data source")
}

// buildSendOptions converts gRPC content parts to SDK send options.
// This enables multimodal messages (images, audio, files) to be sent to the LLM.
func buildSendOptions(parts []*runtimev1.ContentPart, log logr.Logger) []sdk.SendOption {
	if len(parts) == 0 {
		return nil
	}

	var opts []sdk.SendOption
	for _, part := range parts {
		if part.Media == nil {
			continue
		}

		opt := processMediaPart(part.Media, log)
		if opt != nil {
			opts = append(opts, opt)
		}
	}

	return opts
}

// processMediaPart converts a single media part to an SDK send option based on its type.
func processMediaPart(media *runtimev1.MediaContent, log logr.Logger) sdk.SendOption {
	switch {
	case isImageContentType(media.MimeType):
		return processImageMedia(media, log)
	case isAudioContentType(media.MimeType):
		return processAudioMedia(media, log)
	default:
		return processFileMedia(media, log)
	}
}

// processImageMedia handles image content (base64 data or URL).
func processImageMedia(media *runtimev1.MediaContent, log logr.Logger) sdk.SendOption {
	if media.Data != "" {
		data, err := decodeMediaData(media.Data)
		if err != nil {
			log.Error(err, "failed to decode image data")
			return nil
		}
		log.V(1).Info("adding image from data", "mimeType", media.MimeType, "size", len(data))
		return sdk.WithImageData(data, media.MimeType)
	}
	if media.Url != "" {
		log.V(1).Info("adding image from URL", "url", media.Url)
		return sdk.WithImageURL(media.Url)
	}
	return nil
}

// processAudioMedia handles audio content (base64 data or URL).
func processAudioMedia(media *runtimev1.MediaContent, log logr.Logger) sdk.SendOption {
	if media.Data != "" {
		data, err := decodeMediaData(media.Data)
		if err != nil {
			log.Error(err, "failed to decode audio data")
			return nil
		}
		log.V(1).Info("adding audio from data", "mimeType", media.MimeType, "size", len(data))
		return sdk.WithAudioData(data, media.MimeType)
	}
	if media.Url != "" {
		log.V(1).Info("adding audio from URL", "url", media.Url)
		return sdk.WithAudioFile(media.Url)
	}
	return nil
}

// processFileMedia handles generic file content.
func processFileMedia(media *runtimev1.MediaContent, log logr.Logger) sdk.SendOption {
	if media.Data != "" {
		data, err := decodeMediaData(media.Data)
		if err != nil {
			log.Error(err, "failed to decode file data")
			return nil
		}
		log.V(1).Info("adding file from data", "mimeType", media.MimeType, "size", len(data))
		return sdk.WithFile(media.MimeType, data)
	}
	return nil
}
