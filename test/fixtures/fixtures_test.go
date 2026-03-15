package fixtures

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/facade"
	"github.com/altairalabs/omnia/internal/session"
)

func TestSession_Defaults(t *testing.T) {
	s := Session()
	assert.Equal(t, DefaultSessionID, s.ID)
	assert.Equal(t, DefaultAgent, s.AgentName)
	assert.Equal(t, DefaultNamespace, s.Namespace)
	assert.Equal(t, DefaultWorkspace, s.WorkspaceName)
	assert.Equal(t, session.SessionStatusActive, s.Status)
	assert.Empty(t, s.Messages)
}

func TestSession_WithOptions(t *testing.T) {
	s := Session(
		WithSessionID("custom-id"),
		WithAgent("my-agent"),
		WithNamespace("prod"),
		WithWorkspace("ws-1"),
		WithStatus(session.SessionStatusCompleted),
	)
	assert.Equal(t, "custom-id", s.ID)
	assert.Equal(t, "my-agent", s.AgentName)
	assert.Equal(t, "prod", s.Namespace)
	assert.Equal(t, "ws-1", s.WorkspaceName)
	assert.Equal(t, session.SessionStatusCompleted, s.Status)
}

func TestSession_WithMessages(t *testing.T) {
	resetSeq()
	msgs := Conversation()
	s := Session(WithMessages(msgs...))
	assert.Len(t, s.Messages, 3)
	assert.Equal(t, int32(3), s.MessageCount)
	assert.Equal(t, session.RoleUser, s.Messages[0].Role)
	assert.Equal(t, session.RoleAssistant, s.Messages[1].Role)
}

func TestConversation(t *testing.T) {
	msgs := Conversation()
	require.Len(t, msgs, 3)
	assert.Equal(t, "Hello", msgs[0].Content)
	assert.Equal(t, session.RoleUser, msgs[0].Role)
	assert.Equal(t, session.RoleAssistant, msgs[1].Role)
	assert.Equal(t, session.RoleUser, msgs[2].Role)
	// Sequence numbers are sequential
	assert.Equal(t, int32(1), msgs[0].SequenceNum)
	assert.Equal(t, int32(2), msgs[1].SequenceNum)
	assert.Equal(t, int32(3), msgs[2].SequenceNum)
}

func TestClientMsg(t *testing.T) {
	msg := ClientMsg("hello")
	assert.Equal(t, facade.MessageTypeMessage, msg.Type)
	assert.Equal(t, DefaultSessionID, msg.SessionID)
	assert.Equal(t, "hello", msg.Content)
}

func TestClientMsgWithParts(t *testing.T) {
	msg := ClientMsgWithParts(
		TextPart("What's this?"),
		ImagePart("https://example.com/img.jpg", "image/jpeg"),
	)
	require.Len(t, msg.Parts, 2)
	assert.Equal(t, facade.ContentPartTypeText, msg.Parts[0].Type)
	assert.Equal(t, facade.ContentPartTypeImage, msg.Parts[1].Type)
	assert.Equal(t, "image/jpeg", msg.Parts[1].Media.MimeType)
}

func TestClientToolCall(t *testing.T) {
	tc := ClientToolCall("search")
	assert.Equal(t, "tc-search", tc.ID)
	assert.Equal(t, "search", tc.Name)
}

func TestClientToolCallWithConsent(t *testing.T) {
	tc := ClientToolCallWithConsent("get_location", "Allow?", "location", "privacy")
	assert.Equal(t, "get_location", tc.Name)
	assert.Equal(t, "Allow?", tc.ConsentMessage)
	assert.Equal(t, []string{"location", "privacy"}, tc.Categories)
}

func TestClientToolResult(t *testing.T) {
	msg := ClientToolResult("tc-1", map[string]string{"city": "Denver"})
	assert.Equal(t, facade.MessageTypeToolResult, msg.Type)
	require.NotNil(t, msg.ToolResult)
	assert.Equal(t, "tc-1", msg.ToolResult.CallID)
	assert.Empty(t, msg.ToolResult.Error)
}

func TestClientToolRejection(t *testing.T) {
	msg := ClientToolRejection("tc-1", "denied")
	require.NotNil(t, msg.ToolResult)
	assert.Equal(t, "denied", msg.ToolResult.Error)
}

func TestToolResult(t *testing.T) {
	tr := ToolResult("tc-1", "ok")
	assert.Equal(t, "tc-1", tr.ID)
	assert.Equal(t, "ok", tr.Result)
	assert.Empty(t, tr.Error)
}

func TestToolError(t *testing.T) {
	tr := ToolError("tc-1", "failed")
	assert.Equal(t, "tc-1", tr.ID)
	assert.Equal(t, "failed", tr.Error)
}

func TestContentParts(t *testing.T) {
	t.Run("text", func(t *testing.T) {
		p := TextPart("hello")
		assert.Equal(t, facade.ContentPartTypeText, p.Type)
		assert.Equal(t, "hello", p.Text)
	})

	t.Run("image URL", func(t *testing.T) {
		p := ImagePart("https://example.com/img.png", "image/png")
		assert.Equal(t, facade.ContentPartTypeImage, p.Type)
		require.NotNil(t, p.Media)
		assert.Equal(t, "https://example.com/img.png", p.Media.URL)
	})

	t.Run("image base64", func(t *testing.T) {
		p := ImagePartBase64("abc123", "image/jpeg")
		assert.Equal(t, "abc123", p.Media.Data)
	})

	t.Run("audio", func(t *testing.T) {
		p := AudioPart("https://example.com/audio.mp3", "audio/mp3", 30000)
		assert.Equal(t, facade.ContentPartTypeAudio, p.Type)
		assert.Equal(t, int64(30000), p.Media.DurationMs)
	})
}

func TestSessionOpts(t *testing.T) {
	opts := SessionOpts()
	assert.Equal(t, DefaultAgent, opts.AgentName)
	assert.Equal(t, DefaultNamespace, opts.Namespace)
	assert.Equal(t, DefaultWorkspace, opts.WorkspaceName)
}
