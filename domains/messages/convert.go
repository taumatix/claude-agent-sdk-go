package messages

import (
	"encoding/json"
	"fmt"

	"github.com/taumatix/claude-agent-sdk-go/domains/protocol"
)

// FromWire converts a protocol.WireMessage to a messages.Message.
// Returns nil, nil for empty WireMessages (unknown types).
func FromWire(w *protocol.WireMessage) (*Message, error) {
	switch {
	case w.User != nil:
		return fromWireUser(w.User)
	case w.Assistant != nil:
		return fromWireAssistant(w.Assistant)
	case w.System != nil:
		return fromWireSystem(w.System)
	case w.Result != nil:
		return fromWireResult(w.Result)
	case w.StreamEvent != nil:
		return fromWireStreamEvent(w.StreamEvent)
	case w.RateLimitEvent != nil:
		return fromWireRateLimitEvent(w.RateLimitEvent)
	case w.ConvReset != nil:
		return fromWireConvReset(w.ConvReset)
	default:
		return nil, nil
	}
}

func fromWireUser(m *protocol.InboundRoleMessage) (*Message, error) {
	blocks, err := contentBlocksFromRaw(m.Message.Content)
	if err != nil {
		return nil, fmt.Errorf("parse user content: %w", err)
	}
	return &Message{
		User: &UserMessage{
			SessionID:       m.SessionID,
			UUID:            m.UUID,
			Content:         blocks,
			ParentToolUseID: m.ParentToolUseID,
			ToolUseResult:   m.ToolUseResult,
			Origin:          m.Origin,
		},
	}, nil
}

func fromWireAssistant(m *protocol.InboundRoleMessage) (*Message, error) {
	blocks, err := contentBlocksFromRaw(m.Message.Content)
	if err != nil {
		return nil, fmt.Errorf("parse assistant content: %w", err)
	}
	return &Message{
		Assistant: &AssistantMessage{
			SessionID:       m.SessionID,
			UUID:            m.UUID,
			Model:           m.Model,
			Content:         blocks,
			Usage:           m.Usage,
			StopReason:      m.StopReason,
			MessageID:       m.MessageID,
			Error:           m.Error,
			ParentToolUseID: m.ParentToolUseID,
		},
	}, nil
}

func fromWireSystem(m *protocol.SystemMessage) (*Message, error) {
	msg := &Message{
		System: &SystemMessage{
			SessionID: m.SessionID,
			Subtype:   m.Subtype,
			Data:      m.Data,
			TaskID:    m.TaskID,
			UUID:      m.UUID,
			Raw:       m.Raw,
		},
	}
	// Decodes the typed lifecycle payload alongside System, leaving it nil for
	// a subtype this SDK does not model or a payload that will not decode. It
	// returns no error on purpose: a lifecycle event must not fail the stream.
	systemPayloadFromWire(msg, m)
	return msg, nil
}

func fromWireResult(m *protocol.ResultMessage) (*Message, error) {
	return &Message{
		Result: &ResultMessage{
			SessionID:    m.SessionID,
			UUID:         m.UUID,
			Subtype:      m.Subtype,
			IsError:      m.IsError,
			NumTurns:     m.NumTurns,
			DurationMS:   m.DurationMS,
			Result:       m.Result,
			TotalCostUSD: m.TotalCostUSD,
			StopReason:   m.StopReason,
			Usage:        m.Usage,

			DurationAPIMS:     m.DurationAPIMS,
			TerminalReason:    m.TerminalReason,
			APIErrorStatus:    m.APIErrorStatus,
			StructuredOutput:  m.StructuredOutput,
			ModelUsage:        m.ModelUsage,
			PermissionDenials: m.PermissionDenials,
			Errors:            m.Errors,
			Origin:            m.Origin,
		},
	}, nil
}

func fromWireConvReset(m *protocol.ConversationResetMessage) (*Message, error) {
	return &Message{
		ConvReset: &ConversationResetMessage{
			NewConversationID: m.NewConversationID,
			UUID:              m.UUID,
			SessionID:         m.SessionID,
		},
	}, nil
}

func fromWireStreamEvent(m *protocol.StreamEvent) (*Message, error) {
	return &Message{
		StreamEvent: &StreamEventMessage{
			UUID:            m.UUID,
			SessionID:       m.SessionID,
			Event:           m.Event,
			ParentToolUseID: m.ParentToolUseID,
		},
	}, nil
}

