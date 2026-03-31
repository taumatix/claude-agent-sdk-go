package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	sdkerrors "github.com/taumatix/claude-agent-sdk-go/domains/errors"
	"github.com/taumatix/claude-agent-sdk-go/domains/messages"
	"github.com/taumatix/claude-agent-sdk-go/domains/protocol"
	"github.com/taumatix/claude-agent-sdk-go/domains/transport"
	"github.com/taumatix/claude-agent-sdk-go/shared/reqid"
)

const (
	msgChannelBuffer  = 64
	initializeTimeout = 60 * time.Second
	controlReqTimeout = 60 * time.Second
)

type controlResult struct {
	body json.RawMessage
	err  string
}

type sessionManager struct {
	transport   transport.Transport
	opts        Options
	msgCh       chan messages.Message
	errCh       chan error
	pendingCtrl sync.Map // string → chan controlResult
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	hookIDs     map[string]HookHandler
}

func newSessionManager(ctx context.Context, t transport.Transport, opts Options) *sessionManager {
	ctx2, cancel := context.WithCancel(ctx)
	sm := &sessionManager{
		transport: t,
		opts:      opts,
		msgCh:     make(chan messages.Message, msgChannelBuffer),
		errCh:     make(chan error, 1),
		ctx:       ctx2,
		cancel:    cancel,
		hookIDs:   make(map[string]HookHandler),
	}
	sm.wg.Add(1)
	go sm.readLoop()
	return sm
}

func (sm *sessionManager) initialize() error {
	// Build hooks wire format
	hookWire := map[string]interface{}{}
	hookCounter := 0
	for event, matchers := range sm.opts.HookHandlers {
		entries := []map[string]interface{}{}
		for _, m := range matchers {
			callbackID := fmt.Sprintf("hook_%d", hookCounter)
			hookCounter++
			sm.hookIDs[callbackID] = m.Handler
			entry := map[string]interface{}{
				"hookCallbackIds": []string{callbackID},
			}
			if m.Matcher != "" {
				entry["matcher"] = m.Matcher
			}
			if m.Timeout > 0 {
				entry["timeout"] = m.Timeout.Seconds()
			}
			entries = append(entries, entry)
		}
		hookWire[event] = entries
	}

	reqID := reqid.New()
	body := protocol.InitializeRequestBody{
		Subtype: protocol.SubtypeInitialize,
		Hooks:   hookWire,
		Agents:  map[string]interface{}{},
	}
	data, err := protocol.BuildControlRequest(reqID, body)
	if err != nil {
		return fmt.Errorf("build initialize request: %w", err)
	}

	ch := make(chan controlResult, 1)
	sm.pendingCtrl.Store(reqID, ch)
	defer sm.pendingCtrl.Delete(reqID)

	if err := sm.transport.Send(sm.ctx, data); err != nil {
		return fmt.Errorf("send initialize: %w", err)
	}

	ctx, cancel := context.WithTimeout(sm.ctx, initializeTimeout)
	defer cancel()

	select {
	case res, ok := <-ch:
		if !ok {
			return &sdkerrors.InitializeError{Msg: "initialize request cancelled"}
		}
		if res.err != "" {
			return &sdkerrors.InitializeError{Msg: "initialize error: " + res.err}
		}
		return nil
	case <-ctx.Done():
		return &sdkerrors.InitializeError{Msg: "initialize timeout: " + ctx.Err().Error()}
	}
}

func (sm *sessionManager) sendUserMessage(content string) error {
	data, err := protocol.BuildUserMessage("", content, nil)
	if err != nil {
		return err
	}
	return sm.transport.Send(sm.ctx, data)
}

// Messages returns the channel on which inbound SDK messages are delivered.
func (sm *sessionManager) Messages() <-chan messages.Message { return sm.msgCh }

// Errors returns the channel on which terminal errors (or nil on clean EOF) are delivered.
func (sm *sessionManager) Errors() <-chan error { return sm.errCh }

// Interrupt sends an interrupt control request to the CLI.
func (sm *sessionManager) Interrupt(ctx context.Context) error {
	data, err := protocol.BuildInterruptRequest(reqid.New())
	if err != nil {
		return err
	}
	return sm.transport.Send(ctx, data)
}

// SetPermissionMode requests a permission mode change.
func (sm *sessionManager) SetPermissionMode(ctx context.Context, mode PermissionMode) error {
	_, err := sm.sendControlRequest(ctx, protocol.SetPermModeRequestBody{
		Subtype: protocol.SubtypeSetPermMode,
		Mode:    string(mode),
	})
	return err
}

// SetModel requests a model change.
func (sm *sessionManager) SetModel(ctx context.Context, model string) error {
	_, err := sm.sendControlRequest(ctx, protocol.SetModelRequestBody{
		Subtype: protocol.SubtypeSetModel,
		Model:   model,
	})
	return err
}

// Close cancels the context, waits for the read loop to finish, and closes the transport.
func (sm *sessionManager) Close() error {
	sm.cancel()
	sm.wg.Wait()
	return sm.transport.Close()
}

