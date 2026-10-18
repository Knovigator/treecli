package api

import (
	"context"
	"fmt"
)

// ToolDefinition preserves the backend-owned schema and annotations without a
// second model/option registry in the CLI.
type ToolDefinition map[string]interface{}
type ToolList struct {
	Tools []ToolDefinition `json:"tools"`
}
type ToolResult struct {
	Content           []map[string]interface{} `json:"content"`
	StructuredContent interface{}              `json:"structuredContent,omitempty"`
	IsError           bool                     `json:"isError,omitempty"`
}

func (c *Client) ListTools(ctx context.Context) (ToolList, error) {
	var out ToolList
	if c.bearer != "" {
		return out, c.mcpCall(ctx, "tools/list", nil, &out)
	}
	err := c.DoJSON(ctx, "GET", "/api/v1/treecli/tools", nil, nil, &out)
	return out, err
}
func (c *Client) CallTool(ctx context.Context, name string, args map[string]interface{}) (ToolResult, error) {
	if name == "" || len(name) > 128 {
		return ToolResult{}, fmt.Errorf("tool name is required (maximum 128 characters)")
	}
	if args == nil {
		args = map[string]interface{}{}
	}
	var out ToolResult
	if c.bearer != "" {
		return out, c.mcpCall(ctx, "tools/call", map[string]interface{}{"name": name, "arguments": args}, &out)
	}
	err := c.DoJSON(ctx, "POST", "/api/v1/treecli/call", nil, map[string]interface{}{"name": name, "arguments": args}, &out)
	return out, err
}
