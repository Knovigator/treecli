package integration_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestCompiledPromptOptionalMediaUsesSharedPolicyAndCatalog(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	video := []byte("fixture-video")
	example := "treecli generate topazvid --reference @clip.mp4 --quote --max-price-cents 200"
	row := map[string]interface{}{"action": "topazvid", "kind": "video", "direct_generation": true, "prompt_optional": true, "accepts_reference": true, "requires_reference": true, "reference_kinds": []string{"video"}, "examples": []string{example}, "settings": []map[string]string{{"name": "prompt", "description": "Optional: reference media is enough."}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/storage" {
			if r.Method != "PUT" {
				t.Errorf("storage method %s", r.Method)
			}
			for _, key := range []string{"access-token", "client", "uid"} {
				if r.Header.Get(key) != "" {
					t.Errorf("storage leaked %s", key)
				}
			}
			data, _ := io.ReadAll(r.Body)
			if !reflect.DeepEqual(data, video) {
				t.Errorf("upload changed %q", data)
			}
			mu.Lock()
			calls = append(calls, "storage_put")
			mu.Unlock()
			return
		}
		if r.Method != "POST" || r.URL.Path != "/api/v1/treecli/call" || r.Header.Get("access-token") != "test-token" {
			t.Errorf("unexpected RPC %s %s", r.Method, r.URL)
		}
		var call struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
			t.Error(err)
			return
		}
		mu.Lock()
		calls = append(calls, call.Name)
		mu.Unlock()
		a := call.Arguments
		var result interface{}
		switch call.Name {
		case "register_upload":
			if a["purpose"] != "reference" || a["filename"] != "clip.mp4" || a["byte_size"] != float64(len(video)) {
				t.Errorf("registration %#v", a)
			}
			result = map[string]interface{}{"signed_id": "signed-video", "direct_upload": map[string]interface{}{"url": "http://" + r.Host + "/storage", "headers": map[string]string{}}}
		case "finish_reference_upload":
			if a["signed_id"] != "signed-video" {
				t.Errorf("finish %#v", a)
			}
			result = map[string]string{"id": "video-run"}
		case "quote_generation":
			if a["model"] != "topazvid" || a["prompt"] != "" || a["reference"] != "run:video-run" || a["max_price_cents"] != float64(200) {
				t.Errorf("quote lost media-only input %#v", a)
			}
			result = map[string]interface{}{"quote_id": "media-quote", "price_cents": 150}
		case "treechat_read":
			args, _ := a["arguments"].(map[string]interface{})
			switch a["operation"] {
			case "generation_actions":
				if args["direct_only"] != true {
					t.Errorf("lost direct-only %#v", args)
				}
				result = map[string]interface{}{"actions": []interface{}{row}}
			case "describe_action":
				if args["action"] != "topazvid" {
					t.Errorf("lost action %#v", args)
				}
				result = row
			default:
				t.Errorf("unexpected read %#v", a)
			}
		default:
			t.Errorf("unexpected operation (quote must never execute): %s", call.Name)
			http.Error(w, "unexpected operation", 500)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"structuredContent": result})
	}))
	defer server.Close()
	binary := buildCLI(t, repositoryRoot(t))
	config := t.TempDir()
	configPath := filepath.Join(config, "treecli", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		t.Fatal(err)
	}
	saved := fmt.Sprintf("[environments.prod]\nbackend_url = %q\nactive_account = \"default\"\n[accounts.prod.default]\nbackend_url = %q\naccess_token = \"test-token\"\nclient = \"test-client\"\nuid = \"test-uid\"\n", server.URL, server.URL)
	if err := os.WriteFile(configPath, []byte(saved), 0600); err != nil {
		t.Fatal(err)
	}
	reference := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(reference, video, 0600); err != nil {
		t.Fatal(err)
	}
	output, stderr, err := runCLI(t, binary, config, "", "generate", "topazvid", "--reference", "@"+reference, "--quote", "--max-price-cents", "200")
	if err != nil || !strings.Contains(output, "media-quote") {
		t.Fatalf("media-only quote: %v %s %s", err, output, stderr)
	}
	mu.Lock()
	gotCalls := append([]string(nil), calls...)
	mu.Unlock()
	if !reflect.DeepEqual(gotCalls, []string{"register_upload", "storage_put", "finish_reference_upload", "quote_generation"}) {
		t.Fatalf("unexpected quote flow %v", gotCalls)
	}
	for _, args := range [][]string{{"generate", "actions", "--direct-only", "--json"}, {"generate", "describe", "topazvid", "--json"}, {"generate", "describe", "topazvid"}, {"generate", "actions", "--direct-only", "--verbose"}} {
		output, stderr, err = runCLI(t, binary, config, "", args...)
		if err != nil || !strings.Contains(output, example) || !strings.Contains(output, "Optional: reference media is enough.") {
			t.Fatalf("catalog %v: %v %s %s", args, err, output, stderr)
		}
		if args[len(args)-1] == "--json" && !strings.Contains(output, `"prompt_optional": true`) {
			t.Fatalf("catalog lost prompt_optional: %s", output)
		}
	}
}
