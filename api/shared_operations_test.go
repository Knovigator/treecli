package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSharedUploadSeparatesStorageAndBackendCredentials(t *testing.T) {
	for _, purpose := range []string{"attachment", "reference"} {
		t.Run(purpose, func(t *testing.T) {
			data := []byte("\x89PNG\r\n\x1a\n")
			path := filepath.Join(t.TempDir(), "image.png")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			puts := 0
			storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				puts++
				if r.Method != "PUT" {
					t.Errorf("method=%s", r.Method)
				}
				for _, key := range []string{"access-token", "client", "uid", "Authorization"} {
					if r.Header.Get(key) != "" {
						t.Errorf("leaked %s", key)
					}
				}
				got, _ := io.ReadAll(r.Body)
				if !reflect.DeepEqual(got, data) {
					t.Errorf("bytes changed %q", got)
				}
				w.WriteHeader(200)
			}))
			defer storage.Close()
			calls := []string{}
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/treecli/call" || r.Header.Get("access-token") != "secret" {
					t.Errorf("bad authenticated RPC %s", r.URL)
				}
				var call struct {
					Name      string                 `json:"name"`
					Arguments map[string]interface{} `json:"arguments"`
				}
				json.NewDecoder(r.Body).Decode(&call)
				calls = append(calls, call.Name)
				var result interface{}
				switch call.Name {
				case "register_upload":
					if call.Arguments["purpose"] != purpose || call.Arguments["filename"] != "image.png" || call.Arguments["byte_size"] != float64(len(data)) || call.Arguments["checksum"] != md5Base64(data) {
						t.Errorf("registration %#v", call.Arguments)
					}
					result = map[string]interface{}{"signed_id": "blob-id", "direct_upload": map[string]interface{}{"url": storage.URL, "headers": map[string]string{"Content-Type": "image/png"}}}
				case "finish_reference_upload":
					if call.Arguments["signed_id"] != "blob-id" {
						t.Errorf("finish %#v", call.Arguments)
					}
					result = map[string]interface{}{"id": "owned-run"}
				default:
					t.Errorf("unexpected operation %s", call.Name)
				}
				json.NewEncoder(w).Encode(map[string]interface{}{"structuredContent": result})
			}))
			defer backend.Close()
			result, err := UploadSharedFile(backend.URL, "secret", "client", "uid", path, purpose)
			expected := []string{"register_upload"}
			if purpose == "reference" {
				expected = append(expected, "finish_reference_upload")
			}
			if err != nil || puts != 1 || !reflect.DeepEqual(calls, expected) {
				t.Fatalf("result=%v err=%v calls=%v puts=%d", result, err, calls, puts)
			}
			if purpose == "attachment" && result["signed_id"] != "blob-id" || purpose == "reference" && result["id"] != "owned-run" {
				t.Fatalf("bad result %v", result)
			}
		})
	}
}

func TestDecodeGenerationNeverTreatsQuotedPriceAsActualCharge(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       float64
	}{
		{"quote only", `{"generation_id":"g","price_cents":125}`, 0},
		{"explicit actual charge", `{"generation_id":"g","price_cents":125,"amount_usd":0.75}`, 0.75},
		{"explicit zero charge", `{"generation_id":"g","price_cents":125,"amount_usd":0}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := DecodeGeneration(json.RawMessage(tc.body))
			if err != nil || result.AmountUSD != tc.want || result.ID != "g" {
				t.Fatalf("result=%#v err=%v", result, err)
			}
		})
	}
}
