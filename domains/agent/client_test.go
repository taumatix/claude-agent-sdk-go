package agent_test

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/taumatix/claude-agent-sdk-go/domains/agent"
	"github.com/taumatix/claude-agent-sdk-go/domains/messages"
	"github.com/taumatix/claude-agent-sdk-go/domains/transport"
)

// FakeTransport is a test double for transport.Transport.
// It records all Send calls and uses receiveFunc to supply data.
type FakeTransport struct {
	sendFunc    func(ctx context.Context, data []byte) error
	receiveFunc func(ctx context.Context) ([]byte, error)
	closeFunc   func() error

	mu         sync.Mutex
	SendCalls  [][]byte
	CloseCalls int
}

var _ transport.Transport = (*FakeTransport)(nil)

func (f *FakeTransport) Send(ctx context.Context, data []byte) error {
	f.mu.Lock()
	f.SendCalls = append(f.SendCalls, append([]byte(nil), data...))
	f.mu.Unlock()
	if f.sendFunc != nil {
		return f.sendFunc(ctx, data)
	}
	return nil
}

func (f *FakeTransport) Receive(ctx context.Context) ([]byte, error) {
	return f.receiveFunc(ctx)
}

func (f *FakeTransport) Close() error {
	f.mu.Lock()
	f.CloseCalls++
	f.mu.Unlock()
	if f.closeFunc != nil {
		return f.closeFunc()
	}
	return nil
}

// sequentialReceiver returns a receiveFunc that serves msgs one by one then blocks until ctx done.
func sequentialReceiver(msgs [][]byte) func(context.Context) ([]byte, error) {
	i := 0
	var mu sync.Mutex
	return func(ctx context.Context) ([]byte, error) {
		mu.Lock()
		idx := i
		if idx < len(msgs) {
			i++
			b := msgs[idx]
			mu.Unlock()
			return b, nil
		}
		mu.Unlock()
		// Block until context cancelled (simulates process staying alive)
		<-ctx.Done()
		return nil, io.EOF
	}
}

// buildInitSuccessResponse builds a control_response for initialize.
func buildInitSuccessResponse(requestID string) []byte {
	resp := map[string]interface{}{
		"type": "control_response",
		"response": map[string]interface{}{
			"subtype":    "success",
			"request_id": requestID,
			"response":   map[string]interface{}{},
		},
	}
	data, _ := json.Marshal(resp)
	return data
}

// autoInitTransport intercepts the first control_request (initialize),
// captures its request_id, and prepends the success response to the message queue.
func autoInitTransport(extraMsgs [][]byte) *FakeTransport {
	var initReqID string
	var initOnce sync.Once
	var pendingResp []byte
	var pendingMu sync.Mutex
	extraIdx := 0
	var extraMu sync.Mutex

	ft := &FakeTransport{}
	ft.sendFunc = func(ctx context.Context, data []byte) error {
		initOnce.Do(func() {
			// Extract request_id from the initialize control_request
			var env struct {
				Type      string          `json:"type"`
				RequestID string          `json:"request_id"`
				Request   json.RawMessage `json:"request"`
			}
			if err := json.Unmarshal(data, &env); err == nil && env.Type == "control_request" {
				var req struct {
					Subtype string `json:"subtype"`
				}
				if json.Unmarshal(env.Request, &req) == nil && req.Subtype == "initialize" {
					pendingMu.Lock()
					initReqID = env.RequestID
					pendingResp = buildInitSuccessResponse(initReqID)
					pendingMu.Unlock()
				}
			}
		})
		return nil
	}
	ft.receiveFunc = func(ctx context.Context) ([]byte, error) {
		// Wait for the init response to be ready
		for {
			pendingMu.Lock()
			resp := pendingResp
			reqID := initReqID
			pendingMu.Unlock()

			if resp != nil && reqID != "" {
				pendingMu.Lock()
				pendingResp = nil
				pendingMu.Unlock()
				return resp, nil
			}

			// Poll briefly
			select {
			case <-ctx.Done():
				return nil, io.EOF
			case <-time.After(time.Millisecond):
			}
		}
	}

	// Once init response is served, subsequent calls serve extraMsgs then block
	origReceive := ft.receiveFunc
	initServed := false
	ft.receiveFunc = func(ctx context.Context) ([]byte, error) {
		if !initServed {
			b, err := origReceive(ctx)
			if err != nil {
				return nil, err
			}
			initServed = true
			return b, nil
		}
		extraMu.Lock()
		idx := extraIdx
		if idx < len(extraMsgs) {
			extraIdx++
			b := extraMsgs[idx]
			extraMu.Unlock()
			return b, nil
		}
		extraMu.Unlock()
		<-ctx.Done()
		return nil, io.EOF
	}

	return ft
}

