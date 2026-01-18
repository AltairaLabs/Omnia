# Multimodal Test Fixtures

Test fixtures for multimodal E2E testing with mock responses. These fixtures provide a complete setup for testing image, audio, and mixed media capabilities without requiring real LLM API calls.

## Directory Structure

```
multimodal-test/
├── README.md                    # This file
├── pack.json                    # PromptPack configuration (compiled)
├── mock-responses.yaml          # Canned mock responses for scenarios
├── media/                       # Sample media files
│   ├── test-image-small.jpg     # Small JPEG test image (~1KB)
│   ├── test-image-small.png     # Small PNG test image (~1KB)
│   ├── test-audio-short.mp3     # Short MP3 audio file (~49KB, 3 seconds)
│   └── annotated-response.png   # Mock annotated response image (~3KB)
└── k8s/                         # Kubernetes manifests
    ├── configmap-pack.yaml      # ConfigMap with PromptPack
    ├── configmap-responses.yaml # ConfigMap with mock responses
    └── agentruntime.yaml        # AgentRuntime + PromptPack resources
```

## Scenarios

The mock responses cover the following test scenarios:

| Scenario | Description | Response Type |
|----------|-------------|---------------|
| `image-analysis` | Analyze image and return annotated result | Text + Image |
| `audio-transcription` | Transcribe audio to text | Text only |
| `audio-response` | Respond with audio content | Text + Audio |
| `mixed-media` | Combined image and audio response | Text + Image + Audio |
| `tool-with-image` | Tool call that returns an image | Tool call → Text + Image |
| `text-only` | Baseline text-only conversation | Text only |
| `error-handling` | Error scenario responses | Text only |
| `large-image` | High-resolution image response | Text + Image |

## Usage

### Local Development

Use the fixtures directly for local testing:

```go
import "testing"

func TestMultimodalResponses(t *testing.T) {
    // Load mock repository from fixtures
    repo, err := mock.NewFileMockRepository("test/fixtures/multimodal-test/mock-responses.yaml")
    if err != nil {
        t.Fatal(err)
    }

    // Get response for a scenario
    params := mock.ResponseParams{
        ScenarioID: "image-analysis",
        TurnNumber: 1,
    }
    turn, err := repo.GetTurn(context.Background(), params)
    // ... assert on turn.Parts for multimodal content
}
```

### Kubernetes Deployment

Deploy the test agent to a cluster:

```bash
# Apply all resources
kubectl apply -f test/fixtures/multimodal-test/k8s/

# Or apply individually
kubectl apply -f test/fixtures/multimodal-test/k8s/configmap-pack.yaml
kubectl apply -f test/fixtures/multimodal-test/k8s/configmap-responses.yaml
kubectl apply -f test/fixtures/multimodal-test/k8s/agentruntime.yaml
```

For media files, create a ConfigMap (small files) or use a PVC (larger files):

```bash
# Create ConfigMap from media files (for small test files)
kubectl create configmap multimodal-test-media \
  --from-file=test/fixtures/multimodal-test/media/

# Verify
kubectl get configmap multimodal-test-media -o yaml
```

### Dashboard E2E Tests

Use with Playwright tests:

```typescript
// In your test file
test('should display image in response', async ({ connectedConsolePage }) => {
  // Send a message that triggers the image-analysis scenario
  await connectedConsolePage.sendMessageAndWait('analyze this image');

  // Wait for image to appear
  await connectedConsolePage.waitForImageInMessage();

  // Verify image is displayed
  const image = connectedConsolePage.page.locator('[data-testid="message-image"]');
  await expect(image).toBeVisible();
});
```

## Mock Response Format

Responses use the YAML format defined in `mock_repository.go`:

```yaml
scenarios:
  scenario-id:
    defaultResponse: "Fallback response"
    turns:
      # Simple text response
      1: "Hello, world!"

      # Structured multimodal response
      2:
        type: multimodal
        content: "Here's an image:"
        parts:
          - type: text
            text: "Description text"
          - type: image
            image_url:
              url: "mock://filename.png"
              detail: "high"
            metadata:
              format: "PNG"
              width: 800
              height: 600
          - type: audio
            audio_url:
              url: "mock://audio.mp3"
            metadata:
              format: "MP3"
              duration_seconds: 5

      # Tool call response
      3:
        type: tool_calls
        content: "I'll use a tool"
        tool_calls:
          - name: tool_name
            arguments:
              key: value
```

## Media URL Resolution

Mock media URLs use the `mock://` scheme:

- `mock://filename.png` → Resolved to `{media.basePath}/filename.png`
- The AgentRuntime's `media.basePath` config determines the base directory
- Files are served via the facade's media endpoint

## Selfplay Testing

For agent-vs-agent testing, use the `selfplay` section:

```yaml
selfplay:
  curious-user:
    defaultResponse: "Tell me more!"
    turns:
      1: "Can you show me an example?"
      2: "What about audio?"
```

Access with `ArenaRole: "self_play_user"` and `PersonaID: "curious-user"`.

## Extending Fixtures

### Adding New Scenarios

1. Add the scenario to `mock-responses.yaml`
2. Update `configmap-responses.yaml` to include the new scenario
3. Add any required media files to `media/`
4. Document the scenario in this README

### Replacing Media Files

The included media files are minimal test files. For more realistic testing:

1. Replace files in `media/` with actual test content
2. Keep file sizes reasonable for ConfigMap storage (<1MB each)
3. For larger files, use a PVC instead of ConfigMap

## Related Resources

- [Mock Provider Documentation](../../../promptkit-local/runtime/providers/mock/README.md)
- [AgentRuntime with Mock Media Sample](../../../config/samples/omnia_v1alpha1_agentruntime_mock_media.yaml)
- [Dashboard Multimodal Tests](../../../dashboard/e2e/tests/multimodal/)