func (sm *sessionManager) readLoop() {
	defer sm.wg.Done()
	defer close(sm.msgCh)

	for {
		data, err := sm.transport.Receive(sm.ctx)
		if err != nil {
			if err == io.EOF || sm.ctx.Err() != nil {
				sm.sendErr(nil)
			} else {
				sm.sendErr(&sdkerrors.ProcessError{Msg: err.Error()})
			}
			// Unblock all pending control requests
			sm.pendingCtrl.Range(func(key, value interface{}) bool {
				close(value.(chan controlResult))
				sm.pendingCtrl.Delete(key)
				return true
			})
			return
		}

		wire, parseErr := protocol.ParseLine(data)
		if parseErr != nil {
			sm.sendErr(parseErr)
			return
		}

		switch {
		case wire.ControlResponse != nil:
			sm.handleControlResponse(wire.ControlResponse)
		case wire.ControlRequest != nil:
			sm.wg.Add(1)
			go func() {
				defer sm.wg.Done()
				sm.handleControlRequest(wire.ControlRequest)
			}()
		case wire.ControlCancel != nil:
			sm.cancelPendingRequest(wire.ControlCancel.RequestID)
		case wire.End != nil:
			sm.sendErr(nil)
			return
		case wire.Error != nil:
			sm.sendErr(&sdkerrors.ProcessError{Msg: wire.Error.Message})
			return
		default:
			msg, convertErr := messages.FromWire(wire)
			if convertErr != nil || msg == nil {
				continue
			}
			select {
			case sm.msgCh <- *msg:
			case <-sm.ctx.Done():
				return
			}
		}
	}
}

func (sm *sessionManager) handleControlResponse(env *protocol.ControlResponseEnvelope) {
	v, ok := sm.pendingCtrl.Load(env.Response.RequestID)
	if !ok {
		return
	}
	ch := v.(chan controlResult)
	select {
	case ch <- controlResult{body: env.Response.Response, err: env.Response.Error}:
	default:
	}
}

func (sm *sessionManager) cancelPendingRequest(reqID string) {
	v, ok := sm.pendingCtrl.LoadAndDelete(reqID)
	if !ok {
		return
	}
	close(v.(chan controlResult))
}

func (sm *sessionManager) handleControlRequest(env *protocol.ControlRequestEnvelope) {
	body, err := protocol.ParseControlRequestBody(env)
	if err != nil {
		return
	}

	switch req := body.(type) {
	case *protocol.CanUseToolRequest:
		allow, reason := true, ""
		if sm.opts.ToolPermissionHandler != nil {
			allow, reason = sm.opts.ToolPermissionHandler(sm.ctx, req.ToolName, req.Input, req.ToolUseID)
		}
		behavior := "allow"
		if !allow {
			behavior = "deny"
		}
		respBody := protocol.CanUseToolResponseBody{Behavior: behavior, Message: reason}
		data, marshalErr := protocol.BuildControlResponse(env.RequestID, respBody)
		if marshalErr != nil {
			errResp, _ := protocol.BuildControlErrorResponse(env.RequestID, "internal marshal error")
			_ = sm.transport.Send(sm.ctx, errResp)
			return
		}
		if sendErr := sm.transport.Send(sm.ctx, data); sendErr != nil {
			sm.sendErr(sendErr)
		}

	case *protocol.HookCallbackRequest:
		handler, ok := sm.hookIDs[req.CallbackID]
		if !ok {
			return
		}
		output, hookErr := handler(sm.ctx, req.CallbackID, req.Input)
		if hookErr != nil {
			errResp, _ := protocol.BuildControlErrorResponse(env.RequestID, hookErr.Error())
			_ = sm.transport.Send(sm.ctx, errResp)
			return
		}
		if output == nil {
			output = map[string]interface{}{}
		}
		// Python keyword renames: async_ → async, continue_ → continue
		if v, ok := output["async_"]; ok {
			output["async"] = v
			delete(output, "async_")
		}
		if v, ok := output["continue_"]; ok {
			output["continue"] = v
			delete(output, "continue_")
		}
		data, marshalErr := protocol.BuildControlResponse(env.RequestID, output)
		if marshalErr != nil {
			errResp, _ := protocol.BuildControlErrorResponse(env.RequestID, "internal marshal error")
			_ = sm.transport.Send(sm.ctx, errResp)
			return
		}
		if sendErr := sm.transport.Send(sm.ctx, data); sendErr != nil {
			sm.sendErr(sendErr)
		}
	}
}

func (sm *sessionManager) sendControlRequest(ctx context.Context, body interface{}) (json.RawMessage, error) {
	reqID := reqid.New()
	ch := make(chan controlResult, 1)
	sm.pendingCtrl.Store(reqID, ch)
	defer sm.pendingCtrl.Delete(reqID)

	data, err := protocol.BuildControlRequest(reqID, body)
	if err != nil {
		return nil, err
	}
	if err := sm.transport.Send(ctx, data); err != nil {
		return nil, err
	}

	select {
	case res, ok := <-ch:
		if !ok {
			return nil, &sdkerrors.InitializeError{Msg: "control request cancelled"}
		}
		if res.err != "" {
			return nil, fmt.Errorf("control request error: %s", res.err)
		}
		return res.body, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (sm *sessionManager) sendErr(err error) {
	select {
	case sm.errCh <- err:
	default:
	}
}