// makeAssistantMsg builds a wire assistant message JSON.
func makeAssistantMsg(text string) []byte {
	msg := map[string]interface{}{
		"type":       "assistant",
		"session_id": "sess_123",
		"message": map[string]interface{}{
			"role": "assistant",
			"content": []map[string]interface{}{
				{"type": "text", "text": text},
			},
		},
		"model": "claude-opus-4-6",
	}
	data, _ := json.Marshal(msg)
	return data
}

// makeResultMsg builds a wire result message JSON.
func makeResultMsg() []byte {
	msg := map[string]interface{}{
		"type":            "result",
		"subtype":         "success",
		"session_id":      "sess_123",
		"duration_ms":     1234,
		"duration_api_ms": 800,
		"is_error":        false,
		"num_turns":       1,
	}
	data, _ := json.Marshal(msg)
	return data
}

func TestQuery_SimpleTextResponse(t *testing.T) {
	ft := autoInitTransport([][]byte{
		makeAssistantMsg("4"),
		makeResultMsg(),
	})

	opts := agent.DefaultOptions()
	opts.Transport = ft

	var received []messages.Message
	for msg, err := range agent.Query(context.Background(), "What is 2+2?", opts) {
		require.NoError(t, err)
		received = append(received, msg)
	}

	require.Len(t, received, 2)
	assert.NotNil(t, received[0].Assistant)
	assert.Len(t, received[0].Assistant.Content, 1)
	assert.NotNil(t, received[0].Assistant.Content[0].Text)
	assert.Equal(t, "4", received[0].Assistant.Content[0].Text.Text)
	assert.NotNil(t, received[1].Result)
	assert.False(t, received[1].Result.IsError)
}

func TestQuery_ToolPermissionAllow(t *testing.T) {
	canUseToolReq := map[string]interface{}{
		"type":       "control_request",
		"request_id": "req_cli_1",
		"request": map[string]interface{}{
			"subtype":     "can_use_tool",
			"tool_name":   "Bash",
			"input":       map[string]interface{}{"command": "ls"},
			"tool_use_id": "toolu_01",
		},
	}
	reqData, _ := json.Marshal(canUseToolReq)

	var handlerCalled bool
	var handlerToolName string
	var handlerMu sync.Mutex

	ft := autoInitTransport([][]byte{
		reqData,
		makeAssistantMsg("done"),
		makeResultMsg(),
	})

	opts := agent.DefaultOptions()
	opts.Transport = ft
	opts.ToolPermissionHandler = func(ctx context.Context, toolName string, input json.RawMessage, toolUseID string) (bool, string) {
		handlerMu.Lock()
		handlerCalled = true
		handlerToolName = toolName
		handlerMu.Unlock()
		return true, ""
	}

	for msg, err := range agent.Query(context.Background(), "run ls", opts) {
		require.NoError(t, err)
		_ = msg
	}

	handlerMu.Lock()
	assert.True(t, handlerCalled)
	assert.Equal(t, "Bash", handlerToolName)
	handlerMu.Unlock()

	// Verify a control_response with behavior=allow was sent
	ft.mu.Lock()
	defer ft.mu.Unlock()
	var foundAllow bool
	for _, sent := range ft.SendCalls {
		var env struct {
			Type     string `json:"type"`
			Response struct {
				Subtype  string `json:"subtype"`
				Response struct {
					Behavior string `json:"behavior"`
				} `json:"response"`
			} `json:"response"`
		}
		if json.Unmarshal(sent, &env) == nil && env.Type == "control_response" {
			if env.Response.Response.Behavior == "allow" {
				foundAllow = true
			}
		}
	}
	assert.True(t, foundAllow, "expected a control_response with behavior=allow")
}

func TestQuery_ToolPermissionDeny(t *testing.T) {
	canUseToolReq := map[string]interface{}{
		"type":       "control_request",
		"request_id": "req_cli_2",
		"request": map[string]interface{}{
			"subtype":     "can_use_tool",
			"tool_name":   "Bash",
			"input":       map[string]interface{}{"command": "rm -rf /"},
			"tool_use_id": "toolu_02",
		},
	}
	reqData, _ := json.Marshal(canUseToolReq)

	ft := autoInitTransport([][]byte{
		reqData,
		makeAssistantMsg("denied"),
		makeResultMsg(),
	})

	opts := agent.DefaultOptions()
	opts.Transport = ft
	opts.ToolPermissionHandler = func(ctx context.Context, toolName string, input json.RawMessage, toolUseID string) (bool, string) {
		return false, "dangerous command"
	}

	for _, err := range agent.Query(context.Background(), "run rm", opts) {
		require.NoError(t, err)
	}

	ft.mu.Lock()
	defer ft.mu.Unlock()
	var foundDeny bool
	for _, sent := range ft.SendCalls {
		var env struct {
			Type     string `json:"type"`
			Response struct {
				Response struct {
					Behavior string `json:"behavior"`
					Message  string `json:"message"`
				} `json:"response"`
			} `json:"response"`
		}
		if json.Unmarshal(sent, &env) == nil && env.Type == "control_response" {
			if env.Response.Response.Behavior == "deny" {
				assert.Equal(t, "dangerous command", env.Response.Response.Message)
				foundDeny = true
			}
		}
	}
	assert.True(t, foundDeny, "expected a control_response with behavior=deny")
}

