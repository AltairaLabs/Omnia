package facade

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/sessiontest"
	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
	"github.com/altairalabs/omnia/pkg/session/httpclient"
)

const (
	recTestSessionID = "11111111-1111-4111-8111-111111111111"
	userContent      = "hello"
)

// fakeRuntime is a minimal RuntimeService: Converse replies to each client
// message with a Done carrying fixed content+usage; Invoke echoes the same.
type fakeRuntime struct {
	runtimev1.UnimplementedRuntimeServiceServer
	content string
	usage   *runtimev1.Usage
}

func (f *fakeRuntime) Health(context.Context, *runtimev1.HealthRequest) (*runtimev1.HealthResponse, error) {
	return &runtimev1.HealthResponse{}, nil
}

func (f *fakeRuntime) Converse(stream grpc.BidiStreamingServer[runtimev1.ClientMessage, runtimev1.ServerMessage]) error {
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := stream.Send(&runtimev1.ServerMessage{
			Message: &runtimev1.ServerMessage_Done{
				Done: &runtimev1.Done{FinalContent: f.content, Usage: f.usage},
			},
		}); err != nil {
			return err
		}
	}
}

func (f *fakeRuntime) Invoke(context.Context, *runtimev1.InvocationRequest) (*runtimev1.InvocationResponse, error) {
	return &runtimev1.InvocationResponse{OutputJson: f.content, Usage: f.usage}, nil
}

type staticPolicy struct {
	p *httpclient.PrivacyPolicyResponse
}

func (s staticPolicy) Get(context.Context) *httpclient.PrivacyPolicyResponse { return s.p }

func policyResp(enabled, facadeData, runtimeData bool) *httpclient.PrivacyPolicyResponse {
	p := &httpclient.PrivacyPolicyResponse{}
	p.Recording.Enabled = enabled
	p.Recording.FacadeData = facadeData
	p.Recording.RuntimeData = runtimeData
	return p
}

func startFakeRuntime(t *testing.T, fr *fakeRuntime) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer()
	runtimev1.RegisterRuntimeServiceServer(srv, fr)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

// newRecordingClient builds a RuntimeClient against the fake runtime, with a
// fresh MemoryStore seeded with the test session, and the given policy.
func newRecordingClient(t *testing.T, p *httpclient.PrivacyPolicyResponse) (*RuntimeClient, session.Store) {
	t.Helper()
	addr := startFakeRuntime(t, &fakeRuntime{
		content: "assistant reply",
		usage:   &runtimev1.Usage{InputTokens: 10, OutputTokens: 5},
	})
	store := sessiontest.NewStore()
	_, err := store.EnsureSessionRecord(context.Background(),
		session.SessionRecordOptions{ID: recTestSessionID, AgentName: "a", Namespace: "ns"})
	require.NoError(t, err)

	rc, err := NewRuntimeClient(RuntimeClientConfig{
		Address:         addr,
		DialTimeout:     5 * time.Second,
		Log:             logr.Discard(),
		SessionStore:    store,
		RecordingPool:   NewRecordingPool(2, 16, logr.Discard(), nil),
		RecordingPolicy: staticPolicy{p},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = rc.Close() })
	return rc, store
}

func recordedRoles(t *testing.T, store session.Store) []session.MessageRole {
	t.Helper()
	s, err := store.GetSession(context.Background(), recTestSessionID)
	require.NoError(t, err)
	roles := make([]session.MessageRole, 0, len(s.Messages))
	for _, m := range s.Messages {
		roles = append(roles, m.Role)
	}
	return roles
}

func doConverseTurn(t *testing.T, rc *RuntimeClient) {
	t.Helper()
	stream, err := rc.Converse(context.Background())
	require.NoError(t, err)
	require.NoError(t, stream.Send(&runtimev1.ClientMessage{SessionId: recTestSessionID, Content: userContent}))
	_, err = stream.Recv() // the Done
	require.NoError(t, err)
	require.NoError(t, stream.CloseSend())
}

// Full recording: user (facadeData) + assistant (runtimeData) both land, once each.
func TestBusRecorder_Converse_RecordsBothTurns(t *testing.T) {
	rc, store := newRecordingClient(t, policyResp(true, true, true))
	doConverseTurn(t, rc)

	require.Eventually(t, func() bool { return len(recordedRoles(t, store)) == 2 },
		2*time.Second, 10*time.Millisecond)
	roles := recordedRoles(t, store)
	assert.Equal(t, []session.MessageRole{session.RoleUser, session.RoleAssistant}, roles)

	s, _ := store.GetSession(context.Background(), recTestSessionID)
	assert.Equal(t, int32(10), s.Messages[1].InputTokens, "assistant carries usage from Done")
	assert.Equal(t, int32(5), s.Messages[1].OutputTokens)
}

// Recorded messages must carry a non-empty ID: the postgres warm store binds it
// into a NOT NULL uuid column, so an empty ID fails the INSERT (the MemoryStore
// used elsewhere in these tests masks that by generating one). Regression guard
// for the in-cluster "0 messages persisted" failure.
func TestBusRecorder_MessagesCarryID(t *testing.T) {
	assert.NotEmpty(t, userMessage("hi").ID, "user message must carry an ID")
	assert.NotEmpty(t, assistantMessage("yo", nil).ID, "assistant message must carry an ID")
}

// facadeData:false gates the user turn but keeps the assistant (runtimeData).
func TestBusRecorder_Converse_FacadeDataFalse_DropsUserOnly(t *testing.T) {
	rc, store := newRecordingClient(t, policyResp(true, false, true))
	doConverseTurn(t, rc)

	require.Eventually(t, func() bool { return len(recordedRoles(t, store)) >= 1 },
		2*time.Second, 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, []session.MessageRole{session.RoleAssistant}, recordedRoles(t, store))
}

// runtimeData:false gates the assistant content but keeps the user turn (facadeData).
func TestBusRecorder_Converse_RuntimeDataFalse_DropsAssistantOnly(t *testing.T) {
	rc, store := newRecordingClient(t, policyResp(true, true, false))
	doConverseTurn(t, rc)

	require.Eventually(t, func() bool { return len(recordedRoles(t, store)) >= 1 },
		2*time.Second, 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond) // allow any (incorrect) assistant write to surface
	assert.Equal(t, []session.MessageRole{session.RoleUser}, recordedRoles(t, store))
}

// recording.enabled:false drops everything.
func TestBusRecorder_Converse_RecordingDisabled_DropsAll(t *testing.T) {
	rc, store := newRecordingClient(t, policyResp(false, true, true))
	doConverseTurn(t, rc)

	time.Sleep(150 * time.Millisecond)
	assert.Empty(t, recordedRoles(t, store))
}

// Unary Invoke (Functions facade) records the input + output as messages.
func TestBusRecorder_Invoke_RecordsInputAndOutput(t *testing.T) {
	rc, store := newRecordingClient(t, policyResp(true, true, true))
	_, err := rc.Invoke(context.Background(),
		&runtimev1.InvocationRequest{InvocationId: recTestSessionID, InputJson: `{"q":"hi"}`})
	require.NoError(t, err)

	require.Eventually(t, func() bool { return len(recordedRoles(t, store)) == 2 },
		2*time.Second, 10*time.Millisecond)
	assert.Equal(t, []session.MessageRole{session.RoleUser, session.RoleAssistant}, recordedRoles(t, store))
}
