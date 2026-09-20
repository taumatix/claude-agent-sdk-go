// Command fakecli is a stand-in for the `claude` binary, used by the
// end-to-end test in domains/agent. It speaks the same stdio control protocol
// over real pipes: answer the initialize control_request, then reply to a user
// message with a turn whose content blocks are the ones the real CLI emits for
// a server-side tool call.
//
// The block types below are copied from the content-block switch in the
// `claude` binary (2.1.220) — `server_tool_use`, `web_search_tool_result`,
// `mcp_tool_use`. They are not what this SDK invented; see
// domains/protocol/types.go.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// turn is the reply to any user message: a server-side search whose result
// block is the one the SDK used to drop, plus a block no SDK models.
const turn = `{"type":"assistant","session_id":"e2e","model":"claude-sonnet-5","message":{"role":"assistant","content":[
{"type":"server_tool_use","id":"srvtoolu_01","name":"web_search","input":{"query":"go 1.26 release date"}},
{"type":"web_search_tool_result","tool_use_id":"srvtoolu_01","content":[{"type":"web_search_result","title":"Go 1.26","url":"https://go.dev/doc/devel/release"}]},
{"type":"mcp_tool_use","id":"mcptoolu_01","name":"lookup","server_name":"docs","input":{"q":"iter.Seq2"}},
{"type":"text","text":"Go 1.26 is out."}
]}}`

const result = `{"type":"result","subtype":"success","session_id":"e2e","duration_ms":12,` +
	`"duration_api_ms":9,"is_error":false,"num_turns":1,"result":"Go 1.26 is out.","terminal_reason":"completed"}`

func main() {
	// A `-v` probe from the transport's version check, not a protocol session.
	for _, arg := range os.Args[1:] {
		if arg == "-v" || arg == "--version" {
			fmt.Println("2.1.220 (Claude Code)")
			return
		}
	}

	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	emit := func(line string) {
		fmt.Fprintln(out, line)
		out.Flush()
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var frame struct {
			Type      string `json:"type"`
			RequestID string `json:"request_id"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			continue
		}

		switch frame.Type {
		case "control_request":
			emit(fmt.Sprintf(
				`{"type":"control_response","response":{"subtype":"success","request_id":%q,"response":{}}}`,
				frame.RequestID))
		case "user":
			emit(compact(turn))
			emit(result)
		}
	}
}

// compact strips the newlines that keep the turn literal readable, so it goes
// out as one JSON line.
func compact(s string) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(s)); err != nil {
		panic(err)
	}
	return buf.String()
}