func TestQuery_HookCallback(t *testing.T) {
	hookReq := map[string]interface{}{
		"type":       "control_request",
		"request_id": "req_hook_1",
		"request": map[string]interface{}{
			"subtype":     "hook_callback",
			"callback_id": "hook_0",
			"input":       map[string]interface{}{"hook_event_name": "PreToolUse"},
		},
	}
	hookData, _ := json.Marshal(hookReq)

	var hookCalled bool
	var hookMu sync.Mutex

	ft := autoInitTransport([][]byte{
		hookData,
		makeAssistantMsg("ok"),
		makeResultMsg(),
	})

	opts := agent.DefaultOptions()
	opts.Transport = ft
	opts.HookHandlers = map[string][]agent.HookMatcher{
		"PreToolUse": {
			{
				Handler: func(ctx context.Context, callbackID string, input json.RawMessage) (map[string]interface{}, error) {
					hookMu.Lock()
					hookCalled = true
					hookMu.Unlock()
					return map[string]interface{}{"continue_": true}, nil
				},
			},
		},
	}

	for _, err := range agent.Query(context.Background(), "test", opts) {
		require.NoError(t, err)
	}

	hookMu.Lock()
	assert.True(t, hookCalled)
	hookMu.Unlock()
}

func TestQuery_ContextCancellation(t *testing.T) {
	var initReqID string
	var initOnce sync.Once
	var initReady = make(chan struct{})

	ft := &FakeTransport{}
	ft.sendFunc = func(ctx context.Context, data []byte) error {
		initOnce.Do(func() {
			var env struct {
				RequestID string          `json:"request_id"`
				Request   json.RawMessage `json:"request"`
				Type      string          `json:"type"`
			}
			if json.Unmarshal(data, &env) == nil && env.Type == "control_request" {
				var req struct{ Subtype string `json:"subtype"` }
				if json.Unmarshal(env.Request, &req) == nil && req.Subtype == "initialize" {
					initReqID = env.RequestID
					close(initReady)
				}
			}
		})
		return nil
	}

	initRespSent := false
	ft.receiveFunc = func(ctx context.Context) ([]byte, error) {
		if !initRespSent {
			// Wait until we have the request ID
			select {
			case <-initReady:
			case <-ctx.Done():
				return nil, io.EOF
			}
			initRespSent = true
			return buildInitSuccessResponse(initReqID), nil
		}
		// Block until context cancelled
		<-ctx.Done()
		return nil, io.EOF
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	opts := agent.DefaultOptions()
	opts.Transport = ft

	var gotErr error
	for _, err := range agent.Query(ctx, "hang", opts) {
		if err != nil {
			gotErr = err
		}
	}
	assert.Error(t, gotErr)
}

func TestQuery_ProcessError(t *testing.T) {
	errMsg := map[string]interface{}{
		"type":    "error",
		"message": "process crashed",
	}
	errData, _ := json.Marshal(errMsg)

	ft := autoInitTransport([][]byte{errData})

	opts := agent.DefaultOptions()
	opts.Transport = ft

	var gotErr error
	for _, err := range agent.Query(context.Background(), "test", opts) {
		if err != nil {
			gotErr = err
		}
	}
	assert.Error(t, gotErr)
	assert.Contains(t, gotErr.Error(), "process crashed")
}

func TestQuery_UnknownMessageTypesIgnored(t *testing.T) {
	unknownMsg := []byte(`{"type":"unknown_future_type","data":"something"}`)

	ft := autoInitTransport([][]byte{
		unknownMsg,
		makeAssistantMsg("hello"),
		makeResultMsg(),
	})

	opts := agent.DefaultOptions()
	opts.Transport = ft

	var received []messages.Message
	for msg, err := range agent.Query(context.Background(), "test", opts) {
		require.NoError(t, err)
		received = append(received, msg)
	}

	require.Len(t, received, 2) // only assistant + result, unknown skipped
	assert.NotNil(t, received[0].Assistant)
}

func TestInitialize_HandshakeTimeout(t *testing.T) {
	ft := &FakeTransport{
		receiveFunc: func(ctx context.Context) ([]byte, error) {
			// Never respond — block until context cancelled
			<-ctx.Done()
			return nil, io.EOF
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	opts := agent.DefaultOptions()
	opts.Transport = ft

	var gotErr error
	for _, err := range agent.Query(ctx, "test", opts) {
		if err != nil {
			gotErr = err
		}
	}
	assert.Error(t, gotErr)
}

func TestClient_ConnectAndQuery(t *testing.T) {
	ft := autoInitTransport([][]byte{
		makeAssistantMsg("connected response"),
		makeResultMsg(),
	})

	opts := agent.DefaultOptions()
	opts.Transport = ft

	client := agent.NewClient(opts)
	err := client.Connect(context.Background())
	require.NoError(t, err)
	defer client.Disconnect()

	var received []messages.Message
	for msg, err := range client.Query(context.Background(), "hello") {
		require.NoError(t, err)
		received = append(received, msg)
	}

	require.Len(t, received, 2)
	assert.NotNil(t, received[0].Assistant)
	assert.Equal(t, "connected response", received[0].Assistant.Content[0].Text.Text)
}

func TestQuery_Interrupt(t *testing.T) {
	ft := autoInitTransport([][]byte{
		makeResultMsg(),
	})

	opts := agent.DefaultOptions()
	opts.Transport = ft

	client := agent.NewClient(opts)
	err := client.Connect(context.Background())
	require.NoError(t, err)
	defer client.Disconnect()

	err = client.Interrupt(context.Background())
	require.NoError(t, err)

	// Verify interrupt request was sent
	// Give the goroutine a moment to process
	time.Sleep(10 * time.Millisecond)

	ft.mu.Lock()
	defer ft.mu.Unlock()
	var foundInterrupt bool
	for _, sent := range ft.SendCalls {
		var env struct {
			Type    string `json:"type"`
			Request struct {
				Subtype string `json:"subtype"`
			} `json:"request"`
		}
		if json.Unmarshal(sent, &env) == nil && env.Type == "control_request" && env.Request.Subtype == "interrupt" {
			foundInterrupt = true
		}
	}
	assert.True(t, foundInterrupt, "expected an interrupt control_request")
}

func TestQuery_MultipleContentBlocks(t *testing.T) {
	msgWithBlocks := map[string]interface{}{
		"type":       "assistant",
		"session_id": "sess_123",
		"message": map[string]interface{}{
			"role": "assistant",
			"content": []map[string]interface{}{
				{"type": "text", "text": "Let me think..."},
				{"type": "tool_use", "id": "toolu_1", "name": "Bash", "input": map[string]interface{}{"command": "ls"}},
			},
		},
		"model": "claude-opus-4-6",
	}
	msgData, _ := json.Marshal(msgWithBlocks)

	ft := autoInitTransport([][]byte{msgData, makeResultMsg()})
	opts := agent.DefaultOptions()
	opts.Transport = ft

	var received []messages.Message
	for msg, err := range agent.Query(context.Background(), "test", opts) {
		require.NoError(t, err)
		received = append(received, msg)
	}

	require.Len(t, received, 2)
	require.NotNil(t, received[0].Assistant)
	require.Len(t, received[0].Assistant.Content, 2)
	assert.NotNil(t, received[0].Assistant.Content[0].Text)
	assert.NotNil(t, received[0].Assistant.Content[1].ToolUse)
	assert.Equal(t, "Bash", received[0].Assistant.Content[1].ToolUse.Name)
}

func TestQuery_StringContent(t *testing.T) {
	// Test that string content (not array) is handled
	msgWithString := map[string]interface{}{
		"type":       "user",
		"session_id": "sess_123",
		"message": map[string]interface{}{
			"role":    "user",
			"content": "plain text content",
		},
	}
	msgData, _ := json.Marshal(msgWithString)

	ft := autoInitTransport([][]byte{msgData, makeResultMsg()})
	opts := agent.DefaultOptions()
	opts.Transport = ft

	var received []messages.Message
	for msg, err := range agent.Query(context.Background(), "test", opts) {
		require.NoError(t, err)
		received = append(received, msg)
	}

	require.Len(t, received, 2)
	require.NotNil(t, received[0].User)
	require.Len(t, received[0].User.Content, 1)
	assert.NotNil(t, received[0].User.Content[0].Text)
	assert.Equal(t, "plain text content", received[0].User.Content[0].Text.Text)
}
