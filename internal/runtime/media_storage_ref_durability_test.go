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

package runtime

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/AltairaLabs/PromptKit/runtime/providers/mock"
	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/AltairaLabs/PromptKit/sdk"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/media"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

// =============================================================================
// Task 5 — multi-turn durability of storage_ref resolution (#1817)
//
// The invariant under test: a storage_ref attachment must be re-resolved by
// the injected media.Storage on EVERY provider call, not resolved once and
// cached/frozen. A presigned URL captured at turn 1 would be stale (expired,
// rotated, or simply wrong) by the time turn N runs; a durable ref only
// delivers on its promise if GetDownloadURL is invoked again each time.
//
// Empirical finding (see task report): PromptKit's mock provider
// (runtime/providers/mock, v1.5.5) never calls BaseProvider.MediaLoader() or
// reads MediaContent.StorageReference — confirmed by source grep (zero hits)
// and by the SDK's own e2e test (sdk/e2e_storage_ref_test.go), which
// explicitly skips the mock provider for every storage-ref scenario:
// `if provider.ID == "mock" { t.Skip("Mock provider doesn't support real
// vision") }`. So driving Server.Converse with WithMockProvider does NOT
// exercise the injected store — TestConverse_MockProviderDoesNotResolveStorageRef
// below locks that fact in as a trip-wire rather than silently assuming it.
//
// Two things ARE verified end-to-end against the real, published SDK:
//
//  1. TestOmniaMediaStore_ResolvesFreshEveryProviderCall drives our adapter
//     through the exact mechanism every real provider (Claude/OpenAI/Gemini/
//     Ollama) uses per call — providers.NewMediaLoader, constructed fresh on
//     each call exactly as BaseProvider.MediaLoader() does — and proves the
//     backing store is re-consulted (not cached) on each of two simulated
//     turns.
//  2. TestConverse_CarriesStorageRefAcrossTurns drives two full turns through
//     the real Server.Converse gRPC entry point (buildSendOptions ->
//     sdk.Conversation.Send -> provider.PredictStream) with a
//     request-capturing provider. It proves the raw StorageReference reaches
//     the provider boundary on turn 1, on turn 2's own freshly-built
//     message, AND — the regression case that actually matters — on turn
//     1's message as it is replayed as history inside turn 2's request. That
//     last check is what rules out turn 1's media being frozen into
//     resolved bytes somewhere in the conversation's stored history.
//
// Neither test can exercise a live model API without credentials (matching
// the SDK's own e2e-gated pattern above), so what is NOT verified here is a
// real provider's HTTP call to Claude/OpenAI/etc. actually fetching the
// resolved URL/bytes — that plumbing lives entirely inside PromptKit
// (providers/claude/claude_multimodal.go etc.) and is covered by PromptKit's
// own tests, not Omnia's.
// =============================================================================

// countingStorage implements media.Storage. GetDownloadURL returns a
// distinct, call-counter-suffixed URL every time it is invoked — modeling a
// presigned URL that would expire/rotate between turns — so a test can prove
// the store is re-consulted per turn rather than a stale URL from an earlier
// turn being reused.
type countingStorage struct {
	mu      sync.Mutex
	baseURL string
	getURLN int
}

func (c *countingStorage) GetUploadURL(context.Context, media.UploadRequest) (*media.UploadCredentials, error) {
	return nil, nil
}

func (c *countingStorage) GetDownloadURL(_ context.Context, _ string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.getURLN++
	return fmt.Sprintf("%s?call=%d", c.baseURL, c.getURLN), nil
}

func (c *countingStorage) GetMediaInfo(context.Context, string) (*media.MediaInfo, error) {
	return nil, nil
}

func (c *countingStorage) Delete(context.Context, string) error { return nil }
func (c *countingStorage) Close() error                         { return nil }

func (c *countingStorage) getURLCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.getURLN
}

