package protocol

import (
	"encoding/json"
	"fmt"

	sdkerrors "github.com/taumatix/claude-agent-sdk-go/domains/errors"
)

// WireMessage holds the parsed result of a single protocol line.
// Exactly one field will be non-nil for known message types.
// Unknown types produce a zero-value WireMessage (all nil) for forward compatibility.
type WireMessage struct {
	User            *InboundRoleMessage
	Assistant       *InboundRoleMessage
	System          *SystemMessage
	Result          *ResultMessage
	StreamEvent     *StreamEvent
	RateLimitEvent  *RateLimitEvent
	ConvReset       *ConversationResetMessage
	ControlRequest  *ControlRequestEnvelope
	ControlResponse *ControlResponseEnvelope
	ControlCancel   *ControlCancelRequest
	End             *EndMessage
	Error           *ErrorMessage
}

// ParseLine parses a single newline-delimited JSON message from the CLI.
// Unknown message types return an empty WireMessage (not an error — forward compatibility).
// Invalid JSON returns a CLIJSONDecodeError.
func ParseLine(data []byte) (*WireMessage, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, &sdkerrors.CLIJSONDecodeError{
			Msg:           "failed to decode CLI JSON",
			Line:          append([]byte(nil), data...),
			OriginalError: err,
		}
	}

	msg := &WireMessage{}

	switch env.Type {
	case TypeUser:
		var m InboundRoleMessage
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, &sdkerrors.CLIJSONDecodeError{
				Msg: "failed to decode user message", Line: data, OriginalError: err,
			}
		}
		msg.User = &m

	case TypeAssistant:
		var m InboundRoleMessage
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, &sdkerrors.CLIJSONDecodeError{
				Msg: "failed to decode assistant message", Line: data, OriginalError: err,
			}
		}
		msg.Assistant = &m

	case TypeSystem:
		var m SystemMessage
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, &sdkerrors.CLIJSONDecodeError{
				Msg: "failed to decode system message", Line: data, OriginalError: err,
			}
		}
		m.Raw = append(json.RawMessage(nil), data...)
		msg.System = &m

	case TypeResult:
		var m ResultMessage
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, &sdkerrors.CLIJSONDecodeError{
				Msg: "failed to decode result message", Line: data, OriginalError: err,
			}
		}
		msg.Result = &m

	case TypeStreamEvent:
		var m StreamEvent
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, &sdkerrors.CLIJSONDecodeError{
				Msg: "failed to decode stream_event message", Line: data, OriginalError: err,
			}
		}
		msg.StreamEvent = &m

	case TypeRateLimitEvent:
		var m RateLimitEvent
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, &sdkerrors.CLIJSONDecodeError{
				Msg: "failed to decode rate_limit_event message", Line: data, OriginalError: err,
			}
		}
		msg.RateLimitEvent = &m

	case TypeConversationReset:
		var m ConversationResetMessage
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, &sdkerrors.CLIJSONDecodeError{
				Msg: "failed to decode conversation_reset message", Line: data, OriginalError: err,
			}
		}
		msg.ConvReset = &m

	case TypeControlRequest:
		var m ControlRequestEnvelope
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, &sdkerrors.CLIJSONDecodeError{
				Msg: "failed to decode control_request message", Line: data, OriginalError: err,
			}
		}
		msg.ControlRequest = &m

	case TypeControlResponse:
		var m ControlResponseEnvelope
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, &sdkerrors.CLIJSONDecodeError{
				Msg: "failed to decode control_response message", Line: data, OriginalError: err,
			}
		}
		msg.ControlResponse = &m

	case TypeControlCancelRequest:
		var m ControlCancelRequest
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, &sdkerrors.CLIJSONDecodeError{
				Msg: "failed to decode control_cancel_request message", Line: data, OriginalError: err,
			}
		}
		msg.ControlCancel = &m

	case TypeEnd:
		var m EndMessage
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, &sdkerrors.CLIJSONDecodeError{
				Msg: "failed to decode end message", Line: data, OriginalError: err,
			}
		}
		msg.End = &m

	case TypeError:
		var m ErrorMessage
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, &sdkerrors.CLIJSONDecodeError{
				Msg: "failed to decode error message", Line: data, OriginalError: err,
			}
		}
		msg.Error = &m

	default:
		// Unknown type — return empty WireMessage for forward compatibility.
		return msg, nil
	}

	return msg, nil
}

// ParseControlRequestBody parses the inner request body from a ControlRequestEnvelope.
// Returns one of: *CanUseToolRequest, *HookCallbackRequest, *InitializeRequestBody,
// *InterruptRequestBody, *SetPermModeRequestBody, *SetModelRequestBody.
// Returns an error for unknown subtypes.
func ParseControlRequestBody(env *ControlRequestEnvelope) (interface{}, error) {
	var base ControlRequestBase
	if err := json.Unmarshal(env.Request, &base); err != nil {
		return nil, fmt.Errorf("parse control request subtype: %w", err)
	}

	switch base.Subtype {
	case SubtypeCanUseTool:
		var r CanUseToolRequest
		if err := json.Unmarshal(env.Request, &r); err != nil {
			return nil, fmt.Errorf("parse can_use_tool request: %w", err)
		}
		return &r, nil

	case SubtypeHookCallback:
		var r HookCallbackRequest
		if err := json.Unmarshal(env.Request, &r); err != nil {
			return nil, fmt.Errorf("parse hook_callback request: %w", err)
		}
		return &r, nil

	case SubtypeInitialize:
		var r InitializeRequestBody
		if err := json.Unmarshal(env.Request, &r); err != nil {
			return nil, fmt.Errorf("parse initialize request: %w", err)
		}
		return &r, nil

	case SubtypeInterrupt:
		var r InterruptRequestBody
		if err := json.Unmarshal(env.Request, &r); err != nil {
			return nil, fmt.Errorf("parse interrupt request: %w", err)
		}
		return &r, nil

	case SubtypeSetPermMode:
		var r SetPermModeRequestBody
		if err := json.Unmarshal(env.Request, &r); err != nil {
			return nil, fmt.Errorf("parse set_permission_mode request: %w", err)
		}
		return &r, nil

	case SubtypeSetModel:
		var r SetModelRequestBody
		if err := json.Unmarshal(env.Request, &r); err != nil {
			return nil, fmt.Errorf("parse set_model request: %w", err)
		}
		return &r, nil

	default:
		return nil, fmt.Errorf("unknown control request subtype: %s", base.Subtype)
	}
}
