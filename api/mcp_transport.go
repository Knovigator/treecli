package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// OAuthClient marks a saved profile whose credentials are an OAuth bearer from
// `treecli connect`. Such profiles reach the backend only through /mcp.
const OAuthClient = "oauth"

const mcpProtocolVersion = "2025-06-18"

var (
	oauthRefreshMu sync.Mutex
	oauthRefresh   func(stale string) (string, error)
)

// SetOAuthRefresher installs the hook that trades an expired bearer for a fresh
// one (and persists it). The CLI owns credential storage; this package never does.
func SetOAuthRefresher(refresh func(stale string) (string, error)) {
	oauthRefreshMu.Lock()
	defer oauthRefreshMu.Unlock()
	oauthRefresh = refresh
}

// refreshBearer holds the lock for the whole refresh, so concurrent callers in
// one process share a single renewal instead of racing to rotate the token.
func refreshBearer(stale string) (string, error) {
	oauthRefreshMu.Lock()
	defer oauthRefreshMu.Unlock()
	if oauthRefresh == nil {
		return "", fmt.Errorf("Treechat connection expired; run treecli connect again")
	}
	return oauthRefresh(stale)
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// mcpCall sends one JSON-RPC request to the backend's stateless /mcp endpoint
// with the bearer, refreshing it once if the server says it has expired.
func (c *Client) mcpCall(ctx context.Context, method string, params, out interface{}) error {
	for attempt := 0; ; attempt++ {
		body := map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": method}
		if params != nil {
			body["params"] = params
		}
		response, err := scopedRequest(ctx, c.bearer, "", "", "", 0).
			SetHeader("Content-Type", "application/json").
			SetHeader("Accept", "application/json, text/event-stream").
			SetHeader("MCP-Protocol-Version", mcpProtocolVersion).
			SetBody(body).
			Post(c.origin + "/mcp")
		if err != nil {
			return fmt.Errorf("API request failed")
		}
		if response.StatusCode() == http.StatusUnauthorized && attempt == 0 {
			fresh, refreshErr := refreshBearer(c.bearer)
			if refreshErr != nil {
				return refreshErr
			}
			c.bearer = fresh
			continue
		}
		if response.StatusCode() < 200 || response.StatusCode() >= 300 {
			return fmt.Errorf("API request failed (status %d): %s", response.StatusCode(), SafeCredentialResponseBody(response.Body(), c.bearer))
		}
		var rpc rpcResponse
		if err := json.Unmarshal(response.Body(), &rpc); err != nil {
			return fmt.Errorf("invalid API response: %w", err)
		}
		if rpc.Error != nil {
			return fmt.Errorf("Treechat request failed: %s", rpc.Error.Message)
		}
		if out == nil {
			return nil
		}
		if err := json.Unmarshal(rpc.Result, out); err != nil {
			return fmt.Errorf("invalid API response: %w", err)
		}
		return nil
	}
}
