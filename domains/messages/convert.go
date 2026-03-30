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
	return &Message{
		System: &SystemMessage{
			SessionID: m.SessionID,
			Subtype:   m.Subtype,
			Data:      m.Data,
			TaskID:    m.TaskID,
			UUID:      m.UUID,
		},
	}, nil
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

	// Try as array of protocol ContentBlocks
	var wireBlocks []protocol.ContentBlock
	if err := json.Unmarshal(raw, &wireBlocks); err != nil {
		return nil, fmt.Errorf("parse content blocks: %w", err)
	}

	blocks := make([]ContentBlock, 0, len(wireBlocks))
	for i := range wireBlocks {
		b, err := contentBlockFromWire(&wireBlocks[i])
		if err != nil {
			return nil, fmt.Errorf("content block %d: %w", i, err)
		}
		blocks = append(blocks, b)
	}
	return blocks, nil
}

// contentBlockFromWire converts a single protocol ContentBlock to a messages ContentBlock.
func contentBlockFromWire(b *protocol.ContentBlock) (ContentBlock, error) {
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
		// Unknown block type — return empty text block for forward compat
		return ContentBlock{Text: &TextBlock{Text: ""}}, nil
	}
}