// TestOmniaMediaStore_ResolvesFreshEveryProviderCall proves the resolution
// mechanism itself (providers.MediaLoader + our adapter) re-consults the
// backing store on every call, using two independently constructed loaders —
// the same "fresh loader per call" pattern BaseProvider.MediaLoader() uses in
// every real (non-mock) provider.
func TestOmniaMediaStore_ResolvesFreshEveryProviderCall(t *testing.T) {
	var served int
	imgServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		served++
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47, byte(served)}) // vary bytes per call
	}))
	defer imgServer.Close()

	store := &countingStorage{baseURL: imgServer.URL}
	ref := "omnia://sessions/s1/media/m1"

	var lastData string
	for turn := 1; turn <= 2; turn++ {
		// A fresh loader per turn mirrors BaseProvider.MediaLoader() being
		// called anew on every provider dispatch — it is never held onto
		// across turns/conversation lifetime.
		loader := providers.NewMediaLoader(providers.MediaLoaderConfig{
			StorageService: newOmniaMediaStore(store, imgServer.Client()),
		})

		data, err := loader.GetBase64Data(context.Background(), &types.MediaContent{
			StorageReference: &ref,
			MIMEType:         "image/png",
		})
		require.NoError(t, err, "turn %d", turn)
		require.NotEmpty(t, data, "turn %d", turn)
		assert.NotEqual(t, lastData, data, "turn %d: bytes should differ from the prior turn, proving a fresh fetch happened rather than a cached result being replayed", turn)
		lastData = data
	}

	assert.Equal(t, 2, store.getURLCalls(),
		"GetDownloadURL must be re-consulted on every turn — a URL/bytes snapshot from turn 1 would be stale by turn 2")
}

// TestConverse_MockProviderDoesNotResolveStorageRef is the empirical check
// backing the finding documented above: driving two Converse turns through
// WithMockProvider with a storage_ref attachment must NOT invoke the
// injected store, because the mock provider never wires a MediaLoader. If a
// future PromptKit upgrade changes this, this test fails loudly instead of
// letting the durability story silently rely on an assumption.
func TestConverse_MockProviderDoesNotResolveStorageRef(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.promptpack"
	require.NoError(t, writeTestFile(t, packPath, invokeTestPack))

	store := &countingStorage{baseURL: "http://example.invalid"}
	server := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithMockProvider(true),
		WithMediaStorage(store),
	)
	defer func() { _ = server.Close() }()

	parts := []*runtimev1.ContentPart{{
		Type:  "image",
		Media: &runtimev1.MediaContent{StorageRef: "omnia://sessions/s1/media/m1", MimeType: "image/png"},
	}}
	stream := newMockStream(context.Background(), []*runtimev1.ClientMessage{
		{SessionId: "mock-storage-ref", Content: "look at this", Parts: parts},
		{SessionId: "mock-storage-ref", Content: "look again", Parts: parts},
	})

	_ = server.Converse(stream) // error ignored: mockConverseStream ends via a context.Canceled sentinel (see server_test.go)

	assert.Equal(t, 0, store.getURLCalls(),
		"mock provider does not exercise MediaLoader/resolve storage_ref (empirical finding, see file header)")
}

// storageRefRecordingProvider captures every PredictionRequest handed to it
// across calls (unlike invoke_test.go's recordingProvider, which only keeps
// the most recent one) so a test can inspect what each of several turns sent.
type storageRefRecordingProvider struct {
	*mock.Provider
	mu       sync.Mutex
	requests []providers.PredictionRequest
}

func newStorageRefRecordingProvider() *storageRefRecordingProvider {
	return &storageRefRecordingProvider{Provider: mock.NewProvider("rec", "rec-model", false)}
}

func (r *storageRefRecordingProvider) PredictStream(_ context.Context, req providers.PredictionRequest) (<-chan providers.StreamChunk, error) {
	r.mu.Lock()
	r.requests = append(r.requests, req)
	r.mu.Unlock()

	stop := "stop"
	ch := make(chan providers.StreamChunk, 2)
	ch <- providers.StreamChunk{Content: "ok", Delta: "ok"}
	ch <- providers.StreamChunk{Content: "ok", FinishReason: &stop}
	close(ch)
	return ch, nil
}

