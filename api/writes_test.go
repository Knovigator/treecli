package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSharedWriteReturnsTypedHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/treecli/call" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var call struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
			t.Error(err)
		}
		if call.Name != "create_treechat_write" || call.Arguments["quote_id"] != "quoted-write" || call.Arguments["idempotency_key"] != "retry-key" {
			t.Errorf("wrong call: %#v", call)
		}
		w.Write([]byte(`{"structuredContent":{"status":409,"body":{"error":"Thread already exists."}}}`))
	}))
	defer server.Close()
	_, err := SharedOperation(server.URL, "token", "client", "uid", "create_treechat_write", map[string]interface{}{"quote_id": "quoted-write", "idempotency_key": "retry-key"})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 409 || httpErr.Body != `{"error":"Thread already exists."}` {
		t.Fatalf("unexpected error %T %v", err, err)
	}
}
