package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// MCPContent is one block of an MCP tool result.
type MCPContent struct {
	// Type is "text" or "image".
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// Data is the base64 image, and MimeType its type, for Type "image".
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// TextContent is a text block.
func TextContent(s string) MCPContent { return MCPContent{Type: "text", Text: s} }

// MCPToolResult is what a tool handler answers with.
type MCPToolResult struct {
	Content []MCPContent `json:"content"`
	// IsError marks the result as a failure the model should see, as opposed to
	// an error returned by the handler, which is reported the same way.
	IsError bool `json:"isError,omitempty"`
}

// MCPToolAnnotations are the optional hints MCP lets a tool carry.
type MCPToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool  `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`
}

// MCPTool is a function exposed to the model through an in-process MCP server.
type MCPTool struct {
	Name        string
	Description string
	// InputSchema is the JSON Schema of the arguments. When empty the tool
	// takes an object with any properties.
	InputSchema json.RawMessage
	Annotations *MCPToolAnnotations
	// Handler runs the tool. args is the arguments object the model sent.
	Handler func(ctx context.Context, args json.RawMessage) (MCPToolResult, error)
}

// SDKMCPServer is an MCP server that runs inside this process. Put it in
// MCPServerConfig.SDK and the CLI reaches it over the control channel: no
// subprocess and no port.
type SDKMCPServer struct {
	Name    string
	Version string
	Tools   []MCPTool
}

// NewSDKMCPServer builds a server named name (the name the CLI lists it under)
// with the given tools.
func NewSDKMCPServer(name, version string, tools ...MCPTool) *SDKMCPServer {
	if version == "" {
		version = "1.0.0"
	}
	return &SDKMCPServer{Name: name, Version: version, Tools: tools}
}

// SDKMCPServerConfig is the MCPServerConfig that mounts s.
func SDKMCPServerConfig(s *SDKMCPServer) MCPServerConfig {
	return MCPServerConfig{Name: s.Name, Type: "sdk", SDK: s}
}

const (
	jsonRPCMethodNotFound = -32601
	jsonRPCInvalidParams  = -32602
	jsonRPCInternalError  = -32603

	defaultMCPProtocolVersion = "2024-11-05"
)

type rpcMessage struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

func rpcResult(id json.RawMessage, result any) json.RawMessage {
	out, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	return out
}

func rpcError(id json.RawMessage, code int, msg string) json.RawMessage {
	out, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id,
		"error": map[string]any{"code": code, "message": msg},
	})
	return out
}

// handle answers one JSON-RPC message. It returns nil for a notification, which
// has no reply.
func (s *SDKMCPServer) handle(ctx context.Context, raw json.RawMessage) json.RawMessage {
	var m rpcMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return rpcError(nil, jsonRPCInvalidParams, "invalid JSON-RPC message: "+err.Error())
	}
	if len(m.ID) == 0 || string(m.ID) == "null" {
		return nil
	}
	switch m.Method {
	case "initialize":
		version := defaultMCPProtocolVersion
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if json.Unmarshal(m.Params, &p) == nil && p.ProtocolVersion != "" {
			version = p.ProtocolVersion
		}
		return rpcResult(m.ID, map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": s.Name, "version": s.Version},
		})
	case "ping":
		return rpcResult(m.ID, map[string]any{})
	case "tools/list":
		return rpcResult(m.ID, map[string]any{"tools": s.listTools()})
	case "tools/call":
		return s.callTool(ctx, m)
	default:
		return rpcError(m.ID, jsonRPCMethodNotFound, "Method not found: "+m.Method)
	}
}

func (s *SDKMCPServer) listTools() []map[string]any {
	out := make([]map[string]any, 0, len(s.Tools))
	for _, t := range s.Tools {
		schema := t.InputSchema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object"}`)
		}
		entry := map[string]any{"name": t.Name, "description": t.Description, "inputSchema": schema}
		if t.Annotations != nil {
			entry["annotations"] = t.Annotations
		}
		out = append(out, entry)
	}
	return out
}

func (s *SDKMCPServer) callTool(ctx context.Context, m rpcMessage) json.RawMessage {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(m.Params, &p); err != nil || p.Name == "" {
		return rpcError(m.ID, jsonRPCInvalidParams, "tools/call needs a tool name")
	}
	var tool *MCPTool
	for i := range s.Tools {
		if s.Tools[i].Name == p.Name {
			tool = &s.Tools[i]
			break
		}
	}
	if tool == nil || tool.Handler == nil {
		return rpcResult(m.ID, MCPToolResult{IsError: true, Content: []MCPContent{TextContent("Unknown tool: " + p.Name)}})
	}
	if len(p.Arguments) == 0 {
		p.Arguments = json.RawMessage(`{}`)
	}
	res, err := runTool(ctx, tool, p.Arguments)
	if err != nil {
		return rpcResult(m.ID, MCPToolResult{IsError: true, Content: []MCPContent{TextContent(err.Error())}})
	}
	if res.Content == nil {
		res.Content = []MCPContent{}
	}
	return rpcResult(m.ID, res)
}

// runTool calls the handler and turns a panic into an error, so a tool that
// crashes fails its own call and not the process the SDK runs in.
func runTool(ctx context.Context, t *MCPTool, args json.RawMessage) (res MCPToolResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("tool %q panicked: %v", t.Name, r)
		}
	}()
	return t.Handler(ctx, args)
}

// sdkServers indexes the SDK servers in opts by name. A name used twice is an
// error, since the CLI could not tell the two apart.
func sdkServers(opts Options) (map[string]*SDKMCPServer, error) {
	var out map[string]*SDKMCPServer
	for _, c := range opts.MCPServers {
		if c.SDK == nil {
			continue
		}
		name := c.Name
		if name == "" {
			name = c.SDK.Name
		}
		if out == nil {
			out = map[string]*SDKMCPServer{}
		}
		if _, dup := out[name]; dup {
			return nil, errors.New("two SDK MCP servers are named " + name)
		}
		out[name] = c.SDK
	}
	return out, nil
}