// requestCount returns how many PredictStream calls were captured.
func (r *storageRefRecordingProvider) requestCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

// lastStorageRefAt scans the turn-th captured request for the last message
// part carrying a StorageReference, searching newest-to-oldest so it finds
// the part just sent on that turn. NOTE: on a turn where the same ref was
// also attached on an earlier turn, this deliberately matches the freshest
// occurrence and says nothing about any older, replayed-as-history copy of
// that message — use storageRefForText below to target a specific
// historical message by its text content.
func (r *storageRefRecordingProvider) lastStorageRefAt(turn int) *string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if turn >= len(r.requests) {
		return nil
	}
	msgs := r.requests[turn].Messages
	for i := len(msgs) - 1; i >= 0; i-- {
		parts := msgs[i].Parts
		for j := len(parts) - 1; j >= 0; j-- {
			if parts[j].Media != nil && parts[j].Media.StorageReference != nil {
				return parts[j].Media.StorageReference
			}
		}
	}
	return nil
}

// storageRefForText locates, within the turn-th captured request, the
// message whose text part equals wantText, and returns the StorageReference
// carried by that SAME message's media part (if any). Unlike lastStorageRefAt
// (which always lands on the newest/last-appended match), this lets a test
// pin down one specific message by its content — in particular, a message
// that originated on an earlier turn and is now present only as replayed
// conversation history — so it can assert that THAT entry still carries an
// unresolved StorageReference rather than having been frozen into resolved
// bytes somewhere between turns. found reports whether a message with the
// given text was present at all, so a typo in wantText fails loudly instead
// of silently returning a nil ref that could be misread as "not resolved."
func (r *storageRefRecordingProvider) storageRefForText(turn int, wantText string) (ref *string, found bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if turn >= len(r.requests) {
		return nil, false
	}
	for _, msg := range r.requests[turn].Messages {
		matchesText := false
		for _, part := range msg.Parts {
			if part.Type == types.ContentTypeText && part.Text != nil && *part.Text == wantText {
				matchesText = true
				break
			}
		}
		if !matchesText {
			continue
		}
		found = true
		for _, part := range msg.Parts {
			if part.Media != nil {
				return part.Media.StorageReference, found
			}
		}
	}
	return ref, found
}

// TestConverse_CarriesStorageRefAcrossTurns drives two turns of the same
// session through the real Server.Converse -> buildSendOptions ->
// sdk.Conversation.Send -> provider.PredictStream path (bypassing only the
// concrete provider's own request-building/resolution step, which lives in
// PromptKit and is exercised separately by
// TestOmniaMediaStore_ResolvesFreshEveryProviderCall above). It proves the
// StorageReference set by processImageMedia (#1817 Task 4b) reaches the
// provider boundary unresolved on turn 1's own message, on turn 2's own
// freshly-built message, AND on turn 1's message as replayed history inside
// turn 2's request — the last of which is the actual regression case:
// history holding a resolved/frozen byte blob instead of the live
// reference.
func TestConverse_CarriesStorageRefAcrossTurns(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.promptpack"
	require.NoError(t, writeTestFile(t, packPath, invokeTestPack))

	rec := newStorageRefRecordingProvider()
	store := &countingStorage{baseURL: "http://example.invalid"}
	ref := "omnia://sessions/s1/media/m1"

	server := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithMediaStorage(store),
		WithSDKOptions(sdk.WithProvider(rec)),
	)
	defer func() { _ = server.Close() }()

	parts := []*runtimev1.ContentPart{{
		Type:  "image",
		Media: &runtimev1.MediaContent{StorageRef: ref, MimeType: "image/png"},
	}}
	stream := newMockStream(context.Background(), []*runtimev1.ClientMessage{
		{SessionId: "carries-ref", Content: "look", Parts: parts},
		{SessionId: "carries-ref", Content: "again", Parts: parts},
	})

	_ = server.Converse(stream) // error ignored: mockConverseStream ends via a context.Canceled sentinel

	require.Equal(t, 2, rec.requestCount(), "expected one PredictStream call per turn")

	turn0Ref := rec.lastStorageRefAt(0)
	require.NotNil(t, turn0Ref, "turn 1 must carry the storage reference")
	assert.Equal(t, ref, *turn0Ref)

	turn1Ref := rec.lastStorageRefAt(1)
	require.NotNil(t, turn1Ref, "turn 2 must carry the storage reference again, not a resolved/frozen blob")
	assert.Equal(t, ref, *turn1Ref)

	// The regression check that lastStorageRefAt cannot do: turn 2's request
	// carries turn 1's own message ("look") as history, alongside turn 2's
	// new message ("again"). lastStorageRefAt(1) always matches the newest
	// occurrence, i.e. turn 2's own message — it would never notice if turn
	// 1's historical entry had been frozen into resolved bytes. Look up that
	// historical message specifically by its text and assert it still
	// carries the unresolved StorageReference.
	turn1HistoricalRef, found := rec.storageRefForText(1, "look")
	require.True(t, found, "turn 2's request must include turn 1's message ('look') as history")
	require.NotNil(t, turn1HistoricalRef, "turn 1's message must remain an unresolved StorageReference in history on turn 2, not frozen into resolved bytes")
	assert.Equal(t, ref, *turn1HistoricalRef)
}

