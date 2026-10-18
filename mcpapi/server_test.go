package mcpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Knovigator/treecli/api"
)

func TestBackendCatalogAndCallSharedByStdio(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer user-one" {
			t.Error("wrong user context")
		}
		w.Header().Set("Content-Type", "application/json")
		// Bearer (OAuth) clients reach the backend only through its /mcp endpoint.
		if r.URL.Path != "/mcp" || r.Method != "POST" {
			t.Error("unexpected path")
			return
		}
		var body struct {
			Method string                 `json:"method"`
			Params map[string]interface{} `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		switch body.Method {
		case "tools/list":
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"backend_feature","inputSchema":{"type":"object","properties":{"setting":{"type":"string"}}}}]}}`)
		case "tools/call":
			calls++
			if body.Params["name"] != "backend_feature" || body.Params["arguments"].(map[string]interface{})["setting"] != "value" {
				t.Error("arguments changed")
			}
			fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"done"}],"structuredContent":{"ok":true}}}`)
		default:
			t.Error("unexpected method")
		}
	}))
	defer server.Close()
	client, _ := api.NewOAuthClient(server.URL, "user-one")
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"backend_feature","arguments":{"setting":"value"}}}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"backend_feature"}}`,
		`{"jsonrpc":"2.0","id":4,"method":"ping"}`,
	}, "\n")
	var output bytes.Buffer
	if err := Serve(context.Background(), strings.NewReader(input), &output, client, "test"); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 4 || calls != 1 {
		t.Fatalf("responses=%d calls=%d", len(lines), calls)
	}
	if !strings.Contains(lines[1], "backend_feature") || !strings.Contains(lines[2], "structuredContent") {
		t.Fatal(output.String())
	}
}
func TestInvalidInputNeverCallsBackend(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"x"}}` + "\n" + `bad` + "\n"
	var out bytes.Buffer
	if err := Serve(context.Background(), strings.NewReader(input), &out, nil, "test"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Initialize first") || !strings.Contains(out.String(), "-32700") {
		t.Fatal(out.String())
	}
	if err := serveWithLimit(context.Background(), strings.NewReader(strings.Repeat("x", 1025)), &out, nil, "test", 1024); err == nil {
		t.Fatal("accepted oversized message")
	}
}