func fromWireRateLimitEvent(m *protocol.RateLimitEvent) (*Message, error) {
	return &Message{
		RateLimit: &RateLimitMessage{
			UUID:          m.UUID,
			SessionID:     m.SessionID,
			RateLimitInfo: m.RateLimitInfo,
		},
	}, nil
}

// contentBlocksFromRaw parses a content field which may be a JSON string or []ContentBlock.
func contentBlocksFromRaw(raw json.RawMessage) ([]ContentBlock, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	// Try as string first
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []ContentBlock{{Text: &TextBlock{Text: s}}}, nil
	}

	// Try as an array of blocks. Each element is kept in its raw form as well as
	// decoded, so a block type this SDK does not model can still be handed to
	// the caller intact.
	var rawBlocks []json.RawMessage
	if err := json.Unmarshal(raw, &rawBlocks); err != nil {
		return nil, fmt.Errorf("parse content blocks: %w", err)
	}

	blocks := make([]ContentBlock, 0, len(rawBlocks))
	for i, rawBlock := range rawBlocks {
		var wireBlock protocol.ContentBlock
		if err := json.Unmarshal(rawBlock, &wireBlock); err != nil {
			return nil, fmt.Errorf("content block %d: %w", i, err)
		}
		b, err := contentBlockFromWire(&wireBlock, rawBlock)
		if err != nil {
			return nil, fmt.Errorf("content block %d: %w", i, err)
		}
		blocks = append(blocks, b)
	}
	return blocks, nil
}

// contentBlockFromWire converts a single protocol ContentBlock to a messages
// ContentBlock. raw is the same block as it arrived, retained for block types
// this SDK does not model.
func contentBlockFromWire(b *protocol.ContentBlock, raw json.RawMessage) (ContentBlock, error) {
	switch b.Type {
	case protocol.ContentTypeText:
		return ContentBlock{Text: &TextBlock{Text: b.Text}}, nil

	case protocol.ContentTypeThinking:
		return ContentBlock{Thinking: &ThinkingBlock{
			Thinking:  b.Thinking,
			Signature: b.Signature,
		}}, nil

	case protocol.ContentTypeToolUse:
		return ContentBlock{ToolUse: &ToolUseBlock{
			ID:    b.ID,
			Name:  b.Name,
			Input: b.Input,
		}}, nil

	case protocol.ContentTypeServerToolUse:
		return ContentBlock{ServerToolUse: &ServerToolUseBlock{
			ID:    b.ID,
			Name:  b.Name,
			Input: b.Input,
		}}, nil

	// Retained for callers built against the name this SDK used to expect. The
	// CLI never emits it; the real names are handled in the default branch.
	//lint:ignore SA1019 deprecating the constant is the point; it must still decode.
	case protocol.ContentTypeServerToolResult:
		return ContentBlock{ServerToolResult: serverToolResultFromWire(b)}, nil

	case protocol.ContentTypeToolResult:
		isError := false
		if b.IsError != nil {
			isError = *b.IsError
		}
		// content can be string or []ContentBlock
		var innerBlocks []ContentBlock
		if len(b.Content) > 0 {
			var err error
			innerBlocks, err = contentBlocksFromRaw(b.Content)
			if err != nil {
				return ContentBlock{}, fmt.Errorf("tool_result inner content: %w", err)
			}
		}
		return ContentBlock{ToolResult: &ToolResultBlock{
			ToolUseID:  b.ToolUseID,
			Content:    innerBlocks,
			RawContent: b.Content,
			IsError:    isError,
		}}, nil

	default:
		if protocol.IsServerToolResult(b.Type) {
			return ContentBlock{ServerToolResult: serverToolResultFromWire(b)}, nil
		}
		// A block type this SDK does not model. Hand it over whole rather than
		// dropping it: the caller can decode what we cannot name.
		return ContentBlock{Unknown: &UnknownBlock{
			Type: b.Type,
			Raw:  append(json.RawMessage(nil), raw...),
		}}, nil
	}
}

func serverToolResultFromWire(b *protocol.ContentBlock) *ServerToolResultBlock {
	isError := false
	if b.IsError != nil {
		isError = *b.IsError
	}
	return &ServerToolResultBlock{
		ToolUseID: b.ToolUseID,
		Content:   b.Content,
		IsError:   isError,
		Type:      b.Type,
	}
}