// TestConverse_ImageStorageRefWinsOverURL drives a single turn through the
// real Server.Converse -> buildSendOptions -> processImageMedia ->
// sdk.WithImageStorageRef/WithImageURL -> sdk.Conversation.Send ->
// provider.PredictStream path with a MediaContent that carries BOTH a
// StorageRef and a Url (no Data), and inspects the materialized
// types.MediaContent the provider actually receives.
//
// This is the discriminator that a unit test calling processImageMedia
// directly cannot provide (see
// TestProcessImageMedia_StorageRefWithURL_ReturnsOption in
// media_processing_test.go): sdk.WithImageStorageRef and sdk.WithImageURL
// both always construct a non-nil, opaque sdk.SendOption backed by an
// unexported sendConfig, so nil-checking the returned option alone can never
// tell which one ran. Here, instead, we check which field ended up set on
// the message actually handed to the provider: if the StorageRef branch
// fired (as the Data > StorageRef > URL priority order in processImageMedia
// dictates), StorageReference is populated and URL is nil.
func TestConverse_ImageStorageRefWinsOverURL(t *testing.T) {
	tmpDir := t.TempDir()
	packPath := tmpDir + "/pack.promptpack"
	require.NoError(t, writeTestFile(t, packPath, invokeTestPack))

	rec := newStorageRefRecordingProvider()
	ref := "omnia://sessions/s1/media/m1"

	server := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath(packPath),
		WithPromptName("default"),
		WithSDKOptions(sdk.WithProvider(rec)),
	)
	defer func() { _ = server.Close() }()

	parts := []*runtimev1.ContentPart{{
		Type: "image",
		Media: &runtimev1.MediaContent{
			StorageRef: ref,
			Url:        "https://example.com/should-not-be-used.png",
			MimeType:   "image/png",
		},
	}}
	stream := newMockStream(context.Background(), []*runtimev1.ClientMessage{
		{SessionId: "storageref-wins", Content: "look", Parts: parts},
	})

	_ = server.Converse(stream) // error ignored: mockConverseStream ends via a context.Canceled sentinel

	require.Equal(t, 1, rec.requestCount())

	var mediaContent *types.MediaContent
	for _, msg := range rec.requests[0].Messages {
		for _, part := range msg.Parts {
			if part.Media != nil {
				mediaContent = part.Media
			}
		}
	}
	require.NotNil(t, mediaContent, "request must carry a media part")
	require.NotNil(t, mediaContent.StorageReference, "StorageRef branch must have fired")
	assert.Equal(t, ref, *mediaContent.StorageReference)
	assert.Nil(t, mediaContent.URL, "URL must NOT be set on the provider-bound media — proves the StorageRef branch won, not the URL branch")
}
