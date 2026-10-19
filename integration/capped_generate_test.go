package integration_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func TestCompiledCappedGenerationAndBudgetAdapters(t *testing.T) {
	var mu sync.Mutex
	type call struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	var calls []call
	mode := "success"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/media.png" {
			for _, key := range []string{"access-token", "client", "uid"} {
				if r.Header.Get(key) != "" {
					t.Errorf("media request leaked %s", key)
				}
			}
			w.Write([]byte("fixture-image"))
			return
		}
		if r.Method != "POST" || r.URL.Path != "/api/v1/treecli/call" || r.Header.Get("access-token") != "test-token" {
			t.Errorf("incorrect authenticated RPC %s %s", r.Method, r.URL)
		}
		var c call
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			t.Error(err)
			return
		}
		mu.Lock()
		calls = append(calls, c)
		currentMode := mode
		mu.Unlock()
		if currentMode == "denied" {
			w.Write([]byte(`{"isError":true,"structuredContent":{"status":422,"body":{"error":"price exceeds cap"}}}`))
			return
		}
		var result interface{}
		switch c.Name {
		case "generate_media":
			result = map[string]interface{}{"generation_id": "g", "status": "succeeded", "media_urls": []string{"http://" + r.Host + "/media.png"}}
		case "quote_generation":
			result = map[string]interface{}{"quote_id": "preview", "price_cents": 7}
		case "get_generation_budget":
			result = map[string]interface{}{"daily_limit_cents": c.Arguments["daily_limit_cents"], "spent_cents": 10, "reserved_cents": 20, "future_field": "preserved"}
		default:
			t.Errorf("unexpected tool %s", c.Name)
			http.Error(w, "unexpected operation", 500)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"structuredContent": result})
	}))
	defer server.Close()
	binary := buildCLI(t, repositoryRoot(t))
	config := t.TempDir()
	path := filepath.Join(config, "treecli", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	saved := fmt.Sprintf("[environments.prod]\nbackend_url = %q\nactive_account = \"default\"\n[accounts.prod.default]\nbackend_url = %q\naccess_token = \"test-token\"\nclient = \"test-client\"\nuid = \"test-uid\"\n", server.URL, server.URL)
	if err := os.WriteFile(path, []byte(saved), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot := func() []call { mu.Lock(); defer mu.Unlock(); return append([]call(nil), calls...) }
	t.Run("single operation and stable retry key", func(t *testing.T) {
		output := filepath.Join(t.TempDir(), "result.png")
		args := []string{"generate", "!flux", "a kite", "--max-price-cents", "123", "--id", "retry-same", "--reference", "run:ref", "--input", "aspect_ratio=1:1", "--out", output}
		for i := 0; i < 2; i++ {
			out, stderr, err := runCLI(t, binary, config, "", args...)
			if err != nil {
				t.Fatalf("generate: %v %s %s", err, out, stderr)
			}
		}
		got := snapshot()
		if len(got) != 2 {
			t.Fatalf("expected one RPC per attempt: %#v", got)
		}
		for _, c := range got {
			if c.Name != "generate_media" || c.Arguments["model"] != "!flux" || c.Arguments["prompt"] != "a kite" || c.Arguments["max_price_cents"] != float64(123) || c.Arguments["idempotency_key"] != "retry-same" || c.Arguments["reference"] != "run:ref" {
				t.Fatalf("lost capped generation fields %#v", c)
			}
		}
		if !reflect.DeepEqual(got[0], got[1]) {
			t.Fatal("retry request changed")
		}
		data, err := os.ReadFile(output)
		if err != nil || string(data) != "fixture-image" {
			t.Fatalf("download: %q %v", data, err)
		}
	})
	t.Run("preview never executes", func(t *testing.T) {
		before := len(snapshot())
		out, stderr, err := runCLI(t, binary, config, "", "generate", "topazvid", "--reference", "run:video", "--quote", "--max-price-cents", "0")
		if err != nil || !strings.Contains(out, "preview") {
			t.Fatalf("preview: %v %s %s", err, out, stderr)
		}
		got := snapshot()[before:]
		if len(got) != 1 || got[0].Name != "quote_generation" || got[0].Arguments["prompt"] != "" || got[0].Arguments["max_price_cents"] != float64(0) {
			t.Fatalf("unexpected preview %#v", got)
		}
		if _, ok := got[0].Arguments["idempotency_key"]; ok {
			t.Fatal("preview forwarded execution key")
		}
	})
	t.Run("omitted ID generates printable reusable key", func(t *testing.T) {
		before := len(snapshot())
		output := filepath.Join(t.TempDir(), "generated-id.png")
		out, stderr, err := runCLI(t, binary, config, "", "generate", "flux", "prompt", "--max-price-cents", "10", "--out", output)
		if err != nil {
			t.Fatalf("auto ID: %v %s %s", err, out, stderr)
		}
		got := snapshot()[before:]
		if len(got) != 1 || got[0].Name != "generate_media" {
			t.Fatalf("auto ID calls %#v", got)
		}
		key, ok := got[0].Arguments["idempotency_key"].(string)
		if !ok || !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(key) || !strings.Contains(stderr, key) {
			t.Fatalf("missing printable UUID key=%q stderr=%q", key, stderr)
		}
		before = len(snapshot())
		out, stderr, err = runCLI(t, binary, config, "", "generate", "flux", "prompt", "--max-price-cents", "10", "--id", key, "--out", output)
		retried := snapshot()[before:]
		if err != nil || len(retried) != 1 || !reflect.DeepEqual(retried[0], got[0]) {
			t.Fatalf("generated key retry changed: %v %s %s %#v", err, out, stderr, retried)
		}
	})
	t.Run("validation rejects before uploads or RPC", func(t *testing.T) {
		for _, flags := range [][]string{{"--id", "key"}, {"--max-price-cents", "-1", "--id", "key"}, {"--max-price-cents", "2147483648", "--id", "key"}, {"--max-price-cents", "0", "--id", "   "}} {
			before := len(snapshot())
			args := append([]string{"generate", "flux", "prompt", "--reference", "@/missing/file.png", "--out", filepath.Join(t.TempDir(), "out.png")}, flags...)
			out, stderr, err := runCLI(t, binary, config, "", args...)
			if err == nil || len(snapshot()) != before || strings.Contains(stderr, "no such file") {
				t.Fatalf("invalid flags %v reached input processing: %v %s %s", flags, err, out, stderr)
			}
		}
	})
	t.Run("shared failure does not fall back", func(t *testing.T) {
		mu.Lock()
		mode = "denied"
		mu.Unlock()
		defer func() { mu.Lock(); mode = "success"; mu.Unlock() }()
		before := len(snapshot())
		output := filepath.Join(t.TempDir(), "out.png")
		out, stderr, err := runCLI(t, binary, config, "", "generate", "flux", "prompt", "--max-price-cents", "0", "--id", "denied-key", "--out", output)
		if err == nil || !strings.Contains(stderr, "price exceeds cap") || len(snapshot()) != before+1 {
			t.Fatalf("denial: %v %s %s", err, out, stderr)
		}
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Fatalf("denial wrote output: %v", err)
		}
	})
	t.Run("budget read only", func(t *testing.T) {
		before := len(snapshot())
		out, stderr, err := runCLI(t, binary, config, "", "billing", "budget", "--json")
		if err != nil || !strings.Contains(out, "future_field") {
			t.Fatalf("budget: %v %s %s", err, out, stderr)
		}
		got := snapshot()[before:]
		if len(got) != 1 || got[0].Name != "get_generation_budget" || len(got[0].Arguments) != 0 {
			t.Fatalf("budget wire %#v", got)
		}
		for _, args := range [][]string{{"billing", "budget", "set", "250"}, {"billing", "budget", "unset"}} {
			before := len(snapshot())
			out, stderr, err := runCLI(t, binary, config, "", args...)
			if err == nil || len(snapshot()) != before {
				t.Fatalf("removed mutation %v: %v %s %s", args, err, out, stderr)
			}
		}
	})
}
