// Package mcpapi adapts the backend-owned Treechat tools to local MCP stdio.
package mcpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/Knovigator/treecli/api"
)

const MaxMessageBytes = 72 * 1024 * 1024
const ProtocolVersion = "2024-11-05"

type Backend interface {
	ListTools(context.Context) (api.ToolList, error)
	CallTool(context.Context, string, map[string]interface{}) (api.ToolResult, error)
}
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// Serve processes newline-delimited JSON-RPC. The caller supplies one explicit
// authenticated client; the server never loads credentials or executes commands.
func Serve(ctx context.Context, input io.Reader, output io.Writer, backend Backend, version string) error {
	return serveWithLimit(ctx, input, output, backend, version, MaxMessageBytes)
}

func serveWithLimit(ctx context.Context, input io.Reader, output io.Writer, backend Backend, version string, maxMessageBytes int) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, min(4096, maxMessageBytes)), maxMessageBytes)
	encoder := json.NewEncoder(output)
	initialized := false
	reply := func(id json.RawMessage, result interface{}, code int, message string) error {
		if len(id) == 0 {
			id = json.RawMessage("null")
		}
		payload := map[string]interface{}{"jsonrpc": "2.0", "id": id}
		if code != 0 {
			payload["error"] = map[string]interface{}{"code": code, "message": message}
		} else {
			payload["result"] = result
		}
		return encoder.Encode(payload)
	}
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			if err := reply(nil, nil, -32700, "Invalid JSON"); err != nil {
				return err
			}
			continue
		}
		if req.JSONRPC != "2.0" || req.Method == "" {
			if err := reply(req.ID, nil, -32600, "Invalid request"); err != nil {
				return err
			}
			continue
		}
		if len(req.ID) > 0 {
			var id interface{}
			_ = json.Unmarshal(req.ID, &id)
			switch id.(type) {
			case string, float64:
			default:
				if err := reply(nil, nil, -32600, "Invalid request ID"); err != nil {
					return err
				}
				continue
			}
		}
		// Notifications never receive replies and cannot invoke operations.
		if len(req.ID) == 0 {
			continue
		}
		var result interface{}
		code := 0
		message := ""
		switch req.Method {
		case "initialize":
			var params struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			if json.Unmarshal(req.Params, &params) != nil || params.ProtocolVersion == "" {
				code, message = -32602, "Invalid initialize parameters"
				break
			}
			initialized = true
			result = map[string]interface{}{"protocolVersion": ProtocolVersion, "capabilities": map[string]interface{}{"tools": map[string]interface{}{}}, "serverInfo": map[string]string{"name": "treecli-api", "version": version}, "instructions": "Treechat tools use the selected account and the backend-owned catalog. Local filesystem, wallet keys, and recorder sessions are not exposed."}
		case "ping":
			result = map[string]interface{}{}
		case "tools/list":
			if !initialized {
				code, message = -32002, "Initialize first"
				break
			}
			catalog, err := backend.ListTools(ctx)
			if err != nil {
				code, message = -32603, "Could not load Treechat tools"
			} else {
				result = catalog
			}
		case "tools/call":
			if !initialized {
				code, message = -32002, "Initialize first"
				break
			}
			var params struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
			}
			if json.Unmarshal(req.Params, &params) != nil || params.Name == "" || len(params.Name) > 128 {
				code, message = -32602, "Invalid tool parameters"
				break
			}
			called, err := backend.CallTool(ctx, params.Name, params.Arguments)
			if err != nil {
				result = api.ToolResult{IsError: true, Content: []map[string]interface{}{{"type": "text", "text": "Treechat request failed; check account authorization and backend availability."}}}
			} else {
				result = called
			}
		default:
			code, message = -32601, "Method not found"
		}
		if err := reply(req.ID, result, code, message); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("MCP input exceeded limit or could not be read")
	}
	return nil
}
